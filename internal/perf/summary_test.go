package perf_test

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/Reidevl/rn-lens/internal/perf"
)

func TestTraceSummary(t *testing.T) {
	t.Parallel()
	trace := perf.Trace{DataLoss: true, Events: []json.RawMessage{
		json.RawMessage(`{"name":"process_name","ph":"M","ts":0}`),
		json.RawMessage(`{"name":"a","ph":"X","ts":1000,"dur":500}`),
		json.RawMessage(`{"name":"b","ph":"I","ts":3500}`),
	}}
	var buf bytes.Buffer
	if err := perf.WriteTrace(&buf, trace); err != nil {
		t.Fatal(err)
	}
	got := trace.Summarize()
	want := perf.TraceSummary{Events: 3, DurationMs: 2.5, Bytes: buf.Len(), DataLoss: true}
	if got != want {
		t.Fatalf("Summarize = %+v, want %+v", got, want)
	}
	if empty := (perf.Trace{}).Summarize(); empty.DurationMs != 0 || empty.Events != 0 {
		t.Fatalf("empty summary = %+v", empty)
	}
}

func TestProfileSummary(t *testing.T) {
	t.Parallel()
	raw := json.RawMessage(`{"nodes":[{"id":1},{"id":2}],"samples":[1,2,2],"startTime":1000,"endTime":251000}`)
	got := perf.SummarizeProfile(raw)
	want := perf.ProfileSummary{Nodes: 2, Samples: 3, DurationMs: 250, Bytes: len(raw)}
	if got != want {
		t.Fatalf("SummarizeProfile = %+v, want %+v", got, want)
	}
}
