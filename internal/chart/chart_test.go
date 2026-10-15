package chart

import (
	"bytes"
	"math"
	"testing"
	"time"
)

func TestBeatClock(t *testing.T) {
	c, err := NewClock(120, -time.Second, []Change{{Beat: 2, BPM: 240}, {Beat: 4, Stop: time.Second}, {Beat: 6, BPM: 60}})
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		beat float64
		want time.Duration
	}{{0, -time.Second}, {2, 0}, {4, 500 * time.Millisecond}, {5, 1750 * time.Millisecond}, {6, 2 * time.Second}, {7, 3 * time.Second}} {
		got, err := c.At(test.beat)
		if err != nil || got != test.want {
			t.Errorf("beat %v: %v %v, want %v", test.beat, got, err, test.want)
		}
	}
	points, err := c.Timing()
	if err != nil {
		t.Fatal(err)
	}
	chart := New(4)
	chart.Timing = points
	if chart.Position(time.Second) != chart.Position(500*time.Millisecond) {
		t.Fatal("scroll moved during stop")
	}
}
func TestRoundTripAndDigest(t *testing.T) {
	c := New(4)
	c.Timing = []Timing{{BPM: 120, Scroll: 1, Meter: 4}}
	c.Notes = []Note{{At: time.Second, End: 2 * time.Second, Lane: 1, Kind: "hold"}}
	c.Source.Path = "one.osu"
	a, err := c.Digest()
	if err != nil {
		t.Fatal(err)
	}
	c.Source.Path = "two.osu"
	b, err := c.Digest()
	if err != nil || a != b {
		t.Fatal("location changed digest")
	}
	var buf bytes.Buffer
	if err := Write(&buf, c); err != nil {
		t.Fatal(err)
	}
	copy, err := Read(&buf)
	if err != nil {
		t.Fatal(err)
	}
	hash, err := copy.Digest()
	if err != nil || hash != a {
		t.Fatal("roundtrip changed digest")
	}
	c.Timing[0].BPM = math.NaN()
	if c.Validate() == nil {
		t.Fatal("accepted NaN")
	}
}
func TestRejectBadChart(t *testing.T) {
	for _, notes := range [][]Note{{{Lane: 4, Kind: "tap"}}, {{At: 2, End: 1, Kind: "hold"}}, {{At: 1, End: 10, Lane: 1, Kind: "hold"}, {At: 5, End: 5, Lane: 1, Kind: "tap"}}} {
		c := New(4)
		c.Timing = []Timing{{BPM: 120, Scroll: 1, Meter: 4}}
		c.Notes = notes
		if c.Validate() == nil {
			t.Errorf("accepted invalid notes: %+v", notes)
		}
	}
}
