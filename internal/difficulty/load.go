package difficulty

import (
	"math"
	"time"
)

// advanceLoad integrates a bounded memory driven by weighted work per second.
// Demand retains the full instantaneous cost. Corrected cost is never fed back.
//
// dL/dt = work * (1 - L/limit) - recovery * L
// extra cost rate = demand * gain * (L/limit)^2
//
// Moving fingers do not recover; held fingers use HoldRecovery/Recovery;
// released fingers use 1/Recovery. The bounded multiplier is 1..1+RepeatGain.
func advanceLoad(load, work, demand float64, action Action, elapsed time.Duration, o Options) (float64, float64) {
	if elapsed <= 0 || o.LoadLimit == 0 || o.Recovery == 0 {
		return load, 0
	}
	dt := elapsed.Seconds()
	var recovery float64
	switch action {
	case Rest:
		recovery = 1 / o.Recovery.Seconds()
	case Hold:
		recovery = o.HoldRecovery / o.Recovery.Seconds()
	}
	if work == 0 {
		x := recovery * dt
		meanSquare := load * load
		if x != 0 {
			meanSquare *= -math.Expm1(-2*x) / (2 * x)
		}
		extra := demand * dt * o.RepeatGain * meanSquare / (o.LoadLimit * o.LoadLimit)
		return load * math.Exp(-x), extra
	}
	rate := work/o.LoadLimit + recovery
	equilibrium := work / rate
	x := rate * dt
	change := equilibrium - load
	next := load + change*(-math.Expm1(-x))
	// Express moments using 1-exp(-x), with series for small intervals. This
	// avoids cancellation when a window splits a phase near its boundary.
	var first, second float64
	if x < 1e-3 {
		first = x * (0.5 + x*(-1.0/6+x*(1.0/24-x/120)))
		second = x * x * (1.0/3 + x*(-0.25+x*7.0/60))
	} else {
		first = 1 + math.Expm1(-x)/x
		second = 1 + 2*math.Expm1(-x)/x - math.Expm1(-2*x)/(2*x)
	}
	meanSquare := load*load + 2*load*change*first + change*change*second
	extra := demand * dt * o.RepeatGain * max(0, meanSquare) / (o.LoadLimit * o.LoadLimit)
	return min(o.LoadLimit, max(0, next)), extra
}

func (c Cost) rates(elapsed time.Duration, o Options) (work, demand float64) {
	seconds := elapsed.Seconds()
	return (c.Movement + o.InteractionLoad*c.Interaction) / seconds,
		(c.Movement + c.Interaction) / seconds
}

// Costs integrates a clipped segment exactly. Its repeat cost is not uniform
// within the interval, so partial segments must not just multiply full costs
// by a duration fraction. Movement and interaction retain their original units.
func (s Segment) Costs(at, end time.Duration, o Options) []Cost {
	at, end = max(s.At, at), min(s.End, end)
	costs := make([]Cost, len(s.Fingers))
	if end <= at {
		return costs
	}
	fraction := float64(end-at) / float64(s.End-s.At)
	for i, cost := range s.Fingers {
		work, demand := cost.rates(s.End-s.At, o)
		st := s.States[i]
		load, _ := advanceLoad(st.LoadStart, work, demand, st.Action, at-s.At, o)
		_, extra := advanceLoad(load, work, demand, st.Action, end-at, o)
		costs[i] = Cost{Movement: cost.Movement * fraction, Interaction: cost.Interaction * fraction, Repeat: extra}
	}
	return costs
}
