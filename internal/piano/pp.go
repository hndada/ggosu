package piano

import (
	"math"
	"sort"
)

// PP is a placeholder until difficulty and accuracy corrections are designed.
// A play's pp is 10 × judgements per second at 1x × an accuracy factor that
// rises linearly from 0 at 80% to 1 at 100%. Only plays that count for
// Summary achievements count here.
func PP(r Record) (float64, bool) {
	if r.Verification == "unavailable" || r.Verification == "legacy" || r.Mode != "live" || r.Replay.Rules != Rules || r.Version != RecordVersion || !r.Completed || r.Practice || r.Rate != 1 || r.Replay.Mods.Kind != "" || r.Replay.Mods.Visibility != "" || r.Replay.Mods.O2Jam != "" || r.Score.Possible == 0 || r.Replay.Duration <= 0 {
		return 0, false
	}
	density := float64(r.Score.Possible) / r.Replay.Duration.Seconds()
	accuracy := max(0, min(1, (r.Score.Accuracy()-.8)/.2))
	return 10 * density * accuracy, true
}

// TotalPP weights each chart's best play like osu!: the n-th best (from 0) counts 0.95^n.
func TotalPP(records []Record) float64 {
	best := map[string]float64{}
	for _, r := range records {
		if pp, ok := PP(r); ok && pp > best[r.Replay.ChartHash] {
			best[r.Replay.ChartHash] = pp
		}
	}
	values := make([]float64, 0, len(best))
	for _, pp := range best {
		values = append(values, pp)
	}
	sort.Sort(sort.Reverse(sort.Float64Slice(values)))
	total := 0.0
	for i, pp := range values {
		total += pp * math.Pow(.95, float64(i))
	}
	return total
}
