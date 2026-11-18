package sdkws_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/coder/websocket"

	"github.com/Reidevl/rn-lens/internal/adapters/sdkws"
	"github.com/Reidevl/rn-lens/internal/protocol"
)

type event struct {
	kind string
	sess *sdkws.Session
	msg  protocol.Message
	err  error
}

// recorder keeps every handler event so a test can wait for one kind without
// losing the others.
type recorder struct {
	mu     sync.Mutex
	queue  []event
	notify chan struct{}
	// beforeDisconnect, when set, runs before a disconnect event is recorded.
	beforeDisconnect func()
}

func newRecorder() *recorder { return &recorder{notify: make(chan struct{}, 256)} }

func (r *recorder) push(ev event) {
	r.mu.Lock()
	r.queue = append(r.queue, ev)
	r.mu.Unlock()
	select {
	case r.notify <- struct{}{}:
	default:
	}
}

func (r *recorder) OnConnect(s *sdkws.Session) { r.push(event{kind: "connect", sess: s}) }

func (r *recorder) OnMessage(s *sdkws.Session, m protocol.Message) {
	r.push(event{kind: "message", sess: s, msg: m})
}

func (r *recorder) OnDisconnect(s *sdkws.Session, err error) {
	if r.beforeDisconnect != nil {
		r.beforeDisconnect()
	}
	r.push(event{kind: "disconnect", sess: s, err: err})
}

// next removes and returns the oldest event of the given kind.
func (r *recorder) next(t *testing.T, kind string) event {
	t.Helper()
	deadline := time.After(3 * time.Second)
	for {
		r.mu.Lock()
		for i, ev := range r.queue {
			if ev.kind == kind {
				r.queue = append(r.queue[:i], r.queue[i+1:]...)
				r.mu.Unlock()
				return ev
			}
		}
		r.mu.Unlock()
		select {
		case <-r.notify:
		case <-deadline:
			t.Fatalf("no %s event", kind)
		}
	}
}

// count returns how many events of kind are queued.
func (r *recorder) count(kind string) int {
	r.mu.Lock()
	defer r.mu.Unlock()
	n := 0
	for _, ev := range r.queue {
		if ev.kind == kind {
			n++
		}
	}
	return n
}

func startServer(t *testing.T, opts sdkws.Options) (*sdkws.Server, *recorder, string) {
	t.Helper()
	rec := newRecorder()
	opts.Addr = "127.0.0.1:0"
	srv := sdkws.New(opts, rec)
	addr, err := srv.Start()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = srv.Stop(ctx)
	})
	return srv, rec, "ws://" + addr + sdkws.Path
}

func dial(t *testing.T, url string) *websocket.Conn {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	c, _, err := websocket.Dial(ctx, url, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = c.CloseNow() })
	return c
}

func send(t *testing.T, c *websocket.Conn, typ, id string, payload any) {
	t.Helper()
	raw, err := protocol.Encode(typ, id, payload)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := c.Write(ctx, websocket.MessageText, raw); err != nil {
		t.Fatal(err)
	}
}

func read(t *testing.T, c *websocket.Conn) protocol.Message {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	_, raw, err := c.Read(ctx)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	msg, err := protocol.Decode(raw)
	if err != nil {
		t.Fatal(err)
	}
	return msg
}

// readClose waits for the server to close the socket and reports the close
// status. It never fails the test, so goroutines other than the test's own
// may call it and hand the result back over a channel.
func readClose(c *websocket.Conn) (websocket.StatusCode, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	_, _, err := c.Read(ctx)
	if err == nil {
		return 0, errors.New("expected the server to close the socket")
	}
	return websocket.CloseStatus(err), nil
}

func closeStatus(t *testing.T, c *websocket.Conn) websocket.StatusCode {
	t.Helper()
	status, err := readClose(c)
	if err != nil {
		t.Fatal(err)
	}
	return status
}

// closeResult carries a readClose outcome from a reader goroutine to the
// test goroutine, which owns the assertions.
type closeResult struct {
	status websocket.StatusCode
	err    error
}

// readCloseAsync reads the close frame on its own goroutine, so a Stop under
// test can complete its close handshake while the test waits on it.
func readCloseAsync(c *websocket.Conn) <-chan closeResult {
	out := make(chan closeResult, 1)
	go func() {
		status, err := readClose(c)
		out <- closeResult{status: status, err: err}
	}()
	return out
}

func hello(token string) protocol.Hello {
	return protocol.Hello{
		SDKVersion: "0.1.0", ProtocolVersion: protocol.Version,
		App:      protocol.AppInfo{ID: "com.example.app", Name: "Example", Platform: protocol.PlatformIOS},
		Device:   protocol.DeviceInfo{ID: "dev-1", Name: "iPhone"},
		Features: []string{protocol.FeatureQueries}, Token: token,
	}
}

func TestHandshakeWelcomesAndRegistersSession(t *testing.T) {
	t.Parallel()
	srv, rec, url := startServer(t, sdkws.Options{LensVersion: "1.2.3"})
	c := dial(t, url)

	send(t, c, protocol.TypeHello, "", hello(""))

	msg := read(t, c)
	var w protocol.Welcome
	if msg.Type != protocol.TypeWelcome || msg.PayloadInto(&w) != nil || w.LensVersion != "1.2.3" || w.SessionID == "" {
		t.Fatalf("welcome = %+v / %+v", msg, w)
	}
	ev := rec.next(t, "connect")
	if ev.sess.Key != (sdkws.SessionKey{AppID: "com.example.app", DeviceID: "dev-1"}) || ev.sess.Hello.App.Name != "Example" {
		t.Fatalf("session = %+v", ev.sess)
	}
	if got, ok := srv.Session(ev.sess.Key); !ok || got.ID != w.SessionID {
		t.Fatal("session not registered under its key")
	}
}

func TestHandshakeRejections(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		token string
		send  func(t *testing.T, c *websocket.Conn)
		want  websocket.StatusCode
	}{
		{name: "no hello in time", send: func(*testing.T, *websocket.Conn) {}, want: sdkws.CloseHelloTimeout},
		{name: "wrong major", send: func(t *testing.T, c *websocket.Conn) {
			t.Helper()
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			_ = c.Write(ctx, websocket.MessageText, []byte(`{"v":2,"type":"hello","payload":{}}`))
		}, want: sdkws.CloseBadVersion},
		{name: "first frame is not hello", send: func(t *testing.T, c *websocket.Conn) {
			t.Helper()
			send(t, c, protocol.TypePing, "", protocol.Empty{})
		}, want: sdkws.CloseProtocolError},
		{name: "announced protocol mismatch", send: func(t *testing.T, c *websocket.Conn) {
			t.Helper()
			h := hello("")
			h.ProtocolVersion = 9
			send(t, c, protocol.TypeHello, "", h)
		}, want: sdkws.CloseBadVersion},
		{name: "missing token", token: "secret", send: func(t *testing.T, c *websocket.Conn) {
			t.Helper()
			send(t, c, protocol.TypeHello, "", hello(""))
		}, want: sdkws.CloseBadToken},
		{name: "wrong token", token: "secret", send: func(t *testing.T, c *websocket.Conn) {
			t.Helper()
			send(t, c, protocol.TypeHello, "", hello("nope"))
		}, want: sdkws.CloseBadToken},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, rec, url := startServer(t, sdkws.Options{Token: tt.token, HelloTimeout: 150 * time.Millisecond})
			c := dial(t, url)
			tt.send(t, c)
			if got := closeStatus(t, c); got != tt.want {
				t.Fatalf("close status = %d, want %d", got, tt.want)
			}
			time.Sleep(50 * time.Millisecond)
			if n := rec.count("connect"); n != 0 {
				t.Fatalf("handler saw %d connect events for a rejected connection", n)
			}
		})
	}
}

func TestTokenAcceptedWhenItMatches(t *testing.T) {
	t.Parallel()
	_, rec, url := startServer(t, sdkws.Options{Token: "secret"})
	c := dial(t, url)
	send(t, c, protocol.TypeHello, "", hello("secret"))
	if msg := read(t, c); msg.Type != protocol.TypeWelcome {
		t.Fatalf("got %s, want welcome", msg.Type)
	}
	rec.next(t, "connect")
}

func TestMessagesReachTheHandlerAndRepliesReachTheClient(t *testing.T) {
	t.Parallel()
	_, rec, url := startServer(t, sdkws.Options{})
	c := dial(t, url)
	send(t, c, protocol.TypeHello, "", hello(""))
	read(t, c)
	ev := rec.next(t, "connect")

	send(t, c, protocol.TypeOnlineStatus, "", protocol.OnlineStatus{Online: false})
	got := rec.next(t, "message")
	var st protocol.OnlineStatus
	if got.msg.Type != protocol.TypeOnlineStatus || got.msg.PayloadInto(&st) != nil || st.Online {
		t.Fatalf("message = %+v", got.msg)
	}

	if err := ev.sess.Send(protocol.TypeQueryAction, "a1", protocol.QueryAction{ID: "a1", Type: protocol.ActionRefetch, QueryHash: "h"}); err != nil {
		t.Fatal(err)
	}
	reply := read(t, c)
	var act protocol.QueryAction
	if reply.Type != protocol.TypeQueryAction || reply.ID != "a1" || reply.PayloadInto(&act) != nil || act.Type != protocol.ActionRefetch {
		t.Fatalf("reply = %+v", reply)
	}
}

func writeRaw(t *testing.T, c *websocket.Conn, typ websocket.MessageType, raw []byte) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := c.Write(ctx, typ, raw); err != nil {
		t.Fatal(err)
	}
}

// Minors are additive: fields this Lens does not know are ignored, in the
// envelope and in the payload, so a newer SDK still connects.
func TestUnknownFieldsAreIgnored(t *testing.T) {
	t.Parallel()
	_, rec, url := startServer(t, sdkws.Options{})
	c := dial(t, url)
	writeRaw(t, c, websocket.MessageText, []byte(`{"v":1,"type":"hello","future":1,"payload":{`+
		`"sdkVersion":"9.9.0","protocolVersion":1,"app":{"id":"com.example.app","platform":"ios","color":"blue"},`+
		`"device":{"id":"dev-1"},"features":["queries","holograms"],"newOptional":{"x":1}}}`))
	if msg := read(t, c); msg.Type != protocol.TypeWelcome {
		t.Fatalf("got %s, want welcome", msg.Type)
	}
	ev := rec.next(t, "connect")
	if ev.sess.Hello.SDKVersion != "9.9.0" || ev.sess.Key.AppID != "com.example.app" {
		t.Fatalf("hello = %+v", ev.sess.Hello)
	}

	writeRaw(t, c, websocket.MessageText, []byte(`{"v":1,"type":"online.status","future":1,"payload":{"online":false,"since":1}}`))
	got := rec.next(t, "message")
	var st protocol.OnlineStatus
	if got.msg.Type != protocol.TypeOnlineStatus || got.msg.PayloadInto(&st) != nil || st.Online {
		t.Fatalf("message = %+v", got.msg)
	}
}

func TestInvalidFramesAfterHandshakeCloseTheSession(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		typ  websocket.MessageType
		raw  []byte
		want websocket.StatusCode
	}{
		{name: "undecodable json", typ: websocket.MessageText, raw: []byte(`{not json`), want: sdkws.CloseProtocolError},
		{name: "json that is not an envelope", typ: websocket.MessageText, raw: []byte(`[1,2]`), want: sdkws.CloseProtocolError},
		{name: "missing type", typ: websocket.MessageText, raw: []byte(`{"v":1,"payload":{}}`), want: sdkws.CloseProtocolError},
		{name: "binary frame", typ: websocket.MessageBinary, raw: []byte(`{"v":1,"type":"ping","payload":{}}`), want: sdkws.CloseProtocolError},
		{name: "other major", typ: websocket.MessageText, raw: []byte(`{"v":2,"type":"ping","payload":{}}`), want: sdkws.CloseBadVersion},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			srv, rec, url := startServer(t, sdkws.Options{})
			c := dial(t, url)
			send(t, c, protocol.TypeHello, "", hello(""))
			read(t, c)
			sess := rec.next(t, "connect").sess

			writeRaw(t, c, tt.typ, tt.raw)
			if got := closeStatus(t, c); got != tt.want {
				t.Fatalf("close status = %d, want %d", got, tt.want)
			}
			ev := rec.next(t, "disconnect")
			if ev.sess != sess || ev.err == nil {
				t.Fatalf("disconnect = %+v", ev)
			}
			if _, ok := srv.Session(sess.Key); ok {
				t.Fatal("session still registered after a protocol error")
			}
			if n := rec.count("message"); n != 0 {
				t.Fatalf("handler saw %d messages for an invalid frame", n)
			}
		})
	}
}

func TestPingPongAndTimeout(t *testing.T) {
	t.Parallel()
	_, rec, url := startServer(t, sdkws.Options{PingInterval: 30 * time.Millisecond, MissedPings: 2})
	c := dial(t, url)
	send(t, c, protocol.TypeHello, "", hello(""))
	read(t, c)
	rec.next(t, "connect")

	// Answer the first ping; the session must stay alive.
	if msg := read(t, c); msg.Type != protocol.TypePing {
		t.Fatalf("expected ping, got %s", msg.Type)
	}
	send(t, c, protocol.TypePong, "", protocol.Empty{})

	// Stop answering: after MissedPings+1 intervals the server closes 4005.
	var status websocket.StatusCode
	for i := 0; i < 10; i++ {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		_, _, err := c.Read(ctx)
		cancel()
		if err != nil {
			status = websocket.CloseStatus(err)
			break
		}
	}
	if status != sdkws.ClosePingTimeout {
		t.Fatalf("close status = %d, want %d", status, sdkws.ClosePingTimeout)
	}
	ev := rec.next(t, "disconnect")
	if ev.err == nil {
		t.Fatal("disconnect reason missing")
	}
}

func TestClientPingIsAnswered(t *testing.T) {
	t.Parallel()
	_, rec, url := startServer(t, sdkws.Options{})
	c := dial(t, url)
	send(t, c, protocol.TypeHello, "", hello(""))
	read(t, c)
	rec.next(t, "connect")

	send(t, c, protocol.TypePing, "p1", protocol.Empty{})
	if msg := read(t, c); msg.Type != protocol.TypePong || msg.ID != "p1" {
		t.Fatalf("got %+v, want pong p1", msg)
	}
}

func TestNewConnectionReplacesSameAppAndDevice(t *testing.T) {
	t.Parallel()
	srv, rec, url := startServer(t, sdkws.Options{})
	first := dial(t, url)
	send(t, first, protocol.TypeHello, "", hello(""))
	read(t, first)
	rec.next(t, "connect")

	second := dial(t, url)
	send(t, second, protocol.TypeHello, "", hello(""))
	read(t, second)
	ev := rec.next(t, "connect")

	if got := closeStatus(t, first); got != sdkws.CloseReplaced {
		t.Fatalf("first connection closed with %d, want %d", got, sdkws.CloseReplaced)
	}
	rec.next(t, "disconnect")
	if len(srv.Sessions()) != 1 || srv.Sessions()[0].ID != ev.sess.ID {
		t.Fatalf("sessions = %d, want only the second", len(srv.Sessions()))
	}
}

func TestStopClosesSessions(t *testing.T) {
	t.Parallel()
	srv, rec, url := startServer(t, sdkws.Options{})
	c := dial(t, url)
	send(t, c, protocol.TypeHello, "", hello(""))
	read(t, c)
	rec.next(t, "connect")

	// The client reads while Stop runs, as a live SDK does, so the close
	// handshake completes and Stop returns well within its context.
	result := readCloseAsync(c)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := srv.Stop(ctx); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	res := <-result
	if res.err != nil {
		t.Fatal(res.err)
	}
	if res.status != sdkws.CloseServerShutdown {
		t.Fatalf("close status = %d, want %d", res.status, sdkws.CloseServerShutdown)
	}
	rec.next(t, "disconnect")
	if len(srv.Sessions()) != 0 {
		t.Fatalf("sessions after Stop = %d", len(srv.Sessions()))
	}
}

// Stop must return once ctx expires even while a connection is still waiting
// for its hello, and cut that straggler instead of waiting out HelloTimeout.
func TestStopIsBoundedByContextWhileAHandshakeIsPending(t *testing.T) {
	t.Parallel()
	srv, rec, url := startServer(t, sdkws.Options{HelloTimeout: 3 * time.Second})
	c := dial(t, url)                  // never sends hello
	time.Sleep(100 * time.Millisecond) // let the server pick the socket up

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	start := time.Now()
	err := srv.Stop(ctx)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Stop = %v, want the context deadline", err)
	}
	if d := time.Since(start); d > 2*time.Second {
		t.Fatalf("Stop took %s, want it bounded by ctx", d)
	}

	readCtx, readCancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer readCancel()
	if _, _, err := c.Read(readCtx); err == nil || errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("straggler Read = %v, want the socket cut", err)
	}
	if n := rec.count("connect"); n != 0 {
		t.Fatalf("handler saw %d connect events for a connection cut mid-handshake", n)
	}
}

func TestSessionsAreListedInConnectionOrder(t *testing.T) {
	t.Parallel()
	srv, rec, url := startServer(t, sdkws.Options{})
	keys := []string{"dev-c", "dev-a", "dev-b"}
	for _, dev := range keys {
		c := dial(t, url)
		h := hello("")
		h.Device.ID = dev
		send(t, c, protocol.TypeHello, "", h)
		read(t, c)
		rec.next(t, "connect")
	}
	for attempt := range 5 {
		got := srv.Sessions()
		if len(got) != len(keys) {
			t.Fatalf("sessions = %d, want %d", len(got), len(keys))
		}
		ok := true
		for i, sess := range got {
			if sess.Key.DeviceID != keys[i] {
				ok = false
			}
		}
		if ok {
			return
		}
		if attempt == 4 {
			t.Fatalf("sessions not in connection order: %v %v %v", got[0].Key.DeviceID, got[1].Key.DeviceID, got[2].Key.DeviceID)
		}
	}
}

// Stop must not return while a session goroutine is still delivering
// OnDisconnect, and a connection that arrives once Stop began is refused.
func TestStopWaitsForSessionsAndRefusesLateConnections(t *testing.T) {
	t.Parallel()
	srv, rec, url := startServer(t, sdkws.Options{})
	release := make(chan struct{})
	var releaseOnce sync.Once
	// A failed assertion must not leave Stop deadlocked behind OnDisconnect.
	t.Cleanup(func() { releaseOnce.Do(func() { close(release) }) })
	rec.beforeDisconnect = func() { <-release }

	clients := []*websocket.Conn{dial(t, url), dial(t, url)}
	for i, c := range clients {
		h := hello("")
		h.Device.ID = fmt.Sprintf("dev-%d", i)
		send(t, c, protocol.TypeHello, "", h)
		read(t, c)
		rec.next(t, "connect")
	}

	stopped := make(chan error, 1)
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		stopped <- srv.Stop(ctx)
	}()
	// Read the close frames concurrently: Stop closes sessions in map order.
	// The readers only report; assertions stay on the test goroutine.
	results := make([]<-chan closeResult, 0, len(clients))
	for _, c := range clients {
		results = append(results, readCloseAsync(c))
	}
	for _, result := range results {
		res := <-result
		if res.err != nil {
			t.Fatal(res.err)
		}
		if res.status != sdkws.CloseServerShutdown {
			t.Fatalf("close status = %d, want %d", res.status, sdkws.CloseServerShutdown)
		}
	}
	select {
	case err := <-stopped:
		t.Fatalf("Stop returned (%v) before OnDisconnect was delivered", err)
	case <-time.After(150 * time.Millisecond):
	}

	releaseOnce.Do(func() { close(release) })
	select {
	case err := <-stopped:
		if err != nil {
			t.Fatalf("Stop: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Stop did not return after the sessions ended")
	}
	if n := rec.count("disconnect"); n != len(clients) {
		t.Fatalf("Stop returned with %d of %d disconnects delivered", n, len(clients))
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	c, _, err := websocket.Dial(ctx, url, nil)
	if err != nil {
		return // the listener is gone: refused at the TCP level
	}
	t.Cleanup(func() { _ = c.CloseNow() })
	if got := closeStatus(t, c); got != sdkws.CloseServerShutdown && got != websocket.StatusTryAgainLater {
		t.Fatalf("late connection closed with %d, want %d or %d", got, sdkws.CloseServerShutdown, websocket.StatusTryAgainLater)
	}
	if n := rec.count("connect"); n != 0 {
		t.Fatalf("handler saw %d connect events after Stop", n)
	}
}

func TestConcurrentSendsAreSerialized(t *testing.T) {
	t.Parallel()
	_, rec, url := startServer(t, sdkws.Options{})
	c := dial(t, url)
	send(t, c, protocol.TypeHello, "", hello(""))
	read(t, c)
	sess := rec.next(t, "connect").sess

	var wg sync.WaitGroup
	for i := range 20 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = sess.Send(protocol.TypeOnlineSet, "", protocol.OnlineSet{Online: i%2 == 0})
		}()
	}
	wg.Wait()
	for range 20 {
		msg := read(t, c)
		var p protocol.OnlineSet
		if msg.Type != protocol.TypeOnlineSet || json.Unmarshal(msg.Payload, &p) != nil {
			t.Fatalf("frame = %+v", msg)
		}
	}
}
