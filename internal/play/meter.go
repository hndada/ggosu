package play

import (
	"time"

	"ggosu/internal/chart"
	"ggosu/internal/piano"
)

type hit struct{ at, delta time.Duration }

// Larger errors linger so a brief loss of timing remains easy to spot.
func (h hit) lifetime() time.Duration {
	delta := h.delta
	if delta < 0 {
		delta = -delta
	}
	return time.Second + 3*time.Second*min(delta, piano.Window)/piano.Window
}

func (h hit) opacity(at time.Duration) float64 {
	age, life := at-h.at, h.lifetime()
	if age < 0 || age >= life {
		return 0
	}
	return 1 - float64(age)/float64(life)
}

type meter struct {
	hits        [64]hit
	next, count int
	trails      []hit // Visible markers expire by age, independently of the sample window.
}

func (m *meter) add(r piano.Result, n chart.Note) {
	if r.Tail || r.Grade == piano.Miss || n.Kind == "mine" || n.Kind == "fake" {
		return
	}
	m.hits[m.next] = hit{r.At, r.Delta}
	m.next = (m.next + 1) % len(m.hits)
	m.count = min(m.count+1, len(m.hits))
	active := m.trails[:0]
	for _, h := range m.trails {
		if h.opacity(r.At) > 0 {
			active = append(active, h)
		}
	}
	m.trails = append(active, hit{r.At, r.Delta})
}

func (m meter) mean() time.Duration {
	if m.count == 0 {
		return 0
	}
	var sum time.Duration
	for _, h := range m.hits[:m.count] {
		sum += h.delta
	}
	return sum / time.Duration(m.count)
}

func (m meter) last() time.Duration {
	if m.count == 0 {
		return 0
	}
	return m.hits[(m.next+len(m.hits)-1)%len(m.hits)].delta
}
