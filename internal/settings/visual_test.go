package settings

import (
	"math"
	"testing"
	"time"

	"ggosu/internal/chart"
)

func TestVisualDoesNotChangeSource(t *testing.T) {
	c := chart.New(4)
	c.Timing = []chart.Timing{{BPM: 120, Scroll: 1, Meter: 4}, {At: time.Second, BPM: 240, Scroll: 2, Meter: 4, Stop: 50 * time.Millisecond}}
	c.Notes = []chart.Note{{At: 2 * time.Second, End: 2 * time.Second, Kind: "tap"}}
	hash, err := c.Digest()
	if err != nil {
		t.Fatal(err)
	}
	p, err := (Config{}).Profile(4)
	if err != nil {
		t.Fatal(err)
	}
	x := p.Visual(c)
	if math.Abs(x.Position(3*time.Second)-8.8) > 1e-9 {
		t.Fatal("full variation did not apply BPM ratio, scroll multiplier and stop")
	}
	p.SpeedChange = 0
	x = p.Visual(c)
	if x.Timing[1].BPM != 60 || x.Timing[1].Scroll != 1 || x.Timing[1].Stop != c.Timing[1].Stop {
		t.Fatal("fixed scroll did not preserve stops")
	}
	p.SpeedChange = .5
	x = p.Visual(c)
	if x.Timing[1].Scroll != 2.5 {
		t.Fatal("incorrect variation interpolation")
	}
	after, err := c.Digest()
	if err != nil || after != hash {
		t.Fatal("visual setting modified source")
	}
	p.ScrollMode = "exposure"
	p.ExposureMS = 750
	if math.Abs(p.PixelsPerSecond(2, 492)*2-p.PixelsPerSecond(1, 492)) > 1e-9 {
		t.Fatal("exposure ignores playback rate")
	}
}

func TestVisualFollowsBPMRelativeToMainBPM(t *testing.T) {
	for _, bpm := range []float64{60, 120, 180, 300} {
		c := chart.New(1)
		c.Timing = []chart.Timing{{At: -time.Second, BPM: bpm, Scroll: 1, Meter: 4}, {At: time.Second, BPM: bpm * 2, Scroll: 1, Meter: 4}}
		for _, scale := range []float64{0, .5, 1, 2} {
			x := (Profile{SpeedChange: scale}).Visual(c)
			for at := -2 * time.Second; at <= 3*time.Second; at += 100 * time.Millisecond {
				want := (at + time.Second).Seconds()
				if at > time.Second {
					want = 2 + (at-time.Second).Seconds()*(1+scale)
				}
				if got := x.Position(at); math.Abs(got-want) > 1e-9 {
					t.Fatalf("BPM %v, scale %v, at %v: position %v, want %v", bpm, scale, at, got, want)
				}
			}
		}
	}
}

func TestMainBPMIsHeldLongest(t *testing.T) {
	c := chart.New(1)
	c.Timing = []chart.Timing{{BPM: 200, Scroll: 1, Meter: 4}, {At: time.Second, BPM: 100, Scroll: 1, Meter: 4}, {At: 4 * time.Second, BPM: 200, Scroll: 1, Meter: 4}}
	c.Notes = []chart.Note{{At: 5 * time.Second, End: 5 * time.Second, Kind: "tap"}}
	if got := c.MainBPM(); got != 100 {
		t.Fatalf("main BPM %v, want 100", got)
	}
}
