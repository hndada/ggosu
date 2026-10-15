package main

import (
	"sync"

	"ggosu/internal/clock"
	"ggosu/internal/keyboard"
)

const queueSize = 32768

// inbox is the only shared state between the native receiver and Ebitengine.
// A bounded ring keeps the callback allocation-free and retains every edge.
// Overflow is counted explicitly. The producer only acquires a short lock;
// it does not wait for a frame, drawing, or downstream event processing.
type inbox struct {
	clock     *clock.Counter
	mu        sync.Mutex
	items     [queueSize]keyboard.Event
	head      int
	count     int
	received  uint64
	raw       uint64
	synthetic uint64
	dropped   uint64
	highWater int
	delivery  latency
	post      latency
}

func (b *inbox) push(e keyboard.Event) {
	b.mu.Lock()
	defer b.mu.Unlock()
	now := b.clock.Now()
	b.delivery.add(now - e.At)
	b.received++
	if e.Synthetic {
		b.synthetic++
		b.post.add(e.At - e.PostedAt)
	} else {
		b.raw++
	}
	if b.count == len(b.items) {
		b.dropped++
		return
	}
	b.items[(b.head+b.count)%len(b.items)] = e
	b.count++
	b.highWater = max(b.highWater, b.count)
}

func (b *inbox) drain(dst []keyboard.Event) []keyboard.Event {
	b.mu.Lock()
	defer b.mu.Unlock()
	for b.count > 0 {
		dst = append(dst, b.items[b.head])
		b.items[b.head] = keyboard.Event{}
		b.head = (b.head + 1) % len(b.items)
		b.count--
	}
	return dst
}

type inboxReport struct {
	Received  uint64       `json:"received"`
	Raw       uint64       `json:"raw_received"`
	Synthetic uint64       `json:"synthetic_received"`
	Dropped   uint64       `json:"queue_dropped"`
	Pending   int          `json:"pending"`
	HighWater int          `json:"queue_high_water"`
	Delivery  distribution `json:"receiver_to_callback"`
	Post      distribution `json:"synthetic_post_to_receiver"`
}

func (b *inbox) report() inboxReport {
	b.mu.Lock()
	r := inboxReport{Received: b.received, Raw: b.raw, Synthetic: b.synthetic,
		Dropped: b.dropped, Pending: b.count, HighWater: b.highWater}
	delivery, post := b.delivery, b.post
	b.mu.Unlock()
	// Sorting is outside the receiver's lock and only done twice a second.
	r.Delivery, r.Post = delivery.summary(), post.summary()
	return r
}
