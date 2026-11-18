package sdkws

import (
	"context"
	"crypto/subtle"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/coder/websocket"

	"github.com/Reidevl/rn-lens/internal/protocol"
)

// Session is one live SDK connection.
type Session struct {
	// ID is unique per connection and sent in welcome.
	ID string
	// Key identifies the app instance; a new connection with the same key
	// replaces the previous session.
	Key SessionKey
	// Hello is what the SDK announced.
	Hello protocol.Hello

	server      *Server
	conn        *websocket.Conn
	remote      string
	connectedAt time.Time
	// seq orders sessions by acceptance; connectedAt alone can tie.
	seq uint64

	writeMu    sync.Mutex
	closeOnce  sync.Once
	closed     chan struct{}
	closeMu    sync.Mutex
	closeErr   error
	missedPing atomic.Int32
}

// Remote is the peer address.
func (s *Session) Remote() string { return s.remote }

// ConnectedAt is when the socket was accepted.
func (s *Session) ConnectedAt() time.Time { return s.connectedAt }

// Done is closed once the session has ended.
func (s *Session) Done() <-chan struct{} { return s.closed }

// Send encodes payload as a frame of the given type and writes it.
func (s *Session) Send(typ, id string, payload any) error {
	raw, err := protocol.Encode(typ, id, payload)
	if err != nil {
		return err
	}
	return s.write(raw)
}

func (s *Session) write(raw []byte) error {
	ctx, cancel := context.WithTimeout(context.Background(), defaultWriteTimeout)
	defer cancel()
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	if err := s.conn.Write(ctx, websocket.MessageText, raw); err != nil {
		return fmt.Errorf("sdkws: write: %w", err)
	}
	return nil
}

// Close ends the session with a close frame. It is idempotent.
func (s *Session) Close(code websocket.StatusCode, reason string) {
	s.closeOnce.Do(func() {
		s.closeMu.Lock()
		s.closeErr = fmt.Errorf("sdkws: closed %d: %s", code, reason)
		s.closeMu.Unlock()
		if s.closed != nil {
			close(s.closed)
		}
		_ = s.conn.Close(code, reason)
	})
}

func (s *Session) closeReason() error {
	s.closeMu.Lock()
	defer s.closeMu.Unlock()
	return s.closeErr
}

// handshake reads the hello frame and answers welcome or closes the socket.
func (s *Session) handshake() error {
	s.closed = make(chan struct{})
	opts := s.server.opts

	// A read context deadline would drop the socket without a close code, so
	// the timeout closes it explicitly with CloseHelloTimeout instead.
	timer := time.AfterFunc(opts.HelloTimeout, func() {
		s.Close(CloseHelloTimeout, "expected hello within the handshake timeout")
	})
	_, raw, err := s.conn.Read(context.Background())
	if !timer.Stop() {
		return errors.New("hello not received before the handshake timeout")
	}
	if err != nil {
		s.Close(CloseProtocolError, "socket ended before hello")
		return fmt.Errorf("hello not received: %w", err)
	}
	msg, err := protocol.Decode(raw)
	if errors.Is(err, protocol.ErrVersion) {
		s.Close(CloseBadVersion, fmt.Sprintf("lens speaks protocol version %d", protocol.Version))
		return err
	}
	if err != nil || msg.Type != protocol.TypeHello {
		s.Close(CloseProtocolError, "first frame must be hello")
		if err == nil {
			err = fmt.Errorf("first frame was %q", msg.Type)
		}
		return err
	}
	var hello protocol.Hello
	if err := msg.PayloadInto(&hello); err != nil {
		s.Close(CloseProtocolError, "malformed hello")
		return err
	}
	if hello.ProtocolVersion != protocol.Version {
		s.Close(CloseBadVersion, fmt.Sprintf("lens speaks protocol version %d", protocol.Version))
		return fmt.Errorf("%w: sdk announced %d", protocol.ErrVersion, hello.ProtocolVersion)
	}
	if opts.Token != "" && subtle.ConstantTimeCompare([]byte(hello.Token), []byte(opts.Token)) != 1 {
		s.Close(CloseBadToken, "pairing token missing or wrong; start the SDK with the token Lens shows")
		return errors.New("pairing token mismatch")
	}

	s.ID = newSessionID()
	s.Key = SessionKey{AppID: hello.App.ID, DeviceID: hello.Device.ID}
	s.Hello = hello
	if err := s.Send(protocol.TypeWelcome, "", protocol.Welcome{
		LensVersion:     opts.LensVersion,
		ProtocolVersion: protocol.Version,
		SessionID:       s.ID,
	}); err != nil {
		s.Close(CloseProtocolError, "welcome could not be sent")
		return err
	}
	return nil
}

// run pumps frames to the handler until the socket ends.
func (s *Session) run() {
	srv := s.server
	srv.handler.OnConnect(s)

	pingCtx, stopPing := context.WithCancel(context.Background())
	defer stopPing()
	go s.pingLoop(pingCtx)

	var reason error
	for {
		typ, raw, err := s.conn.Read(context.Background())
		if err != nil {
			reason = err
			break
		}
		// After the handshake every frame must be a text frame holding a
		// protocol envelope; anything else is a protocol error, not noise:
		// an SDK that disagrees on the major is told so (4002) and a peer
		// that sends garbage is cut (4006) instead of being silently ignored.
		if typ != websocket.MessageText {
			srv.opts.Logger.Info("sdk session closed on a non-text frame", "session", s.ID)
			s.Close(CloseProtocolError, "frames must be text")
			break
		}
		msg, err := protocol.Decode(raw)
		if errors.Is(err, protocol.ErrVersion) {
			srv.opts.Logger.Info("sdk session closed on a protocol version mismatch", "session", s.ID, "err", err)
			s.Close(CloseBadVersion, fmt.Sprintf("lens speaks protocol version %d", protocol.Version))
			break
		}
		if err != nil {
			srv.opts.Logger.Info("sdk session closed on an undecodable frame", "session", s.ID, "err", err)
			s.Close(CloseProtocolError, "frame is not a protocol envelope")
			break
		}
		if msg.Type == protocol.TypePong {
			s.missedPing.Store(0)
			continue
		}
		if msg.Type == protocol.TypePing {
			_ = s.Send(protocol.TypePong, msg.ID, protocol.Empty{})
			continue
		}
		srv.handler.OnMessage(s, msg)
	}

	if err := s.closeReason(); err != nil {
		reason = err
	}
	s.Close(websocket.StatusNormalClosure, "")
	srv.unregister(s)
	srv.handler.OnDisconnect(s, reason)
}

func (s *Session) pingLoop(ctx context.Context) {
	ticker := time.NewTicker(s.server.opts.PingInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-s.closed:
			return
		case <-ticker.C:
			if int(s.missedPing.Add(1)) > s.server.opts.MissedPings {
				s.Close(ClosePingTimeout, "no pong received")
				return
			}
			if err := s.Send(protocol.TypePing, "", protocol.Empty{}); err != nil {
				return
			}
		}
	}
}
