package cdpws_test

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/Reidevl/rn-lens/internal/adapters/cdpws"
	"github.com/Reidevl/rn-lens/internal/adapters/cdpws/cdptest"
	"github.com/Reidevl/rn-lens/internal/platform/cdp"
)

const recordingPath = "testdata/rn088-expo-ios.jsonl"

// TestRecordedSessionFlowsThroughTheTransport replays the real Expo
// recording: the handshake calls get their recorded answers and every
// recorded event, real payload sizes included, arrives in order.
func TestRecordedSessionFlowsThroughTheTransport(t *testing.T) {
	t.Parallel()
	rec := cdptest.LoadRecording(t, recordingPath)
	srv := cdptest.New(t)
	srv.ServeRecording(rec)
	conn := dial(t, srv, cdpws.Options{})
	if !srv.WaitConnected(time.Second) {
		t.Fatal("client did not connect")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	responses := rec.Responses()
	if _, ok := responses[cdp.MethodReactNativeApplicationEnable]; !ok {
		t.Fatalf("recording answers %v, want ReactNativeApplication.enable among them", keys(responses))
	}
	for method, want := range responses {
		got, err := conn.Call(ctx, method, nil)
		if err != nil {
			t.Fatalf("Call(%s): %v", method, err)
		}
		if !jsonEqual(got, want.Result) {
			t.Fatalf("Call(%s) = %s, want recorded %s", method, got, want.Result)
		}
	}
	// The capture ended before Hermes answered Runtime.enable, so the fake
	// falls back to its method-not-found default for it.
	if _, err := conn.Call(ctx, cdp.MethodRuntimeEnable, nil); !errors.As(err, new(*cdp.Error)) {
		t.Fatalf("Call(Runtime.enable) = %v, want the fake's default error for an unrecorded method", err)
	}

	srv.ReplayEvents(rec)
	want := rec.Events()
	if len(want) == 0 {
		t.Fatal("recording has no events")
	}
	for i, ev := range want {
		select {
		case got := <-conn.Events():
			if got.Method != ev.Method || !jsonEqual(got.Params, ev.Params) {
				t.Fatalf("event %d = %s %s, want %s %s", i, got.Method, got.Params, ev.Method, ev.Params)
			}
		case <-time.After(5 * time.Second):
			t.Fatalf("event %d (%s) not received", i, ev.Method)
		}
	}
}

func TestLoadRecordingSplitsEventsAndResponses(t *testing.T) {
	t.Parallel()
	rec := cdptest.LoadRecording(t, recordingPath)
	if len(rec.Frames) == 0 {
		t.Fatal("no frames loaded")
	}
	res := rec.Responses()
	if _, ok := res[cdp.MethodReactNativeApplicationEnable]; !ok {
		t.Fatalf("Responses() = %v, want ReactNativeApplication.enable", keys(res))
	}
	var console int
	for _, ev := range rec.Events() {
		if ev.Method == cdp.EventRuntimeConsoleAPICalled {
			console++
		}
	}
	if console == 0 {
		t.Fatal("Events() has no consoleAPICalled entries")
	}
}

func jsonEqual(a, b json.RawMessage) bool {
	var av, bv any
	if err := json.Unmarshal(a, &av); err != nil {
		return false
	}
	if err := json.Unmarshal(b, &bv); err != nil {
		return false
	}
	return reflect.DeepEqual(av, bv)
}

func keys(m map[string]cdp.Message) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
