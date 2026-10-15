package menu

import (
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"ggosu/internal/chart"
	"ggosu/internal/piano"
	"ggosu/internal/score"
	"ggosu/internal/settings"
)

// Result keeps the full replay on disk rather than sending it through the UI.
type Result struct {
	Submission   *score.Status    `json:"submission,omitempty"`
	Pauses       []piano.Pause    `json:"pauses,omitempty"`
	Health       *piano.Health    `json:"health,omitempty"`
	ID           string           `json:"id"`
	Date         string           `json:"date"`
	Title        string           `json:"title"`
	Difficulty   string           `json:"difficulty"`
	Lanes        int              `json:"lanes"`
	Mode         string           `json:"mode"`
	Completed    bool             `json:"completed"`
	Practice     bool             `json:"practice"`
	Rate         float64          `json:"rate"`
	Pitch        *bool            `json:"pitch,omitempty"`
	Score        piano.Score      `json:"score"`
	Accuracy     float64          `json:"accuracy"`
	Warnings     []string         `json:"warnings"`
	Offsets      settings.Offsets `json:"offsets"`
	Mods         chart.Mod        `json:"mods"`
	ScoreVersion int              `json:"scoreVersion"`
	HitError     *piano.HitError  `json:"hitError,omitempty"`
	Calibration  *Calibration     `json:"calibration,omitempty"` // Set only on the result that changed the offset.
	Verification string           `json:"verification"`
}

func result(r piano.Record) *Result {
	return &Result{Pauses: r.Replay.Pauses, Health: r.Health, ID: r.ID, Date: r.Date.UTC().Format("2006-01-02T15:04:05.000000000Z"), Title: r.Metadata.Title, Difficulty: r.Metadata.Difficulty, Lanes: r.Replay.Lanes, Mode: r.Mode, Completed: r.Completed, Practice: r.Practice, Rate: r.Rate, Pitch: r.Pitch, Score: r.Score, Accuracy: r.Accuracy, Warnings: r.Warnings, Offsets: r.Replay.Offsets, Mods: r.Replay.Mods, ScoreVersion: r.Version, HitError: r.HitError, Verification: r.Verification}
}

func (a *App) recordsDir() (string, error) {
	if a.dataDir != "" {
		return a.dataDir, nil
	}
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	return filepath.Join(filepath.Dir(exe), "data", "records"), nil
}

// lengthsPath sits beside settings.json, like the records directory.
func (a *App) lengthsPath() (string, error) {
	dir, err := a.recordsDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(filepath.Dir(dir), "audio-lengths.json"), nil
}

func (a *App) replayPath(id string) (string, error) {
	b, err := hex.DecodeString(id)
	if err != nil || len(b) != 16 {
		return "", fmt.Errorf("잘못된 기록 ID입니다")
	}
	dir, err := a.recordsDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, id+".json"), nil
}

func (a *App) History(s Selection) ([]Result, error) {
	e, err := a.songs.Lookup(s)
	if err != nil {
		return nil, err
	}
	charts, err := chart.Load(e.Path)
	if err != nil {
		return nil, err
	}
	if s.Index < 0 || s.Index >= len(charts) {
		return nil, fmt.Errorf("채보가 변경됐습니다")
	}
	hash, err := charts[s.Index].Digest()
	if err != nil {
		return nil, err
	}
	records, err := a.records()
	if err != nil {
		return nil, err
	}
	results := []Result{}
	for _, r := range records {
		if r.Replay.ChartHash == hash && piano.Known(r.Replay.Rules) {
			x := result(r)
			dir, err := a.submissionDir()
			if err != nil {
				return nil, err
			}
			j, err := readSubmission(filepath.Join(dir, r.ID+".json"))
			if err == nil {
				x.Submission = &j.Status
			} else if !os.IsNotExist(err) {
				return nil, err
			}
			results = append(results, *x)
		}
	}
	sort.Slice(results, func(i, j int) bool { return results[i].Date > results[j].Date })
	return results, nil
}

// Played lists chart digests that have at least one saved live play under the current piano.
func (a *App) Played() ([]string, error) {
	rows, err := a.Progress()
	if err != nil {
		return nil, err
	}
	hashes := make([]string, 0, len(rows))
	for _, r := range rows {
		hashes = append(hashes, r.Hash)
	}
	return hashes, nil
}

func (a *App) Progress() ([]piano.Summary, error) {
	records, err := a.records()
	if err != nil {
		return nil, err
	}
	return piano.Summarize(records), nil
}

func (a *App) records() ([]piano.Record, error) {
	a.dataMu.Lock()
	defer a.dataMu.Unlock()
	dir, err := a.recordsDir()
	if err != nil {
		return nil, err
	}
	files, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var records []piano.Record
	charts := map[string]*chart.Chart{}
	for _, f := range files {
		if f.IsDir() || filepath.Ext(f.Name()) != ".json" {
			continue
		}
		r, err := a.inspectRecord(filepath.Join(dir, f.Name()), charts)
		if err != nil {
			if errors.Is(err, piano.ErrRecord) {
				continue
			}
			return nil, fmt.Errorf("기록 %s 읽기: %w", f.Name(), err)
		}
		records = append(records, r)
	}
	return records, nil
}
