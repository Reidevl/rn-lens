package clock_test

import (
	"testing"
	"time"

	"github.com/Reidevl/rn-lens/internal/platform/clock"
)

func TestFakeAfterFiresOnlyWhenAdvancedPastDuration(t *testing.T) {
	t.Parallel()
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	c := clock.NewFake(start)
	timer := c.After(2 * time.Second)

	c.Advance(time.Second)
	select {
	case <-timer:
		t.Fatal("timer fired early")
	default:
	}

	c.Advance(time.Second)
	select {
	case at := <-timer:
		if !at.Equal(start.Add(2 * time.Second)) {
			t.Fatalf("timer fired at %v, want %v", at, start.Add(2*time.Second))
		}
	default:
		t.Fatal("timer did not fire after enough time passed")
	}
	if got := c.Now(); !got.Equal(start.Add(2 * time.Second)) {
		t.Fatalf("Now = %v, want %v", got, start.Add(2*time.Second))
	}
}

func TestRealClockMovesForward(t *testing.T) {
	t.Parallel()
	var c clock.Real
	before := c.Now()
	<-c.After(time.Millisecond)
	if !c.Now().After(before) {
		t.Fatal("real clock did not advance")
	}
}
