package settings

import (
	"fmt"
	"math"

	"ggosu/internal/chart"
)

// Playback is the selection window's last chosen play options.
type Playback struct {
	Visibility string  `json:"visibility,omitempty"`
	O2Jam      string  `json:"o2jam,omitempty"`
	Pills      bool    `json:"pills,omitempty"`
	Health     int     `json:"health"`
	HealthFail int     `json:"healthFail"`
	Mode       string  `json:"mode"`
	Mod        string  `json:"mod"`
	Rate       float64 `json:"rate"`
	Pitch      bool    `json:"pitch"`
}

func (p Playback) Validate() error {
	if err := chart.ValidateVisibility(p.Visibility); err != nil {
		return err
	}
	if err := chart.ValidateO2Jam(p.O2Jam); err != nil {
		return err
	}
	if p.Health < 0 || p.Health > 3 || p.HealthFail < 0 || p.HealthFail > 3 || p.Health > 0 && p.HealthFail > p.Health {
		return fmt.Errorf("health must be off or level 1..3")
	}
	if p.Mode != "live" && p.Mode != "auto" {
		return fmt.Errorf("play mode must be live or auto")
	}
	if p.Mod != "" && p.Mod != "mirror" && p.Mod != "random" && p.Mod != "mirror+random" {
		return fmt.Errorf("note arrangement must be empty, mirror, random or mirror+random")
	}
	if math.IsNaN(p.Rate) || math.IsInf(p.Rate, 0) || p.Rate < 0.5 || p.Rate > 2 {
		return fmt.Errorf("time rate must be between 0.5 and 2")
	}
	return nil
}

func (c Config) PlaybackSettings() Playback {
	if c.Playback != nil {
		return *c.Playback
	}
	return Playback{Health: 3, Mode: "live", Rate: 1, Pitch: true}
}
