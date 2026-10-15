package menu

import (
	"fmt"
	"math"
	"os"
	"path/filepath"

	"ggosu/internal/chart"
)

// MedleyRequest asks for songs of one key count within a calculated level range.
type MedleyRequest struct {
	Lanes int     `json:"lanes"`
	Low   float64 `json:"low"`
	High  float64 `json:"high"`
}

// MedleyPick draws up to three songs of a medley mode run.
func (a *App) MedleyPick(r MedleyRequest) ([]chart.Entry, error) {
	if r.Lanes < 1 || r.Lanes > 18 || math.IsNaN(r.Low) || math.IsNaN(r.High) || r.Low > r.High {
		return nil, fmt.Errorf("키 수나 레벨 범위가 올바르지 않습니다")
	}
	return a.songs.Medley(r.Lanes, r.Low, r.High, 3)
}

// medleyChart joins the selected charts and writes the medley as cache/medley.ggc,
// whose clips refer back to each song's own folder.
func (a *App) medleyChart(parts []Selection) (*chart.Chart, string, error) {
	if len(parts) < 1 || len(parts) > 3 {
		return nil, "", fmt.Errorf("메들리는 1~3곡입니다")
	}
	charts := make([]*chart.Chart, len(parts))
	for i, s := range parts {
		e, err := a.songs.Lookup(s)
		if err != nil {
			return nil, "", err
		}
		cs, err := chart.Load(e.Path)
		if err != nil {
			return nil, "", err
		}
		if s.Index < 0 || s.Index >= len(cs) {
			return nil, "", fmt.Errorf("난이도가 변경됐습니다. 목록을 새로 읽어 주세요")
		}
		c := *cs[s.Index]
		if c.Source.Path, err = filepath.Abs(e.Path); err != nil {
			return nil, "", err
		}
		charts[i] = &c
	}
	plan, err := chart.PlanMedley(charts)
	if err != nil {
		return nil, "", err
	}
	m, err := chart.Medley(plan)
	if err != nil {
		return nil, "", err
	}
	settingsPath, _, err := a.settingsKey(Selection{})
	if err != nil {
		return nil, "", err
	}
	path := filepath.Join(filepath.Dir(settingsPath), "cache", "medley.ggc")
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return nil, "", err
	}
	m.Source.Path = path
	f, err := os.Create(path)
	if err != nil {
		return nil, "", err
	}
	writeErr := chart.Write(f, m)
	if closeErr := f.Close(); writeErr == nil {
		writeErr = closeErr
	}
	if writeErr != nil {
		return nil, "", fmt.Errorf("메들리 저장: %w", writeErr)
	}
	return m, path, nil
}
