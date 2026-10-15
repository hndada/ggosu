package piano

import (
	"sort"
	"time"
)

// Summary keeps catalogue progress small and leaves replay events on disk.
// Points use current live completions at 1x across lane mods.
// Achievements use unmodified completions.
type Summary struct {
	Hash       string    `json:"hash"`
	LastPlayed time.Time `json:"lastPlayed"`
	Completed  bool      `json:"completed"`
	FullCombo  bool      `json:"fullCombo"`
	Perfect    bool      `json:"perfect"` // Every judgement COOL or better.
	Gauge      int       `json:"gauge"`   // Highest health stage still above 0% at the end; 0 for none.
	BestPoints *int      `json:"bestPoints"`
}

func Summarize(records []Record) []Summary {
	byHash := make(map[string]*Summary)
	for _, r := range records {
		if r.Mode != "live" || !Known(r.Replay.Rules) {
			continue
		}
		hash := r.Replay.ChartHash
		s := byHash[hash]
		if s == nil {
			s = &Summary{Hash: hash}
			byHash[hash] = s
		}
		if r.Date.After(s.LastPlayed) {
			s.LastPlayed = r.Date
		}
		if r.Verification == "unavailable" || r.Verification == "legacy" || r.Version != RecordVersion || !r.Completed || r.Practice || r.Rate != 1 || r.Score.Possible == 0 {
			continue
		}
		if r.Replay.Mods.O2Jam != "" {
			continue // The event mod judges with other windows.
		}
		if s.BestPoints == nil || r.Score.Points > *s.BestPoints {
			points := r.Score.Points
			s.BestPoints = &points
		}
		if r.Replay.Mods.Kind != "" || r.Replay.Mods.Visibility != "" {
			continue
		}
		s.Completed = true
		s.FullCombo = s.FullCombo || r.Score.Miss == 0 && r.Score.MaxCombo == r.Score.Possible
		s.Perfect = s.Perfect || r.Score.Kool+r.Score.Cool == r.Score.Possible
		// Stages fall automatically, so one play clears the stage it ends on. Older gauge tables do not count.
		if h := r.Health; h != nil && h.Gauge == HealthGauge && !h.Failed && h.Value > 0 {
			s.Gauge = max(s.Gauge, h.Level)
		}
	}
	rows := make([]Summary, 0, len(byHash))
	for _, s := range byHash {
		rows = append(rows, *s)
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].Hash < rows[j].Hash })
	return rows
}
