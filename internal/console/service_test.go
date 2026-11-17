package console_test

import (
	"context"
	"encoding/json"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Reidevl/rn-lens/internal/adapters/cdpws"
	"github.com/Reidevl/rn-lens/internal/adapters/cdpws/cdptest"
	"github.com/Reidevl/rn-lens/internal/console"
	"github.com/Reidevl/rn-lens/internal/platform/cdp"
)

func newService(t *testing.T, srv *cdptest.Server) (*console.Service, *atomic.Int64) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	conn, err := cdpws.Dial(ctx, srv.URL(), cdpws.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	var notified atomic.Int64
	svc := console.NewService(console.NewStore(100, nil), conn, console.ServiceOptions{Notify: func() { notified.Add(1) }})
	return svc, &notified
}

func TestOnAttachedEnablesRuntimeThenLogAndMarksBoundaries(t *testing.T) {
	t.Parallel()
	srv := cdptest.New(t)
	srv.Handle(cdp.MethodRuntimeEnable, func(cdptest.Request) (any, *cdptest.Error) { return nil, nil })
	svc, notified := newService(t, srv)

	if err := svc.OnAttached(context.Background(), "Example"); err != nil {
		t.Fatalf("OnAttached: %v", err)
	}
	if err := svc.OnAttached(context.Background(), "Example"); err != nil {
		t.Fatalf("second OnAttached: %v", err)
	}

	methods := srv.Methods()
	if len(methods) != 4 || methods[0] != cdp.MethodRuntimeEnable || methods[1] != cdp.MethodLogEnable {
		t.Fatalf("methods = %v, want Runtime.enable then Log.enable twice", methods)
	}
	entries := svc.Store().Range(1, 0)
	if len(entries) != 2 || entries[0].Text != "Attached to Example" || entries[1].Kind != console.KindSystem || entries[1].Epoch != 2 {
		t.Fatalf("entries = %+v", entries)
	}
	if notified.Load() < 2 {
		t.Fatalf("notified %d times, want at least 2", notified.Load())
	}
}

func TestOnAttachedFailsWhenRuntimeEnableFails(t *testing.T) {
	t.Parallel()
	srv := cdptest.New(t)
	srv.Handle(cdp.MethodRuntimeEnable, func(cdptest.Request) (any, *cdptest.Error) {
		return nil, &cdptest.Error{Code: -32000, Message: "no runtime"}
	})
	svc, _ := newService(t, srv)
	err := svc.OnAttached(context.Background(), "x")
	var cdpErr *cdp.Error
	if !errors.As(err, &cdpErr) {
		t.Fatalf("err = %v, want *cdp.Error", err)
	}
}

func TestHandleEventIngestsAndNotifies(t *testing.T) {
	t.Parallel()
	srv := cdptest.New(t)
	svc, notified := newService(t, srv)
	raw, _ := json.Marshal(cdp.ConsoleAPICalled{Type: "log", Args: []cdp.RemoteObject{str("hi")}})

	svc.HandleEvent(cdp.Event{Method: cdp.EventRuntimeConsoleAPICalled, Params: raw})
	svc.HandleEvent(cdp.Event{Method: "Network.loadingFinished", Params: json.RawMessage(`{}`)})

	if got := svc.Store().Range(1, 0); len(got) != 1 || got[0].Text != "hi" {
		t.Fatalf("entries = %+v", got)
	}
	if notified.Load() != 1 {
		t.Fatalf("notified %d times, want 1", notified.Load())
	}
}

func TestEvaluateRecordsCommandAndResult(t *testing.T) {
	t.Parallel()
	srv := cdptest.New(t)
	srv.Handle(cdp.MethodRuntimeEvaluate, func(req cdptest.Request) (any, *cdptest.Error) {
		var p cdp.EvaluateParams
		_ = json.Unmarshal(req.Params, &p)
		if !p.GeneratePreview || !p.IncludeCommandLineAPI || !p.AwaitPromise {
			return nil, &cdptest.Error{Code: -1, Message: "unexpected params"}
		}
		switch p.Expression {
		case "1+1":
			return cdp.EvaluateResult{Result: cdp.RemoteObject{Type: "number", Value: json.RawMessage("2"), Description: "2"}}, nil
		case "boom()":
			return cdp.EvaluateResult{
				Result:           cdp.RemoteObject{Type: "object", Subtype: "error", Description: "ReferenceError: boom is not defined"},
				ExceptionDetails: &cdp.ExceptionDetails{Text: "Uncaught", Exception: &cdp.RemoteObject{Type: "object", Subtype: "error", Description: "ReferenceError: boom is not defined\n    at <eval>"}},
			}, nil
		}
		return nil, &cdptest.Error{Code: -32000, Message: "nope"}
	})
	svc, _ := newService(t, srv)

	res, err := svc.Evaluate(context.Background(), "1+1")
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	if res.Kind != console.KindResult || res.Text != "2" || res.Source != console.SourceEval {
		t.Fatalf("result = %+v", res)
	}

	exc, err := svc.Evaluate(context.Background(), "boom()")
	if err != nil {
		t.Fatalf("Evaluate(boom): %v", err)
	}
	if exc.Kind != console.KindException || exc.Text != "ReferenceError: boom is not defined" || exc.Exception == nil {
		t.Fatalf("exception entry = %+v", exc)
	}

	failed, err := svc.Evaluate(context.Background(), "???")
	if err == nil || failed.Kind != console.KindError {
		t.Fatalf("protocol error: entry=%+v err=%v", failed, err)
	}

	kinds := make([]console.Kind, 0, 6)
	for _, e := range svc.Store().Range(1, 0) {
		kinds = append(kinds, e.Kind)
	}
	want := []console.Kind{console.KindCommand, console.KindResult, console.KindCommand, console.KindException, console.KindCommand, console.KindError}
	if len(kinds) != len(want) {
		t.Fatalf("kinds = %v", kinds)
	}
	for i := range want {
		if kinds[i] != want[i] {
			t.Fatalf("kinds = %v, want %v", kinds, want)
		}
	}
}

func TestPropertiesExpandsAndRefusesStaleHandles(t *testing.T) {
	t.Parallel()
	srv := cdptest.New(t)
	srv.Handle(cdp.MethodRuntimeGetProperties, func(req cdptest.Request) (any, *cdptest.Error) {
		var p cdp.GetPropertiesParams
		_ = json.Unmarshal(req.Params, &p)
		if p.ObjectID != "7" || !p.OwnProperties {
			return nil, &cdptest.Error{Code: -1, Message: "unexpected params"}
		}
		return cdp.GetPropertiesResult{Result: []cdp.PropertyDescriptor{
			{Name: "n", Value: &cdp.RemoteObject{Type: "number", Value: json.RawMessage("1")}, IsOwn: true, Enumerable: true, Writable: true},
			{Name: "nested", Value: &cdp.RemoteObject{Type: "object", ObjectID: "8", Description: "Object"}, IsOwn: true, Enumerable: true},
			{Name: "lazy", Get: &cdp.RemoteObject{Type: "function"}, IsOwn: true},
		}}, nil
	})
	svc, _ := newService(t, srv)

	props, err := svc.Properties(context.Background(), "7", svc.Store().Epoch())
	if err != nil {
		t.Fatalf("Properties: %v", err)
	}
	if len(props) != 3 || props[0].Summary != "1" || props[1].Value.ObjectID != "8" || !props[2].HasGetter || props[2].Summary != "(getter)" {
		t.Fatalf("props = %+v", props)
	}

	old := svc.Store().Epoch()
	svc.Store().ResetContext("reload")
	if _, err := svc.Properties(context.Background(), "7", old); !errors.Is(err, console.ErrStaleObject) {
		t.Fatalf("stale err = %v, want ErrStaleObject", err)
	}
	if n := len(srv.Methods()); n != 1 {
		t.Fatalf("stale lookup reached the target (%d calls)", n)
	}
}
