package multi

import (
	"testing"
	"time"
)

func TestClockExcludesServerQueueAndRejectsStaleSamples(t *testing.T) {
	var c Clock
	// The server clock is 400ms ahead. A 200ms server queue is not network RTT.
	for i := range 3 {
		t1 := time.Duration(i)*time.Second + time.Second
		t2 := t1 + 400*time.Millisecond + 10*time.Millisecond
		t3 := t2 + 200*time.Millisecond
		t4 := t1 + 220*time.Millisecond
		if err := c.Observe(Stamp{int64(t1), int64(t2), int64(t3)}, t4); err != nil {
			t.Fatal(err)
		}
	}
	e, ok := c.Estimate(4 * time.Second)
	if !ok || e.Offset != 400*time.Millisecond || e.RTT != 20*time.Millisecond || e.Uncertainty != 10*time.Millisecond {
		t.Fatalf("wrong clock estimate: %+v %v", e, ok)
	}
	start, err := e.LocalStart(int64(6*time.Second), 4*time.Second)
	if err != nil || start != 5600*time.Millisecond {
		t.Fatalf("wrong local reservation: %s %v", start, err)
	}
	if _, err := e.LocalStart(int64(4*time.Second), 4*time.Second); err == nil {
		t.Fatal("late start accepted")
	}
	if _, ok := c.Estimate(40 * time.Second); ok {
		t.Fatal("stale minimum RTT retained indefinitely")
	}
}

func TestClockRecentLowDelayMedian(t *testing.T) {
	var c Clock
	for i, delay := range []time.Duration{10, 12, 11, 500} {
		t1 := time.Duration(i+1) * time.Second
		t2 := t1 + 2*time.Second + delay*time.Millisecond
		t3 := t2 + time.Millisecond
		t4 := t1 + 2*delay*time.Millisecond + time.Millisecond
		if err := c.Observe(Stamp{int64(t1), int64(t2), int64(t3)}, t4); err != nil {
			t.Fatal(err)
		}
	}
	e, ok := c.Estimate(6 * time.Second)
	if !ok || e.Offset != 2*time.Second || e.RTT != 20*time.Millisecond || e.Uncertainty != 12*time.Millisecond {
		t.Fatalf("queued sample changed the estimate: %+v", e)
	}
	if err := c.Observe(Stamp{100, 200, 300}, 150); err == nil {
		t.Fatal("negative RTT accepted")
	}
}

func TestClockAsymmetryIsNotHidden(t *testing.T) {
	var c Clock
	for i := range 3 {
		t1 := time.Duration(i+1) * time.Second
		// 10ms uplink + 70ms downlink has a 30ms offset bias.
		t2 := t1 + 500*time.Millisecond + 10*time.Millisecond
		if err := c.Observe(Stamp{int64(t1), int64(t2), int64(t2)}, t1+80*time.Millisecond); err != nil {
			t.Fatal(err)
		}
	}
	e, ok := c.Estimate(4 * time.Second)
	if !ok || e.Offset != 470*time.Millisecond || e.Uncertainty < 30*time.Millisecond {
		t.Fatalf("asymmetric path uncertainty hidden: %+v", e)
	}
}
