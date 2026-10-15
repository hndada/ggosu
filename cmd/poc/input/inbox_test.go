package main

import (
	"testing"
	"time"

	"ggosu/internal/clock"
	"ggosu/internal/keyboard"
)

func testInbox(t *testing.T) *inbox {
	t.Helper()
	counter, err := clock.New()
	if err != nil {
		t.Fatal(err)
	}
	return &inbox{clock: counter}
}

func TestTransitionsBetweenFrames(t *testing.T) {
	g := &game{box: testInbox(t)}
	for i, down := range []bool{true, false, true} {
		g.receive(keyboard.Event{Seq: uint64(i + 1), Key: 'D', Down: down, At: g.box.clock.Now()})
	}
	if g.box.report().Received != 3 || g.handled != 0 {
		t.Fatal("delivery incorrectly depended on Update")
	}
	batch := g.box.drain(make([]keyboard.Event, 0, 3))
	for _, e := range batch {
		g.apply(e)
	}
	if len(batch) != 3 || !g.held['D'] || g.handled != 3 || g.repeats != 0 || g.orderErrors != 0 || g.sequenceGaps != 0 {
		t.Fatalf("lost or reordered edges between frames: %+v", batch)
	}
}

func TestInboxOverflowAndWrap(t *testing.T) {
	b := testInbox(t)
	for i := 0; i < queueSize+1; i++ {
		b.push(keyboard.Event{Seq: uint64(i + 1), At: b.clock.Now()})
	}
	r := b.report()
	if r.Dropped != 1 || r.Pending != queueSize || r.HighWater != queueSize {
		t.Fatalf("overflow not reported: %+v", r)
	}
	events := b.drain(make([]keyboard.Event, 0, queueSize))
	for i, e := range events {
		if e.Seq != uint64(i+1) {
			t.Fatalf("overflow overwrote pending event %d", i)
		}
	}
	for i := 0; i < 100; i++ {
		b.push(keyboard.Event{Seq: uint64(queueSize + 2 + i), At: b.clock.Now()})
	}
	events = b.drain(events[:0])
	if len(events) != 100 || events[0].Seq != queueSize+2 || b.report().Pending != 0 {
		t.Fatal("ring did not recover after overflow")
	}
}

func TestConcurrentDeliveryAndDrain(t *testing.T) {
	b := testInbox(t)
	const n = 16000
	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := 0; i < n; i++ {
			b.push(keyboard.Event{Seq: uint64(i + 1), At: b.clock.Now()})
		}
	}()
	batch := make([]keyboard.Event, 0, queueSize)
	var count uint64
	for count < n {
		batch = b.drain(batch[:0])
		for _, e := range batch {
			count++
			if e.Seq != count {
				t.Fatalf("sequence %d, want %d", e.Seq, count)
			}
		}
		if len(batch) == 0 {
			time.Sleep(time.Microsecond)
		}
	}
	<-done
	if b.report().Dropped != 0 {
		t.Fatal("concurrent queue dropped events")
	}
}

func TestLatencyWindowAndTotals(t *testing.T) {
	var l latency
	for i := 0; i < sampleCount*2; i++ {
		l.add(time.Duration(i) * time.Microsecond)
	}
	r := l.summary()
	if r.Count != sampleCount*2 || r.Samples != sampleCount || r.MaxUS != sampleCount*2-1 || r.P50US < sampleCount {
		t.Fatalf("incorrect rolling percentiles or total: %+v", r)
	}
}

func TestInboxDeliveryAllocations(t *testing.T) {
	b := testInbox(t)
	if n := testing.AllocsPerRun(1000, func() {
		b.push(keyboard.Event{At: b.clock.Now(), Key: 'D', Down: true})
	}); n != 0 {
		t.Fatalf("event handoff allocates %.1f objects per event", n)
	}
}
