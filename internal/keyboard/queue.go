package keyboard

import "sync"

// Queue retains transitions between game updates. Its zero value is ready to
// use. Push can run on the receiver goroutine; Drain runs on the game goroutine.
// Overflow remains observable until the queue is replaced for a new attempt.
type Queue struct {
	mu      sync.Mutex
	events  [8192]Event
	size    int
	dropped bool
}

func (q *Queue) Push(e Event) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.size == len(q.events) {
		q.dropped = true
		return
	}
	q.events[q.size] = e
	q.size++
}

func (q *Queue) Drain(dst []Event) ([]Event, bool) {
	q.mu.Lock()
	defer q.mu.Unlock()
	dst = append(dst[:0], q.events[:q.size]...)
	q.size = 0
	return dst, q.dropped
}
