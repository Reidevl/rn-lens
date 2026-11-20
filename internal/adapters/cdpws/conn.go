// Package cdpws is the WebSocket transport for the Chrome DevTools Protocol.
// It dials Metro's /inspector/debug endpoint and exposes correlated calls and
// an ordered event stream using the vocabulary in internal/platform/cdp.
package cdpws

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/coder/websocket"

	"github.com/Reidevl/rn-lens/internal/platform/cdp"
)

// Options tunes a connection.
//
// Memory bound: events the consumer has not read yet are held in memory, at
// most EventBuffer of them in the Events channel plus EventQueueBytes of
// payload in an overflow queue (defaults: 256 entries and 64 MiB). Each entry
// is at most MaxFrameBytes, so the theoretical worst case for the channel is
// EventBuffer × MaxFrameBytes, reached only by a peer that sends maximum-size
// events to a consumer that never drains. Past the queue bound the read loop
// pauses, so the socket applies backpressure instead of dropping frames.
type Options struct {
	// HTTPClient is used for the WebSocket handshake. Nil means http.DefaultClient.
	HTTPClient *http.Client
	// Header is sent with the handshake, for example a User-Agent.
	Header http.Header
	// EventBuffer is the capacity of the Events channel. Zero means 256.
	EventBuffer int
	// EventQueueBytes bounds the payload bytes of events queued in memory
	// while the Events channel is full. Zero means MaxFrameBytes. Responses
	// to Call keep flowing while the queue is under this bound; past it the
	// read loop pauses until the consumer drains Events.
	EventQueueBytes int64
	// MaxFrameBytes caps incoming frames. Zero means 64 MiB, enough for a
	// Network.getResponseBody reply.
	MaxFrameBytes int64
	// PingInterval is how often a WebSocket ping probes the peer. Zero means
	// 20 seconds; a negative value disables the keepalive.
	PingInterval time.Duration
	// PingTimeout bounds the wait for the pong, the read deadline of the
	// liveness probe. Zero means 10 seconds. A peer that does not answer in
	// time, a half-open socket after a Wi-Fi drop or a laptop sleep, ends the
	// connection with Err matching both ErrClosed and ErrKeepAlive.
	PingTimeout time.Duration
	// OnFrame, when set, observes every raw frame. It must not block.
	OnFrame func(dir cdp.Direction, raw []byte)
}

// ErrClosed is returned by Call once the connection is gone, whoever ended
// it. Err wraps it together with the cause when the peer or the transport
// ended the connection.
var ErrClosed = errors.New("cdpws: connection closed")

// ErrKeepAlive is wrapped by Err when the keepalive ended the connection
// because the peer did not answer a ping within PingTimeout.
var ErrKeepAlive = errors.New("cdpws: keepalive: peer did not answer ping")

// Conn is a live CDP connection. Call and Close are safe for concurrent use.
type Conn struct {
	ws     *websocket.Conn
	opts   Options
	nextID atomic.Int64

	pendingMu sync.Mutex
	pending   map[int64]chan cdp.Message

	// events is fed by pumpEvents from queue, never by readLoop directly, so
	// a full channel never blocks the dispatch of Call responses.
	events      chan cdp.Event
	queueMu     sync.Mutex
	queue       []cdp.Event
	queueBytes  int64
	queueNotify chan struct{} // readLoop -> pumpEvents: something was queued
	queueSpace  chan struct{} // pumpEvents -> readLoop: something was dequeued
	// waiting counts the goroutines blocked on queue space (readLoop and
	// Barrier callers); pauses counts every such wait that began. Pongs are
	// consumed by readLoop, so the keepalive must not blame the peer while a
	// wait may be holding readLoop. A counter, not a flag: one waiter
	// resuming must not hide another that is still blocked.
	waiting atomic.Int32
	pauses  atomic.Uint64

	done chan struct{}

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
	if opts.EventQueueBytes <= 0 {
		opts.EventQueueBytes = opts.MaxFrameBytes
	}
	if opts.PingInterval == 0 {
		opts.PingInterval = 20 * time.Second
	}
	if opts.PingTimeout <= 0 {
		opts.PingTimeout = 10 * time.Second
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
		ws:          ws,
		opts:        opts,
		pending:     make(map[int64]chan cdp.Message),
		events:      make(chan cdp.Event, opts.EventBuffer),
		queueNotify: make(chan struct{}, 1),
		queueSpace:  make(chan struct{}, 1),
		done:        make(chan struct{}),
	}
	// The connection outlives the dial context on purpose: ctx bounds the
	// handshake only, Close ends the read loop.
	go c.readLoop() //nolint:gosec // G118: intentional, see comment above
	go c.pumpEvents()
	if opts.PingInterval > 0 {
		go c.keepAlive() //nolint:gosec // G118: ends with the connection
	}
	return c, nil
}

// Call sends method with params and waits for the matching response.
// A protocol error is returned as *cdp.Error. params may be nil.
func (c *Conn) Call(ctx context.Context, method string, params any) (json.RawMessage, error) {
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
	select {
	case <-c.done:
		// finish already cleared pending; registering now would never be answered.
		c.pendingMu.Unlock()
		return nil, c.Err()
	default:
	}
	c.pending[id] = reply
	c.pendingMu.Unlock()

	if err := c.send(ctx, req); err != nil {
		c.dropPending(id)
		select {
		case <-c.done:
			return nil, c.Err()
		default:
		}
		if errors.Is(err, net.ErrClosed) {
			// Close sent its close frame and is waiting for the peer's
			// answer: the socket refuses writes but finish has not run yet.
			// Report it the way every other call on a gone connection is.
			return nil, fmt.Errorf("%w: %w", ErrClosed, err)
		}
		return nil, err
	}

	// The reply channel is the only completion signal: readLoop sends the
	// response on it and finish closes it when the connection ends, so a
	// response delivered before the close is never reported as a failure.
	select {
	case res, ok := <-reply:
		if !ok {
			return nil, c.Err()
		}
		return result(res)
	case <-ctx.Done():
		if c.dropPending(id) {
			return nil, ctx.Err()
		}
		// readLoop or finish took the channel over: a reply or a close is
		// imminent, and a delivered reply wins over the cancellation.
		res, ok := <-reply
		if !ok {
			return nil, ctx.Err()
		}
		return result(res)
	}
}

func result(res cdp.Message) (json.RawMessage, error) {
	if res.Error != nil {
		return nil, res.Error
	}
	return res.Result, nil
}

// Events returns the stream of target notifications, in arrival order. It is
// closed after the connection ends. Consumers must drain it: undrained events
// are queued in memory up to the bound described on Options, past which the
// socket applies backpressure rather than dropping frames.
func (c *Conn) Events() <-chan cdp.Event { return c.events }

// Done is closed once the connection has ended for any reason.
func (c *Conn) Done() <-chan struct{} { return c.done }

// Err reports why the connection ended; nil while it is alive. Once the
// connection is gone it matches ErrClosed and wraps the cause, if any.
func (c *Conn) Err() error {
	c.closeMu.Lock()
	defer c.closeMu.Unlock()
	return c.closeErr
}

// Reason reports the parsed close reason once the connection has ended.
// Without a close frame, Message carries the transport error.
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
	ctx := context.Background()
	for {
		_, raw, err := c.ws.Read(ctx)
		if err != nil {
			status := websocket.CloseStatus(err)
			var ce websocket.CloseError
			reasonText := err.Error() // no close frame: keep the transport error
			if errors.As(err, &ce) {
				reasonText = ce.Reason
			}
			c.closeOnce.Do(func() {
				_ = c.ws.CloseNow()
				c.finish(fmt.Errorf("%w: read: %w", ErrClosed, err), cdp.ParseCloseReason(int(status), reasonText))
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
				reply <- msg // buffered, never blocks: Call owns the only receive
			}
			continue
		}

		if msg.Method != "" {
			if !c.enqueueEvent(cdp.Event{Method: msg.Method, Params: msg.Params}) {
				return
			}
		}
	}
}

// Barrier queues a cdp.EventLensBarrier event carrying token behind every
// event read so far. Replies skip the event queue, so a caller that got a
// reply and wants every event the target sent before it (heap snapshot
// chunks, for instance) waits for its barrier on Events. It reports false
// once the connection ended.
func (c *Conn) Barrier(token string) bool {
	params, err := json.Marshal(token)
	if err != nil {
		return false
	}
	select {
	case <-c.done:
		return false
	default:
	}
	return c.enqueueEvent(cdp.Event{Method: cdp.EventLensBarrier, Params: params})
}

// enqueueEvent hands ev to pumpEvents. It blocks only while the queue is over
// EventQueueBytes, which pauses the socket, and reports false once the
// connection ended meanwhile.
func (c *Conn) enqueueEvent(ev cdp.Event) bool {
	for {
		c.queueMu.Lock()
		// An empty queue always accepts, so one event larger than the bound
		// still gets through instead of wedging the loop.
		if len(c.queue) == 0 || c.queueBytes < c.opts.EventQueueBytes {
			c.queue = append(c.queue, ev)
			c.queueBytes += int64(len(ev.Params))
			c.queueMu.Unlock()
			select {
			case c.queueNotify <- struct{}{}:
			default:
			}
			return true
		}
		c.queueMu.Unlock()
		c.pauses.Add(1)
		c.waiting.Add(1)
		select {
		case <-c.queueSpace:
			c.waiting.Add(-1)
		case <-c.done:
			c.waiting.Add(-1)
			return false
		}
	}
}

// dequeueEvent pops the oldest queued event, if any.
func (c *Conn) dequeueEvent() (cdp.Event, bool) {
	c.queueMu.Lock()
	defer c.queueMu.Unlock()
	if len(c.queue) == 0 {
		return cdp.Event{}, false
	}
	ev := c.queue[0]
	c.queue[0] = cdp.Event{}
	c.queue = c.queue[1:]
	c.queueBytes -= int64(len(ev.Params))
	select {
	case c.queueSpace <- struct{}{}:
	default:
	}
	return ev, true
}

// pumpEvents moves queued events into the Events channel in order. It is
// the only sender on events, so it is the only closer too.
func (c *Conn) pumpEvents() {
	defer close(c.events)
	for {
		ev, ok := c.dequeueEvent()
		if !ok {
			select {
			case <-c.queueNotify:
				continue
			case <-c.done:
				c.flushEvents()
				return
			}
		}
		select {
		case c.events <- ev:
		case <-c.done:
			c.flushEvents(ev)
			return
		}
	}
}

// flushEvents hands over what still fits in the channel once the connection
// has ended, so a consumer that drains after Done sees as many trailing
// events as the buffer can hold without blocking this goroutine forever.
func (c *Conn) flushEvents(first ...cdp.Event) {
	for _, ev := range first {
		select {
		case c.events <- ev:
		default:
			return
		}
	}
	for {
		ev, ok := c.dequeueEvent()
		if !ok {
			return
		}
		select {
		case c.events <- ev:
		default:
			return
		}
	}
}

// keepAlive pings the peer every PingInterval and ends the connection when
// no pong arrives within PingTimeout. The pong is consumed by readLoop, so a
// half-open socket, which never errors on Read by itself, is detected here.
func (c *Conn) keepAlive() {
	ticker := time.NewTicker(c.opts.PingInterval)
	defer ticker.Stop()
	for {
		select {
		case <-c.done:
			return
		case <-ticker.C:
		}
		if c.waiting.Load() > 0 {
			// readLoop is parked on backpressure and cannot read the pong.
			continue
		}
		pausesBefore := c.pauses.Load()
		ctx, cancel := context.WithTimeout(context.Background(), c.opts.PingTimeout)
		err := c.ws.Ping(ctx)
		cancel()
		if err == nil {
			continue
		}
		if stalledDuringPing(c.waiting.Load(), pausesBefore, c.pauses.Load()) {
			// readLoop stalled while the pong was due, so it may never have
			// read it: not the peer's fault. Try again on the next tick.
			continue
		}
		if errors.Is(err, net.ErrClosed) {
			// Someone else is closing the socket; let them report the reason.
			return
		}
		c.closeOnce.Do(func() {
			_ = c.ws.CloseNow()
			c.finish(
				fmt.Errorf("%w: %w (no pong within %s): %w", ErrClosed, ErrKeepAlive, c.opts.PingTimeout, err),
				cdp.ParseCloseReason(-1, "keepalive: no pong within "+c.opts.PingTimeout.String()),
			)
		})
		return
	}
}

// stalledDuringPing reports whether a failed ping may be the consumer's
// fault rather than the peer's: a queue wait is still in progress, or one
// began after the ping was sent (pausesBefore), so readLoop may not have
// read the pong.
func stalledDuringPing(waitingNow int32, pausesBefore, pausesAfter uint64) bool {
	return waitingNow > 0 || pausesAfter != pausesBefore
}

// finish records the terminal state and wakes every waiter. Callers hold closeOnce.
func (c *Conn) finish(err error, reason cdp.CloseReason) {
	c.closeMu.Lock()
	c.closeErr = err
	c.reason = reason
	c.closeMu.Unlock()

	close(c.done)

	// Closing a pending reply channel tells its Call that no response will
	// come. A channel readLoop already removed is not here and gets its reply.
	c.pendingMu.Lock()
	for id, reply := range c.pending {
		close(reply)
		delete(c.pending, id)
	}
	c.pendingMu.Unlock()
}

// dropPending removes id from the pending map and reports whether it was
// still there; false means readLoop or finish already took it over.
func (c *Conn) dropPending(id int64) bool {
	c.pendingMu.Lock()
	defer c.pendingMu.Unlock()
	_, ok := c.pending[id]
	delete(c.pending, id)
	return ok
}
