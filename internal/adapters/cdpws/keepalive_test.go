package cdpws_test

import (
	"testing"

	"github.com/Reidevl/rn-lens/internal/adapters/cdpws"
)

func TestKeepAliveBlamesThePeerOnlyWhenNothingStalledTheReader(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name          string
		waiting       int32
		before, after uint64
		want          bool
	}{
		{"no wait at all: the peer did not answer", 0, 5, 5, false},
		{"readLoop still blocked on queue space", 1, 5, 5, true},
		// readLoop and a Barrier caller both waited; the Barrier caller
		// resumed and refilled the queue, readLoop is still blocked.
		{"one of two waiters resumed", 1, 7, 7, true},
		{"a wait began and ended while the pong was due", 0, 5, 6, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := cdpws.StalledDuringPing(tc.waiting, tc.before, tc.after); got != tc.want {
				t.Fatalf("StalledDuringPing(%d, %d, %d) = %v, want %v", tc.waiting, tc.before, tc.after, got, tc.want)
			}
		})
	}
}
