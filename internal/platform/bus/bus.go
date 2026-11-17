// Package bus is a minimal in-process publish/subscribe hub used to tell
// interested parties that something changed. Messages are notifications, not
// data: subscribers pull the current state afterwards (ADR-0003), so a slow
// subscriber may miss intermediate notifications without losing information.
package bus

import "sync"

// Message is what subscribers receive.
type Message struct {
	Topic   string
	Payload any
}

// Bus routes messages by topic. The zero value is ready to use.
type Bus struct {
	mu   sync.RWMutex
	subs map[string]map[uint64]chan Message
	next uint64
}

// New returns an empty bus.
func New() *Bus {
	return &Bus{subs: make(map[string]map[uint64]chan Message)}
}

// Subscribe returns a channel that receives messages for topic and a function
// that cancels the subscription. The channel holds up to buffer messages; when
// it is full, new messages for that subscriber are dropped rather than
// blocking the publisher. A buffer below 1 is treated as 1.
func (b *Bus) Subscribe(topic string, buffer int) (<-chan Message, func()) {
	if buffer < 1 {
		buffer = 1
	}
	ch := make(chan Message, buffer)

	b.mu.Lock()
	if b.subs == nil {
		b.subs = make(map[string]map[uint64]chan Message)
	}
	id := b.next
	b.next++
	if b.subs[topic] == nil {
		b.subs[topic] = make(map[uint64]chan Message)
	}
	b.subs[topic][id] = ch
	b.mu.Unlock()

	var once sync.Once
	cancel := func() {
		once.Do(func() {
			b.mu.Lock()
			if subs := b.subs[topic]; subs != nil {
				delete(subs, id)
				if len(subs) == 0 {
					delete(b.subs, topic)
				}
			}
			b.mu.Unlock()
			close(ch)
		})
	}
	return ch, cancel
}

// Publish delivers payload to every subscriber of topic without blocking.
// It returns the number of subscribers that received the message.
func (b *Bus) Publish(topic string, payload any) int {
	msg := Message{Topic: topic, Payload: payload}

	b.mu.RLock()
	defer b.mu.RUnlock()

	delivered := 0
	for _, ch := range b.subs[topic] {
		select {
		case ch <- msg:
			delivered++
		default:
			// Subscriber is behind; it will pull fresh state on its next read.
		}
	}
	return delivered
}

// Subscribers returns how many subscriptions topic currently has.
func (b *Bus) Subscribers(topic string) int {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return len(b.subs[topic])
}
