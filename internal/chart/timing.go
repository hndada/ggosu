package chart

import (
	"fmt"
	"math"
	"sort"
	"time"
)

// Change expresses a beat-domain tempo change or stop before conversion.
type Change struct {
	Beat, BPM float64
	Stop      time.Duration
}
type beatPoint struct {
	beat, seconds, bpm float64
	stop               time.Duration
}
type Clock struct{ points []beatPoint }

// NewClock converts beats once. Notes on a stop's beat occur before the stop;
// later beats include the stop. Offset is already on the audio timeline.
func NewClock(bpm float64, offset time.Duration, changes []Change) (*Clock, error) {
	if !finite(bpm) || bpm <= 0 {
		return nil, fmt.Errorf("chart: invalid initial BPM")
	}
	x := append([]Change(nil), changes...)
	sort.SliceStable(x, func(i, j int) bool { return x[i].Beat < x[j].Beat })
	c := &Clock{points: []beatPoint{{seconds: offset.Seconds(), bpm: bpm}}}
	for _, v := range x {
		if !finite(v.Beat) || v.Beat < 0 || !finite(v.BPM) || v.BPM < 0 || v.Stop < 0 {
			return nil, fmt.Errorf("chart: invalid beat change")
		}
		p := c.points[len(c.points)-1]
		seconds := p.seconds + p.stop.Seconds() + (v.Beat-p.beat)*60/p.bpm
		if v.Beat == p.beat {
			seconds = p.seconds
		}
		if v.BPM > 0 {
			bpm = v.BPM
		}
		if v.Beat == p.beat {
			c.points[len(c.points)-1].bpm = bpm
			c.points[len(c.points)-1].stop += v.Stop
		} else {
			c.points = append(c.points, beatPoint{v.Beat, seconds, bpm, v.Stop})
		}
	}
	return c, nil
}

func (c *Clock) At(beat float64) (time.Duration, error) {
	if !finite(beat) || beat < 0 {
		return 0, fmt.Errorf("chart: invalid beat")
	}
	i := sort.Search(len(c.points), func(i int) bool { return c.points[i].beat > beat }) - 1
	p := c.points[max(i, 0)]
	s := p.seconds + (beat-p.beat)*60/p.bpm
	if beat > p.beat {
		s += p.stop.Seconds()
	}
	return Seconds(s)
}

func Seconds(s float64) (time.Duration, error) {
	if !finite(s) || math.Abs(s) >= float64(math.MaxInt64)/1e9 {
		return 0, fmt.Errorf("chart: time out of range")
	}
	return time.Duration(math.Round(s * 1e9)), nil
}
func (c *Clock) Timing() ([]Timing, error) {
	var out []Timing
	for _, p := range c.points {
		at, err := Seconds(p.seconds)
		if err != nil {
			return nil, err
		}
		out = append(out, Timing{At: at, BPM: p.bpm, Scroll: 1, Meter: 4, Stop: p.stop})
	}
	return out, nil
}

// Position integrates BPM and scroll velocity in beats, freezing at stops.
// It is pure and supports seeking; judgement always uses time, not position.
func (c *Chart) Position(at time.Duration) float64 {
	if len(c.Timing) == 0 {
		return at.Seconds() * 2
	}
	pos := 0.0
	last := c.Timing[0].At
	t := c.Timing[0]
	if at < last {
		return (at - last).Seconds() * t.BPM / 60 * t.Scroll
	}
	for _, next := range c.Timing[1:] {
		if next.At > at {
			break
		}
		pos += max(time.Duration(0), next.At-last-t.Stop).Seconds() * t.BPM / 60 * t.Scroll
		last = next.At
		t = next
	}
	return pos + max(time.Duration(0), at-last-t.Stop).Seconds()*t.BPM/60*t.Scroll
}

// MainBPM is the BPM held longest from the first timing point to the last note,
// like lazer's most common beat length; it is the 1x reference for BPM scroll.
func (c *Chart) MainBPM() float64 {
	if len(c.Timing) == 0 {
		return 60
	}
	end := c.Timing[len(c.Timing)-1].At
	for _, n := range c.Notes {
		end = max(end, n.At, n.End)
	}
	held := make(map[float64]time.Duration)
	for i, t := range c.Timing {
		next := end
		if i+1 < len(c.Timing) {
			next = c.Timing[i+1].At
		}
		held[t.BPM] += max(0, next-t.At)
	}
	bpm, longest := c.Timing[0].BPM, time.Duration(-1)
	for _, t := range c.Timing {
		if held[t.BPM] > longest {
			bpm, longest = t.BPM, held[t.BPM]
		}
	}
	return bpm
}
