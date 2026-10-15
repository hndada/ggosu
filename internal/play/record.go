package play

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/hajimehoshi/ebiten/v2"

	"ggosu/internal/piano"
	"ggosu/internal/score"
)

func (g *scene) endRecording() error {
	g.recording.Duration = max(0, g.position)
	if g.paused {
		now := g.startAt
		if n := len(g.recording.Pauses); n > 0 {
			now += g.recording.Pauses[n-1].Elapsed
		}
		if g.clock != nil {
			now = g.clock.Now()
		}
		g.logPause(now, "end", "exit")
	}
	return g.release(g.recording.Duration)
}

func (g *scene) resultRecord(id string) (piano.Record, error) {
	r := piano.Record{Version: piano.RecordVersion, ID: id, Date: time.Now().UTC(), Metadata: g.chart.Metadata, Mode: g.mode, Completed: g.completed, Practice: g.practice, Rate: g.initialRate, Pitch: &g.pitch, Score: g.session.Score, Accuracy: g.session.Score.Accuracy(), Replay: g.recording, Warnings: g.warnings, HitError: g.session.HitError()}
	if g.recording.Health > 0 {
		h := g.session.Health
		r.Health = &h
	}
	r, _, err := r.Recalculate(g.chart)
	return r, err
}

func (g *scene) save() error {
	var submitErr error
	if g.record != "" || g.result != "" {
		if err := g.endRecording(); err != nil {
			return err
		}
		if g.record != "" {
			if err := writeFile(g.record, func(f *os.File) error { return piano.WriteReplay(f, g.recording) }); err != nil {
				return err
			}
		}
	}
	if g.result != "" && (g.completed || g.session.Health.Failed) {
		r, err := g.resultRecord(strings.TrimSuffix(filepath.Base(g.result), filepath.Ext(g.result)))
		if err != nil {
			return err
		}
		if r.Retained() {
			g.session.Score, g.warnings = r.Score, r.Warnings
			if err := piano.SaveRecord(g.result, r); err != nil {
				return err
			}
			if g.scoreServer != "" && score.Eligible(r) {
				if _, err := score.Submit(context.Background(), g.scoreServer, score.Request{Player: g.playerName, Record: r}); err != nil {
					// Local output was already saved. A standalone caller can resubmit it.
					submitErr = err
				}
			}
		}
	}
	if g.report != "" {
		return errors.Join(submitErr, writeFile(g.report, func(f *os.File) error {
			return json.NewEncoder(f).Encode(struct {
				Score     piano.Score `json:"score"`
				Accuracy  float64     `json:"accuracy"`
				Rules     string      `json:"rules"`
				Warnings  []string    `json:"warnings"`
				TargetTPS int         `json:"target_tps"`
				ActualTPS float64     `json:"actual_tps"`
				ActualFPS float64     `json:"actual_fps"`
			}{g.session.Score, g.session.Score.Accuracy(), piano.Rules, g.warnings, g.display.TickRate(), ebiten.ActualTPS(), ebiten.ActualFPS()})
		}))
	}
	return submitErr
}

func writeFile(path string, write func(*os.File) error) (err error) {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".gosu-*")
	if err != nil {
		return err
	}
	temp := f.Name()
	defer func() {
		if e := os.Remove(temp); e != nil && !os.IsNotExist(e) {
			err = errors.Join(err, e)
		}
	}()
	writeErr := write(f)
	closeErr := f.Close()
	if writeErr != nil {
		return writeErr
	}
	if closeErr != nil {
		return closeErr
	}
	return os.Rename(temp, path)
}
