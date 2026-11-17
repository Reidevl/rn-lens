package console

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sync/atomic"
	"time"

	"github.com/Reidevl/rn-lens/internal/platform/cdp"
)

// Caller sends CDP requests to the attached app. *targets.Session satisfies it.
type Caller interface {
	Call(ctx context.Context, method string, params any) (json.RawMessage, error)
}

// ErrStaleObject is returned when a remote object handle predates the current
// JavaScript context (the app reloaded or Lens re-attached since).
var ErrStaleObject = errors.New("console: object is no longer available after the context was reset")

// Property is one property of an expanded remote object.
type Property struct {
	Name       string            `json:"name"`
	Value      *cdp.RemoteObject `json:"value,omitempty"`
	Summary    string            `json:"summary"`
	IsOwn      bool              `json:"isOwn"`
	Enumerable bool              `json:"enumerable"`
	Writable   bool              `json:"writable"`
	HasGetter  bool              `json:"hasGetter"`
}

// ServiceOptions configures a Service.
type ServiceOptions struct {
	// Notify is called after every appended entry. It must not block.
	Notify func()
	Logger *slog.Logger
	// EnableTimeout bounds the domain enable calls. Zero means 5 s.
	EnableTimeout time.Duration
}

// Service connects the store to a live session: it enables the Runtime and
// Log domains, ingests their events, evaluates expressions and expands objects.
type Service struct {
	store  *Store
	caller Caller
	opts   ServiceOptions

	attachments atomic.Uint64
}

// NewService creates a service over store using caller for requests.
func NewService(store *Store, caller Caller, opts ServiceOptions) *Service {
	if opts.Logger == nil {
		opts.Logger = slog.Default()
	}
	if opts.EnableTimeout <= 0 {
		opts.EnableTimeout = 5 * time.Second
	}
	return &Service{store: store, caller: caller, opts: opts}
}

// Store exposes the underlying log.
func (s *Service) Store() *Store { return s.store }

// HandleEvent ingests a CDP event. Wire it to the session's event handlers.
func (s *Service) HandleEvent(ev cdp.Event) {
	if _, ok := s.store.Ingest(ev); ok {
		s.notify()
	}
}

// OnAttached runs after every successful attach. It records the boundary,
// starts a new epoch on re-attach (a new debugger session invalidates every
// object handle) and enables the Runtime and Log domains. Hermes replays its
// buffered console messages on Runtime.enable, so earlier lines may repeat.
func (s *Service) OnAttached(ctx context.Context, appName string) error {
	n := s.attachments.Add(1)
	label := appName
	if label == "" {
		label = "app"
	}
	if n == 1 {
		s.store.Append(Entry{Kind: KindSystem, Source: SourceSystem, Text: "Attached to " + label})
	} else {
		s.store.ResetContext("Re-attached to " + label + " (earlier lines may repeat)")
	}
	s.notify()

	callCtx, cancel := context.WithTimeout(ctx, s.opts.EnableTimeout)
	defer cancel()
	if _, err := s.caller.Call(callCtx, cdp.MethodRuntimeEnable, nil); err != nil {
		return fmt.Errorf("console: enable Runtime: %w", err)
	}
	if _, err := s.caller.Call(callCtx, cdp.MethodLogEnable, nil); err != nil {
		// Older targets have no Log domain; console output still works.
		s.opts.Logger.Debug("Log.enable failed", "err", err)
	}
	return nil
}

// Evaluate runs expression in the app and records the command and its
// result (or the thrown exception). The returned entry is the result.
func (s *Service) Evaluate(ctx context.Context, expression string) (Entry, error) {
	s.store.Append(Entry{Kind: KindCommand, Source: SourceEval, Text: expression})
	s.notify()

	raw, err := s.caller.Call(ctx, cdp.MethodRuntimeEvaluate, cdp.EvaluateParams{
		Expression:            expression,
		IncludeCommandLineAPI: true,
		GeneratePreview:       true,
		AwaitPromise:          true,
	})
	if err != nil {
		e := s.store.Append(Entry{Kind: KindError, Source: SourceEval, Text: err.Error()})
		s.notify()
		return e, err
	}
	var res cdp.EvaluateResult
	if err := json.Unmarshal(raw, &res); err != nil {
		e := s.store.Append(Entry{Kind: KindError, Source: SourceEval, Text: "console: decode evaluate result: " + err.Error()})
		s.notify()
		return e, err
	}
	var e Entry
	if res.ExceptionDetails != nil {
		d := *res.ExceptionDetails
		text := d.Text
		if d.Exception != nil && d.Exception.Description != "" {
			text = firstLine(d.Exception.Description)
		}
		e = Entry{Kind: KindException, Source: SourceEval, Text: text, Stack: d.StackTrace, Exception: &d}
		if d.Exception != nil {
			e.Args = []cdp.RemoteObject{*d.Exception}
		}
	} else {
		e = Entry{Kind: KindResult, Source: SourceEval, Text: FormatRemoteObject(res.Result), Args: []cdp.RemoteObject{res.Result}}
	}
	e = s.store.Append(e)
	s.notify()
	return e, nil
}

// Properties expands a remote object. epoch is the entry's epoch; handles
// from an older epoch are refused locally with ErrStaleObject.
func (s *Service) Properties(ctx context.Context, objectID string, epoch uint64) ([]Property, error) {
	if objectID == "" {
		return nil, errors.New("console: empty object id")
	}
	if s.store.IsStale(epoch) {
		return nil, ErrStaleObject
	}
	raw, err := s.caller.Call(ctx, cdp.MethodRuntimeGetProperties, cdp.GetPropertiesParams{
		ObjectID:        objectID,
		OwnProperties:   true,
		GeneratePreview: true,
	})
	if err != nil {
		return nil, err
	}
	var res cdp.GetPropertiesResult
	if err := json.Unmarshal(raw, &res); err != nil {
		return nil, fmt.Errorf("console: decode properties: %w", err)
	}
	out := make([]Property, 0, len(res.Result))
	for _, d := range res.Result {
		p := Property{Name: d.Name, Value: d.Value, IsOwn: d.IsOwn, Enumerable: d.Enumerable, Writable: d.Writable, HasGetter: d.Get != nil}
		switch {
		case d.Value != nil:
			p.Summary = FormatRemoteObject(*d.Value)
		case d.Get != nil:
			p.Summary = "(getter)"
		default:
			p.Summary = "undefined"
		}
		out = append(out, p)
	}
	return out, nil
}

func (s *Service) notify() {
	if s.opts.Notify != nil {
		s.opts.Notify()
	}
}
