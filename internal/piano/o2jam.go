package piano

import (
	"sort"
	"time"

	"ggosu/internal/chart"
)

// The O2Jam mod follows open2jam 11384b3: BeatJudgment.java for the windows,
// Render.java for the life bar and pills. KOOL, COOL, GOOD and MISS stand for
// O2Jam's COOL, GOOD, BAD and MISS.
const (
	o2Beat = 0.664 // BeatJudgment divides the beat distance by this.
	o2Kool = 0.2 * o2Beat
	o2Cool = 0.5 * o2Beat
	o2Good = 0.8 * o2Beat
)

// O2JamGauge marks a Health that holds the O2Jam life bar instead of stages.
const O2JamGauge = 2

// o2Life is the full bar for easy, normal and hard (base 12000 times 2, 3 and 4).
var o2Life = [3]int{24000, 36000, 48000}

// O2JamLevel returns 1..3 for easy, normal and hard, or 0 when the mod is off.
func O2JamLevel(name string) int {
	switch name {
	case "easy":
		return 1
	case "normal":
		return 2
	case "hard":
		return 3
	}
	return 0
}

// NewO2JamHealth starts full. The game ends when the bar empties.
func NewO2JamHealth(level int) Health {
	return Health{Level: level, Value: 10000, Gauge: O2JamGauge, Life: o2Life[level-1]}
}

func (h *Health) addO2Jam(g Grade, at time.Duration) {
	full := o2Life[h.Level-1]
	switch g {
	case Kool:
		h.Life += 96
		if h.Level == 3 {
			h.Life -= 48
		}
	case Good:
		h.Life -= 240
	case Miss:
		h.Life -= 1440
	}
	h.Life = min(full, max(0, h.Life))
	h.Value = o2Percent(h.Life, full)
	if h.Life == 0 {
		h.Failed, h.At = true, at
	}
}

// o2Percent keeps any life above 0% visible as at least 0.01%.
func o2Percent(life, full int) int {
	v := life * 10000 / full
	if life > 0 {
		v = max(v, 1)
	}
	return v
}

func (h Health) validO2Jam(r Replay) bool {
	if h.Gauge != O2JamGauge || r.HealthGauge != O2JamGauge || h.Level < 1 || h.Level > 3 || h.Level != r.Health || O2JamLevel(r.Mods.O2Jam) != h.Level {
		return false
	}
	if h.Start != 0 || h.FailLevel != 0 || h.Values != [3]int{} || h.Life < 0 || h.Life > o2Life[h.Level-1] || h.Value != o2Percent(h.Life, o2Life[h.Level-1]) {
		return false
	}
	return h.Failed == (h.Life == 0) && (h.Failed || h.At == 0)
}

// beatClock counts beats from the chart's timing points; stops hold the count.
type beatClock struct {
	at   []time.Duration
	beat []float64
	bpm  []float64
	stop []time.Duration
}

func newBeatClock(timing []chart.Timing) beatClock {
	var c beatClock
	if len(timing) == 0 {
		// open2jam always has a BPM; charts without timing count 120 BPM.
		timing = []chart.Timing{{BPM: 120}}
	}
	beat := 0.0
	for i, t := range timing {
		if i > 0 {
			p := len(c.at) - 1
			beat += float64(max(0, t.At-c.at[p]-c.stop[p])) / float64(time.Minute) * c.bpm[p]
		}
		c.at = append(c.at, t.At)
		c.beat = append(c.beat, beat)
		c.bpm = append(c.bpm, t.BPM)
		c.stop = append(c.stop, t.Stop)
	}
	return c
}

func (c beatClock) beatAt(t time.Duration) float64 {
	i := sort.Search(len(c.at), func(i int) bool { return c.at[i] > t }) - 1
	if i < 0 {
		return c.beat[0] + float64(t-c.at[0])/float64(time.Minute)*c.bpm[0]
	}
	return c.beat[i] + float64(max(0, t-c.at[i]-c.stop[i]))/float64(time.Minute)*c.bpm[i]
}

// first returns the earliest time whose beat is at least b (or above b when strict).
func (c beatClock) first(b float64, strict bool) time.Duration {
	lo, hi := -chart.MaxTime-time.Hour, chart.MaxTime+time.Hour
	for lo < hi {
		mid := lo + (hi-lo)/2
		v := c.beatAt(mid)
		if v > b || !strict && v == b {
			hi = mid
		} else {
			lo = mid + 1
		}
	}
	return lo
}

type o2jam struct {
	clock  beatClock
	beats  [][2]float64       // Head and tail beat of every note.
	edges  [][4]time.Duration // Head early and late, tail early and late.
	pills  bool
	held   int // Pills in stock, at most 5.
	streak int // KOOLs since the last pill, GOOD or MISS.
}

// SetO2Jam applies the O2Jam mod from m to a new session. It replaces the
// health gauge, judgement windows, combo and adds pills when enabled.
func (s *Session) SetO2Jam(m chart.Mod) {
	level := O2JamLevel(m.O2Jam)
	if level == 0 {
		return
	}
	s.Health = NewO2JamHealth(level)
	o := &o2jam{clock: newBeatClock(s.Chart.Timing), pills: m.Pills}
	o.beats = make([][2]float64, len(s.Chart.Notes))
	o.edges = make([][4]time.Duration, len(s.Chart.Notes))
	for i, n := range s.Chart.Notes {
		head, tail := o.clock.beatAt(n.At), o.clock.beatAt(n.End)
		o.beats[i] = [2]float64{head, tail}
		o.edges[i] = [4]time.Duration{o.clock.first(head-o2Good, false), o.clock.first(head+o2Good, true) - 1, o.clock.first(tail-o2Good, false), o.clock.first(tail+o2Good, true) - 1}
	}
	s.o2jam = o
}

// O2Jam reports whether the O2Jam mod judges this session.
func (s *Session) O2Jam() bool { return s.o2jam != nil }

// Pills reports pills in stock, or -1 when pills are off.
func (s *Session) Pills() int {
	if s.o2jam == nil || !s.o2jam.pills {
		return -1
	}
	return s.o2jam.held
}

// O2JamWindows returns the KOOL, COOL and GOOD half widths at time at.
func (s *Session) O2JamWindows(at time.Duration) [3]time.Duration {
	c := s.o2jam.clock
	b := c.beatAt(at)
	return [3]time.Duration{c.first(b+o2Kool, false) - at, c.first(b+o2Cool, false) - at, c.first(b+o2Good, false) - at}
}

// window is the span in which an input can judge note i's head or tail.
func (s *Session) window(i int, tail bool) (early, late time.Duration) {
	n := s.Chart.Notes[i]
	if s.o2jam != nil {
		e := s.o2jam.edges[i]
		if tail {
			return e[2], e[3]
		}
		return e[0], e[1]
	}
	if tail {
		return n.End - Window, n.End + Window
	}
	return n.At - Window, n.At + Window
}

// o2Grade judges by beat distance; inputs outside the window never get here.
func (s *Session) o2Grade(i int, tail bool, at time.Duration) Grade {
	b := s.o2jam.beats[i][0]
	if tail {
		b = s.o2jam.beats[i][1]
	}
	d := s.o2jam.clock.beatAt(at) - b
	if d < 0 {
		d = -d
	}
	switch {
	case d <= o2Kool:
		return Kool
	case d <= o2Cool:
		return Cool
	case d <= o2Good:
		return Good
	}
	return Miss
}

// o2Pill spends a pill on a GOOD and earns one every 15 KOOLs, as open2jam does.
func (o *o2jam) pill(g Grade) Grade {
	if !o.pills {
		return g
	}
	if g == Good && o.held > 0 {
		o.held--
		g = Cool
		o.streak = 0
		return g
	}
	if g != Kool {
		o.streak = 0
		return g
	}
	o.streak++
	if o.streak >= 15 && o.held < 5 {
		o.streak -= 15
		o.held++
	}
	return g
}

// addScore counts g; under O2Jam a GOOD (O2Jam's BAD) breaks the combo too.
func (s *Session) addScore(g Grade) {
	best := s.Score.MaxCombo
	s.Score.add(g)
	if s.o2jam != nil && g == Good {
		s.Score.Combo, s.Score.MaxCombo = 0, best
	}
}
