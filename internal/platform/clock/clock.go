// Package clock abstracts time so reconnect backoff and timestamps are
// deterministic in tests.
package clock

import "time"

// Clock is the subset of package time that Lens code depends on.
type Clock interface {
	Now() time.Time
	After(d time.Duration) <-chan time.Time
}

// Real uses the system clock.
type Real struct{}

// Now returns the current time.
func (Real) Now() time.Time { return time.Now() }

// After waits for d and then sends the current time on the returned channel.
func (Real) After(d time.Duration) <-chan time.Time { return time.After(d) }

// Fake is a manually advanced clock for tests. It is not safe for concurrent
// use without external synchronization beyond Advance/Now.
type Fake struct {
	now     time.Time
	waiters []waiter
}

type waiter struct {
	at time.Time
	ch chan time.Time
}

// NewFake returns a fake clock starting at start.
func NewFake(start time.Time) *Fake {
	return &Fake{now: start}
}

// Now returns the fake time.
func (f *Fake) Now() time.Time { return f.now }

// After returns a channel that fires once Advance moves the clock past d.
func (f *Fake) After(d time.Duration) <-chan time.Time {
	ch := make(chan time.Time, 1)
	f.waiters = append(f.waiters, waiter{at: f.now.Add(d), ch: ch})
	return ch
}

// Advance moves the clock forward and fires every timer that is now due.
func (f *Fake) Advance(d time.Duration) {
	f.now = f.now.Add(d)
	remaining := f.waiters[:0]
	for _, w := range f.waiters {
		if !w.at.After(f.now) {
			w.ch <- f.now
			continue
		}
		remaining = append(remaining, w)
	}
	f.waiters = remaining
}
