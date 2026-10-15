// Package difficulty estimates keyboard-chart strain from scheduled finger
// movements. Its example coefficients are hypotheses, not measured physiology.
package difficulty

import (
	"encoding/json"
	"fmt"
	"math"
	"time"
)

type Action uint8

const (
	Rest Action = iota
	Press
	Hold
	Release
)

func (a Action) String() string {
	switch a {
	case Rest:
		return "rest"
	case Press:
		return "press"
	case Hold:
		return "hold"
	case Release:
		return "release"
	}
	return "unknown"
}

func (a Action) MarshalJSON() ([]byte, error) { return json.Marshal(a.String()) }

func moving(a Action) bool { return a == Press || a == Release }

// Finger corresponds to one lane. A lane cannot change fingers during analysis.
type Finger struct {
	Name        string        `json:"name"`
	Hand        int           `json:"hand"`
	PressTime   time.Duration `json:"press_ns"`
	ReleaseTime time.Duration `json:"release_ns"`
	PressCost   float64       `json:"press_cost"`
	ReleaseCost float64       `json:"release_cost"`
}

// Share assigns separate additional costs to the two fingers of a pair.
// These are recipient costs, not two copies of an already combined pair cost.
type Share struct {
	A float64 `json:"a"`
	B float64 `json:"b"`
}

// Matrix rows are A's action, columns are B's action, in the order
// rest, press, hold, release. Coefficients are per reference movement.
type Matrix [4][4]Share

type Pair struct {
	A     int    `json:"a"`
	B     int    `json:"b"`
	Costs Matrix `json:"costs"`
}

type Profile struct {
	Name    string   `json:"name"`
	Fingers []Finger `json:"fingers"`
	Pairs   []Pair   `json:"pairs"`
}

// NewProfile returns an editable, deliberately illustrative profile. Adjacent
// and distant fingers have the same coefficients until calibrated explicitly.
func NewProfile(hands []int) Profile {
	p := Profile{Name: "example-v1", Fingers: make([]Finger, len(hands))}
	for i, hand := range hands {
		p.Fingers[i] = Finger{
			Name: fmt.Sprintf("finger-%d", i), Hand: hand,
			PressTime: 50 * time.Millisecond, ReleaseTime: 50 * time.Millisecond,
			PressCost: 1, ReleaseCost: 1,
		}
	}
	for a := range hands {
		for b := a + 1; b < len(hands); b++ {
			if hands[a] != hands[b] {
				continue
			}
			var m Matrix
			m[Press][Press] = Share{0.05, 0.05}
			m[Release][Release] = Share{0.05, 0.05}
			m[Press][Release] = Share{0.35, 0.35}
			m[Release][Press] = Share{0.35, 0.35}
			m[Press][Hold] = Share{0.25, 0.20}
			m[Release][Hold] = Share{0.25, 0.20}
			m[Hold][Press] = Share{0.20, 0.25}
			m[Hold][Release] = Share{0.20, 0.25}
			p.Pairs = append(p.Pairs, Pair{A: a, B: b, Costs: m})
		}
	}
	return p
}

type Options struct {
	Window          time.Duration `json:"window_ns"`
	TapHold         time.Duration `json:"tap_hold_ns"`
	Weight          float64       `json:"weight"`
	SpeedPower      float64       `json:"speed_power"`
	Rate            float64       `json:"rate"`
	Recovery        time.Duration `json:"recovery_ns"`
	HoldRecovery    float64       `json:"hold_recovery"`
	LoadLimit       float64       `json:"load_limit"`
	RepeatGain      float64       `json:"repeat_gain"` // Maximum extra fraction of movement and interaction cost.
	InteractionLoad float64       `json:"interaction_load"`
}

func DefaultOptions() Options {
	return Options{Window: 200 * time.Millisecond, TapHold: 50 * time.Millisecond,
		Weight: 0.9, SpeedPower: 1, Rate: 1,
		// History adds at most half the immediate cost; speed is already
		// accounted for by movement, so repeat work stays a secondary term.
		Recovery: 250 * time.Millisecond, HoldRecovery: 0.1, LoadLimit: 4, RepeatGain: 0.5, InteractionLoad: 0.25}
}

func (p Profile) validate(lanes int) error {
	if len(p.Fingers) != lanes {
		return fmt.Errorf("difficulty: profile has %d fingers for %d lanes", len(p.Fingers), lanes)
	}
	for i, f := range p.Fingers {
		if f.Hand < 0 || f.PressTime <= 0 || f.ReleaseTime <= 0 ||
			f.PressTime > time.Minute || f.ReleaseTime > time.Minute ||
			!nonnegative(f.PressCost) || !nonnegative(f.ReleaseCost) {
			return fmt.Errorf("difficulty: invalid finger %d", i)
		}
	}
	seen := make(map[[2]int]bool)
	for _, pair := range p.Pairs {
		if pair.A < 0 || pair.A >= pair.B || pair.B >= lanes {
			return fmt.Errorf("difficulty: pair must have 0 <= A < B < %d", lanes)
		}
		key := [2]int{pair.A, pair.B}
		if seen[key] {
			return fmt.Errorf("difficulty: duplicate pair %v", key)
		}
		seen[key] = true
		for a := Rest; a <= Release; a++ {
			for b := Rest; b <= Release; b++ {
				s := pair.Costs[a][b]
				if !nonnegative(s.A) || !nonnegative(s.B) {
					return fmt.Errorf("difficulty: invalid cost for pair %v", key)
				}
				if !moving(a) && !moving(b) && (s.A != 0 || s.B != 0) {
					return fmt.Errorf("difficulty: stationary pair %v cannot have a movement cost", key)
				}
			}
		}
	}
	return nil
}

func (o Options) validate() error {
	if o.Window < time.Millisecond || o.Window > time.Hour || o.TapHold < 0 || o.TapHold > time.Minute ||
		!nonnegative(o.Weight) || o.Weight >= 1 || !nonnegative(o.SpeedPower) ||
		!nonnegative(o.Rate) || o.Rate == 0 {
		return fmt.Errorf("difficulty: invalid window, tap hold, weight, speed power or rate")
	}
	if !nonnegative(o.RepeatGain) || !nonnegative(o.LoadLimit) ||
		!nonnegative(o.HoldRecovery) || o.HoldRecovery > 1 ||
		o.Recovery < 0 || o.Recovery > time.Hour ||
		(o.RepeatGain > 0 && (o.LoadLimit == 0 || o.Recovery == 0)) {
		return fmt.Errorf("difficulty: invalid recovery, held recovery, load limit or repeat gain")
	}
	if !nonnegative(o.InteractionLoad) || o.InteractionLoad > 1 {
		return fmt.Errorf("difficulty: interaction load must be finite and in [0, 1]")
	}
	return nil
}

func nonnegative(v float64) bool { return !math.IsNaN(v) && !math.IsInf(v, 0) && v >= 0 }
