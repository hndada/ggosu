package piano

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"slices"

	"ggosu/internal/chart"
)

// ReplayScore runs the same judgement code as gameplay, on the original 1x
// input timeline. It proves consistency with these inputs, not their origin.
func ReplayScore(c *chart.Chart, r Replay) (Score, *HitError, error) {
	s, err := replaySession(c, r)
	if err != nil {
		return Score{}, nil, err
	}
	return s.Score, s.HitError(), nil
}

func replaySession(c *chart.Chart, r Replay) (*Session, error) {
	if !Known(r.Rules) {
		return nil, fmt.Errorf("unknown judgement rules %q", r.Rules)
	}
	if err := r.Match(c, Rules); err != nil {
		return nil, err
	}
	c, err := r.Offsets.Apply(c)
	if err != nil {
		return nil, err
	}
	c, err = r.Mods.Apply(c)
	if err != nil {
		return nil, err
	}
	s, err := NewSession(c)
	if err != nil {
		return nil, err
	}
	s.Rules = r.Rules
	s.Health = replayHealth(r.Health, r.HealthFail, r.HealthStages, r.HealthGauge)
	s.SetO2Jam(r.Mods)
	for _, e := range r.Events {
		if err := s.Input(e.At, int(e.Key), e.Down); err != nil {
			return nil, err
		}
		// Verification never plays or accumulates audio.
		s.Sounds, s.Music = nil, nil
	}
	if err := s.Advance(r.Duration); err != nil {
		return nil, err
	}
	return s, nil
}

// Recalculate replaces derived fields using a canonical chart. Legacy totals
// remain readable and are not silently migrated to a different scoring rule.
func (r Record) Recalculate(c *chart.Chart) (Record, bool, error) {
	if r.Version != RecordVersion {
		if err := r.Validate(); err != nil {
			return Record{}, false, err
		}
		if err := r.Replay.Match(c, Rules); err != nil {
			return Record{}, false, err
		}
		r.Verification = "legacy"
		return r, false, nil
	}
	session, err := replaySession(c, r.Replay)
	if err != nil {
		return Record{}, false, err
	}
	score, hit := session.Score, session.HitError()
	x := r
	x.Health = nil
	if r.Replay.Health > 0 {
		h := session.Health
		x.Health = &h
	}
	if session.Health.Failed {
		x.Completed = false
	}
	x.Score, x.Accuracy, x.HitError, x.Metadata = score, score.Accuracy(), hit, c.Metadata
	if score.Total != score.Possible {
		x.Completed = false
	}
	x.Practice = r.Practice || r.Rate != 1
	if err := x.Validate(); err != nil {
		return Record{}, false, err
	}
	changed := !reflect.DeepEqual(r.Health, x.Health) || r.Score != x.Score || r.Accuracy != x.Accuracy || r.Completed != x.Completed || r.Practice != x.Practice || !sameHitError(r.HitError, x.HitError) || r.Metadata != x.Metadata
	x.Verification = "replayed"
	if r.Replay.Rules != Rules {
		x.Verification = "legacy"
	}
	if changed {
		x.Verification = "repaired"
		x.Warnings = append(slices.Clone(r.Warnings), "저장 결과와 리플레이 계산이 달라 결과를 복구했습니다.")
	}
	return x, changed, nil
}

// sameHitError ignores Fast and Slow: files saved before they existed lack them.
func sameHitError(a, b *HitError) bool {
	if a == nil || b == nil {
		return a == b
	}
	return a.MeanMS == b.MeanMS && a.Count == b.Count
}

// PreserveRecord moves an invalid file aside. The extension stops it from
// entering the record list again. The original bytes are kept for inspection.
func PreserveRecord(path string) (string, error) {
	f, err := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+".invalid-*")
	if err != nil {
		return "", err
	}
	if err := f.Close(); err != nil {
		return "", err
	}
	if err := os.Rename(path, f.Name()); err != nil {
		if rmErr := os.Remove(f.Name()); rmErr != nil {
			return "", fmt.Errorf("preserve record: %w; remove temporary file: %v", err, rmErr)
		}
		return "", err
	}
	return f.Name(), nil
}
