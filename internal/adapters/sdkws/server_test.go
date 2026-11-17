package sdkws_test

import (
	"context"
	"encoding/json"
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

func closeStatus(t *testing.T, c *websocket.Conn) websocket.StatusCode {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	_, _, err := c.Read(ctx)
	if err == nil {
		t.Fatal("expected the server to close the socket")
	}
	return websocket.CloseStatus(err)
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

	// Malformed frames are ignored, not fatal.
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	_ = c.Write(ctx, websocket.MessageText, []byte(`not json`))
	cancel()
	send(t, c, protocol.TypeOnlineStatus, "", protocol.OnlineStatus{Online: true})
	if got := rec.next(t, "message"); got.msg.Type != protocol.TypeOnlineStatus {
		t.Fatalf("after garbage got %s", got.msg.Type)
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

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := srv.Stop(ctx); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if got := closeStatus(t, c); got != sdkws.CloseServerShutdown {
		t.Fatalf("close status = %d, want %d", got, sdkws.CloseServerShutdown)
	}
	rec.next(t, "disconnect")
	if len(srv.Sessions()) != 0 {
		t.Fatalf("sessions after Stop = %d", len(srv.Sessions()))
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
