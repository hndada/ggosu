//go:build linux || darwin

package keyboard

import (
	"testing"
	"time"

	"ggosu/internal/clock"
)

func TestNativeLoopDeliveryAndShutdown(t *testing.T) {
	counter, err := clock.New()
	if err != nil {
		t.Fatal(err)
	}
	events := make(chan Event, 8192)
	s, err := Start(counter, func(e Event) { events <- e })
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := s.Close(); err != nil {
			t.Error(err)
		}
	})
	// No physical input is enabled. This checks the OS loop's own wake path.
	for i := 0; i < 8000; i++ {
		if err := s.Post('D', i%2 == 0); err != nil {
			t.Fatal(err)
		}
		if i%8 == 7 {
			time.Sleep(time.Millisecond)
		}
	}
	timer := time.NewTimer(3 * time.Second)
	defer timer.Stop()
	for i := 0; i < 8000; i++ {
		select {
		case e := <-events:
			if e.Seq != uint64(i+1) || !e.Synthetic || e.Key != 'D' || e.Down != (i%2 == 0) || e.At < e.PostedAt {
				t.Fatalf("event %d: %+v", i, e)
			}
		case <-timer.C:
			t.Fatalf("native loop stopped delivering at event %d", i)
		}
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	if err := s.Post('D', true); err == nil {
		t.Fatal("post succeeded after receiver shutdown")
	}
}
