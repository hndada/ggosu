package clock

import "testing"

func TestCounterMonotonicAndDistinct(t *testing.T) {
	c, err := New()
	if err != nil {
		t.Fatal(err)
	}
	if c.Frequency() <= 0 {
		t.Fatal("counter has no frequency")
	}
	previous, distinct := c.Now(), 0
	for i := 0; i < 10000; i++ {
		now := c.Now()
		if now < previous {
			t.Fatal("counter moved backwards")
		}
		if now > previous {
			distinct++
		}
		previous = now
	}
	if distinct == 0 {
		t.Fatal("counter could not distinguish consecutive samples")
	}
	t.Logf("frequency %d Hz; distinct consecutive samples %d/10000", c.Frequency(), distinct)
}

func TestCounterReadAllocations(t *testing.T) {
	c, err := New()
	if err != nil {
		t.Fatal(err)
	}
	if n := testing.AllocsPerRun(1000, func() { c.Now() }); n != 0 {
		t.Fatalf("clock measurement allocates %.1f objects per event", n)
	}
}
