package play

import (
	"fmt"
	"sort"
	"time"

	"ggosu/internal/piano"
)

type point struct {
	At    time.Duration `json:"at_ns"`
	Score piano.Score   `json:"score"`
}

// history stores changes, not frame samples. Through is a watermark: looking
// beyond it must wait, even if the last score would appear to be unchanged.
// Keep it on the game goroutine. There is no interpolation or network timing.
type history struct {
	points  []point
	through time.Duration
}

func (h *history) Reset(at time.Duration, score piano.Score) {
	h.points = append(h.points[:0], point{At: at, Score: score})
	h.through = at
}

func (h *history) Append(at time.Duration, score piano.Score) error {
	if len(h.points) == 0 {
		return fmt.Errorf("play: score history has not been initialized")
	}
	last := &h.points[len(h.points)-1]
	if at < last.At || at < h.through {
		return fmt.Errorf("play: score history moved backwards")
	}
	if at == last.At {
		last.Score = score // Last judgement wins for simultaneous changes.
	} else if score != last.Score {
		h.points = append(h.points, point{At: at, Score: score})
	}
	return nil
}

func (h *history) Advance(at time.Duration) error {
	if len(h.points) == 0 || at < h.through || at < h.points[len(h.points)-1].At {
		return fmt.Errorf("play: invalid score history watermark")
	}
	h.through = at
	return nil
}

func (h *history) At(at time.Duration) (piano.Score, bool) {
	if len(h.points) == 0 || at > h.through || at < h.points[0].At {
		return piano.Score{}, false
	}
	i := sort.Search(len(h.points), func(i int) bool { return h.points[i].At > at }) - 1
	return h.points[i].Score, true
}

// Finish extends coverage only when every judgement is final. Personal offsets
// can put an opponent's final timestamp slightly beyond our local end; the
// completed score is known to remain constant throughout that final interval.
func (h *history) Finish(through time.Duration) error {
	if len(h.points) == 0 {
		return fmt.Errorf("play: empty final score history")
	}
	s := h.points[len(h.points)-1].Score
	if s.Possible == 0 || s.Total != s.Possible {
		return fmt.Errorf("play: cannot extend an incomplete score history")
	}
	return h.Advance(max(h.through, through))
}

func (h *history) Points() []point { return append([]point(nil), h.points...) }
