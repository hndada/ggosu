package play

import (
	"math"
	"testing"
	"time"

	"ggosu/internal/chart"
	"ggosu/internal/settings"
)

func fieldChart() *chart.Chart {
	c := chart.New(4)
	c.Timing = []chart.Timing{{At: -time.Second, BPM: 120, Scroll: 1, Meter: 4}, {At: time.Second, BPM: 120, Scroll: 0, Meter: 4}, {At: 2 * time.Second, BPM: 180, Scroll: 2, Meter: 3, Stop: 300 * time.Millisecond}, {At: 2 * time.Second, BPM: 150, Scroll: .5, Meter: 3, Stop: 200 * time.Millisecond}, {At: 4 * time.Second, BPM: 240, Scroll: 3, Meter: 4}}
	c.Notes = []chart.Note{{At: 0, End: 5 * time.Second, Lane: 0, Kind: "hold"}, {At: time.Second, End: time.Second, Lane: 0, Kind: "fake"}, {At: 1500 * time.Millisecond, End: 1500 * time.Millisecond, Lane: 1, Kind: "tap"}, {At: 3 * time.Second, End: 3 * time.Second, Lane: 2, Kind: "tap"}, {At: 6 * time.Second, End: 6 * time.Second, Lane: 0, Kind: "tap"}}
	return c
}

func TestIndexedScrollMatchesSource(t *testing.T) {
	c := fieldChart()
	if err := c.Validate(); err != nil {
		t.Fatal(err)
	}
	s := newScroll(c)
	for at := -2 * time.Second; at <= 8*time.Second; at += 7 * time.Millisecond {
		if math.Abs(s.position(at)-c.Position(at)) > 1e-9 {
			t.Fatalf("at %v: indexed %.9f vs source %.9f", at, s.position(at), c.Position(at))
		}
	}
}

func TestVisibleRangeRetainsLongNotesAndZeroScroll(t *testing.T) {
	c := fieldChart()
	f := newField(c, c, "")
	for _, speed := range []float64{30, 110, 300, 1200} {
		for at := -time.Second; at <= 7*time.Second; at += 57 * time.Millisecond {
			f.update(at, speed)
			for k, list := range f.notes {
				w := f.windows[k]
				if w.first > w.last {
					t.Fatalf("invalid window: %+v", w)
				}
				for j, n := range list {
					head, tail := f.y(n.head, speed), f.y(n.tail, speed)
					visible := head >= f.layout.top-16 && tail <= f.layout.line+16
					if visible && (j < w.first || j >= w.last) {
						t.Fatalf("lost visible note %d at %v, speed %v", n.note, at, speed)
					}
				}
			}
		}
	}
}

func TestCircleRemainsVisiblePastHitLine(t *testing.T) {
	c := fieldChart()
	c.Timing = []chart.Timing{{BPM: 120, Scroll: 1, Meter: 4}}
	c.Notes = []chart.Note{{At: time.Second, End: time.Second, Kind: "tap"}}
	f := newField(c, c, "circles")
	f.update(1010*time.Millisecond, 1200)
	if f.windows[0] != (window{first: 0, last: 1}) {
		t.Fatal("circle disappeared while its upper half was visible", f.windows[0])
	}
	f.update(1040*time.Millisecond, 1200)
	if f.windows[0].first != f.windows[0].last {
		t.Fatal("offscreen circle was retained", f.windows[0])
	}
}

func TestLayoutsKeepFieldClearOfHUD(t *testing.T) {
	for n := 1; n <= 18; n++ {
		c := chart.New(n)
		if n == 14 {
			for i := 0; i < n; i++ {
				role := "key"
				if i%7 == 0 {
					role = "scratch"
				}
				c.Layout = append(c.Layout, chart.Lane{Role: role, Group: i / 7})
			}
		}
		for _, skin := range skinNames {
			l := newLayout(c, skin)
			if l.left < 216 || l.left+l.width > 1064 {
				t.Fatalf("%dK overlaps HUD: %+v", n, l)
			}
			for i, lane := range l.lanes {
				if lane.width < 30 || (i > 0 && lane.x != l.lanes[i-1].x+l.lanes[i-1].width) {
					t.Fatalf("invalid lane at %dK", n)
				}
			}
		}
	}
}

func TestPresentationPhases(t *testing.T) {
	if phase(-time.Second, 0, false, false, false) != Ready || phase(-50*time.Millisecond, -100*time.Millisecond, false, false, false) != Playing {
		t.Fatal("lead-in hides negative-time gameplay")
	}
	if phase(3*time.Second, 0, true, false, false) != Paused || phase(5*time.Second, 0, false, false, true) != Outro || phase(5*time.Second, 0, false, true, true) != Finished {
		t.Fatal("wrong pause or completion phase")
	}
}

var visibleTotal int

func benchmarkChart() *chart.Chart {
	c := chart.New(7)
	for i := 0; i < 20000; i++ {
		at := time.Duration(i) * 10 * time.Millisecond
		c.Notes = append(c.Notes, chart.Note{At: at, End: at, Lane: i % 7, Kind: "tap"})
	}
	for i := 0; i < 256; i++ {
		c.Timing = append(c.Timing, chart.Timing{At: time.Duration(i) * time.Second, BPM: 120 + float64(i%3)*30, Scroll: 1 + float64(i%2)*.5, Meter: 4})
	}
	return c
}

func BenchmarkFieldIndexed(b *testing.B) {
	f := newField(benchmarkChart(), benchmarkChart(), "")
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		f.update(12*time.Second, 110)
		n := 0
		for _, w := range f.windows {
			n += w.last - w.first
		}
		visibleTotal = n
	}
}

// Same CPU lookups as the former renderer: every future note, with linear
// timing integration for its head and tail. This is not a GPU/FPS benchmark.
func BenchmarkFieldFormerScan(b *testing.B) {
	c := benchmarkChart()
	l := newLayout(c, "")
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cursor := c.Position(12 * time.Second)
		n := 0
		for _, note := range c.Notes {
			if note.End < 12*time.Second-135*time.Millisecond {
				continue
			}
			head, tail := c.Position(note.At), c.Position(note.End)
			if head <= cursor+float64(l.line-l.top+16)/110 && tail >= cursor-16.0/110 {
				n++
			}
		}
		visibleTotal = n
	}
}

// A plain 8K chart reads as 7K plus a left scratch (hndada/gosu KeyScratchModes); BMS keeps its layout.
func TestEightKeysScratchOnLeft(t *testing.T) {
	c := chart.New(8)
	if !scratchLane(c, 0) || scratchLane(c, 7) {
		t.Fatal("8K scratch lane")
	}
	l := newLayout(c, "triangles")
	if l.lanes[0].color != trianglesSpecial || l.lanes[0].width <= l.lanes[1].width {
		t.Fatal("8K scratch lane is not drawn as scratch", l.lanes[0])
	}
	if scratchLane(chart.New(7), 0) || scratchLane(chart.New(10), 0) {
		t.Fatal("only 8K gets a scratch")
	}
}

// The visual chart sets every BPM to 60, so bar lines must come from the real tempo.
func TestBarLinesFollowMusicTempo(t *testing.T) {
	c := chart.New(4)
	c.Timing = []chart.Timing{{At: 0, BPM: 180, Scroll: 1, Meter: 4}}
	c.Notes = []chart.Note{{At: 4 * time.Second, End: 4 * time.Second, Lane: 0, Kind: "tap"}}
	visual := (settings.Profile{SpeedChange: 1}).Visual(c)
	f := newField(visual, c, "")
	bar := (4 * 60 * time.Second / 180).Seconds()
	if len(f.bars) != 4 {
		t.Fatalf("got %d bars, want 4", len(f.bars))
	}
	for i, b := range f.bars {
		if want := visual.Position(time.Duration(float64(i) * bar * float64(time.Second))); math.Abs(b.position-want) > 1e-6 {
			t.Fatalf("bar %d at %.6f, want %.6f", i, b.position, want)
		}
	}
}
