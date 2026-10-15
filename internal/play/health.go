package play

import (
	"math"
	"time"

	"ggosu/internal/chart"
)

const healthHeightRatio = 0.7

// Only the presentation eases; judgement and the percentage use exact health.
type healthMotion struct {
	level          int
	target         int
	value, trail   float32
	glow           float32 // The glow leads a rise and the fill follows, as in ArgonHealthDisplay.
	riseFrom       float32
	glowFrom       float32
	at, gain, loss time.Duration
	rise           time.Duration
	fallback       bool // A stage fallback drains from full more slowly than damage.
}

func newHealthMotion(value int) healthMotion {
	return healthMotion{target: value, value: float32(value), trail: float32(value), glow: float32(value), at: -chart.MaxTime, gain: -chart.MaxTime, loss: -chart.MaxTime, rise: -chart.MaxTime}
}

func (h *healthMotion) update(value int, at time.Duration) {
	if h.at == -chart.MaxTime || at < h.at {
		level := h.level
		*h = newHealthMotion(value)
		h.level = level
		h.at = at
		return
	}
	if value < h.target {
		h.loss = at
		h.trail = max(h.trail, h.value)
	} else if value > h.target {
		h.gain, h.rise = at, at
		h.riseFrom, h.glowFrom = h.value, h.glow
		h.fallback = false
	}
	dt := max(time.Duration(0), at-h.at)
	h.target, h.at = value, at
	if h.rise > h.loss {
		// Argon: the fill rises over 500 ms and its glow over 250 ms, both OutQuint.
		age := at - h.rise
		h.value = float32(tween(age, 500*time.Millisecond, float64(h.riseFrom), float64(value)))
		h.glow = float32(tween(age, 250*time.Millisecond, float64(h.glowFrom), float64(value)))
	} else {
		tau := 0.085
		if h.fallback {
			tau = 0.2
		}
		h.value += (float32(value) - h.value) * float32(1-math.Exp(-dt.Seconds()/tau))
		h.glow = h.value
		if h.value-float32(value) < 1 {
			h.fallback = false
		}
	}
	if at-h.loss > 180*time.Millisecond {
		h.trail += (h.value - h.trail) * float32(1-math.Exp(-dt.Seconds()/0.24))
	}
	h.trail = max(h.trail, h.value)
	if value == 0 {
		h.value, h.glow = 0, 0
	}
}

func healthPulse(age, duration time.Duration) float64 {
	if age < 0 || age >= duration {
		return 0
	}
	t := 1 - float64(age)/float64(duration)
	return t * t
}

// healthFlash is Argon's hit flash: the glow turns white in 30 ms and returns in 300 ms, both OutQuint.
func healthFlash(age time.Duration) float64 {
	if age < 0 {
		return 0
	}
	if age < 30*time.Millisecond {
		return tween(age, 30*time.Millisecond, 0, 1)
	}
	return tween(age-30*time.Millisecond, 300*time.Millisecond, 1, 0)
}

// healthBeat is Quaver's JudgementHitBurst bump (183 ms OutExpo), widened to 8 px and
// repeated on every beat of the current timing point.
func healthBeat(timing []chart.Timing, at time.Duration) float32 {
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
		return 0
	}
	beat := time.Duration(float64(time.Minute) / bpm)
	age := (at - origin) % beat
	if age < 0 {
		age += beat
	}
	const bump, duration = 8, 183 * time.Millisecond
	if age >= duration {
		return 0
	}
	return bump * float32(math.Pow(2, -10*float64(age)/float64(duration)))
}

func (p *view) healthEdge() float32 {
	l := p.field.layout
	x := l.left + l.width
	if p.skin == "o2jam" {
		x += 8 // Attach to the outside of the stage frame.
	}
	return x
}
