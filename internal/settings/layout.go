package settings

import "fmt"

// Point is an offset in the game's 1280 by 720 reference frame.
type Point struct {
	X     int `json:"x"`
	Y     int `json:"y"`
	Scale int `json:"scale,omitempty"` // Percent; zero means 100.
}

// Layout keeps presentation preferences out of charts and replay input data.
// The zero value retains the default layout and combo/judgement presets.
type Layout struct {
	Health      Point `json:"health,omitzero"`
	HitPosition int   `json:"hitPosition,omitempty"` // Absolute Y; zero uses the skin default.
	Song        Point `json:"song,omitzero"`
	Comparison  Point `json:"comparison,omitzero"`
	Ghost       Point `json:"ghost,omitzero"`
	Score       Point `json:"score,omitzero"`
	Counts      Point `json:"counts,omitzero"`
	Combo       Point `json:"combo,omitzero"`
	Judgement   Point `json:"judgement,omitzero"`
	Time        Point `json:"time,omitzero"`
	Gauges      Point `json:"gauges,omitzero"`
	Meter       Point `json:"meter,omitzero"`
}

const MinHitPosition, MaxHitPosition = 240, 640

func (l Layout) Validate() error {
	if l.HitPosition != 0 && (l.HitPosition < MinHitPosition || l.HitPosition > MaxHitPosition) {
		return fmt.Errorf("hit position must be between %d and %d", MinHitPosition, MaxHitPosition)
	}
	for _, p := range []Point{l.Health, l.Song, l.Comparison, l.Ghost, l.Score, l.Counts, l.Combo, l.Judgement, l.Time, l.Gauges, l.Meter} {
		if p.Scale != 0 && (p.Scale < 25 || p.Scale > 200) {
			return fmt.Errorf("HUD scale must be 25..200 percent")
		}
		if p.X < -1280 || p.X > 1280 || p.Y < -720 || p.Y > 720 {
			return fmt.Errorf("HUD offset must fit the 1280 by 720 frame")
		}
	}
	return nil
}
