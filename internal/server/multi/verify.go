package multi

import (
	"bytes"
	"fmt"
	"slices"
	"time"

	"ggosu/internal/chart"
	"ggosu/internal/multi"
	"ggosu/internal/piano"
)

func (s *Server) loadCharts(charts []*chart.Chart) error {
	if charts != nil && len(charts) == 0 {
		return fmt.Errorf("multi: empty verification catalogue")
	}
	s.charts = map[string]*chart.Chart{}
	s.selections = map[string]multi.Selection{}
	for _, c := range charts {
		if c == nil {
			return fmt.Errorf("multi: nil canonical chart")
		}
		// Freeze nested slices and maps; callers cannot change a match's chart.
		var b bytes.Buffer
		if err := chart.Write(&b, c); err != nil {
			return err
		}
		copy, err := chart.Read(&b)
		if err != nil {
			return err
		}
		v, err := multi.Describe(copy, chart.Mod{}, 1, copy.AudioDuration(nil))
		if err != nil {
			return err
		}
		if v.Judgements > 100_000 {
			return fmt.Errorf("multi: verified charts support at most 100000 judgements")
		}
		s.charts[v.ChartHash], s.selections[v.ChartHash] = copy, v
	}
	return nil
}

func (s *Server) checkSelection(v multi.Selection) error {
	if len(s.charts) == 0 {
		return nil
	}
	expected, ok := s.selections[v.ChartHash]
	if !ok {
		return fmt.Errorf("chart is not in the server catalogue")
	}
	if v.Lanes != expected.Lanes || v.Judgements != expected.Judgements || v.BeginNS != expected.BeginNS || v.DurationNS != expected.DurationNS {
		return fmt.Errorf("selection does not match the server chart")
	}
	_, err := v.Mods.Apply(s.charts[v.ChartHash])
	return err
}

// verifyFinish snapshots match conditions, then replays outside the room lock.
// dispatch checks the active match again before committing any result.
func (s *Server) verifyFinish(c *client, q multi.Request) (*piano.Score, error) {
	s.mu.Lock()
	r := s.rooms[c.room]
	if r == nil || r.Selection == nil || r.Phase != "playing" || r.MatchID != q.MatchID || r.members[c.id] == nil || r.members[c.id].Status != "playing" || q.Sequence <= r.members[c.id].Sequence || int64(s.tick()) < r.StartNS || int64(s.tick()) >= r.EndNS {
		s.mu.Unlock()
		return nil, fmt.Errorf("not an active match")
	}
	v, offset := *r.Selection, r.members[c.id].OffsetNS
	canonical := s.charts[v.ChartHash]
	s.mu.Unlock()
	f := q.Replay
	if f == nil || len(f.Events) > 400_000 || f.ChartHash != v.ChartHash || f.Duration != time.Duration(v.DurationNS) || int64(f.Offsets.Total()) != offset || f.Mods.Kind != v.Mods.Kind || f.Mods.Visibility != v.Mods.Visibility || f.Mods.Seed != v.Mods.Seed || !slices.Equal(f.Mods.Mapping, v.Mods.Mapping) {
		return nil, fmt.Errorf("finish replay does not match the reserved play conditions")
	}
	score, _, err := piano.ReplayScore(canonical, *f)
	if err != nil {
		return nil, fmt.Errorf("verify finish: %w", err)
	}
	if q.Score == nil || score != *q.Score || score.Total != score.Possible {
		return nil, fmt.Errorf("finish score does not match the replay; previous state retained")
	}
	return &score, nil
}
