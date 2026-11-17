// Package ringbuf provides a bounded, sequence-numbered ring buffer.
//
// Every stream Lens receives (console entries, network requests, query events)
// lands in one of these: the producer appends, the UI pulls a range by
// sequence number (ADR-0003). When the buffer is full the oldest entries are
// evicted and counted, so the UI can tell the user that lines were dropped.
package ringbuf

import "sync"

// Entry pairs a value with its monotonically increasing sequence number.
type Entry[T any] struct {
	Seq   uint64
	Value T
}

// Buffer is a fixed-capacity ring of entries. It is safe for concurrent use.
type Buffer[T any] struct {
	mu      sync.RWMutex
	items   []Entry[T]
	head    int    // index of the oldest entry
	size    int    // number of live entries
	nextSeq uint64 // sequence number the next Append receives
	dropped uint64
}

// New returns an empty buffer that keeps at most capacity entries.
// A capacity below 1 is treated as 1.
func New[T any](capacity int) *Buffer[T] {
	if capacity < 1 {
		capacity = 1
	}
	return &Buffer[T]{items: make([]Entry[T], capacity), nextSeq: 1}
}

// Append stores value and returns its sequence number. When the buffer is
// full the oldest entry is evicted and the dropped counter increases.
func (b *Buffer[T]) Append(value T) uint64 {
	b.mu.Lock()
	defer b.mu.Unlock()

	seq := b.nextSeq
	b.nextSeq++

	if b.size == len(b.items) {
		b.items[b.head] = Entry[T]{Seq: seq, Value: value}
		b.head = (b.head + 1) % len(b.items)
		b.dropped++
		return seq
	}

	b.items[(b.head+b.size)%len(b.items)] = Entry[T]{Seq: seq, Value: value}
	b.size++
	return seq
}

// Range returns up to limit entries whose sequence number is >= fromSeq, in
// ascending order. Entries already evicted are skipped silently; callers
// compare the first returned Seq with fromSeq to detect gaps.
// A limit below 1 returns every matching entry.
func (b *Buffer[T]) Range(fromSeq uint64, limit int) []Entry[T] {
	b.mu.RLock()
	defer b.mu.RUnlock()

	if b.size == 0 {
		return nil
	}

	oldest := b.items[b.head].Seq
	if fromSeq < oldest {
		fromSeq = oldest
	}
	offset := fromSeq - oldest
	if offset >= uint64(b.size) { //nolint:gosec // size is a non-negative int
		return nil
	}
	start := int(offset) //nolint:gosec // offset < size, so it fits in an int

	count := b.size - start
	if limit > 0 && limit < count {
		count = limit
	}

	out := make([]Entry[T], count)
	for i := range count {
		out[i] = b.items[(b.head+start+i)%len(b.items)]
	}
	return out
}

// Len returns the number of live entries.
func (b *Buffer[T]) Len() int {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.size
}

// Cap returns the maximum number of entries the buffer keeps.
func (b *Buffer[T]) Cap() int {
	return len(b.items)
}

// LastSeq returns the sequence number of the newest entry, or 0 when empty.
func (b *Buffer[T]) LastSeq() uint64 {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.nextSeq - 1
}

// OldestSeq returns the sequence number of the oldest live entry, or 0 when empty.
func (b *Buffer[T]) OldestSeq() uint64 {
	b.mu.RLock()
	defer b.mu.RUnlock()
	if b.size == 0 {
		return 0
	}
	return b.items[b.head].Seq
}

// Dropped returns how many entries were evicted since creation or the last Clear.
func (b *Buffer[T]) Dropped() uint64 {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.dropped
}

// Clear removes every entry and resets the dropped counter. Sequence numbers
// keep increasing so readers never see a number twice.
func (b *Buffer[T]) Clear() {
	b.mu.Lock()
	defer b.mu.Unlock()
	var zero Entry[T]
	for i := range b.items {
		b.items[i] = zero
	}
	b.head = 0
	b.size = 0
	b.dropped = 0
}
