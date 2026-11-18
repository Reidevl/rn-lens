package queries_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Reidevl/rn-lens/internal/protocol"
	"github.com/Reidevl/rn-lens/internal/queries"
)

type fakeSender struct {
	mu     sync.Mutex
	frames []protocol.Message
	fail   error
	// reply, when set, answers query.action frames asynchronously.
	reply     func(action protocol.QueryAction) *protocol.ActionResult
	svc       *queries.Service
	key       string
	sessionID string
}

func (f *fakeSender) Send(typ, id string, payload any) error {
	if f.fail != nil {
		return f.fail
	}
	raw, _ := json.Marshal(payload)
	f.mu.Lock()
	f.frames = append(f.frames, protocol.Message{V: 1, Type: typ, ID: id, Payload: raw})
	f.mu.Unlock()
	if typ == protocol.TypeQueryAction && f.reply != nil {
		var a protocol.QueryAction
		_ = json.Unmarshal(raw, &a)
		if res := f.reply(a); res != nil {
			go func() {
				rawRes, _ := json.Marshal(res)
				_ = f.svc.Message(f.key, f.sessionID, protocol.Message{V: 1, Type: protocol.TypeActionResult, Payload: rawRes})
			}()
		}
	}
	return nil
}

func (f *fakeSender) sent(typ string) []protocol.Message {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []protocol.Message
	for _, m := range f.frames {
		if m.Type == typ {
			out = append(out, m)
		}
	}
	return out
}

func fixture(t *testing.T, name string) protocol.Message {
	t.Helper()
	raw, err := os.ReadFile("../../protocol/fixtures/" + name + ".json")
	if err != nil {
		t.Fatal(err)
	}
	msg, err := protocol.Decode(raw)
	if err != nil {
		t.Fatal(err)
	}
	return msg
}

func hello() protocol.Hello {
	return protocol.Hello{
		SDKVersion: "0.1.0", ProtocolVersion: 1,
		App:      protocol.AppInfo{ID: "com.example.app", Name: "Example", Platform: "ios", RNVersion: "0.88.1"},
		Device:   protocol.DeviceInfo{ID: "dev-1", Name: "iPhone"},
		Features: []string{protocol.FeatureQueries},
	}
}

func connect(t *testing.T) (*queries.Service, *fakeSender, string, *int) {
	t.Helper()
	notifies := 0
	var mu sync.Mutex
	svc := queries.NewService(func() { mu.Lock(); notifies++; mu.Unlock() })
	sender := &fakeSender{svc: svc, sessionID: "s1"}
	dev := svc.Connect("s1", hello(), sender)
	sender.key = dev.Key
	return svc, sender, dev.Key, &notifies
}

func TestSnapshotAndLabels(t *testing.T) {
	t.Parallel()
	svc, _, key, _ := connect(t)
	if err := svc.Message(key, "s1", fixture(t, "queries.snapshot")); err != nil {
		t.Fatal(err)
	}
	list, counts, err := svc.Queries(key)
	if err != nil || len(list) != 1 || counts.Total != 1 || counts.Fresh != 1 {
		t.Fatalf("queries = %+v counts = %+v err = %v", list, counts, err)
	}
	if list[0].Label != queries.LabelFresh || list[0].QueryHash != `["todos",{"page":1}]` {
		t.Fatalf("first = %+v", list[0])
	}
	muts, err := svc.Mutations(key)
	if err != nil || len(muts) != 1 || muts[0].MutationID != 7 {
		t.Fatalf("mutations = %+v err = %v", muts, err)
	}
	devs := svc.Devices()
	if len(devs) != 1 || devs[0].Key != key || !devs[0].Online || devs[0].App.Name != "Example" {
		t.Fatalf("devices = %+v", devs)
	}
}

func TestLabelOf(t *testing.T) {
	t.Parallel()
	base := func() protocol.Query {
		return protocol.Query{ObserversCount: 1, State: protocol.QueryState{Status: "success", FetchStatus: "idle"}}
	}
	tests := []struct {
		name string
		mut  func(q *protocol.Query)
		want queries.Label
	}{
		{"fetching wins", func(q *protocol.Query) { q.State.FetchStatus = "fetching"; q.ObserversCount = 0 }, queries.LabelFetching},
		{"no observers is inactive", func(q *protocol.Query) { q.ObserversCount = 0; q.IsStale = true }, queries.LabelInactive},
		{"paused", func(q *protocol.Query) { q.State.FetchStatus = "paused"; q.IsStale = true }, queries.LabelPaused},
		{"stale", func(q *protocol.Query) { q.IsStale = true }, queries.LabelStale},
		{"fresh", func(*protocol.Query) {}, queries.LabelFresh},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			q := base()
			tt.mut(&q)
			if got := queries.LabelOf(q); got != tt.want {
				t.Fatalf("LabelOf = %s, want %s", got, tt.want)
			}
		})
	}
}

func TestEventsApplyDeltasAndOrdering(t *testing.T) {
	t.Parallel()
	svc, _, key, notifies := connect(t)
	must := func(name string) {
		t.Helper()
		if err := svc.Message(key, "s1", fixture(t, name)); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
	}
	must("query.event.added")
	must("query.event.updated") // fetch delta → fetchStatus fetching
	q, err := svc.Query(key, `["todos",{"page":1}]`)
	if err != nil || q.State.FetchStatus != "fetching" || q.Label != queries.LabelFetching || string(q.State.Data) == "" {
		t.Fatalf("after delta: %+v err=%v", q, err)
	}
	must("query.event.observers") // observers 0, stale
	q, _ = svc.Query(key, `["todos",{"page":1}]`)
	if q.ObserversCount != 0 || !q.IsStale || q.Label != queries.LabelFetching {
		t.Fatalf("after observers: %+v", q)
	}

	// Add a second, fresh query and check ordering: fetching first, then fresh.
	second := protocol.Query{
		QueryHash: `["a"]`, QueryKey: json.RawMessage(`["a"]`), ObserversCount: 1,
		State: protocol.QueryState{Status: "success", FetchStatus: "idle", DataUpdatedAt: 5},
	}
	raw, _ := json.Marshal(protocol.QueryEvent{Kind: protocol.EventAdded, QueryHash: `["a"]`, Query: &second})
	if err := svc.Message(key, "s1", protocol.Message{V: 1, Type: protocol.TypeQueryEvent, Payload: raw}); err != nil {
		t.Fatal(err)
	}
	list, counts, _ := svc.Queries(key)
	if len(list) != 2 || list[0].Label != queries.LabelFetching || list[1].Label != queries.LabelFresh || counts.Fetching != 1 || counts.Fresh != 1 {
		t.Fatalf("list = %+v counts = %+v", list, counts)
	}

	must("query.event.removed")
	if _, err := svc.Query(key, `["todos",{"page":1}]`); err == nil {
		t.Fatal("removed query still present")
	}
	must("mutation.event") // updated delta for unknown mutation 7: ignored without error
	if *notifies < 4 {
		t.Fatalf("notified %d times, want at least 4", *notifies)
	}
}

func TestOnlineStatusAndSetOnline(t *testing.T) {
	t.Parallel()
	svc, sender, key, _ := connect(t)
	if err := svc.Message(key, "s1", fixture(t, "online.status")); err != nil {
		t.Fatal(err)
	}
	if svc.Devices()[0].Online {
		t.Fatal("online.status false not applied")
	}
	if err := svc.SetOnline(key, true); err != nil {
		t.Fatal(err)
	}
	if frames := sender.sent(protocol.TypeOnlineSet); len(frames) != 1 || string(frames[0].Payload) != `{"online":true}` {
		t.Fatalf("online.set frames = %+v", frames)
	}
	if !svc.Devices()[0].Online {
		t.Fatal("SetOnline did not mirror the flag")
	}
}

func TestActionRoundTrip(t *testing.T) {
	t.Parallel()
	svc, sender, key, _ := connect(t)
	sender.reply = func(a protocol.QueryAction) *protocol.ActionResult {
		if a.Type == protocol.ActionRemove {
			return &protocol.ActionResult{ID: a.ID, OK: false, Error: "query not found"}
		}
		return &protocol.ActionResult{ID: a.ID, OK: true}
	}

	if err := svc.Action(context.Background(), key, protocol.ActionInvalidate, "h", nil); err != nil {
		t.Fatalf("Action: %v", err)
	}
	err := svc.Action(context.Background(), key, protocol.ActionRemove, "h", nil)
	if !errors.Is(err, queries.ErrActionFailed) {
		t.Fatalf("failed action err = %v", err)
	}
	frames := sender.sent(protocol.TypeQueryAction)
	if len(frames) != 2 || frames[0].ID == "" || frames[0].ID == frames[1].ID {
		t.Fatalf("action frames = %+v", frames)
	}
	var a protocol.QueryAction
	_ = json.Unmarshal(frames[0].Payload, &a)
	if a.ID != frames[0].ID || a.Type != protocol.ActionInvalidate || a.QueryHash != "h" {
		t.Fatalf("action payload = %+v", a)
	}
}

func TestActionTimeoutAndErrors(t *testing.T) {
	t.Parallel()
	svc, sender, key, _ := connect(t)
	svc.ActionTimeout = 50 * time.Millisecond
	err := svc.Action(context.Background(), key, protocol.ActionRefetch, "h", nil)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("timeout err = %v", err)
	}
	sender.fail = errors.New("socket closed")
	if err := svc.Action(context.Background(), key, protocol.ActionRefetch, "h", nil); err == nil {
		t.Fatal("send failure not reported")
	}
	if err := svc.Action(context.Background(), "nope", protocol.ActionRefetch, "h", nil); !errors.Is(err, queries.ErrUnknownDevice) {
		t.Fatalf("unknown device err = %v", err)
	}
}

func TestDisconnectForgetsDeviceAndPendingActions(t *testing.T) {
	t.Parallel()
	svc, _, key, _ := connect(t)
	done := make(chan error, 1)
	go func() { done <- svc.Action(context.Background(), key, protocol.ActionRefetch, "h", nil) }()
	time.Sleep(20 * time.Millisecond)

	svc.Disconnect(key, "wrong-session")
	if len(svc.Devices()) != 1 {
		t.Fatal("disconnect with another session id must not remove the device")
	}
	svc.Disconnect(key, "s1")
	if len(svc.Devices()) != 0 {
		t.Fatal("device still listed after disconnect")
	}
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("pending action resolved without error after disconnect")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("pending action did not resolve on disconnect")
	}
	if err := svc.Message(key, "s1", fixture(t, "online.status")); !errors.Is(err, queries.ErrUnknownDevice) {
		t.Fatalf("message after disconnect err = %v", err)
	}
}

// A frame still in flight from a replaced session must not touch the mirror
// the new session owns.
func TestMessageFromReplacedSessionIsIgnored(t *testing.T) {
	t.Parallel()
	svc, _, key, _ := connect(t)
	if err := svc.Message(key, "s1", fixture(t, "queries.snapshot")); err != nil {
		t.Fatal(err)
	}
	svc.Connect("s2", hello(), &fakeSender{svc: svc, key: key, sessionID: "s2"})
	if list, _, _ := svc.Queries(key); len(list) != 0 {
		t.Fatalf("new session starts with %d queries, want 0", len(list))
	}

	err := svc.Message(key, "s1", fixture(t, "queries.snapshot"))
	if !errors.Is(err, queries.ErrStaleSession) {
		t.Fatalf("stale frame err = %v, want ErrStaleSession", err)
	}
	if list, _, _ := svc.Queries(key); len(list) != 0 {
		t.Fatalf("stale frame changed the mirror: %d queries", len(list))
	}
	if svc.Devices()[0].SessionID != "s2" {
		t.Fatalf("device session = %s, want s2", svc.Devices()[0].SessionID)
	}

	if err := svc.Message(key, "s2", fixture(t, "queries.snapshot")); err != nil {
		t.Fatal(err)
	}
	if list, _, _ := svc.Queries(key); len(list) != 1 {
		t.Fatalf("current session frame not applied: %d queries", len(list))
	}
}

// A delta carries JSON null for a field the device cleared (TRIGGER_LOADING,
// TRIGGER_ERROR, RESET); absent means unchanged. Null clears the mirror.
func TestNullDeltaClearsFields(t *testing.T) {
	t.Parallel()
	svc, _, key, _ := connect(t)
	q := protocol.Query{
		QueryHash: "h", QueryKey: json.RawMessage(`["h"]`), ObserversCount: 1,
		State: protocol.QueryState{
			Status: "error", FetchStatus: "idle",
			Data:  json.RawMessage(`{"__lensTruncated":true,"bytes":5000000}`),
			Error: json.RawMessage(`{"message":"boom"}`), FetchMeta: json.RawMessage(`{"k":1}`),
		},
	}
	added, _ := json.Marshal(protocol.QueryEvent{Kind: protocol.EventAdded, QueryHash: "h", Query: &q})
	if err := svc.Message(key, "s1", protocol.Message{V: 1, Type: protocol.TypeQueryEvent, Payload: added}); err != nil {
		t.Fatal(err)
	}
	if got, _ := svc.Query(key, "h"); !got.DataTruncated {
		t.Fatalf("precondition: marker must set dataTruncated, got %+v", got)
	}

	delta := protocol.Message{V: 1, Type: protocol.TypeQueryEvent, Payload: json.RawMessage(
		`{"kind":"updated","queryHash":"h","action":"setState","delta":{"status":"pending","data":null,"error":null,"fetchMeta":null}}`)}
	if err := svc.Message(key, "s1", delta); err != nil {
		t.Fatal(err)
	}
	got, _ := svc.Query(key, "h")
	if got.State.Status != "pending" || got.State.Data != nil || got.State.Error != nil || got.State.FetchMeta != nil || got.DataTruncated {
		t.Fatalf("null delta did not clear the fields: %+v", got)
	}
	raw, _ := json.Marshal(got)
	for _, field := range []string{`"data"`, `"error"`, `"fetchMeta"`, `"dataTruncated"`} {
		if strings.Contains(string(raw), field) {
			t.Fatalf("cleared field %s still serialized: %s", field, raw)
		}
	}

	// Absent fields stay untouched.
	keep := protocol.Message{V: 1, Type: protocol.TypeQueryEvent, Payload: json.RawMessage(
		`{"kind":"updated","queryHash":"h","action":"success","delta":{"data":{"ok":true}}}`)}
	if err := svc.Message(key, "s1", keep); err != nil {
		t.Fatal(err)
	}
	unchanged := protocol.Message{V: 1, Type: protocol.TypeQueryEvent, Payload: json.RawMessage(
		`{"kind":"updated","queryHash":"h","action":"invalidate","delta":{"isInvalidated":true}}`)}
	if err := svc.Message(key, "s1", unchanged); err != nil {
		t.Fatal(err)
	}
	if got, _ := svc.Query(key, "h"); string(got.State.Data) != `{"ok":true}` || !got.State.IsInvalidated {
		t.Fatalf("delta without data touched it: %+v", got)
	}

	// Mutations follow the same rule.
	m := protocol.Mutation{MutationID: 1, State: protocol.MutationState{Status: "error", Error: json.RawMessage(`"e"`), Data: json.RawMessage(`1`)}}
	mAdded, _ := json.Marshal(protocol.MutationEvent{Kind: protocol.EventAdded, MutationID: 1, Mutation: &m})
	if err := svc.Message(key, "s1", protocol.Message{V: 1, Type: protocol.TypeMutationEvent, Payload: mAdded}); err != nil {
		t.Fatal(err)
	}
	mDelta := protocol.Message{V: 1, Type: protocol.TypeMutationEvent, Payload: json.RawMessage(
		`{"kind":"updated","mutationId":1,"delta":{"status":"pending","error":null,"data":null}}`)}
	if err := svc.Message(key, "s1", mDelta); err != nil {
		t.Fatal(err)
	}
	muts, _ := svc.Mutations(key)
	if len(muts) != 1 || muts[0].State.Status != "pending" || muts[0].State.Error != nil || muts[0].State.Data != nil {
		t.Fatalf("null mutation delta did not clear the fields: %+v", muts)
	}
}

func TestOversizedDataIsCapped(t *testing.T) {
	t.Parallel()
	svc, _, key, _ := connect(t)
	big := make([]byte, queries.MaxDataBytes+10)
	for i := range big {
		big[i] = 'x'
	}
	q := protocol.Query{
		QueryHash: "big", QueryKey: json.RawMessage(`["big"]`), ObserversCount: 1,
		State: protocol.QueryState{Status: "success", FetchStatus: "idle", Data: json.RawMessage(`"` + string(big) + `"`)},
	}
	raw, _ := json.Marshal(protocol.QueryEvent{Kind: protocol.EventAdded, QueryHash: "big", Query: &q})
	if err := svc.Message(key, "s1", protocol.Message{V: 1, Type: protocol.TypeQueryEvent, Payload: raw}); err != nil {
		t.Fatal(err)
	}
	got, _ := svc.Query(key, "big")
	if !got.DataTruncated || len(got.State.Data) > 100 {
		t.Fatalf("data not capped: truncated=%v len=%d", got.DataTruncated, len(got.State.Data))
	}
}

func TestDataTruncatedFollowsEveryDelta(t *testing.T) {
	svc, _, key, _ := connect(t)
	q := protocol.Query{
		QueryHash: "h", QueryKey: json.RawMessage(`["h"]`), ObserversCount: 1, IsActive: true,
		State: protocol.QueryState{Status: "success", FetchStatus: "idle", Data: json.RawMessage(`1`)},
	}
	ev := func(delta *protocol.QueryStateDelta) protocol.Message {
		raw, _ := json.Marshal(protocol.QueryEvent{Kind: "updated", QueryHash: "h", Action: "success", Delta: delta})
		return protocol.Message{V: 1, Type: protocol.TypeQueryEvent, Payload: raw}
	}
	added, _ := json.Marshal(protocol.QueryEvent{Kind: "added", QueryHash: "h", Query: &q})
	if err := svc.Message(key, "s1", protocol.Message{V: 1, Type: protocol.TypeQueryEvent, Payload: added}); err != nil {
		t.Fatal(err)
	}
	marker := json.RawMessage(`{"__lensTruncated":true,"bytes":2000000}`)
	if err := svc.Message(key, "s1", ev(&protocol.QueryStateDelta{Data: marker})); err != nil {
		t.Fatal(err)
	}
	if got, _ := svc.Query(key, "h"); !got.DataTruncated {
		t.Fatalf("SDK marker in a delta must set dataTruncated, got %+v", got)
	}
	big := json.RawMessage(`"` + strings.Repeat("x", queries.MaxDataBytes+1) + `"`)
	if err := svc.Message(key, "s1", ev(&protocol.QueryStateDelta{Data: big})); err != nil {
		t.Fatal(err)
	}
	if got, _ := svc.Query(key, "h"); !got.DataTruncated {
		t.Fatal("server-side cap must set dataTruncated")
	}
	if err := svc.Message(key, "s1", ev(&protocol.QueryStateDelta{Data: json.RawMessage(`{"ok":true}`)})); err != nil {
		t.Fatal(err)
	}
	if got, _ := svc.Query(key, "h"); got.DataTruncated || string(got.State.Data) != `{"ok":true}` {
		t.Fatalf("small data after a capped one must clear dataTruncated, got %+v", got)
	}
}

func TestQueriesOrderNewestDataFirstWithinALabel(t *testing.T) {
	svc, _, key, _ := connect(t)
	add := func(hash string, updatedAt float64) {
		t.Helper()
		q := protocol.Query{
			QueryHash: hash, QueryKey: json.RawMessage(hash), ObserversCount: 1, IsActive: true,
			State: protocol.QueryState{Status: "success", FetchStatus: "idle", DataUpdatedAt: updatedAt},
		}
		raw, _ := json.Marshal(protocol.QueryEvent{Kind: protocol.EventAdded, QueryHash: hash, Query: &q})
		if err := svc.Message(key, "s1", protocol.Message{V: 1, Type: protocol.TypeQueryEvent, Payload: raw}); err != nil {
			t.Fatal(err)
		}
	}
	add(`["a"]`, 10)
	add(`["z"]`, 20)
	add(`["m"]`, 20)
	list, _, _ := svc.Queries(key)
	got := []string{list[0].QueryHash, list[1].QueryHash, list[2].QueryHash}
	want := []string{`["m"]`, `["z"]`, `["a"]`}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("order = %v, want newest data first then hash: %v", got, want)
		}
	}
}

func TestMutationPayloadsAreCappedServerSide(t *testing.T) {
	svc, _, key, _ := connect(t)
	big := json.RawMessage(`"` + strings.Repeat("v", queries.MaxDataBytes+1) + `"`)
	m := protocol.Mutation{
		MutationID: 3, MutationKey: json.RawMessage(`["upload"]`),
		State: protocol.MutationState{Status: "success", Variables: big, Data: big, Context: json.RawMessage(`{"small":true}`)},
	}
	raw, _ := json.Marshal(protocol.MutationEvent{Kind: protocol.EventAdded, MutationID: 3, Mutation: &m})
	if err := svc.Message(key, "s1", protocol.Message{V: 1, Type: protocol.TypeMutationEvent, Payload: raw}); err != nil {
		t.Fatal(err)
	}
	list, err := svc.Mutations(key)
	if err != nil || len(list) != 1 {
		t.Fatalf("mutations = %+v, %v", list, err)
	}
	st := list[0].State
	if !strings.Contains(string(st.Variables), `"__lensTruncated":true`) || !strings.Contains(string(st.Data), `"__lensTruncated":true`) {
		t.Fatalf("oversized payloads must be replaced by the marker: variables=%d bytes data=%d bytes", len(st.Variables), len(st.Data))
	}
	if string(st.Context) != `{"small":true}` {
		t.Fatalf("small payloads must stay intact, got %s", st.Context)
	}
}

func TestDisconnectOfAnotherSessionDoesNotNotify(t *testing.T) {
	svc, _, key, notifies := connect(t)
	before := *notifies
	svc.Disconnect(key, "not-the-current-session")
	if *notifies != before {
		t.Fatalf("notified %d times for a no-op disconnect", *notifies-before)
	}
	if _, err := svc.Query(key, "missing"); errors.Is(err, queries.ErrUnknownDevice) {
		t.Fatal("device must survive a stale disconnect")
	}
}

// Seen records any frame of the device's session (queries, storage or
// plugins), so status can tell an idle device from a busy one.
func TestSeenRecordsTheLastMessage(t *testing.T) {
	t.Parallel()
	svc, _, key, _ := connect(t)
	dev := svc.Devices()[0]
	if !dev.LastMessageAt.Equal(dev.ConnectedAt) {
		t.Fatalf("LastMessageAt at connect = %v, want the connect time %v", dev.LastMessageAt, dev.ConnectedAt)
	}
	time.Sleep(5 * time.Millisecond)
	svc.Seen(key, "s1")
	if got := svc.Devices()[0].LastMessageAt; !got.After(dev.ConnectedAt) {
		t.Fatalf("LastMessageAt after Seen = %v", got)
	}
	before := svc.Devices()[0].LastMessageAt
	svc.Seen(key, "old-session") // a replaced session's frame is ignored
	svc.Seen("other|dev", "s1")
	if got := svc.Devices()[0].LastMessageAt; !got.Equal(before) {
		t.Fatalf("a stale Seen moved LastMessageAt to %v", got)
	}
}
