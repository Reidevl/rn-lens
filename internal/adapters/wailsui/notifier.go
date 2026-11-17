package wailsui

import (
	"context"
	"sync"
	"time"

	"github.com/Reidevl/rn-lens/internal/platform/bus"
)

// ChangeNotice is the payload of every change event sent to the frontend.
// It carries no data on purpose (ADR-0003): the UI pulls what it needs.
type ChangeNotice struct {
	Topic string `json:"topic"`
	// Seq increases per emitted notice so the UI can ignore stale ones.
	Seq uint64 `json:"seq"`
}

// Coalescer turns bursts of bus notifications into at most one UI event per
// topic per tick, so a storm of console lines costs one webview call.
type Coalescer struct {
	bus    *bus.Bus
	topics []string
	tick   time.Duration
	emit   func(topic string, notice ChangeNotice)

	mu      sync.Mutex
	pending map[string]bool
	seq     uint64
}

// NewCoalescer subscribes to topics on b and forwards them through emit.
// A tick below 1ms is treated as 16ms (roughly one frame).
func NewCoalescer(b *bus.Bus, topics []string, tick time.Duration, emit func(string, ChangeNotice)) *Coalescer {
	if tick < time.Millisecond {
		tick = 16 * time.Millisecond
	}
	return &Coalescer{bus: b, topics: topics, tick: tick, emit: emit, pending: make(map[string]bool)}
}

// Run forwards notifications until ctx is cancelled.
func (c *Coalescer) Run(ctx context.Context) {
	var wg sync.WaitGroup
	for _, topic := range c.topics {
		ch, cancel := c.bus.Subscribe(topic, 1)
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer cancel()
			for {
				select {
				case <-ctx.Done():
					return
				case _, ok := <-ch:
					if !ok {
						return
					}
					c.mark(topic)
				}
			}
		}()
	}

	ticker := time.NewTicker(c.tick)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			wg.Wait()
			return
		case <-ticker.C:
			c.flush()
		}
	}
}

func (c *Coalescer) mark(topic string) {
	c.mu.Lock()
	c.pending[topic] = true
	c.mu.Unlock()
}

func (c *Coalescer) flush() {
	c.mu.Lock()
	if len(c.pending) == 0 {
		c.mu.Unlock()
		return
	}
	due := make([]string, 0, len(c.pending))
	for topic := range c.pending {
		due = append(due, topic)
	}
	clear(c.pending)
	c.mu.Unlock()

	for _, topic := range due {
		c.mu.Lock()
		c.seq++
		notice := ChangeNotice{Topic: topic, Seq: c.seq}
		c.mu.Unlock()
		c.emit(topic, notice)
	}
}
