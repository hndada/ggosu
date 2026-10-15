package songselect

import (
	"fmt"
	"math/rand/v2"
	"path/filepath"
	"strings"

	"ggosu/internal/chart"
)

// Medley picks up to n charts of one key count whose calculated level is in [low, high],
// each from a different song and at least a minute long.
func (l *Songs) Medley(lanes int, low, high float64, n int) ([]chart.Entry, error) {
	l.mu.Lock()
	var pool []chart.Entry
	for _, e := range l.entries {
		if e.Lanes == lanes && e.Rating > 0 && e.Rating >= low && e.Rating <= high && e.Length >= 60 {
			pool = append(pool, e)
		}
	}
	l.mu.Unlock()
	rand.Shuffle(len(pool), func(i, j int) { pool[i], pool[j] = pool[j], pool[i] })
	var picked []chart.Entry
	used := map[string]bool{}
	for _, e := range pool {
		// A song is its folder, except O2Jam: one .ojn holds a whole song and o2jam folders keep thousands side by side.
		song := filepath.Dir(e.Path)
		if strings.EqualFold(filepath.Ext(e.Path), ".ojn") {
			song = e.Path
		}
		if !used[song] {
			used[song] = true
			picked = append(picked, e)
			if len(picked) == n {
				return picked, nil
			}
		}
	}
	if len(picked) == 0 {
		return nil, fmt.Errorf("조건에 맞는 곡이 없습니다")
	}
	return picked, nil
}
