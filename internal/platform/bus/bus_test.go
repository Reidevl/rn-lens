package bus_test

import (
	"testing"
	"time"

	"github.com/Reidevl/rn-lens/internal/platform/bus"
)

func TestPublishReachesOnlyMatchingSubscribers(t *testing.T) {
	t.Parallel()
	b := bus.New()
	console, cancelConsole := b.Subscribe("console:changed", 4)
	defer cancelConsole()
	network, cancelNetwork := b.Subscribe("network:changed", 4)
	defer cancelNetwork()

	if got := b.Publish("console:changed", 7); got != 1 {
		t.Fatalf("Publish delivered to %d subscribers, want 1", got)
	}

	select {
	case msg := <-console:
		if msg.Topic != "console:changed" || msg.Payload != 7 {
			t.Fatalf("got %+v, want console:changed/7", msg)
		}
	case <-time.After(time.Second):
		t.Fatal("console subscriber did not receive the message")
	}

	select {
	case msg := <-network:
		t.Fatalf("network subscriber received %+v, want nothing", msg)
	default:
	}
}

func TestPublishDropsWhenSubscriberIsFull(t *testing.T) {
	t.Parallel()
	b := bus.New()
	ch, cancel := b.Subscribe("t", 1)
	defer cancel()

	if got := b.Publish("t", 1); got != 1 {
		t.Fatalf("first Publish delivered %d, want 1", got)
	}
	if got := b.Publish("t", 2); got != 0 {
		t.Fatalf("second Publish delivered %d, want 0 (dropped)", got)
	}
	if msg := <-ch; msg.Payload != 1 {
		t.Fatalf("received %v, want 1", msg.Payload)
	}
}

func TestCancelClosesChannelAndIsIdempotent(t *testing.T) {
	t.Parallel()
	b := bus.New()
	ch, cancel := b.Subscribe("t", 1)

	cancel()
	cancel()

	if _, open := <-ch; open {
		t.Fatal("channel still open after cancel")
	}
	if got := b.Subscribers("t"); got != 0 {
		t.Fatalf("Subscribers = %d, want 0", got)
	}
	if got := b.Publish("t", 1); got != 0 {
		t.Fatalf("Publish after cancel delivered %d, want 0", got)
	}
}

func TestZeroValueBusIsUsable(t *testing.T) {
	t.Parallel()
	var b bus.Bus
	ch, cancel := b.Subscribe("t", 0)
	defer cancel()

	b.Publish("t", "x")
	if msg := <-ch; msg.Payload != "x" {
		t.Fatalf("received %v, want x", msg.Payload)
	}
}
