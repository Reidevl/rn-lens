// Package network mirrors the HTTP traffic of the attached React Native app,
// as reported by the CDP Network domain (React Native 0.83+).
package network

import (
	"encoding/json"
	"net"
	"sort"
	"strconv"
	"sync"

	"github.com/Reidevl/rn-lens/internal/platform/cdp"
	"github.com/Reidevl/rn-lens/internal/platform/clock"
)

// DefaultCapacity is how many requests a store keeps by default.
const DefaultCapacity = 2_000

// MaxInlinePostData is the largest request body kept from the
// requestWillBeSent event; larger bodies are fetched on demand.
const MaxInlinePostData = 64 * 1024

// State is where a request is in its lifecycle.
type State string

// Request states.
const (
	StatePending  State = "pending"
	StateComplete State = "complete"
	StateFailed   State = "failed"
)

// Summary is the row the table shows. Times are CDP monotonic seconds (zero
// when unknown); StartedAt is wall-clock milliseconds since the Unix epoch.
type Summary struct {
	Seq          uint64  `json:"seq"`
	Epoch        uint64  `json:"epoch"`
	Version      uint64  `json:"version"`
	RequestID    string  `json:"requestId"`
	URL          string  `json:"url"`
	Method       string  `json:"method"`
	ResourceType string  `json:"resourceType"`
	State        State   `json:"state"`
	Status       int     `json:"status"`
	StatusText   string  `json:"statusText"`
	MimeType     string  `json:"mimeType"`
	StartedAt    int64   `json:"startedAt"`
	StartTime    float64 `json:"startTime"`
	ResponseTime float64 `json:"responseTime"`
	EndTime      float64 `json:"endTime"`
	EncodedSize  int64   `json:"encodedSize"`
	DecodedSize  int64   `json:"decodedSize"`
	ErrorText    string  `json:"errorText,omitempty"`
	Canceled     bool    `json:"canceled,omitempty"`
	Redirected   bool    `json:"redirected,omitempty"`
	FromCache    bool    `json:"fromCache,omitempty"`
}

// Request is a row plus everything the detail tabs show.
type Request struct {
	Summary
	RequestHeaders  cdp.Headers         `json:"requestHeaders"`
	ResponseHeaders cdp.Headers         `json:"responseHeaders"`
	HasPostData     bool                `json:"hasPostData"`
	PostData        string              `json:"postData,omitempty"`
	Protocol        string              `json:"protocol,omitempty"`
	RemoteAddress   string              `json:"remoteAddress,omitempty"`
	Timing          *cdp.ResourceTiming `json:"timing,omitempty"`
	Initiator       *cdp.Initiator      `json:"initiator,omitempty"`
}

// Stats summarises a store for the notify-then-pull UI. A reader keeps the
// last Version it saw and asks for rows changed since; rows below OldestSeq
// were evicted and a new Cleared value means every row is gone.
type Stats struct {
	Version   uint64 `json:"version"`
	Len       int    `json:"len"`
	OldestSeq uint64 `json:"oldestSeq"`
	LastSeq   uint64 `json:"lastSeq"`
	Dropped   uint64 `json:"dropped"`
	Epoch     uint64 `json:"epoch"`
	Cleared   uint64 `json:"cleared"`
}

// Store keeps the most recent requests in creation order. Unlike the console
// log, rows change after they are created (response, sizes, end), so every
// change stamps the row with a store-wide version. It is safe for concurrent
// use.
type Store struct {
	mu       sync.Mutex
	clk      clock.Clock
	capacity int
	rows     map[uint64]*Request
	order    []uint64          // seqs, oldest first
	byID     map[string]uint64 // requestId → seq of its current hop
	lastSeq  uint64
	version  uint64
	dropped  uint64
	epoch    uint64
	cleared  uint64
}

// NewStore creates a store keeping capacity requests (DefaultCapacity when <= 0).
func NewStore(capacity int, clk clock.Clock) *Store {
	if capacity <= 0 {
		capacity = DefaultCapacity
	}
	if clk == nil {
		clk = clock.Real{}
	}
	return &Store{
		clk:      clk,
		capacity: capacity,
		rows:     make(map[uint64]*Request),
		byID:     make(map[string]uint64),
		epoch:    1,
	}
}

// Ingest applies a Network event. It reports false for events the store does
// not use: other domains, malformed params and requests it no longer holds.
func (s *Store) Ingest(ev cdp.Event) bool {
	switch ev.Method {
	case cdp.EventNetworkRequestWillBeSent:
		var p cdp.RequestWillBeSentParams
		if json.Unmarshal(ev.Params, &p) != nil || p.RequestID == "" {
			return false
		}
		s.mu.Lock()
		defer s.mu.Unlock()
		s.requestWillBeSent(p)
		return true
	case cdp.EventNetworkResponseReceived:
		var p cdp.ResponseReceivedParams
		if json.Unmarshal(ev.Params, &p) != nil {
			return false
		}
		return s.update(p.RequestID, func(r *Request) {
			applyResponse(r, p.Response)
			r.ResponseTime = p.Timestamp
			if p.Type != "" {
				r.ResourceType = p.Type
			}
		})
	case cdp.EventNetworkDataReceived:
		var p cdp.DataReceivedParams
		if json.Unmarshal(ev.Params, &p) != nil {
			return false
		}
		return s.update(p.RequestID, func(r *Request) {
			r.DecodedSize += int64(p.DataLength)
			r.EncodedSize += int64(p.EncodedDataLength)
		})
	case cdp.EventNetworkLoadingFinished:
		var p cdp.LoadingFinishedParams
		if json.Unmarshal(ev.Params, &p) != nil {
			return false
		}
		return s.update(p.RequestID, func(r *Request) {
			r.State = StateComplete
			r.EndTime = p.Timestamp
			if p.EncodedDataLength > 0 {
				r.EncodedSize = int64(p.EncodedDataLength)
			}
		})
	case cdp.EventNetworkLoadingFailed:
		var p cdp.LoadingFailedParams
		if json.Unmarshal(ev.Params, &p) != nil {
			return false
		}
		return s.update(p.RequestID, func(r *Request) {
			r.State = StateFailed
			r.EndTime = p.Timestamp
			r.ErrorText = p.ErrorText
			r.Canceled = p.Canceled
		})
	default:
		return false
	}
}

// requestWillBeSent requires s.mu. A known requestId with a redirect
// response closes that hop and starts a new row for the next URL.
func (s *Store) requestWillBeSent(p cdp.RequestWillBeSentParams) {
	if seq, ok := s.byID[p.RequestID]; ok {
		if prev := s.rows[seq]; prev != nil {
			switch {
			case p.RedirectResponse != nil:
				applyResponse(prev, *p.RedirectResponse)
				prev.Redirected = true
				prev.State = StateComplete
				prev.EndTime = p.Timestamp
				s.touch(prev)
			case prev.State == StatePending:
				// The id was reused without a redirect: nothing can finish
				// the earlier row any more.
				prev.State = StateFailed
				prev.ErrorText = ErrorReplaced
				prev.EndTime = p.Timestamp
				s.touch(prev)
			}
		}
	}
	startedAt := s.clk.Now().UnixMilli()
	if p.WallTime > 0 {
		startedAt = int64(p.WallTime * 1000)
	}
	r := &Request{
		Summary: Summary{
			RequestID:    p.RequestID,
			URL:          p.Request.URL + p.Request.URLFragment,
			Method:       p.Request.Method,
			ResourceType: p.Type,
			State:        StatePending,
			StartedAt:    startedAt,
			StartTime:    p.Timestamp,
		},
		RequestHeaders: p.Request.Headers,
		HasPostData:    p.Request.HasPostData || p.Request.PostData != "",
		Initiator:      p.Initiator,
	}
	if len(p.Request.PostData) <= MaxInlinePostData {
		r.PostData = p.Request.PostData
	}
	s.lastSeq++
	r.Seq = s.lastSeq
	r.Epoch = s.epoch
	s.rows[r.Seq] = r
	s.order = append(s.order, r.Seq)
	s.byID[p.RequestID] = r.Seq
	s.touch(r)
	s.evict()
}

// update applies fn to the current hop of requestID and bumps its version.
func (s *Store) update(requestID string, fn func(*Request)) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	seq, ok := s.byID[requestID]
	if !ok {
		return false
	}
	r := s.rows[seq]
	if r == nil {
		return false
	}
	fn(r)
	s.touch(r)
	return true
}

// touch requires s.mu.
func (s *Store) touch(r *Request) {
	s.version++
	r.Version = s.version
}

// evict requires s.mu.
func (s *Store) evict() {
	for len(s.order) > s.capacity {
		seq := s.order[0]
		s.order = s.order[1:]
		if r := s.rows[seq]; r != nil && s.byID[r.RequestID] == seq {
			delete(s.byID, r.RequestID)
		}
		delete(s.rows, seq)
		s.dropped++
	}
}

func applyResponse(r *Request, resp cdp.NetworkResponse) {
	r.Status = resp.Status
	r.StatusText = resp.StatusText
	r.MimeType = resp.MimeType
	r.ResponseHeaders = resp.Headers
	r.Protocol = resp.Protocol
	r.FromCache = resp.FromDiskCache
	r.Timing = resp.Timing
	if resp.RemoteIPAddress != "" {
		r.RemoteAddress = resp.RemoteIPAddress
		if resp.RemotePort > 0 {
			r.RemoteAddress = net.JoinHostPort(resp.RemoteIPAddress, strconv.Itoa(resp.RemotePort))
		}
	}
}

// Since returns up to limit rows (all when limit <= 0) whose version is
// greater than version, in version order, so a reader can page by passing
// the last version it received.
func (s *Store) Since(version uint64, limit int) []Summary {
	s.mu.Lock()
	out := make([]Summary, 0)
	for _, seq := range s.order {
		if r := s.rows[seq]; r != nil && r.Version > version {
			out = append(out, r.Summary)
		}
	}
	s.mu.Unlock()
	sort.Slice(out, func(i, j int) bool { return out[i].Version < out[j].Version })
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out
}

// Get returns a copy of the request with the given seq. The copy shares its
// header maps and timing with the store, which only ever replaces them; do
// not modify them in place.
func (s *Store) Get(seq uint64) (Request, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	r, ok := s.rows[seq]
	if !ok {
		return Request{}, false
	}
	return *r, true
}

// All returns copies of every request, oldest first (see Get about sharing).
func (s *Store) All() []Request {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Request, 0, len(s.order))
	for _, seq := range s.order {
		if r := s.rows[seq]; r != nil {
			out = append(out, *r)
		}
	}
	return out
}

// Stats reports the store position for the notify-then-pull UI.
func (s *Store) Stats() Stats {
	s.mu.Lock()
	defer s.mu.Unlock()
	st := Stats{
		Version: s.version,
		Len:     len(s.order),
		LastSeq: s.lastSeq,
		Dropped: s.dropped,
		Epoch:   s.epoch,
		Cleared: s.cleared,
	}
	if len(s.order) > 0 {
		st.OldestSeq = s.order[0]
	}
	return st
}

// Clear drops every request. Sequence numbers keep increasing and the
// cleared generation advances so readers can detect the clear.
func (s *Store) Clear() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.rows = make(map[uint64]*Request)
	s.byID = make(map[string]uint64)
	s.order = nil
	s.cleared++
	s.version++
}

// ErrorReplaced is the failure text of a pending request whose id the app
// reused for a new request.
const ErrorReplaced = "Replaced by a new request with the same id"

// ErrorSessionEnded is the failure text of requests still pending when the
// debugger session they belonged to ended.
const ErrorSessionEnded = "The debugger reconnected before the request finished"

// NewEpoch starts a new attach generation: requests from older epochs stay
// listed but their bodies can no longer be fetched, and the ones still pending
// are marked failed because their events died with the old session. It
// returns the new epoch.
func (s *Store) NewEpoch() uint64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, seq := range s.order {
		if r := s.rows[seq]; r != nil && r.State == StatePending {
			r.State = StateFailed
			r.ErrorText = ErrorSessionEnded
			s.touch(r)
		}
	}
	s.byID = make(map[string]uint64)
	s.epoch++
	s.version++
	return s.epoch
}

// Epoch is the current attach generation.
func (s *Store) Epoch() uint64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.epoch
}
