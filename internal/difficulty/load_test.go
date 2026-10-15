package difficulty

import (
	"math"
	"reflect"
	"testing"
	"time"

	"ggosu/internal/chart"
)

func TestLoadIntegrationAndRecovery(t *testing.T) {
	o := DefaultOptions()
	for _, action := range []Action{Press, Release, Hold, Rest} {
		for _, span := range []time.Duration{time.Nanosecond, time.Microsecond, 50 * time.Millisecond, time.Second} {
			end, extra := advanceLoad(1.3, 20, 20, action, span, o)
			mid, a := advanceLoad(1.3, 20, 20, action, span/2, o)
			split, b := advanceLoad(mid, 20, 20, action, span-span/2, o)
			near(t, end, split)
			near(t, extra, a+b)
			if end < 0 || end > o.LoadLimit || extra < 0 || extra > 20*span.Seconds()*o.RepeatGain {
				t.Fatal("load and repeat multiplier must stay bounded")
			}
		}
	}
	free, freeCost := advanceLoad(2, 0, 0, Rest, o.Recovery, o)
	held, heldCost := advanceLoad(2, 0, 0, Hold, o.Recovery, o)
	near(t, free, 2/math.E)
	if held <= free || freeCost != 0 || heldCost != 0 {
		t.Fatal("maintaining a hold must recover more slowly without generating standalone cost")
	}
}

func TestHistoryDoesNotDependOnReportingCuts(t *testing.T) {
	p, o := NewProfile([]int{0, 0}), DefaultOptions()
	c := pattern(true, 160*time.Millisecond, 40)
	a := analyze(t, c, p, o)
	o.Window = 7 * time.Millisecond
	b := analyze(t, c, p, o)
	near(t, a.Strain, b.Strain)
	for i := range p.Fingers {
		near(t, a.Fingers[i].Movement, b.Fingers[i].Movement)
		near(t, a.Fingers[i].Interaction, b.Fingers[i].Interaction)
		near(t, a.Fingers[i].Repeat, b.Fingers[i].Repeat)
	}
	if a.Fingers[0].Repeat <= 0 {
		t.Fatal("repeated work must add a separate history cost")
	}
}

func TestPartialSegmentCost(t *testing.T) {
	p, o := NewProfile([]int{0, 0}), DefaultOptions()
	r := analyze(t, pattern(false, 200*time.Millisecond, 8), p, o)
	for _, s := range r.Segments {
		mid := s.At + (s.End-s.At)/3
		a, b := s.Costs(s.At, mid, o), s.Costs(mid, s.End, o)
		for i, full := range s.Fingers {
			near(t, a[i].Movement+b[i].Movement, full.Movement)
			near(t, a[i].Interaction+b[i].Interaction, full.Interaction)
			near(t, a[i].Repeat+b[i].Repeat, full.Repeat)
		}
	}
}

func TestHeldRecipientAddsLoadWithoutNewMovement(t *testing.T) {
	c := chart.New(2)
	c.Timing = []chart.Timing{{BPM: 120, Scroll: 1, Meter: 4}}
	c.Notes = []chart.Note{
		{At: 0, End: time.Second, Lane: 0, Kind: "hold"},
		{At: 400 * time.Millisecond, End: 400 * time.Millisecond, Lane: 1, Kind: "tap"},
	}
	r := analyze(t, c, NewProfile([]int{0, 0}), DefaultOptions())
	found := false
	for _, s := range r.Segments {
		st, cost := s.States[0], s.Fingers[0]
		if st.Action == Hold && cost.Interaction > 0 {
			if cost.Movement != 0 || st.LoadEnd <= st.LoadStart || cost.Repeat <= 0 {
				t.Fatal("held recipient must accumulate interaction-driven work without a new keystroke")
			}
			found = true
		}
	}
	if !found {
		t.Fatal("missing held-recipient interaction")
	}
}

func TestHistoryUsesOriginalWorkAndKeepsTimeInvariants(t *testing.T) {
	c, p, o := pattern(false, 200*time.Millisecond, 8), NewProfile([]int{0, 0}), DefaultOptions()
	a := analyze(t, c, p, o)
	o.RepeatGain = 0
	b := analyze(t, c, p, o)
	for k, s := range a.Segments {
		for i, st := range s.States {
			near(t, st.LoadStart, b.Segments[k].States[i].LoadStart)
			near(t, st.LoadEnd, b.Segments[k].States[i].LoadEnd)
		}
	}
	// Changing repeat gain never feeds corrected costs back into load.
	o.RepeatGain = 12
	d := analyze(t, c, p, o)
	for k, s := range a.Segments {
		for i, st := range s.States {
			near(t, st.LoadEnd, d.Segments[k].States[i].LoadEnd)
		}
	}
	o = DefaultOptions()
	for i := range c.Notes {
		c.Notes[i].At -= 123456789 * time.Nanosecond
		c.Notes[i].End -= 123456789 * time.Nanosecond
	}
	shifted := analyze(t, c, p, o)
	near(t, a.Difficulty, shifted.Difficulty)
	near(t, a.Strain, shifted.Strain)
}

func TestInvalidRecovery(t *testing.T) {
	c, p := pattern(false, 200*time.Millisecond, 2), NewProfile([]int{0, 0})
	for _, invalid := range []func(*Options){
		func(o *Options) { o.LoadLimit = 0 },
		func(o *Options) { o.Recovery = 0 },
		func(o *Options) { o.HoldRecovery = 1.1 },
		func(o *Options) { o.RepeatGain = math.NaN() },
		func(o *Options) { o.InteractionLoad = -0.1 },
		func(o *Options) { o.InteractionLoad = 1.1 },
		func(o *Options) { o.InteractionLoad = math.NaN() },
	} {
		o := DefaultOptions()
		invalid(&o)
		if _, err := Analyze(c, p, o); err == nil {
			t.Fatal("invalid recovery settings must be rejected")
		}
	}
}

func TestHistoryRetainsChartAndIgnoresChordOrder(t *testing.T) {
	c, p, o := pattern(false, 200*time.Millisecond, 12), NewProfile([]int{0, 0}), DefaultOptions()
	before := append([]chart.Note(nil), c.Notes...)
	a := analyze(t, c, p, o)
	if !reflect.DeepEqual(before, c.Notes) {
		t.Fatal("history analysis changed stored notes")
	}
	for i := 0; i < len(c.Notes); i += 2 {
		c.Notes[i], c.Notes[i+1] = c.Notes[i+1], c.Notes[i]
	}
	b := analyze(t, c, p, o)
	near(t, a.Difficulty, b.Difficulty)
	near(t, a.Strain, b.Strain)
	o.Rate = 2
	before = append([]chart.Note(nil), c.Notes...)
	analyze(t, c, p, o)
	if !reflect.DeepEqual(before, c.Notes) {
		t.Fatal("history/rate evaluation rescaled source notes")
	}
}

func TestWeightedWorkIntegration(t *testing.T) {
	o := DefaultOptions()
	for _, work := range []float64{0, 5, 20} {
		for _, action := range []Action{Press, Hold, Rest} {
			end, extra := advanceLoad(1.3, work, 20, action, 50*time.Millisecond, o)
			mid, a := advanceLoad(1.3, work, 20, action, 17*time.Millisecond, o)
			split, b := advanceLoad(mid, work, 20, action, 33*time.Millisecond, o)
			near(t, end, split)
			near(t, extra, a+b)
			if extra <= 0 {
				t.Fatal("existing load must still affect demand when no new load is accumulated")
			}
		}
	}
	load, extra := advanceLoad(2, 0, 20, Press, 50*time.Millisecond, o)
	near(t, load, 2)
	near(t, extra, 0.125)
	// Repetition is output only. It never contributes to either original rate.
	cost := Cost{Movement: 2, Interaction: 8, Repeat: 100}
	work, demand := cost.rates(time.Second, o)
	near(t, work, 4)
	near(t, demand, 10)
	o.InteractionLoad = 0
	work, demand = cost.rates(time.Second, o)
	near(t, work, 2)
	near(t, demand, 10)
}

func TestRepeatRemainsSecondary(t *testing.T) {
	p, o := NewProfile([]int{0, 0}), DefaultOptions()
	for _, rate := range []float64{1, 2} {
		o.Rate = rate
		for _, period := range []time.Duration{40, 100, 200, 800} {
			for _, trill := range []bool{false, true} {
				c := pattern(trill, period*time.Millisecond, 80)
				r := analyze(t, c, p, o)
				for _, w := range r.Windows {
					for _, cost := range w.Fingers {
						base := cost.Movement + cost.Interaction
						if cost.Repeat > 0.5*base+1e-8 {
							t.Fatalf("rate %v period %v trill %v: repeat %v exceeds half of base %v", rate, period, trill, cost.Repeat, base)
						}
					}
				}
				withoutRepeat := o
				withoutRepeat.RepeatGain = 0
				base := analyze(t, c, p, withoutRepeat)
				if r.Difficulty <= base.Difficulty || r.Difficulty > 1.5*base.Difficulty+1e-8 {
					t.Fatalf("repeat difficulty %v must be above %v and at most 1.5 times it", r.Difficulty, base.Difficulty)
				}
			}
		}
	}
}
