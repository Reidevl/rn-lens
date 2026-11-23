package console_test

import (
	"bufio"
	"encoding/json"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/Reidevl/rn-lens/internal/console"
	"github.com/Reidevl/rn-lens/internal/platform/cdp"
	"github.com/Reidevl/rn-lens/internal/platform/clock"
)

func event(t *testing.T, method string, params any) cdp.Event {
	t.Helper()
	raw, err := json.Marshal(params)
	if err != nil {
		t.Fatal(err)
	}
	return cdp.Event{Method: method, Params: raw}
}

func TestIngestConsoleAPICalled(t *testing.T) {
	t.Parallel()
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	s := console.NewStore(10, clock.NewFake(start))

	e, ok := s.Ingest(event(t, cdp.EventRuntimeConsoleAPICalled, cdp.ConsoleAPICalled{
		Type:               "warning",
		Args:               []cdp.RemoteObject{str("careful"), num(7)},
		ExecutionContextID: 1,
		StackTrace:         &cdp.StackTrace{CallFrames: []cdp.CallFrame{{FunctionName: "f", URL: "http://localhost:8081/index.bundle", LineNumber: 10, ColumnNumber: 4}}},
	}))
	if !ok {
		t.Fatal("event was not ingested")
	}
	if e.Seq != 1 || e.Kind != console.KindWarning || e.Level != console.LevelWarning || e.Source != console.SourceConsole {
		t.Fatalf("entry = %+v", e)
	}
	if e.Text != "careful 7" || len(e.Args) != 2 || e.ContextID != 1 || e.Epoch != 1 {
		t.Fatalf("entry = %+v", e)
	}
	if e.Location == nil || e.Location.Line != 10 || e.Location.URL == "" {
		t.Fatalf("location = %+v", e.Location)
	}
	if !e.Timestamp.Equal(start) {
		t.Fatalf("timestamp = %v, want the clock's now when CDP omits it", e.Timestamp)
	}
}

func TestIngestStripsANSIEscapeCodes(t *testing.T) {
	t.Parallel()
	s := console.NewStore(10, nil)
	// React Native's DevTools banner, colored for a terminal.
	banner := "\x1b[48;2;253;247;231m\x1b[30mWelcome to \x1b[1mReact Native DevTools\x1b[22m\x1b[0m"
	e, ok := s.Ingest(event(t, cdp.EventRuntimeConsoleAPICalled, cdp.ConsoleAPICalled{
		Type: "log",
		Args: []cdp.RemoteObject{str(banner), num(7)},
	}))
	if !ok {
		t.Fatal("event was not ingested")
	}
	if e.Text != "Welcome to React Native DevTools 7" {
		t.Fatalf("text = %q", e.Text)
	}
	var first string
	if err := json.Unmarshal(e.Args[0].Value, &first); err != nil || first != "Welcome to React Native DevTools" {
		t.Fatalf("first arg = %q (%v)", first, err)
	}
	if string(e.Args[1].Value) != "7" {
		t.Fatalf("number arg changed: %s", e.Args[1].Value)
	}

	logged, _ := s.Ingest(event(t, cdp.EventLogEntryAdded, cdp.LogEntryAdded{Entry: cdp.LogEntry{Source: "javascript", Level: "info", Text: "\x1b[32mready\x1b(B\x1b[m"}}))
	if logged.Text != "ready" {
		t.Fatalf("log entry text = %q", logged.Text)
	}
}

func TestIngestStripsANSIFromExceptions(t *testing.T) {
	t.Parallel()
	s := console.NewStore(10, nil)
	e, ok := s.Ingest(event(t, cdp.EventRuntimeExceptionThrown, cdp.ExceptionThrown{
		ExceptionDetails: cdp.ExceptionDetails{
			Text:      "Uncaught",
			Exception: &cdp.RemoteObject{Type: "object", Subtype: "error", Description: "Error: \x1b[31mboom\x1b[0m\n    at f"},
		},
	}))
	if !ok {
		t.Fatal("not ingested")
	}
	if e.Text != "Error: boom" {
		t.Fatalf("text = %q", e.Text)
	}
}

func TestIngestUsesCDPTimestampWhenPresent(t *testing.T) {
	t.Parallel()
	s := console.NewStore(10, clock.NewFake(time.Unix(0, 0)))
	e, _ := s.Ingest(event(t, cdp.EventRuntimeConsoleAPICalled, cdp.ConsoleAPICalled{Type: "log", Timestamp: 1_700_000_000_500}))
	if e.Timestamp.UnixMilli() != 1_700_000_000_500 {
		t.Fatalf("timestamp = %v", e.Timestamp)
	}
}

func TestIngestExceptionThrown(t *testing.T) {
	t.Parallel()
	s := console.NewStore(10, nil)
	e, ok := s.Ingest(event(t, cdp.EventRuntimeExceptionThrown, cdp.ExceptionThrown{
		ExceptionDetails: cdp.ExceptionDetails{
			Text:       "Uncaught",
			LineNumber: 3,
			URL:        "http://localhost:8081/index.bundle",
			Exception:  &cdp.RemoteObject{Type: "object", Subtype: "error", Description: "Error: Lens example: intentional error\n    at onPress"},
			StackTrace: &cdp.StackTrace{CallFrames: []cdp.CallFrame{{FunctionName: "onPress"}}},
		},
	}))
	if !ok {
		t.Fatal("not ingested")
	}
	if e.Kind != console.KindException || e.Level != console.LevelError || e.Text != "Error: Lens example: intentional error" {
		t.Fatalf("entry = %+v", e)
	}
	if e.Exception == nil || e.Location == nil || e.Location.Line != 3 || len(e.Args) != 1 {
		t.Fatalf("entry = %+v", e)
	}
}

func TestIngestLogEntryAdded(t *testing.T) {
	t.Parallel()
	s := console.NewStore(10, nil)
	e, ok := s.Ingest(event(t, cdp.EventLogEntryAdded, cdp.LogEntryAdded{Entry: cdp.LogEntry{Source: "network", Level: "error", Text: "Failed to load resource", URL: "https://x/y", LineNumber: 0}}))
	if !ok {
		t.Fatal("not ingested")
	}
	if e.Kind != console.KindError || e.Source != console.SourceLog || e.Text != "[network] Failed to load resource" {
		t.Fatalf("entry = %+v", e)
	}
	verbose, _ := s.Ingest(event(t, cdp.EventLogEntryAdded, cdp.LogEntryAdded{Entry: cdp.LogEntry{Source: "javascript", Level: "verbose", Text: "v"}}))
	if verbose.Kind != console.KindDebug || verbose.Level != console.LevelVerbose || verbose.Text != "v" {
		t.Fatalf("verbose entry = %+v", verbose)
	}
}

func TestContextResetBumpsEpochAndMarksOldEntriesStale(t *testing.T) {
	t.Parallel()
	s := console.NewStore(10, nil)
	before, _ := s.Ingest(event(t, cdp.EventRuntimeConsoleAPICalled, cdp.ConsoleAPICalled{Type: "log", Args: []cdp.RemoteObject{str("a")}}))

	marker, ok := s.Ingest(event(t, cdp.EventRuntimeExecutionContextsClear, nil))
	if !ok || marker.Kind != console.KindSystem || marker.Level != console.LevelSystem || marker.Text == "" {
		t.Fatalf("marker = %+v", marker)
	}
	after, _ := s.Ingest(event(t, cdp.EventRuntimeConsoleAPICalled, cdp.ConsoleAPICalled{Type: "log", Args: []cdp.RemoteObject{str("b")}}))

	if before.Epoch != 1 || marker.Epoch != 2 || after.Epoch != 2 || s.Epoch() != 2 {
		t.Fatalf("epochs = %d %d %d (store %d)", before.Epoch, marker.Epoch, after.Epoch, s.Epoch())
	}
	if !s.IsStale(before.Epoch) || s.IsStale(after.Epoch) {
		t.Fatal("staleness does not follow the epoch")
	}
}

func TestIgnoredEventsAndEviction(t *testing.T) {
	t.Parallel()
	s := console.NewStore(2, nil)
	if _, ok := s.Ingest(event(t, cdp.EventRuntimeExecutionContextCreated, cdp.ExecutionContextCreated{})); ok {
		t.Fatal("executionContextCreated should be ignored")
	}
	if _, ok := s.Ingest(cdp.Event{Method: "Network.requestWillBeSent"}); ok {
		t.Fatal("unrelated events should be ignored")
	}
	for _, text := range []string{"1", "2", "3"} {
		s.Ingest(event(t, cdp.EventRuntimeConsoleAPICalled, cdp.ConsoleAPICalled{Type: "log", Args: []cdp.RemoteObject{str(text)}}))
	}
	st := s.Stats()
	if st.Len != 2 || st.Dropped != 1 || st.OldestSeq != 2 || st.LastSeq != 3 {
		t.Fatalf("stats = %+v", st)
	}
	got := s.Range(1, 0)
	if len(got) != 2 || got[0].Text != "2" || got[0].Seq != 2 || got[1].Text != "3" {
		t.Fatalf("range = %+v", got)
	}
	s.Clear()
	if st := s.Stats(); st.Len != 0 || st.Dropped != 0 {
		t.Fatalf("stats after clear = %+v", st)
	}
}

// TestIngestRecordedSession replays the real Expo recording.
func TestIngestRecordedSession(t *testing.T) {
	t.Parallel()
	f, err := os.Open("../adapters/cdpws/testdata/rn088-expo-ios.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	s := console.NewStore(0, nil)
	consoleFrames, ingested := 0, 0
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 1<<24)
	for sc.Scan() {
		var rec struct {
			Dir   string      `json:"dir"`
			Frame cdp.Message `json:"frame"`
		}
		if err := json.Unmarshal(sc.Bytes(), &rec); err != nil {
			t.Fatal(err)
		}
		if rec.Dir != "recv" || rec.Frame.Method == "" {
			continue
		}
		if rec.Frame.Method == cdp.EventRuntimeConsoleAPICalled {
			consoleFrames++
		}
		if _, ok := s.Ingest(cdp.Event{Method: rec.Frame.Method, Params: rec.Frame.Params}); ok {
			ingested++
		}
	}
	if consoleFrames == 0 || ingested != consoleFrames {
		t.Fatalf("ingested %d entries from %d console frames", ingested, consoleFrames)
	}
	first := s.Range(1, 1)[0]
	if first.Text != "tick Object" || first.Kind != console.KindLog || first.Args[1].ObjectID == "" {
		t.Fatalf("first entry = %+v", first)
	}
}

func TestClearAdvancesTheClearedGeneration(t *testing.T) {
	t.Parallel()
	s := console.NewStore(10, nil)
	s.Append(console.Entry{Kind: console.KindLog, Text: "a"})
	if s.Stats().Cleared != 0 {
		t.Fatal("fresh store should report zero clears")
	}
	s.Clear()
	s.Append(console.Entry{Kind: console.KindLog, Text: "b"})
	st := s.Stats()
	if st.Cleared != 1 || st.Len != 1 {
		t.Fatalf("stats after clear + append = %+v", st)
	}
}

// Appends racing a context reset must never land on the wrong side of the
// marker with the wrong epoch.
func TestAppendAndResetContextKeepEpochsOrdered(t *testing.T) {
	t.Parallel()
	s := console.NewStore(10_000, nil)
	var wg sync.WaitGroup
	for w := range 4 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range 300 {
				if w == 0 && i%50 == 0 {
					s.ResetContext("reset")
					continue
				}
				s.Append(console.Entry{Kind: console.KindLog, Text: "x"})
			}
		}()
	}
	wg.Wait()

	epoch := uint64(0)
	for _, e := range s.Range(1, 0) {
		if e.Epoch < epoch {
			t.Fatalf("entry %d has epoch %d after an entry with epoch %d", e.Seq, e.Epoch, epoch)
		}
		if e.Kind == console.KindSystem && e.Epoch == epoch {
			t.Fatalf("marker %d did not start a new epoch", e.Seq)
		}
		epoch = e.Epoch
	}
}
