package cdptest

import (
	"bufio"
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/Reidevl/rn-lens/internal/platform/cdp"
)

// Frame is one line of a recording written by `rnlens record`: a raw CDP
// frame with its direction, as seen from Lens, and its offset from the start.
type Frame struct {
	At    time.Duration   `json:"at"`
	Dir   cdp.Direction   `json:"dir"`
	Frame json.RawMessage `json:"frame"`
}

// Recording is a parsed JSONL capture of a debugger session, for example
// internal/adapters/cdpws/testdata/rn088-expo-ios.jsonl.
type Recording struct {
	Frames []Frame
}

// LoadRecording reads a JSONL capture. path is relative to the test's
// working directory, its package directory.
func LoadRecording(tb testing.TB, path string) Recording {
	tb.Helper()
	f, err := os.Open(path) //nolint:gosec // G304: a fixture path chosen by the test
	if err != nil {
		tb.Fatalf("cdptest: open recording: %v", err)
	}
	defer f.Close()

	var rec Recording
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 1<<24)
	for line := 1; sc.Scan(); line++ {
		var fr Frame
		if err := json.Unmarshal(sc.Bytes(), &fr); err != nil {
			tb.Fatalf("cdptest: decode %s line %d: %v", path, line, err)
		}
		rec.Frames = append(rec.Frames, fr)
	}
	if err := sc.Err(); err != nil {
		tb.Fatalf("cdptest: read recording: %v", err)
	}
	return rec
}

// Events returns the notifications the target sent, in order.
func (r Recording) Events() []cdp.Event {
	var out []cdp.Event
	for _, msg := range r.received() {
		if msg.ID == nil && msg.Method != "" {
			out = append(out, cdp.Event{Method: msg.Method, Params: msg.Params})
		}
	}
	return out
}

// Responses returns, per method, the response to the first recorded request
// of that method, correlated by id.
func (r Recording) Responses() map[string]cdp.Message {
	methods := make(map[int64]string)
	for _, fr := range r.Frames {
		if fr.Dir != cdp.DirectionSend {
			continue
		}
		var msg cdp.Message
		if err := json.Unmarshal(fr.Frame, &msg); err == nil && msg.ID != nil {
			methods[*msg.ID] = msg.Method
		}
	}
	out := make(map[string]cdp.Message)
	for _, msg := range r.received() {
		if msg.ID == nil {
			continue
		}
		method, ok := methods[*msg.ID]
		if !ok {
			continue
		}
		if _, seen := out[method]; !seen {
			out[method] = msg
		}
	}
	return out
}

func (r Recording) received() []cdp.Message {
	var out []cdp.Message
	for _, fr := range r.Frames {
		if fr.Dir != cdp.DirectionReceive {
			continue
		}
		var msg cdp.Message
		if err := json.Unmarshal(fr.Frame, &msg); err == nil {
			out = append(out, msg)
		}
	}
	return out
}

// ServeRecording registers one handler per recorded method that answers
// with the recorded result or error. Methods the recording never answered
// keep the method-not-found default.
func (s *Server) ServeRecording(r Recording) {
	for method, res := range r.Responses() {
		s.Handle(method, func(Request) (any, *Error) {
			if res.Error != nil {
				return nil, &Error{Code: res.Error.Code, Message: res.Error.Message}
			}
			return res.Result, nil
		})
	}
}

// ReplayEvents writes every recorded event frame to the connected clients,
// byte for byte and in order. Recorded timing is not reproduced.
func (s *Server) ReplayEvents(r Recording) {
	for _, fr := range r.Frames {
		if fr.Dir != cdp.DirectionReceive {
			continue
		}
		var msg cdp.Message
		if err := json.Unmarshal(fr.Frame, &msg); err != nil || msg.ID != nil || msg.Method == "" {
			continue
		}
		s.EmitRaw(fr.Frame)
	}
}
