package network

import (
	"container/list"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Reidevl/rn-lens/internal/platform/cdp"
)

// Caller sends CDP requests to the attached app. *targets.Session satisfies it.
type Caller interface {
	Call(ctx context.Context, method string, params any) (json.RawMessage, error)
}

// Errors returned by the body getters.
var (
	// ErrStale means the request predates the current attach: React Native
	// keeps bodies per debugger session, so they can no longer be fetched.
	ErrStale = errors.New("network: the request belongs to an earlier session; its body is no longer available")
	// ErrNotFound means the request was evicted or cleared.
	ErrNotFound = errors.New("network: request not found")
	// ErrBodyTooLarge means a request body exceeds MaxBodyBytes.
	ErrBodyTooLarge = errors.New("network: the request body is larger than Lens shows")
)

// ReasonNeedsNewerReactNative is the availability reason when the target has
// no Network domain.
const ReasonNeedsNewerReactNative = "Network inspection needs RN 0.83+"

// cdpMethodNotFound is the JSON-RPC code for an unknown method.
const cdpMethodNotFound = -32601

// Defaults for the body cache.
const (
	DefaultMaxBodyBytes = 5 << 20
	DefaultCacheBytes   = 32 << 20
)

// Availability tells the UI whether network inspection works on the attached
// target. Checked stays false until the first attach finished enabling.
type Availability struct {
	Checked   bool   `json:"checked"`
	Supported bool   `json:"supported"`
	Reason    string `json:"reason,omitempty"`
}

// Body is a response body. Base64 is set for binary bodies (Text is then
// base64). Size is the decoded byte count. Truncated bodies were larger than
// the per-body limit (measured on the text as received): Text is empty.
type Body struct {
	Text      string `json:"text"`
	Base64    bool   `json:"base64"`
	Truncated bool   `json:"truncated"`
	Size      int    `json:"size"`
}

// ServiceOptions configures a Service.
type ServiceOptions struct {
	// Notify is called after every store change. It must not block.
	Notify func()
	Logger *slog.Logger
	// EnableTimeout bounds Network.enable. Zero means 5 s.
	EnableTimeout time.Duration
	// MaxBodyBytes is the largest body returned. Zero means 5 MiB.
	MaxBodyBytes int
	// CacheBytes caps the body cache. Zero means 32 MiB.
	CacheBytes int
}

// Service connects the store to a live session: it enables the Network
// domain, ingests its events and fetches bodies on demand.
type Service struct {
	store  *Store
	caller Caller
	opts   ServiceOptions

	attachments atomic.Uint64

	mu           sync.Mutex
	availability Availability
	cache        *bodyCache
}

// NewService creates a service over store using caller for requests.
func NewService(store *Store, caller Caller, opts ServiceOptions) *Service {
	if opts.Logger == nil {
		opts.Logger = slog.Default()
	}
	if opts.EnableTimeout <= 0 {
		opts.EnableTimeout = 5 * time.Second
	}
	if opts.MaxBodyBytes <= 0 {
		opts.MaxBodyBytes = DefaultMaxBodyBytes
	}
	if opts.CacheBytes <= 0 {
		opts.CacheBytes = DefaultCacheBytes
	}
	return &Service{store: store, caller: caller, opts: opts, cache: newBodyCache(opts.CacheBytes)}
}

// Store exposes the underlying requests.
func (s *Service) Store() *Store { return s.store }

// HandleEvent ingests a CDP event. Wire it to the session's event handlers.
func (s *Service) HandleEvent(ev cdp.Event) {
	if s.store.Ingest(ev) {
		s.notify()
	}
}

// BeginSession marks a new debugger session. Call it synchronously as soon as
// the session is attached, before any request can reach it: a re-attach starts
// a new epoch, so bodies of earlier requests (gone with the old session, and
// whose requestIds the app may reuse) are refused from then on. It does no I/O.
func (s *Service) BeginSession() {
	if s.attachments.Add(1) > 1 {
		s.store.NewEpoch()
		s.mu.Lock()
		s.cache.purge()
		s.mu.Unlock()
		s.notify()
	}
}

// OnAttached enables the Network domain after BeginSession; a target without
// one is reported through Availability, not as an error.
func (s *Service) OnAttached(ctx context.Context) error {
	callCtx, cancel := context.WithTimeout(ctx, s.opts.EnableTimeout)
	defer cancel()
	_, err := s.caller.Call(callCtx, cdp.MethodNetworkEnable, nil)

	avail := Availability{Checked: true, Supported: err == nil}
	var protoErr *cdp.Error
	switch {
	case err == nil:
	case errors.As(err, &protoErr) && protoErr.Code == cdpMethodNotFound:
		avail.Reason = ReasonNeedsNewerReactNative
	case errors.As(err, &protoErr):
		avail.Reason = "Network.enable failed: " + protoErr.Message
	default:
		avail.Reason = "Network.enable failed: " + err.Error()
	}
	s.mu.Lock()
	s.availability = avail
	s.mu.Unlock()
	if err != nil {
		s.opts.Logger.Debug("Network.enable failed", "err", err)
	}
	s.notify()
	return nil
}

// Availability reports whether network inspection works on the target.
func (s *Service) Availability() Availability {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.availability
}

// Clear drops every request and cached body.
func (s *Service) Clear() {
	s.store.Clear()
	s.mu.Lock()
	s.cache.purge()
	s.mu.Unlock()
	s.notify()
}

// lookup returns the request when it exists in the current epoch.
func (s *Service) lookup(seq, epoch uint64) (Request, error) {
	if epoch != s.store.Epoch() {
		return Request{}, ErrStale
	}
	r, ok := s.store.Get(seq)
	if !ok {
		return Request{}, ErrNotFound
	}
	if r.Epoch != epoch {
		return Request{}, ErrStale
	}
	return r, nil
}

// ResponseBody returns the response body of request seq, fetching it from the
// app on first use. epoch is the request's epoch as the caller saw it.
func (s *Service) ResponseBody(ctx context.Context, seq, epoch uint64) (Body, error) {
	r, err := s.lookup(seq, epoch)
	if err != nil {
		return Body{}, err
	}
	key := cacheKey{seq: seq, epoch: epoch}
	s.mu.Lock()
	cached, hit := s.cache.get(key)
	s.mu.Unlock()
	if hit {
		return cached, nil
	}

	raw, err := s.caller.Call(ctx, cdp.MethodNetworkGetResponseBody, cdp.GetResponseBodyParams{RequestID: r.RequestID})
	if err != nil {
		return Body{}, fmt.Errorf("network: get response body: %w", err)
	}
	var res cdp.GetResponseBodyResult
	if err := json.Unmarshal(raw, &res); err != nil {
		return Body{}, fmt.Errorf("network: decode response body: %w", err)
	}
	// A re-attach while the app answered: the reply came from the new session
	// and may describe another request that reused the requestId.
	if s.store.Epoch() != epoch {
		return Body{}, ErrStale
	}
	body := Body{Text: res.Body, Base64: res.Base64Encoded, Size: len(res.Body)}
	if body.Base64 {
		body.Size = decodedLen(res.Body)
	}
	if len(res.Body) > s.opts.MaxBodyBytes {
		body.Text = ""
		body.Truncated = true
	}
	s.mu.Lock()
	s.cache.put(key, body)
	s.mu.Unlock()
	return body, nil
}

// RequestBody returns the body the app sent with request seq: the inline post
// data when the event carried it, otherwise fetched from the app.
func (s *Service) RequestBody(ctx context.Context, seq, epoch uint64) (string, error) {
	r, err := s.lookup(seq, epoch)
	if err != nil {
		return "", err
	}
	if len(r.PostData) > s.opts.MaxBodyBytes {
		return "", fmt.Errorf("%w: %d bytes", ErrBodyTooLarge, len(r.PostData))
	}
	if r.PostData != "" || !r.HasPostData {
		return r.PostData, nil
	}
	raw, err := s.caller.Call(ctx, cdp.MethodNetworkGetRequestPostData, cdp.GetRequestPostDataParams{RequestID: r.RequestID})
	if err != nil {
		return "", fmt.Errorf("network: get request body: %w", err)
	}
	var res cdp.GetRequestPostDataResult
	if err := json.Unmarshal(raw, &res); err != nil {
		return "", fmt.Errorf("network: decode request body: %w", err)
	}
	if s.store.Epoch() != epoch {
		return "", ErrStale
	}
	if len(res.PostData) > s.opts.MaxBodyBytes {
		return "", fmt.Errorf("%w: %d bytes", ErrBodyTooLarge, len(res.PostData))
	}
	return res.PostData, nil
}

// decodedLen is the byte size of a base64 body.
func decodedLen(b64 string) int {
	n := base64.StdEncoding.DecodedLen(len(b64))
	return n - strings.Count(b64[max(0, len(b64)-2):], "=")
}

func (s *Service) notify() {
	if s.opts.Notify != nil {
		s.opts.Notify()
	}
}

type cacheKey struct{ seq, epoch uint64 }

type cacheItem struct {
	key  cacheKey
	body Body
}

// bodyCache is a least-recently-used cache bounded by the bytes of the bodies
// it holds. It is not safe for concurrent use; Service guards it with mu.
type bodyCache struct {
	max   int
	used  int
	order *list.List // front = most recent
	items map[cacheKey]*list.Element
}

func newBodyCache(maxBytes int) *bodyCache {
	return &bodyCache{max: maxBytes, order: list.New(), items: make(map[cacheKey]*list.Element)}
}

func (c *bodyCache) get(k cacheKey) (Body, bool) {
	el, ok := c.items[k]
	if !ok {
		return Body{}, false
	}
	c.order.MoveToFront(el)
	return el.Value.(*cacheItem).body, true //nolint:forcetypeassert // only *cacheItem is stored
}

func (c *bodyCache) put(k cacheKey, b Body) {
	if el, ok := c.items[k]; ok {
		c.used -= len(el.Value.(*cacheItem).body.Text) //nolint:forcetypeassert // only *cacheItem is stored
		c.order.Remove(el)
		delete(c.items, k)
	}
	if len(b.Text) > c.max {
		return
	}
	c.items[k] = c.order.PushFront(&cacheItem{key: k, body: b})
	c.used += len(b.Text)
	for c.used > c.max {
		last := c.order.Back()
		it := last.Value.(*cacheItem) //nolint:forcetypeassert // only *cacheItem is stored
		c.order.Remove(last)
		delete(c.items, it.key)
		c.used -= len(it.body.Text)
	}
}

func (c *bodyCache) purge() {
	c.order.Init()
	c.items = make(map[cacheKey]*list.Element)
	c.used = 0
}
