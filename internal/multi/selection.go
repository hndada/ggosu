package multi

import (
	"time"

	"ggosu/internal/chart"
	"ggosu/internal/piano"
)

// Describe shares the exact lead-in and reporting end between lobby and game.
// audioDuration is Chart.AudioDuration, on the original chart timeline.
func Describe(c *chart.Chart, mods chart.Mod, rate float64, audioDuration time.Duration) (Selection, error) {
	hash, err := c.Digest()
	if err != nil {
		return Selection{}, err
	}
	begin := -max(2*time.Second, c.Metadata.LeadIn)
	for _, n := range c.Notes {
		begin = min(begin, n.At-3*time.Second)
	}
	for _, a := range c.Audio {
		begin = min(begin, a.At-time.Second)
	}
	end := max(c.Duration()+2*time.Second+piano.Window+time.Millisecond, audioDuration+time.Second)
	session, err := piano.NewSession(c)
	if err != nil {
		return Selection{}, err
	}
	s := Selection{Title: c.Metadata.Title, ChartHash: hash, Lanes: c.Lanes, Rules: piano.Rules,
		ScoreVersion: piano.RecordVersion, BeginNS: int64(begin), DurationNS: int64(end),
		Judgements: session.Score.Possible, Rate: rate, Pitch: true, Mods: mods}
	return s, s.Validate()
}
