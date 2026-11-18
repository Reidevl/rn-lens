package network

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/url"
	"sort"
	"strings"
	"time"
)

// HAR 1.2 types (http://www.softwareishard.com/blog/har-12-spec/). Only the
// fields Lens can fill are present; required ones are always written.

// HAR is the top-level archive.
type HAR struct {
	Log HARLog `json:"log"`
}

// HARLog is the archive body.
type HARLog struct {
	Version string     `json:"version"`
	Creator HARCreator `json:"creator"`
	Pages   []struct{} `json:"pages"`
	Entries []HAREntry `json:"entries"`
}

// HARCreator names the tool that wrote the archive.
type HARCreator struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// HAREntry is one request/response pair.
type HAREntry struct {
	StartedDateTime string      `json:"startedDateTime"`
	Time            float64     `json:"time"`
	Request         HARRequest  `json:"request"`
	Response        HARResponse `json:"response"`
	Cache           struct{}    `json:"cache"`
	Timings         HARTimings  `json:"timings"`
	ServerIPAddress string      `json:"serverIPAddress,omitempty"`
	ResourceType    string      `json:"_resourceType,omitempty"`
	Error           string      `json:"_error,omitempty"`
}

// HARNameValue is a header or query parameter.
type HARNameValue struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

// HARPostData is a request body.
type HARPostData struct {
	MimeType string `json:"mimeType"`
	Text     string `json:"text"`
}

// HARRequest describes the request.
type HARRequest struct {
	Method      string         `json:"method"`
	URL         string         `json:"url"`
	HTTPVersion string         `json:"httpVersion"`
	Cookies     []struct{}     `json:"cookies"`
	Headers     []HARNameValue `json:"headers"`
	QueryString []HARNameValue `json:"queryString"`
	PostData    *HARPostData   `json:"postData,omitempty"`
	HeadersSize int            `json:"headersSize"`
	BodySize    int            `json:"bodySize"`
}

// HARContent is a response body.
type HARContent struct {
	Size     int64  `json:"size"`
	MimeType string `json:"mimeType"`
	Text     string `json:"text,omitempty"`
	Encoding string `json:"encoding,omitempty"`
	Comment  string `json:"comment,omitempty"`
}

// HARResponse describes the response.
type HARResponse struct {
	Status      int            `json:"status"`
	StatusText  string         `json:"statusText"`
	HTTPVersion string         `json:"httpVersion"`
	Cookies     []struct{}     `json:"cookies"`
	Headers     []HARNameValue `json:"headers"`
	Content     HARContent     `json:"content"`
	RedirectURL string         `json:"redirectURL"`
	HeadersSize int            `json:"headersSize"`
	BodySize    int64          `json:"bodySize"`
}

// HARTimings are phase durations in milliseconds; -1 means not applicable.
type HARTimings struct {
	Blocked float64 `json:"blocked"`
	DNS     float64 `json:"dns"`
	Connect float64 `json:"connect"`
	SSL     float64 `json:"ssl"`
	Send    float64 `json:"send"`
	Wait    float64 `json:"wait"`
	Receive float64 `json:"receive"`
}

// HARBodies holds the bodies fetched for an export, keyed by request seq.
type HARBodies struct {
	Responses map[uint64]Body
	Requests  map[uint64]string
}

// BuildHAR converts requests into a HAR 1.2 archive. It is pure: bodies are
// fetched beforehand (see Service.ExportHAR).
func BuildHAR(reqs []Request, lensVersion string, bodies HARBodies) HAR {
	entries := make([]HAREntry, 0, len(reqs))
	for _, r := range reqs {
		entries = append(entries, harEntry(r, bodies))
	}
	return HAR{Log: HARLog{
		Version: "1.2",
		Creator: HARCreator{Name: "Lens", Version: lensVersion},
		Pages:   []struct{}{},
		Entries: entries,
	}}
}

func harEntry(r Request, bodies HARBodies) HAREntry {
	timings := harTimings(r)
	e := HAREntry{
		StartedDateTime: time.UnixMilli(r.StartedAt).UTC().Format("2006-01-02T15:04:05.000Z"),
		Time:            harTotal(timings),
		Timings:         timings,
		ServerIPAddress: hostOnly(r.RemoteAddress),
		ResourceType:    strings.ToLower(r.ResourceType),
		Error:           r.ErrorText,
		Request: HARRequest{
			Method:      r.Method,
			URL:         r.URL,
			HTTPVersion: httpVersion(r.Protocol),
			Cookies:     []struct{}{},
			Headers:     nameValues(r.RequestHeaders),
			QueryString: queryString(r.URL),
			HeadersSize: -1,
		},
		Response: HARResponse{
			Status:      r.Status,
			StatusText:  r.StatusText,
			HTTPVersion: httpVersion(r.Protocol),
			Cookies:     []struct{}{},
			Headers:     nameValues(r.ResponseHeaders),
			Content:     HARContent{Size: r.DecodedSize, MimeType: r.MimeType},
			RedirectURL: headerValue(r.ResponseHeaders, "Location"),
			HeadersSize: -1,
			BodySize:    r.EncodedSize,
		},
	}
	if post, ok := bodies.Requests[r.Seq]; ok && post != "" {
		e.Request.PostData = &HARPostData{MimeType: headerValue(r.RequestHeaders, "Content-Type"), Text: post}
		e.Request.BodySize = len(post)
	}
	if b, ok := bodies.Responses[r.Seq]; ok {
		switch {
		case b.Truncated:
			e.Response.Content.Comment = fmt.Sprintf("body omitted: %d bytes exceed the export limit", b.Size)
		case b.Base64:
			e.Response.Content.Text, e.Response.Content.Encoding = b.Text, "base64"
		default:
			e.Response.Content.Text = b.Text
		}
	}
	return e
}

// harTimings maps CDP ResourceTiming (milliseconds after requestTime) and the
// event timestamps (seconds) onto HAR phases.
func harTimings(r Request) HARTimings {
	t := HARTimings{Blocked: -1, DNS: -1, Connect: -1, SSL: -1}
	tm := r.Timing
	if tm == nil {
		if r.ResponseTime > 0 {
			t.Wait = ms(r.ResponseTime - r.StartTime)
			if r.EndTime > 0 {
				t.Receive = ms(r.EndTime - r.ResponseTime)
			}
		} else if r.EndTime > 0 {
			t.Wait = ms(r.EndTime - r.StartTime)
		}
		return t
	}
	for _, start := range []float64{tm.DNSStart, tm.ConnectStart, tm.SendStart} {
		if start >= 0 {
			t.Blocked = start
			break
		}
	}
	if tm.DNSStart >= 0 {
		t.DNS = nonNegative(tm.DNSEnd - tm.DNSStart)
	}
	if tm.ConnectStart >= 0 {
		t.Connect = nonNegative(tm.ConnectEnd - tm.ConnectStart)
	}
	if tm.SSLStart >= 0 {
		t.SSL = nonNegative(tm.SSLEnd - tm.SSLStart)
	}
	t.Send = nonNegative(tm.SendEnd - tm.SendStart)
	t.Wait = nonNegative(tm.ReceiveHeadersEnd - tm.SendEnd)
	if r.EndTime > 0 {
		t.Receive = nonNegative(ms(r.EndTime-tm.RequestTime) - tm.ReceiveHeadersEnd)
	}
	return t
}

// harTotal sums the phases; ssl is already part of connect in HAR 1.2.
func harTotal(t HARTimings) float64 {
	total := 0.0
	for _, v := range []float64{t.Blocked, t.DNS, t.Connect, t.Send, t.Wait, t.Receive} {
		if v > 0 {
			total += v
		}
	}
	return round3(total)
}

func ms(seconds float64) float64 { return round3(nonNegative(seconds * 1000)) }

func round3(v float64) float64 { return float64(int64(v*1000+0.5)) / 1000 }

func nonNegative(v float64) float64 {
	if v < 0 {
		return 0
	}
	return round3(v)
}

func httpVersion(protocol string) string {
	switch strings.ToLower(protocol) {
	case "h2", "http/2", "http/2.0":
		return "HTTP/2"
	case "h3", "http/3":
		return "HTTP/3"
	case "http/1.0":
		return "HTTP/1.0"
	case "", "http/1.1":
		return "HTTP/1.1"
	default:
		return protocol
	}
}

func nameValues(h map[string]string) []HARNameValue {
	out := make([]HARNameValue, 0, len(h))
	for k, v := range h {
		out = append(out, HARNameValue{Name: k, Value: v})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

func headerValue(h map[string]string, name string) string {
	for k, v := range h {
		if strings.EqualFold(k, name) {
			return v
		}
	}
	return ""
}

func queryString(raw string) []HARNameValue {
	out := []HARNameValue{}
	u, err := url.Parse(raw)
	if err != nil {
		return out
	}
	for _, pair := range strings.Split(u.RawQuery, "&") {
		if pair == "" {
			continue
		}
		k, v, _ := strings.Cut(pair, "=")
		name, errK := url.QueryUnescape(k)
		value, errV := url.QueryUnescape(v)
		if errK != nil || errV != nil {
			name, value = k, v
		}
		out = append(out, HARNameValue{Name: name, Value: value})
	}
	return out
}

func hostOnly(addr string) string {
	if host, _, err := net.SplitHostPort(addr); err == nil {
		return host
	}
	return addr
}

// ExportHAR builds a HAR archive of every stored request. Bodies of finished
// requests in the current epoch are fetched (best effort: a body the app no
// longer has is left out).
func (s *Service) ExportHAR(ctx context.Context, lensVersion string) ([]byte, error) {
	reqs := s.store.All()
	bodies := HARBodies{Responses: map[uint64]Body{}, Requests: map[uint64]string{}}
	epoch := s.store.Epoch()
	for _, r := range reqs {
		if r.Epoch != epoch {
			continue
		}
		if r.HasPostData {
			if post, err := s.RequestBody(ctx, r.Seq, r.Epoch); err == nil {
				bodies.Requests[r.Seq] = post
			}
		}
		if r.State == StateComplete && !r.Redirected {
			if b, err := s.ResponseBody(ctx, r.Seq, r.Epoch); err == nil {
				bodies.Responses[r.Seq] = b
			}
		}
	}
	return EncodeHAR(BuildHAR(reqs, lensVersion, bodies))
}

// EncodeHAR writes h as indented JSON without HTML escaping, so URLs keep
// their & and < characters readable.
func EncodeHAR(h HAR) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(h); err != nil {
		return nil, fmt.Errorf("network: encode HAR: %w", err)
	}
	return buf.Bytes(), nil
}
