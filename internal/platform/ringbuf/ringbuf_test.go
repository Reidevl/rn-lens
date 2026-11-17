package ringbuf_test

import (
	"sync"
	"testing"

	"github.com/Reidevl/rn-lens/internal/platform/ringbuf"
)

func values(entries []ringbuf.Entry[string]) []string {
	out := make([]string, len(entries))
	for i, e := range entries {
		out[i] = e.Value
	}
	return out
}

func equal(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestAppendAssignsIncreasingSequenceNumbers(t *testing.T) {
	t.Parallel()
	b := ringbuf.New[string](4)

	if got := b.Append("a"); got != 1 {
		t.Fatalf("first Append seq = %d, want 1", got)
	}
	if got := b.Append("b"); got != 2 {
		t.Fatalf("second Append seq = %d, want 2", got)
	}
	if got := b.LastSeq(); got != 2 {
		t.Fatalf("LastSeq = %d, want 2", got)
	}
}

func TestRange(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		cap     int
		push    []string
		fromSeq uint64
		limit   int
		want    []string
	}{
		{name: "empty buffer returns nothing", cap: 3, fromSeq: 1, limit: 10, want: nil},
		{name: "returns everything from the start", cap: 5, push: []string{"a", "b", "c"}, fromSeq: 1, limit: 0, want: []string{"a", "b", "c"}},
		{name: "honours the limit", cap: 5, push: []string{"a", "b", "c"}, fromSeq: 1, limit: 2, want: []string{"a", "b"}},
		{name: "starts at the requested sequence", cap: 5, push: []string{"a", "b", "c"}, fromSeq: 2, limit: 0, want: []string{"b", "c"}},
		{name: "fromSeq past the end returns nothing", cap: 5, push: []string{"a", "b"}, fromSeq: 9, limit: 0, want: nil},
		{name: "evicted entries are skipped after wraparound", cap: 3, push: []string{"a", "b", "c", "d", "e"}, fromSeq: 1, limit: 0, want: []string{"c", "d", "e"}},
		{name: "range inside a wrapped buffer", cap: 3, push: []string{"a", "b", "c", "d", "e"}, fromSeq: 4, limit: 1, want: []string{"d"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			b := ringbuf.New[string](tt.cap)
			for _, v := range tt.push {
				b.Append(v)
			}

			got := values(b.Range(tt.fromSeq, tt.limit))
			if !equal(got, tt.want) {
				t.Fatalf("Range(%d, %d) = %v, want %v", tt.fromSeq, tt.limit, got, tt.want)
			}
		})
	}
}

func TestWraparoundTracksDroppedAndOldest(t *testing.T) {
	t.Parallel()
	b := ringbuf.New[string](2)
	b.Append("a")
	b.Append("b")
	b.Append("c")

	if got := b.Dropped(); got != 1 {
		t.Fatalf("Dropped = %d, want 1", got)
	}
	if got := b.OldestSeq(); got != 2 {
		t.Fatalf("OldestSeq = %d, want 2", got)
	}
	if got := b.Len(); got != 2 {
		t.Fatalf("Len = %d, want 2", got)
	}
	if got := values(b.Range(1, 0)); !equal(got, []string{"b", "c"}) {
		t.Fatalf("Range after wrap = %v, want [b c]", got)
	}
}

func TestClearKeepsSequenceMonotonic(t *testing.T) {
	t.Parallel()
	b := ringbuf.New[string](2)
	b.Append("a")
	b.Append("b")
	b.Append("c")
	b.Clear()

	if got := b.Len(); got != 0 {
		t.Fatalf("Len after Clear = %d, want 0", got)
	}
	if got := b.Dropped(); got != 0 {
		t.Fatalf("Dropped after Clear = %d, want 0", got)
	}
	if got := b.Append("d"); got != 4 {
		t.Fatalf("Append after Clear seq = %d, want 4", got)
	}
}

func TestMinimumCapacityIsOne(t *testing.T) {
	t.Parallel()
	b := ringbuf.New[int](0)
	if got := b.Cap(); got != 1 {
		t.Fatalf("Cap = %d, want 1", got)
	}
}

func TestConcurrentAppendIsSafe(t *testing.T) {
	t.Parallel()
	const writers, perWriter = 8, 500
	b := ringbuf.New[int](100)

	var wg sync.WaitGroup
	for w := range writers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range perWriter {
				b.Append(w*perWriter + i)
			}
		}()
	}
	wg.Wait()

	if got := b.LastSeq(); got != writers*perWriter {
		t.Fatalf("LastSeq = %d, want %d", got, writers*perWriter)
	}
	if got := b.Len(); got != 100 {
		t.Fatalf("Len = %d, want 100", got)
	}
	entries := b.Range(1, 0)
	for i := 1; i < len(entries); i++ {
		if entries[i].Seq != entries[i-1].Seq+1 {
			t.Fatalf("sequence gap between %d and %d", entries[i-1].Seq, entries[i].Seq)
		}
	}
}
