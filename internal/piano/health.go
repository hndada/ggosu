package piano

import "time"

// Health is optional. Value uses hundredths of one percent for exact replay.
type Health struct {
	Level     int           `json:"level"`
	Value     int           `json:"value"`
	Failed    bool          `json:"failed"`
	At        time.Duration `json:"at_ns,omitempty"`
	Start     int           `json:"start,omitempty"`
	FailLevel int           `json:"fail_level,omitempty"`
	Values    [3]int        `json:"values,omitzero"`
	Gauge     int           `json:"gauge,omitempty"` // Delta table; 0 is the first stage table kept for older records.
	Life      int           `json:"life,omitempty"`  // O2Jam life bar units; Value is its percentage.
}

// HealthGauge is the delta table new plays use: stages 1–3 follow beatoraja's
// ASSIST EASY, HARD and EX-HARD gauges at fixed values.
const HealthGauge = 1

// NewHealth calculates every stage from the first judgement. A zero failLevel
// allows play to continue even when stage 1 is empty; stages never promote.
func NewHealth(level, failLevel int) Health {
	if level == 0 {
		return Health{Value: 10000}
	}
	h := Health{Level: level, Value: 10000, Start: level, FailLevel: failLevel, Gauge: HealthGauge}
	h.Values = [3]int{10000, 10000, 10000}
	return h
}

// Older replays used one fixed gauge and ended as soon as it emptied; replays without a gauge use table 0.
func replayHealth(level, failLevel int, stages bool, gauge int) Health {
	if gauge == O2JamGauge {
		return NewO2JamHealth(level)
	}
	if stages {
		h := NewHealth(level, failLevel)
		if h.Level > 0 {
			h.Gauge = gauge
		}
		return h
	}
	return Health{Level: level, Value: 10000}
}

func (h *Health) add(g Grade, at time.Duration) {
	if h.Level == 0 || h.Failed {
		return
	}
	if h.Gauge == O2JamGauge {
		h.addO2Jam(g, at)
		return
	}
	if h.Start == 0 {
		h.Value = min(10000, max(0, h.Value+healthDelta(g, h.Level, 0, h.Value)))
		if h.Value == 0 {
			h.Failed, h.At = true, at
		}
		return
	}
	for i := 0; i < h.Start; i++ {
		v := min(10000, max(0, h.Values[i]+healthDelta(g, i+1, h.Gauge, h.Values[i])))
		if g == Good && i == 2 {
			v = max(v, min(1, h.Values[i])) // Goods alone never empty stage 3.
		}
		h.Values[i] = v
	}
	for {
		h.Value = h.Values[h.Level-1]
		if h.Value > 0 {
			return
		}
		if h.FailLevel > 0 && h.Level <= h.FailLevel {
			h.Failed, h.At = true, at
			return
		}
		if h.Level == 1 {
			return
		}
		h.Level--
	}
}

// Hundredths of a percent for KOOL, COOL, GOOD and MISS. Only stage 3 loses health on GOOD.
var gaugeDeltas = [3][4]int{
	{30, 30, 15, -300},   // ASSIST EASY
	{15, 12, 3, -1000},   // HARD
	{15, 6, -100, -1600}, // EX-HARD
}

// HARD softens damage while low, as beatoraja's HARD guts table does.
var hardGuts = [...]struct{ below, percent int }{{1000, 40}, {2000, 50}, {3000, 60}, {4000, 70}, {5000, 80}}

func healthDelta(g Grade, level, gauge, value int) int {
	if gauge == HealthGauge {
		delta := 0
		switch g {
		case Kool:
			delta = gaugeDeltas[level-1][0]
		case Cool:
			delta = gaugeDeltas[level-1][1]
		case Good:
			delta = gaugeDeltas[level-1][2]
		case Miss:
			delta = gaugeDeltas[level-1][3]
		}
		if level == 2 && delta < 0 {
			for _, gut := range hardGuts {
				if value < gut.below {
					return delta * gut.percent / 100
				}
			}
		}
		return delta
	}
	delta := 0
	switch g {
	case Kool:
		delta = 40
	case Cool:
		delta = 20
	case Good:
		delta = -100 * (level - 1)
	case Miss:
		delta = -400 * level
	}
	if level == 3 && delta > 0 {
		delta /= 2
	}
	return delta
}

func (h Health) valid(r Replay) bool {
	if h.Gauge == O2JamGauge || r.HealthGauge == O2JamGauge {
		return h.validO2Jam(r)
	}
	if h.Level < 1 || h.Level > 3 || h.Value < 0 || h.Value > 10000 || !h.Failed && h.At != 0 {
		return false
	}
	if !r.HealthStages {
		return h.Start == 0 && h.FailLevel == 0 && h.Values == [3]int{} && h.Gauge == 0 && h.Level == r.Health && h.Failed == (h.Value == 0)
	}
	if h.Gauge != r.HealthGauge || h.Start != r.Health || h.Level > h.Start || h.FailLevel != r.HealthFail || h.Value != h.Values[h.Level-1] {
		return false
	}
	for _, value := range h.Values {
		if value < 0 || value > 10000 {
			return false
		}
	}
	if h.Value == 0 && h.Level > 1 && h.FailLevel < h.Level {
		return false // An empty upper stage must have fallen or ended play.
	}
	return h.Failed == (h.Value == 0 && h.FailLevel > 0 && h.Level <= h.FailLevel)
}
