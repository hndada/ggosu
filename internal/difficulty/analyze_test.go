package difficulty

import (
	"math"
	"reflect"
	"testing"
	"time"

	"ggosu/internal/chart"
)

// Preserve v1 accounting assertions explicitly as a history-disabled ablation.
func baselineOptions() Options {
	o := DefaultOptions()
	o.RepeatGain = 0
	return o
}

func pattern(trill bool, period time.Duration, cycles int) *chart.Chart {
	c := chart.New(2)
	c.Timing = []chart.Timing{{BPM: 120, Scroll: 1, Meter: 4}}
	for k := 0; k < cycles; k++ {
		for finger := range 2 {
			at := time.Duration(k) * period
			if trill && finger == 1 {
				at += period / 2
			}
			c.Notes = append(c.Notes, chart.Note{At: at, End: at, Lane: finger, Kind: "tap"})
		}
	}
	c.Normalize()
	return c
}

func analyze(t *testing.T, c *chart.Chart, p Profile, o Options) *Report {
	t.Helper()
	r, err := Analyze(c, p, o)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

// Restrict comparisons to complete interior periods; initial preparation and
// final release otherwise differ between finite chord and trill fixtures.
func interior(t *testing.T, r *Report, start, end time.Duration) (Cost, float64) {
	t.Helper()
	var c Cost
	var ws []Window
	for _, w := range r.Windows {
		if w.At >= start && w.End <= end {
			ws = append(ws, w)
			for _, f := range w.Fingers {
				c.Movement += f.Movement
				c.Interaction += f.Interaction
			}
		}
	}
	if len(ws) == 0 {
		t.Fatal("no complete interior windows")
	}
	return c, Rank(ws, r.Options.Weight)
}

func near(t *testing.T, got, want float64) {
	t.Helper()
	if math.Abs(got-want) > 1e-8*max(1, math.Abs(want)) {
		t.Fatalf("got %.12g, want %.12g", got, want)
	}
}

func TestChordTrillControls(t *testing.T) {
	o := baselineOptions()
	p := NewProfile([]int{0, 0})
	chord := pattern(false, 200*time.Millisecond, 24)
	trill := pattern(true, 200*time.Millisecond, 24)
	for _, c := range []*chart.Chart{chord, trill} {
		if len(c.Notes) != 48 {
			t.Fatal("fixtures must have identical press counts")
		}
		for finger := range 2 {
			var previous time.Duration
			seen := false
			for _, n := range c.Notes {
				if n.Lane != finger {
					continue
				}
				if seen && n.At-previous != 200*time.Millisecond {
					t.Fatal("fixtures must have identical per-finger speed")
				}
				previous, seen = n.At, true
			}
		}
	}
	start, end := 400*time.Millisecond, 4400*time.Millisecond
	a, da := interior(t, analyze(t, chord, p, o), start, end)
	b, db := interior(t, analyze(t, trill, p, o), start, end)
	near(t, a.Movement, 80)
	near(t, b.Movement, 80)
	near(t, a.Interaction, 4)
	near(t, b.Interaction, 28)
	if db <= da {
		t.Fatal("chosen opposite-motion coefficients must increase trill strain")
	}
	// This is an ablation, not proof that real trills are harder than chords.
	p.Pairs = nil
	a, da = interior(t, analyze(t, chord, p, o), start, end)
	b, db = interior(t, analyze(t, trill, p, o), start, end)
	near(t, a.Total(), b.Total())
	near(t, da, db)
	// With identical synchronous/opposed coefficients, separation disappears.
	p = NewProfile([]int{0, 0})
	p.Pairs[0].Costs[Press][Release] = Share{0.05, 0.05}
	p.Pairs[0].Costs[Release][Press] = Share{0.05, 0.05}
	_, da = interior(t, analyze(t, chord, p, o), start, end)
	_, db = interior(t, analyze(t, trill, p, o), start, end)
	near(t, da, db)
	// The example physical-interference profile does not couple separate hands.
	p = NewProfile([]int{0, 1})
	a, da = interior(t, analyze(t, chord, p, o), start, end)
	b, db = interior(t, analyze(t, trill, p, o), start, end)
	near(t, a.Interaction, 0)
	near(t, b.Interaction, 0)
	near(t, da, db)
}

func TestHeldFingerReceivesStrain(t *testing.T) {
	c := chart.New(2)
	c.Timing = []chart.Timing{{BPM: 120, Scroll: 1, Meter: 4}}
	c.Notes = []chart.Note{
		{At: 0, End: time.Second, Lane: 0, Kind: "hold"},
		{At: 400 * time.Millisecond, End: 400 * time.Millisecond, Lane: 1, Kind: "tap"},
	}
	p := NewProfile([]int{0, 0})
	r := analyze(t, c, p, baselineOptions())
	var held Cost
	for _, s := range r.Segments {
		if s.At >= 350*time.Millisecond && s.End <= 500*time.Millisecond {
			if s.States[0].Action != Hold || s.States[0].PositionStart != 1 || s.States[0].PositionEnd != 1 {
				t.Fatal("held finger must retain position throughout the other stroke")
			}
			held.Movement += s.Fingers[0].Movement
			held.Interaction += s.Fingers[0].Interaction
		}
	}
	near(t, held.Movement, 0)
	near(t, held.Interaction, 0.4) // Other finger presses and releases once.
	near(t, r.Fingers[1].Interaction, 0.5)
	near(t, r.Fingers[0].Movement, 2) // LN head and final return, not each tap.
}

func TestMotionContinuityAndCompression(t *testing.T) {
	c := pattern(false, 60*time.Millisecond, 8)
	p, o := NewProfile([]int{0, 1}), baselineOptions()
	phases, _, err := schedule(c, p, o)
	if err != nil {
		t.Fatal(err)
	}
	compressed := false
	for _, lane := range phases {
		position := 0.0
		var end time.Duration
		for k, ph := range lane {
			if k > 0 && ph.start < end {
				t.Fatal("finger movements overlap")
			}
			near(t, ph.position(ph.start), position)
			position, end = ph.position(ph.end), ph.end
			if ph.factor > 1 {
				compressed = true
			}
			if ph.action == Press && ph.position(ph.start) != 0 {
				t.Fatal("a new stroke must start after full reset")
			}
		}
		near(t, position, 0)
	}
	if !compressed {
		t.Fatal("short inter-onset intervals must compress movement time")
	}
	r := analyze(t, c, p, o)
	if r.Fingers[0].Movement <= 16 {
		t.Fatal("compressed phases must cost more than 8 relaxed strokes")
	}
}

func TestHoldCannotResetForNextNoteWithoutTime(t *testing.T) {
	c := chart.New(1)
	c.Timing = []chart.Timing{{BPM: 120, Scroll: 1, Meter: 4}}
	c.Notes = []chart.Note{
		{At: 0, End: time.Second, Kind: "hold"},
		{At: time.Second, End: time.Second, Kind: "tap"},
	}
	if _, err := Analyze(c, NewProfile([]int{0}), baselineOptions()); err == nil {
		t.Fatal("a touching hold/tap cannot silently teleport to released position")
	}
}

func TestWindowCutsDoNotDuplicateCost(t *testing.T) {
	c, p, o := pattern(true, 200*time.Millisecond, 12), NewProfile([]int{0, 0}), baselineOptions()
	a := analyze(t, c, p, o)
	o.Window = 7 * time.Millisecond
	b := analyze(t, c, p, o)
	near(t, a.Strain, b.Strain)
	for i := range p.Fingers {
		near(t, a.Fingers[i].Movement, b.Fingers[i].Movement)
		near(t, a.Fingers[i].Interaction, b.Fingers[i].Interaction)
	}
	var total float64
	for _, w := range b.Windows {
		total += w.Total
	}
	near(t, total, b.Strain)
}

func TestOffsetOrderAndRate(t *testing.T) {
	c, p, o := pattern(false, 200*time.Millisecond, 12), NewProfile([]int{0, 0}), baselineOptions()
	before := append([]chart.Note(nil), c.Notes...)
	a := analyze(t, c, p, o)
	if !reflect.DeepEqual(c.Notes, before) {
		t.Fatal("analysis changed the source chart")
	}
	shift := -123456789 * time.Nanosecond
	for i := range c.Notes {
		c.Notes[i].At += shift
		c.Notes[i].End += shift
	}
	b := analyze(t, c, p, o)
	near(t, a.Difficulty, b.Difficulty)
	near(t, a.Strain, b.Strain)
	for i := range a.Segments {
		if b.Segments[i].At-a.Segments[i].At != shift {
			t.Fatal("shifted report must retain the same relative timeline")
		}
		for finger, state := range a.Segments[i].States {
			other := b.Segments[i].States[finger]
			if state.LastMotionEnd != nil && (other.LastMotionEnd == nil || *other.LastMotionEnd-*state.LastMotionEnd != shift) {
				t.Fatal("last movement timestamps must shift with the chart")
			}
		}
	}
	for i := 0; i < len(c.Notes); i += 2 {
		c.Notes[i], c.Notes[i+1] = c.Notes[i+1], c.Notes[i]
	}
	d := analyze(t, c, p, o)
	near(t, b.Difficulty, d.Difficulty)
	near(t, b.Strain, d.Strain)
	o.Rate = 2
	before = append([]chart.Note(nil), c.Notes...)
	fast := analyze(t, c, p, o)
	if fast.Fingers[0].Movement <= d.Fingers[0].Movement {
		t.Fatal("higher rate must increase compressed-movement cost")
	}
	if !reflect.DeepEqual(c.Notes, before) {
		t.Fatal("rate changed stored chart timestamps")
	}
}

func TestPairSharesAndErrors(t *testing.T) {
	c, p, o := pattern(false, 200*time.Millisecond, 2), NewProfile([]int{0, 0}), baselineOptions()
	p.Pairs[0].Costs[Press][Press] = Share{0.2, 0.7}
	p.Pairs[0].Costs[Release][Release] = Share{}
	r := analyze(t, c, p, o)
	near(t, r.Fingers[0].Interaction, 0.4)
	near(t, r.Fingers[1].Interaction, 1.4)
	p.Pairs = append(p.Pairs, p.Pairs[0])
	if _, err := Analyze(c, p, o); err == nil {
		t.Fatal("duplicate pairs double-count recipient costs")
	}
	p = NewProfile([]int{0, 0})
	p.Pairs[0].Costs[Press][Press].A = math.NaN()
	if _, err := Analyze(c, p, o); err == nil {
		t.Fatal("NaN must be rejected")
	}
	c.Notes[0].Kind = "lift"
	if _, err := Analyze(c, NewProfile([]int{0, 0}), o); err == nil {
		t.Fatal("unsupported note kinds must not be treated as taps")
	}
}

func TestEmptyChart(t *testing.T) {
	c := chart.New(2)
	c.Timing = []chart.Timing{{BPM: 120, Scroll: 1, Meter: 4}}
	r := analyze(t, c, NewProfile([]int{0, 0}), baselineOptions())
	if r.Difficulty != 0 || r.Strain != 0 || len(r.Segments) != 0 {
		t.Fatal("empty chart must have zero strain")
	}
}

func TestVisualNotesAndSilentSuffixDoNotChangeDifficulty(t *testing.T) {
	c, p, o := pattern(true, 200*time.Millisecond, 6), NewProfile([]int{0, 0}), baselineOptions()
	a := analyze(t, c, p, o)
	c.Notes = append(c.Notes, chart.Note{At: time.Hour, End: time.Hour, Kind: "fake"})
	c.Timing = append(c.Timing, chart.Timing{At: 2 * time.Hour, BPM: 120, Scroll: 1, Meter: 4})
	c.Normalize()
	b := analyze(t, c, p, o)
	near(t, a.Strain, b.Strain)
	near(t, a.Difficulty, b.Difficulty)
	if !reflect.DeepEqual(a.Notes, b.Notes) {
		t.Fatal("visual-only notes must not require keystrokes")
	}
}

func TestHoldResetsContinuouslyBeforeNextPress(t *testing.T) {
	c := chart.New(1)
	c.Timing = []chart.Timing{{BPM: 120, Scroll: 1, Meter: 4}}
	c.Notes = []chart.Note{
		{At: 0, End: time.Second, Kind: "hold"},
		{At: time.Second + 40*time.Millisecond, End: time.Second + 40*time.Millisecond, Kind: "tap"},
	}
	p, o := NewProfile([]int{0}), baselineOptions()
	phases, _, err := schedule(c, p, o)
	if err != nil {
		t.Fatal(err)
	}
	up, down := phases[0][2], phases[0][3]
	if up.start != time.Second || up.end != down.start || down.end != c.Notes[1].At {
		t.Fatal("hold must remain down until its end, then reset before the next press")
	}
	near(t, up.position(up.end), 0)
	near(t, down.position(down.start), 0)
	near(t, up.factor, 2.5)
	near(t, down.factor, 2.5)
}
