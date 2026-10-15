package keyboard

import "testing"

func TestShortTransitionsBetweenFrames(t *testing.T) {
	var b Queue
	for _, down := range []bool{true, false, true, false} {
		b.Push(Event{Key: 'D', Down: down})
	}
	events, dropped := b.Drain(nil)
	if dropped || len(events) != 4 {
		t.Fatal("short key transitions were lost")
	}
	for i, down := range []bool{true, false, true, false} {
		if events[i].Down != down {
			t.Fatal("transition order changed")
		}
	}
	events, dropped = b.Drain(events)
	if dropped || len(events) != 0 {
		t.Fatal("events consumed twice")
	}
}

func TestQueueOverflowRemainsVisibleAfterDrain(t *testing.T) {
	var q Queue
	for i := 0; i < 8193; i++ {
		q.Push(Event{Key: uint16(i)})
	}
	events, lost := q.Drain(nil)
	if !lost || len(events) != 8192 {
		t.Fatalf("overflow hidden: %d events, lost=%v", len(events), lost)
	}
	for i, e := range events {
		if e.Key != uint16(i) {
			t.Fatalf("event %d reordered or overwritten: %+v", i, e)
		}
	}
	q.Push(Event{Key: 'D'})
	events, lost = q.Drain(events)
	if !lost || len(events) != 1 || events[0].Key != 'D' {
		t.Fatal("draining cleared the invalid-attempt signal")
	}
}
