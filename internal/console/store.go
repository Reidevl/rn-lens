package console

import (
	"encoding/json"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Reidevl/rn-lens/internal/platform/cdp"
	"github.com/Reidevl/rn-lens/internal/platform/clock"
	"github.com/Reidevl/rn-lens/internal/platform/ringbuf"
)

// DefaultCapacity is how many entries a store keeps by default.
const DefaultCapacity = 10_000

// Stats summarises a store for the notify-then-pull UI.
type Stats struct {
	LastSeq   uint64 `json:"lastSeq"`
	OldestSeq uint64 `json:"oldestSeq"`
	Dropped   uint64 `json:"dropped"`
	Len       int    `json:"len"`
	Epoch     uint64 `json:"epoch"`
	// Cleared counts Clear calls so a reader can tell a clear apart from a
	// buffer that was emptied and refilled between two pulls.
	Cleared uint64 `json:"cleared"`
}

// Store is the bounded console log. It is safe for concurrent use.
type Store struct {
	buf *ringbuf.Buffer[Entry]
	clk clock.Clock
	// mu orders epoch changes against appends so an entry can never carry an
	// epoch from the wrong side of a reset marker. epoch stays atomic for
	// lock-free IsStale reads.
	mu      sync.Mutex
	epoch   atomic.Uint64
	cleared atomic.Uint64
}

// NewStore creates a store keeping capacity entries (DefaultCapacity when <= 0).
func NewStore(capacity int, clk clock.Clock) *Store {
	if capacity <= 0 {
		capacity = DefaultCapacity
	}
	if clk == nil {
		clk = clock.Real{}
	}
	s := &Store{buf: ringbuf.New[Entry](capacity), clk: clk}
	s.epoch.Store(1)
	return s
}

// Append stores e, filling Seq, Epoch, Level and a missing Timestamp.
func (s *Store) Append(e Entry) Entry {
	if e.Timestamp.IsZero() {
		e.Timestamp = s.clk.Now()
	}
	if e.Level == "" {
		e.Level = LevelOf(e.Kind)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.appendLocked(e)
}

// appendLocked requires s.mu.
func (s *Store) appendLocked(e Entry) Entry {
	e.Epoch = s.epoch.Load()
	e.Seq = s.buf.Append(e)
	return e
}

// Range returns up to limit entries with Seq >= fromSeq, oldest first.
func (s *Store) Range(fromSeq uint64, limit int) []Entry {
	items := s.buf.Range(fromSeq, limit)
	out := make([]Entry, len(items))
	for i, it := range items {
		e := it.Value
		e.Seq = it.Seq
		out[i] = e
	}
	return out
}

// Stats reports the buffer position and the current epoch.
func (s *Store) Stats() Stats {
	return Stats{
		LastSeq:   s.buf.LastSeq(),
		OldestSeq: s.buf.OldestSeq(),
		Dropped:   s.buf.Dropped(),
		Len:       s.buf.Len(),
		Epoch:     s.epoch.Load(),
		Cleared:   s.cleared.Load(),
	}
}

// Clear drops every entry. Sequence numbers keep increasing and the cleared
// generation advances so pulls can detect the clear.
func (s *Store) Clear() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.buf.Clear()
	s.cleared.Add(1)
}

// Epoch is the current JavaScript context generation.
func (s *Store) Epoch() uint64 { return s.epoch.Load() }

// IsStale reports whether an entry's object handles are no longer valid.
func (s *Store) IsStale(entryEpoch uint64) bool { return entryEpoch < s.epoch.Load() }

// ResetContext starts a new epoch (the app reloaded or re-attached) and
// records a system entry so the user sees the boundary.
func (s *Store) ResetContext(reason string) Entry {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.epoch.Add(1)
	return s.appendLocked(Entry{Kind: KindSystem, Source: SourceSystem, Text: reason, Timestamp: s.clk.Now(), Level: LevelSystem})
}

// Ingest converts a CDP event into an entry and stores it. It reports false
// for events the console does not care about.
func (s *Store) Ingest(ev cdp.Event) (Entry, bool) {
	switch ev.Method {
	case cdp.EventRuntimeConsoleAPICalled:
		var p cdp.ConsoleAPICalled
		if err := json.Unmarshal(ev.Params, &p); err != nil {
			return Entry{}, false
		}
		kind := kindFromConsoleType(p.Type)
		args := stripANSIArgs(p.Args)
		e := Entry{
			Timestamp: msToTime(p.Timestamp),
			Kind:      kind,
			Source:    SourceConsole,
			Text:      FormatArgs(args),
			Args:      args,
			Stack:     p.StackTrace,
			ContextID: p.ExecutionContextID,
		}
		if kind == KindAssert && e.Text == "" {
			e.Text = "Assertion failed"
		}
		if loc := topFrame(p.StackTrace); loc != nil {
			e.Location = loc
		}
		return s.Append(e), true

	case cdp.EventRuntimeExceptionThrown:
		var p cdp.ExceptionThrown
		if err := json.Unmarshal(ev.Params, &p); err != nil {
			return Entry{}, false
		}
		d := p.ExceptionDetails
		text := d.Text
		if d.Exception != nil {
			if desc := firstLine(d.Exception.Description); desc != "" {
				text = desc
			}
		}
		if text == "" {
			text = "Uncaught exception"
		}
		details := d
		e := Entry{
			Timestamp: msToTime(p.Timestamp),
			Kind:      KindException,
			Source:    SourceException,
			Text:      text,
			Stack:     d.StackTrace,
			ContextID: d.ExecutionContextID,
			Exception: &details,
		}
		if d.Exception != nil {
			e.Args = []cdp.RemoteObject{*d.Exception}
		}
		if d.URL != "" {
			e.Location = &Location{URL: d.URL, Line: d.LineNumber, Column: d.ColumnNumber}
		} else if loc := topFrame(d.StackTrace); loc != nil {
			e.Location = loc
		}
		return s.Append(e), true

	case cdp.EventLogEntryAdded:
		var p cdp.LogEntryAdded
		if err := json.Unmarshal(ev.Params, &p); err != nil {
			return Entry{}, false
		}
		le := p.Entry
		text := stripANSI(le.Text)
		if le.Source != "" && le.Source != "javascript" && le.Source != "other" {
			text = "[" + le.Source + "] " + text
		}
		e := Entry{
			Timestamp: msToTime(le.Timestamp),
			Kind:      kindFromLogLevel(le.Level),
			Source:    SourceLog,
			Text:      text,
			Stack:     le.StackTrace,
		}
		if le.URL != "" {
			e.Location = &Location{URL: le.URL, Line: le.LineNumber}
		}
		return s.Append(e), true

	case cdp.EventRuntimeExecutionContextsClear:
		return s.ResetContext("JavaScript context was reset (app reloaded)"), true

	default:
		return Entry{}, false
	}
}

func topFrame(st *cdp.StackTrace) *Location {
	if st == nil || len(st.CallFrames) == 0 {
		return nil
	}
	f := st.CallFrames[0]
	return &Location{URL: f.URL, Line: f.LineNumber, Column: f.ColumnNumber}
}

func msToTime(ms float64) time.Time {
	if ms <= 0 {
		return time.Time{}
	}
	sec := int64(ms / 1000)
	nsec := int64((ms - float64(sec)*1000) * 1e6)
	return time.Unix(sec, nsec)
}
