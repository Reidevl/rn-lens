package wailsui_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/Reidevl/rn-lens/internal/adapters/wailsui"
	"github.com/Reidevl/rn-lens/internal/platform/bus"
)

func TestCoalescerEmitsAtMostOncePerTopicPerTick(t *testing.T) {
	t.Parallel()
	b := bus.New()
	var mu sync.Mutex
	emitted := map[string]int{}
	var lastSeq uint64
	c := wailsui.NewCoalescer(b, []string{"a", "b"}, 5*time.Millisecond, func(topic string, n wailsui.ChangeNotice) {
		mu.Lock()
		defer mu.Unlock()
		emitted[topic]++
		if n.Seq <= lastSeq {
			t.Errorf("seq did not increase: %d after %d", n.Seq, lastSeq)
		}
		lastSeq = n.Seq
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go c.Run(ctx)

	// Give the subscriptions a moment to register, then burst.
	time.Sleep(10 * time.Millisecond)
	for range 100 {
		b.Publish("a", nil)
	}
	b.Publish("b", nil)
	time.Sleep(30 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()
	if emitted["a"] < 1 || emitted["a"] > 2 {
		t.Fatalf("topic a emitted %d times for a 100-message burst, want 1 or 2", emitted["a"])
	}
	if emitted["b"] != 1 {
		t.Fatalf("topic b emitted %d times, want 1", emitted["b"])
	}
}
