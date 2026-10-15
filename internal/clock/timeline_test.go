package clock

import (
	"math"
	"testing"
	"time"
)

func TestTimelinePauseAndRate(t *testing.T) {
	c := NewTimeline(10 * time.Second)
	for _, change := range []struct {
		at   time.Duration
		rate float64
	}{{12 * time.Second, 0}, {17 * time.Second, 2}, {20 * time.Second, 0.5}} {
		if err := c.SetRate(change.at, change.rate); err != nil {
			t.Fatal(err)
		}
	}
	// Historical queries model input received before, but consumed after,
	// multiple pause/speed boundaries in a stalled Update.
	for _, tc := range []struct{ at, want time.Duration }{
		{9 * time.Second, 0}, {11 * time.Second, time.Second},
		{12 * time.Second, 2 * time.Second}, {16 * time.Second, 2 * time.Second},
		{18 * time.Second, 4 * time.Second}, {20 * time.Second, 8 * time.Second},
		{22 * time.Second, 9 * time.Second},
	} {
		if got := c.Position(tc.at); got != tc.want {
			t.Errorf("Position(%s) = %s, want %s", tc.at, got, tc.want)
		}
	}
}

func TestTimelineRejectsInvalidChanges(t *testing.T) {
	c := NewTimeline(time.Second)
	for _, rate := range []float64{-1, math.NaN(), math.Inf(1), math.Inf(-1)} {
		if err := c.SetRate(time.Second, rate); err == nil {
			t.Errorf("accepted rate %v", rate)
		}
	}
	if err := c.SetRate(0, 2); err == nil || c.Rate() != 1 {
		t.Fatal("accepted backwards change or changed state on error")
	}
	if err := c.SetRate(time.Second, 0); err != nil {
		t.Fatal(err)
	}
	if err := c.SetRate(time.Second, 2); err != nil || c.Position(2*time.Second) != 2*time.Second {
		t.Fatal("same-timestamp changes were not applied in order")
	}
}

func TestTimelineZeroValue(t *testing.T) {
	var c Timeline
	if c.Position(time.Second) != 0 || c.Rate() != 0 {
		t.Fatal("zero value must be paused")
	}
	if err := c.SetRate(time.Second, 1); err != nil || c.Position(2*time.Second) != time.Second {
		t.Fatal("zero value did not resume")
	}
}
