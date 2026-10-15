package play

import (
	"image/color"
	"math"
	"sort"
	"time"

	"ggosu/internal/chart"
)

const Width, Height = 1280, 720

type lane struct {
	x, width float32
	color    color.NRGBA
}

type layout struct {
	left, width       float32
	top, line, bottom float32
	lanes             []lane
}

// newLayout keeps columns touching across skins and player groups: argon has 60-unit columns with the middle of
// an odd count twice as wide, triangles 80-unit columns with a 70-unit middle, classic 48-unit columns
// without gaps.
func newLayout(c *chart.Chart, skin string) layout {
	l := layout{line: skinLine(skin), bottom: Height, lanes: make([]lane, c.Lanes)}
	column, special := argonColumn, float32(2)
	switch skin {
	case "triangles":
		column, special = 80*argonScale, 70.0/80
	case "classic":
		column, special = 48*argonScale, 1
	case "o2jam":
		column, special = 52, 1.35
	case "circles":
		column, special = 64*argonScale, 1
	}
	weights := make([]float32, c.Lanes)
	var units float32
	for i := range weights {
		weights[i] = 1
		if argonSpecialColumn(c.Lanes, i) {
			weights[i] = special
		}
		if scratchLane(c, i) {
			weights[i] = 1.25
		}
		units += weights[i]
	}
	unit := min(column, 820/units)
	l.width = unit * units
	l.left = (Width - l.width) / 2
	x := l.left
	for i, w := range weights {
		col := skinColour(skin, c.Lanes, i, scratchLane(c, i))
		l.lanes[i] = lane{x, unit * w, col}
		x += unit * w
	}
	return l
}

// scratchLane follows the chart's own layout (BMS); a plain 8K chart is 7K plus a left scratch, as
// hndada/gosu's KeyScratchModes{8: ScratchModeLeft}.
func scratchLane(c *chart.Chart, i int) bool {
	if len(c.Layout) > 0 {
		return c.Layout[i].Role == "scratch"
	}
	return c.Lanes == 8 && i == 0
}

// scroll indexes cumulative positions once; a frame queries one current time.
// Stops, zero scroll and coincident timing points follow chart.Chart.Position.
type scroll struct {
	timing    []chart.Timing
	positions []float64
}

func newScroll(c *chart.Chart) scroll {
	s := scroll{timing: c.Timing, positions: make([]float64, len(c.Timing))}
	for i := 1; i < len(s.timing); i++ {
		p, n := s.timing[i-1], s.timing[i]
		s.positions[i] = s.positions[i-1] + max(time.Duration(0), n.At-p.At-p.Stop).Seconds()*p.BPM/60*p.Scroll
	}
	return s
}

func (s scroll) position(at time.Duration) float64 {
	i := sort.Search(len(s.timing), func(i int) bool { return s.timing[i].At > at }) - 1
	if i < 0 {
		t := s.timing[0]
		return (at - t.At).Seconds() * t.BPM / 60 * t.Scroll
	}
	t := s.timing[i]
	return s.positions[i] + max(time.Duration(0), at-t.At-t.Stop).Seconds()*t.BPM/60*t.Scroll
}

type span struct {
	note            int
	head, tail, end float64
}
type window struct{ first, last int }
type barLine struct {
	position float64
	major    bool
}
type field struct {
	margin  float64
	bars    []barLine
	layout  layout
	scroll  scroll
	notes   [][]span
	windows []window
	cursor  float64
}

// c is the visual chart, whose BPMs are normalised for scrolling; music keeps
// the real BPMs and meters, so bar lines follow the song's measures.
func newField(c, music *chart.Chart, skin string) field {
	f := field{margin: 16, layout: newLayout(c, skin), scroll: newScroll(c), notes: make([][]span, c.Lanes), windows: make([]window, c.Lanes)}
	if skin == "circles" {
		for _, lane := range f.layout.lanes {
			f.margin = max(f.margin, float64(lane.width)*0.7+16)
		}
	}
	var last time.Duration
	for i, n := range c.Notes {
		last = max(last, n.End)
		list := f.notes[n.Lane]
		x := span{note: i, head: f.scroll.position(n.At), tail: f.scroll.position(n.End)}
		x.end = x.tail
		if len(list) > 0 {
			x.end = max(x.end, list[len(list)-1].end)
		}
		f.notes[n.Lane] = append(list, x)
	}
	for _, m := range music.Measures(last + time.Millisecond) {
		f.bars = append(f.bars, barLine{f.scroll.position(m.At), m.Major})
	}
	return f
}

func (f *field) update(at time.Duration, speed float64) {
	f.cursor = f.scroll.position(at)
	// Heads can extend slightly below the judgement line. Prefix tail maxima
	// retain active long notes even when their heads have left the screen.
	low := f.cursor - f.margin/speed
	high := f.cursor + float64(f.layout.line-f.layout.top+16)/speed
	for k, list := range f.notes {
		first := sort.Search(len(list), func(i int) bool { return list[i].end >= low })
		last := sort.Search(len(list), func(i int) bool { return list[i].head > high })
		f.windows[k] = window{first, last}
	}
}

func (f field) y(position, speed float64) float32 {
	return f.layout.line - float32((position-f.cursor)*speed)
}

func (l layout) surfaceSize() (int, int) {
	return int(math.Ceil(float64(l.width))), int(l.line-l.top) + 16
}
