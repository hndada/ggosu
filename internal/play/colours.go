package play

import (
	"image/color"
	"math"
	"time"

	"ggosu/internal/chart"
)

// Okabe–Ito colours, which stay apart under every common colour-vision deficiency:
// sky blue and orange alternate from the stage edges, yellow marks the special column.
var (
	assistBlue    = color.NRGBA{86, 180, 233, 255}
	assistOrange  = color.NRGBA{230, 159, 0, 255}
	assistSpecial = color.NRGBA{240, 228, 66, 255}
)

// setNoteColour applies a note colour choice to a freshly built layout.
func (p *view) setNoteColour(choice string) {
	p.noteColour, p.snap = choice, nil
	c := p.session.Chart
	switch choice {
	case "assist":
		for i := range p.field.layout.lanes {
			col := assistBlue
			if min(i, c.Lanes-1-i)%2 == 1 {
				col = assistOrange
			}
			if scratchLane(c, i) || argonSpecialColumn(c.Lanes, i) {
				col = assistSpecial
			}
			p.field.layout.lanes[i].color = col
		}
	case "snap":
		p.snap = snapColours(c)
	}
}

// snapColours is osu!lazer's timing-based note colouring: each note takes the colour of
// BindableBeatDivisor.GetColourFor(ControlPointInfo.GetClosestBeatDivisor(start time)).
// A timing point starts a new beat grid where the BPM changes; scroll-only points keep it.
func snapColours(c *chart.Chart) []color.NRGBA {
	out := make([]color.NRGBA, len(c.Notes))
	for i, n := range c.Notes {
		out[i] = divisorColour(closestDivisor(c.Timing, n.At))
	}
	return out
}

var snapDivisors = []int{1, 2, 3, 4, 5, 6, 7, 8, 9, 12, 16} // BindableBeatDivisor.PREDEFINED_DIVISORS

func closestDivisor(timing []chart.Timing, at time.Duration) int {
	origin, bpm := time.Duration(0), 0.0
	for i, t := range timing {
		if i > 0 && t.At > at {
			break
		}
		if i == 0 || t.BPM != bpm {
			origin, bpm = t.At, t.BPM
		}
	}
	if bpm <= 0 {
		return 1
	}
	ms := func(d time.Duration) float64 { return float64(d) / float64(time.Millisecond) }
	beat, start, x := 60000/bpm, ms(origin), ms(at)
	if x < 0 {
		x += math.Ceil(-x/beat) * beat
	}
	best, closest := 0, math.MaxFloat64
	for _, d := range snapDivisors {
		length := beat / float64(d)
		snapped := start + math.Round((max(x, 0)-start)/length)*length
		if snapped < 0 {
			snapped += length
		}
		if dist := math.Abs(x - snapped); closest-dist > 1e-7 {
			best, closest = d, dist
		}
	}
	return best
}

// divisorColour is BindableBeatDivisor.GetColourFor with OsuColour values.
func divisorColour(divisor int) color.NRGBA {
	hex := func(v uint32) color.NRGBA { return color.NRGBA{uint8(v >> 16), uint8(v >> 8), uint8(v), 255} }
	switch divisor {
	case 1:
		return white
	case 2:
		return hex(0xed1121)
	case 4:
		return hex(0x66ccff)
	case 8:
		return hex(0xffcc22)
	case 16:
		return hex(0x6644cc)
	case 3:
		return hex(0x8866ee)
	case 6:
		return hex(0xeeaa00)
	case 12:
		return hex(0xcc6600)
	case 5, 7, 9:
		return hex(0xb3d944)
	}
	return hex(0xff0000)
}
