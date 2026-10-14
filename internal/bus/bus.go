// Package bus is a small in-process publish/subscribe bus. Code publishes
// bytes to a named topic, and every current subscriber of that topic
// receives them.
package bus

import "sync"

// subscriberBuffer is the capacity of each subscription's channel.
const subscriberBuffer = 16

// Bus delivers messages published to a topic to that topic's subscribers.
// It is safe for concurrent use.
type Bus struct {
	mu     sync.Mutex
	topics map[string]map[*subscription]struct{}
}

// subscription is one live subscriber of a topic.
type subscription struct {
	ch chan []byte
}

// New returns an empty Bus with no topics and no subscribers.
func New() *Bus {
	return &Bus{topics: make(map[string]map[*subscription]struct{})}
}

// Publish sends data to every current subscriber of topic. It never blocks:
// a subscriber whose buffer is full misses this message.
func (b *Bus) Publish(topic string, data []byte) {
	b.mu.Lock()
	defer b.mu.Unlock()
	for sub := range b.topics[topic] {
		select {
		case sub.ch <- data:
		default:
		}
	}
}

// Subscribe returns a channel that receives the data of every message
// published to topic from now on, and a cancel function that ends the
// subscription. The cancel function may be called more than once. It does
// not close the channel.
func (b *Bus) Subscribe(topic string) (<-chan []byte, func()) {
	sub := &subscription{ch: make(chan []byte, subscriberBuffer)}

	b.mu.Lock()
	subs, ok := b.topics[topic]
	if !ok {
		subs = make(map[*subscription]struct{})
		b.topics[topic] = subs
	}
	subs[sub] = struct{}{}
	b.mu.Unlock()

	var once sync.Once
	cancel := func() {
		once.Do(func() {
			b.mu.Lock()
			defer b.mu.Unlock()
			subs := b.topics[topic]
			delete(subs, sub)
			if len(subs) == 0 {
				delete(b.topics, topic)
			}
		})
	}
	return sub.ch, cancel
}
