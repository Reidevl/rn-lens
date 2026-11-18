package perf

import "encoding/json"

// TraceSummary describes a finished trace for the panel.
type TraceSummary struct {
	Events int `json:"events"`
	// DurationMs spans the first to the last timed event.
	DurationMs float64 `json:"durationMs"`
	// Bytes is the size of the exported file.
	Bytes    int  `json:"bytes"`
	DataLoss bool `json:"dataLoss"`
}

// Summarize measures t. Metadata events (ph "M") carry no time and are
// left out of the duration.
func (t Trace) Summarize() TraceSummary {
	s := TraceSummary{Events: len(t.Events), DataLoss: t.DataLoss, Bytes: len(`{"traceEvents":[]}`) + 1}
	first, last, timed := 0.0, 0.0, false
	for i, raw := range t.Events {
		s.Bytes += len(raw)
		if i > 0 {
			s.Bytes++
		}
		var ev struct {
			Ph  string   `json:"ph"`
			Ts  *float64 `json:"ts"`
			Dur float64  `json:"dur"`
		}
		if json.Unmarshal(raw, &ev) != nil || ev.Ts == nil || ev.Ph == "M" {
			continue
		}
		start, end := *ev.Ts, *ev.Ts+ev.Dur
		if !timed || start < first {
			first = start
		}
		if !timed || end > last {
			last = end
		}
		timed = true
	}
	if timed {
		s.DurationMs = (last - first) / 1000
	}
	return s
}

// ProfileSummary describes a CPU profile for the panel.
type ProfileSummary struct {
	Nodes      int     `json:"nodes"`
	Samples    int     `json:"samples"`
	DurationMs float64 `json:"durationMs"`
	Bytes      int     `json:"bytes"`
}

// SummarizeProfile measures a Profiler.Profile.
func SummarizeProfile(profile json.RawMessage) ProfileSummary {
	var p struct {
		Nodes     []json.RawMessage `json:"nodes"`
		Samples   []int64           `json:"samples"`
		StartTime float64           `json:"startTime"`
		EndTime   float64           `json:"endTime"`
	}
	_ = json.Unmarshal(profile, &p)
	return ProfileSummary{
		Nodes:      len(p.Nodes),
		Samples:    len(p.Samples),
		DurationMs: (p.EndTime - p.StartTime) / 1000,
		Bytes:      len(profile),
	}
}
