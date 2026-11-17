// Package cdpws is the WebSocket transport for the Chrome DevTools Protocol.
// It dials Metro's /inspector/debug endpoint and exposes correlated calls and
// an ordered event stream using the vocabulary in internal/platform/cdp.
package cdpws

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"sync/atomic"

	"github.com/coder/websocket"

	"github.com/Reidevl/rn-lens/internal/platform/cdp"
)

// Options tunes a connection.
type Options struct {
	// HTTPClient is used for the WebSocket handshake. Nil means http.DefaultClient.
	HTTPClient *http.Client
	// Header is sent with the handshake, for example a User-Agent.
	Header http.Header
	// EventBuffer is the capacity of the Events channel. Zero means 256.
	EventBuffer int
	// MaxFrameBytes caps incoming frames. Zero means 64 MiB, enough for a
	// Network.getResponseBody reply.
	MaxFrameBytes int64
	// OnFrame, when set, observes every raw frame. It must not block.
	OnFrame func(dir cdp.Direction, raw []byte)
}

// ErrClosed is returned by Call once the connection is gone.
var ErrClosed = errors.New("cdpws: connection closed")

// Conn is a live CDP connection. Call and Close are safe for concurrent use.
type Conn struct {
	ws     *websocket.Conn
	opts   Options
	nextID atomic.Int64

	pendingMu sync.Mutex
	pending   map[int64]chan cdp.Message

	events chan cdp.Event
	done   chan struct{}

	closeOnce sync.Once
	closeMu   sync.Mutex
	closeErr  error
	reason    cdp.CloseReason
}

// Dial opens a CDP connection to url (ws:// or wss://).
func Dial(ctx context.Context, url string, opts Options) (*Conn, error) {
	if opts.EventBuffer <= 0 {
		opts.EventBuffer = 256
	}
	if opts.MaxFrameBytes <= 0 {
		opts.MaxFrameBytes = 64 << 20
	}

	ws, resp, err := websocket.Dial(ctx, url, &websocket.DialOptions{
		HTTPClient: opts.HTTPClient,
		HTTPHeader: opts.Header,
	})
	if resp != nil && resp.Body != nil {
		_ = resp.Body.Close()
	}
	if err != nil {
		return nil, fmt.Errorf("cdpws: dial %s: %w", url, err)
	}
	ws.SetReadLimit(opts.MaxFrameBytes)

	c := &Conn{
		ws:      ws,
		opts:    opts,
		pending: make(map[int64]chan cdp.Message),
		events:  make(chan cdp.Event, opts.EventBuffer),
		done:    make(chan struct{}),
	}
	// The connection outlives the dial context on purpose: ctx bounds the
	// handshake only, Close ends the read loop.
	go c.readLoop() //nolint:gosec // G118: intentional, see comment above
	return c, nil
}

// Call sends method with params and waits for the matching response.
// A protocol error is returned as *cdp.Error. params may be nil.
func (c *Conn) Call(ctx context.Context, method string, params any) (json.RawMessage, error) {
	select {
	case <-c.done:
		return nil, ErrClosed
	default:
	}

	id := c.nextID.Add(1)
	req := cdp.Message{ID: &id, Method: method}
	if params != nil {
		raw, err := json.Marshal(params)
		if err != nil {
			return nil, fmt.Errorf("cdpws: marshal params for %s: %w", method, err)
		}
		req.Params = raw
	}

	reply := make(chan cdp.Message, 1)
	c.pendingMu.Lock()
	c.pending[id] = reply
	c.pendingMu.Unlock()

	if err := c.send(ctx, req); err != nil {
		c.dropPending(id)
		return nil, err
	}

	select {
	case res := <-reply:
		if res.Error != nil {
			return nil, res.Error
		}
		return res.Result, nil
	case <-ctx.Done():
		c.dropPending(id)
		return nil, ctx.Err()
	case <-c.done:
		return nil, c.Err()
	}
}

// Events returns the stream of target notifications. It is closed after the
// connection ends. Consumers must drain it; a full buffer applies
// backpressure to the socket rather than dropping frames.
func (c *Conn) Events() <-chan cdp.Event { return c.events }

// Done is closed once the connection has ended for any reason.
func (c *Conn) Done() <-chan struct{} { return c.done }

// Err reports why the connection ended; nil while it is alive.
func (c *Conn) Err() error {
	c.closeMu.Lock()
	defer c.closeMu.Unlock()
	return c.closeErr
}

// Reason reports the parsed close reason once the connection has ended.
func (c *Conn) Reason() cdp.CloseReason {
	c.closeMu.Lock()
	defer c.closeMu.Unlock()
	return c.reason
}

// Close ends the connection with a normal closure. It is idempotent.
func (c *Conn) Close() error {
	var err error
	c.closeOnce.Do(func() {
		err = c.ws.Close(websocket.StatusNormalClosure, "lens: closed by user")
		c.finish(ErrClosed, cdp.CloseReason{Status: int(websocket.StatusNormalClosure), Message: "closed by user"})
	})
	return err
}

func (c *Conn) send(ctx context.Context, msg cdp.Message) error {
	raw, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("cdpws: marshal %s: %w", msg.Method, err)
	}
	if c.opts.OnFrame != nil {
		c.opts.OnFrame(cdp.DirectionSend, raw)
	}
	if err := c.ws.Write(ctx, websocket.MessageText, raw); err != nil {
		return fmt.Errorf("cdpws: send %s: %w", msg.Method, err)
	}
	return nil
}

func (c *Conn) readLoop() {
	// readLoop is the only sender on events, so it is the only closer too.
	defer close(c.events)

	ctx := context.Background()
	for {
		_, raw, err := c.ws.Read(ctx)
		if err != nil {
			status := websocket.CloseStatus(err)
			var ce websocket.CloseError
			reasonText := ""
			if errors.As(err, &ce) {
				reasonText = ce.Reason
			}
			c.closeOnce.Do(func() {
				_ = c.ws.CloseNow()
				c.finish(fmt.Errorf("cdpws: read: %w", err), cdp.ParseCloseReason(int(status), reasonText))
			})
			return
		}
		if c.opts.OnFrame != nil {
			c.opts.OnFrame(cdp.DirectionReceive, raw)
		}

		var msg cdp.Message
		if err := json.Unmarshal(raw, &msg); err != nil {
			// A malformed frame is not fatal; the protocol is line-oriented.
			continue
		}

		if msg.ID != nil {
			c.pendingMu.Lock()
			reply, ok := c.pending[*msg.ID]
			if ok {
				delete(c.pending, *msg.ID)
			}
			c.pendingMu.Unlock()
			if ok {
				reply <- msg
			}
			continue
		}

		if msg.Method != "" {
			select {
			case c.events <- cdp.Event{Method: msg.Method, Params: msg.Params}:
			case <-c.done:
				return
			}
		}
	}
}

// finish records the terminal state and wakes every waiter. Callers hold closeOnce.
func (c *Conn) finish(err error, reason cdp.CloseReason) {
	c.closeMu.Lock()
	c.closeErr = err
	c.reason = reason
	c.closeMu.Unlock()

	close(c.done)

	c.pendingMu.Lock()
	for id := range c.pending {
		delete(c.pending, id)
	}
	c.pendingMu.Unlock()
}

func (c *Conn) dropPending(id int64) {
	c.pendingMu.Lock()
	delete(c.pending, id)
	c.pendingMu.Unlock()
}
