package difficulty

import (
	"fmt"
	"math"
	"time"

	"ggosu/internal/chart"
)

// A phase has a continuous linear position, rather than an instantaneous reset.
// Press completes at a note onset; release starts at a hold end (or tap dwell).
type phase struct {
	start, end time.Duration
	action     Action
	note       int
	factor     float64
}

type stroke struct {
	at, release time.Duration
	note        int
	hold        bool
}

func schedule(c *chart.Chart, p Profile, o Options) ([][]phase, []int, error) {
	lanes := make([][]stroke, c.Lanes)
	counts := make([]int, c.Lanes)
	for i, n := range c.Notes {
		if n.Kind == "fake" {
			continue
		}
		if n.Kind != "tap" && n.Kind != "hold" {
			return nil, nil, fmt.Errorf("difficulty: note %d kind %q is outside the tap/hold POC", i, n.Kind)
		}
		at, err := scaled(n.At, o.Rate)
		if err != nil {
			return nil, nil, err
		}
		end, err := scaled(n.End, o.Rate)
		if err != nil {
			return nil, nil, err
		}
		lanes[n.Lane] = append(lanes[n.Lane], stroke{at: at, release: end, note: i, hold: n.Kind == "hold"})
		counts[n.Lane]++
	}
	phases := make([][]phase, c.Lanes)
	for lane, notes := range lanes {
		f := p.Fingers[lane]
		down := f.PressTime
		for i, n := range notes {
			releaseAt := n.release
			if !n.hold {
				dwell := o.TapHold
				if i+1 < len(notes) {
					// Fixed policy: release taps early; retain at most a quarter
					// of the inter-onset interval as dwell, leaving reset time.
					dwell = min(dwell, (notes[i+1].at-n.at)/4)
				}
				releaseAt = n.at + dwell
			}
			up, nextDown := f.ReleaseTime, f.PressTime
			if i+1 < len(notes) {
				gap := notes[i+1].at - releaseAt
				if gap <= 1 {
					return nil, nil, fmt.Errorf("difficulty: lane %d notes %d/%d leave no time for release and a distinct press", lane, n.note, notes[i+1].note)
				}
				if gap < up+nextDown {
					// Compress both phases proportionally, keeping position
					// continuous and leaving a genuine release between presses.
					up = time.Duration(float64(gap) * float64(up) / float64(up+nextDown))
					up = max(1, min(up, gap-1))
					nextDown = gap - up
				}
			}
			press := phase{n.at - down, n.at, Press, n.note, speed(f.PressTime, down, o.SpeedPower)}
			returning := phase{releaseAt, releaseAt + up, Release, n.note, speed(f.ReleaseTime, up, o.SpeedPower)}
			if !nonnegative(press.factor) || !nonnegative(returning.factor) {
				return nil, nil, fmt.Errorf("difficulty: speed correction overflow at note %d", n.note)
			}
			phases[lane] = append(phases[lane], press)
			if releaseAt > n.at {
				phases[lane] = append(phases[lane], phase{n.at, releaseAt, Hold, n.note, 0})
			}
			phases[lane] = append(phases[lane], returning)
			down = nextDown
		}
	}
	return phases, counts, nil
}

func speed(reference, actual time.Duration, power float64) float64 {
	return math.Pow(max(1, float64(reference)/float64(actual)), power)
}

func scaled(t time.Duration, rate float64) (time.Duration, error) {
	v := float64(t) / rate
	// Keep enough headroom for reference motions and subtraction of endpoints.
	if math.IsNaN(v) || math.IsInf(v, 0) || math.Abs(v) > float64(chart.MaxTime) {
		return 0, fmt.Errorf("difficulty: rated timestamp exceeds chart time range")
	}
	return time.Duration(math.Round(v)), nil
}

func (p phase) position(at time.Duration) float64 {
	switch p.action {
	case Press:
		return float64(at-p.start) / float64(p.end-p.start)
	case Release:
		return 1 - float64(at-p.start)/float64(p.end-p.start)
	case Hold:
		return 1
	}
	return 0
}
