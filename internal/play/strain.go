package play

import (
	"fmt"

	"ggosu/internal/chart"
	"ggosu/internal/difficulty"
)

// noteStrain is the debug overlay's per-note strain: the song select's model
// (default profile, half the lanes per hand) at the current Time Rate.
type noteStrain struct {
	rate   float64
	values []float64
	byRate map[float64][]float64
	err    string
}

func (s *noteStrain) update(c *chart.Chart, rate float64) {
	if s.byRate != nil && rate == s.rate {
		return
	}
	if s.byRate == nil {
		s.byRate = make(map[float64][]float64)
	}
	s.rate, s.err = rate, ""
	if v, ok := s.byRate[rate]; ok {
		s.values = v
		return
	}
	s.values = nil
	o := difficulty.DefaultOptions()
	o.Rate = rate
	r, err := difficulty.Analyze(c, difficulty.NewProfile(difficulty.Hands(c.Lanes)), o)
	if err != nil {
		s.err = err.Error()
		return
	}
	notes := r.Attribute(len(c.Notes))
	s.values = make([]float64, len(notes))
	for i, n := range notes {
		s.values[i] = n.Cost.Total()
	}
	s.byRate[rate] = s.values
}

// drawStrain labels a note just above its head on the note surface, so lane
// covers and visibility masks hide the label with the note.
func (p *view) drawStrain(note int, x, y float32) {
	if note >= len(p.strain.values) || p.strain.values[note] == 0 {
		return
	}
	p.hud.Center(p.notes, fmt.Sprintf("%.2f", p.strain.values[note]), small, x, y, ink)
}
