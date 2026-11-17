package cdp_test

import (
	"bufio"
	"encoding/json"
	"os"
	"testing"

	"github.com/Reidevl/rn-lens/internal/platform/cdp"
)

// fixtureEvents returns every received event frame of the recorded session.
func fixtureEvents(t *testing.T) []cdp.Event {
	t.Helper()
	f, err := os.Open("../../adapters/cdpws/testdata/rn088-expo-ios.jsonl")
	if err != nil {
		t.Fatalf("open fixture: %v", err)
	}
	defer f.Close()

	var events []cdp.Event
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 1<<24)
	for sc.Scan() {
		var rec struct {
			Dir   string      `json:"dir"`
			Frame cdp.Message `json:"frame"`
		}
		if err := json.Unmarshal(sc.Bytes(), &rec); err != nil {
			t.Fatalf("decode fixture line: %v", err)
		}
		if rec.Dir == "recv" && rec.Frame.ID == nil && rec.Frame.Method != "" {
			events = append(events, cdp.Event{Method: rec.Frame.Method, Params: rec.Frame.Params})
		}
	}
	return events
}

func TestFixtureConsoleEventsDecode(t *testing.T) {
	t.Parallel()
	var calls int
	var firstTick *cdp.ConsoleAPICalled
	for _, ev := range fixtureEvents(t) {
		if ev.Method != cdp.EventRuntimeConsoleAPICalled {
			continue
		}
		var p cdp.ConsoleAPICalled
		if err := json.Unmarshal(ev.Params, &p); err != nil {
			t.Fatalf("decode consoleAPICalled: %v", err)
		}
		calls++
		if firstTick == nil && len(p.Args) > 0 && string(p.Args[0].Value) == `"tick"` {
			firstTick = &p
		}
	}
	if calls == 0 {
		t.Fatal("fixture has no consoleAPICalled events")
	}
	if firstTick == nil {
		t.Fatal("fixture has no tick entry")
	}
	if firstTick.Type != "log" || firstTick.ExecutionContextID != 1 {
		t.Fatalf("tick entry = %+v", firstTick)
	}
	obj := firstTick.Args[1]
	if obj.Type != "object" || obj.ObjectID == "" || obj.Description != "Object" {
		t.Fatalf("object arg = %+v, want an object handle", obj)
	}
	if obj.Preview != nil {
		t.Fatal("Hermes sent a preview; the lazy-properties assumption no longer holds")
	}
	if firstTick.StackTrace == nil || len(firstTick.StackTrace.CallFrames) == 0 || firstTick.StackTrace.CallFrames[0].URL == "" {
		t.Fatalf("stack trace missing: %+v", firstTick.StackTrace)
	}
}

func TestFixtureHasExecutionContextCreated(t *testing.T) {
	t.Parallel()
	for _, ev := range fixtureEvents(t) {
		if ev.Method != cdp.EventRuntimeExecutionContextCreated {
			continue
		}
		var p cdp.ExecutionContextCreated
		if err := json.Unmarshal(ev.Params, &p); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if p.Context.ID != 1 || p.Context.Name != "main" {
			t.Fatalf("context = %+v", p.Context)
		}
		return
	}
	t.Fatal("fixture has no executionContextCreated event")
}

func TestEvaluateAndPropertiesRoundTrip(t *testing.T) {
	t.Parallel()
	params, _ := json.Marshal(cdp.EvaluateParams{Expression: "1+1", GeneratePreview: true, IncludeCommandLineAPI: true, AwaitPromise: true})
	if string(params) != `{"expression":"1+1","includeCommandLineAPI":true,"generatePreview":true,"awaitPromise":true}` {
		t.Fatalf("EvaluateParams json = %s", params)
	}

	var res cdp.EvaluateResult
	if err := json.Unmarshal([]byte(`{"result":{"type":"number","value":2,"description":"2"}}`), &res); err != nil {
		t.Fatal(err)
	}
	if res.Result.Type != "number" || string(res.Result.Value) != "2" {
		t.Fatalf("result = %+v", res.Result)
	}

	var props cdp.GetPropertiesResult
	raw := `{"result":[{"name":"n","value":{"type":"number","value":1,"description":"1"},"writable":true,"configurable":true,"enumerable":true,"isOwn":true},{"name":"get","get":{"type":"function","className":"Function","description":"function get() {}","objectId":"9"},"configurable":true,"enumerable":false,"isOwn":true}]}`
	if err := json.Unmarshal([]byte(raw), &props); err != nil {
		t.Fatal(err)
	}
	if len(props.Result) != 2 || props.Result[0].Value == nil || props.Result[1].Get == nil {
		t.Fatalf("properties = %+v", props.Result)
	}
}
