// Records store a key-mode result and its replay as one local file.
package piano

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"time"

	"ggosu/internal/chart"
)

const RecordVersion = 2

var ErrRecord = errors.New("invalid record")

// ErrRecordState means JSON decoding succeeded but the claimed fields failed
// validation. Derived fields may still be recovered from a valid replay.
var ErrRecordState = errors.New("invalid record state")

type Record struct {
	Health       *Health        `json:"health,omitempty"`
	Version      int            `json:"version"`
	ID           string         `json:"id"`
	Date         time.Time      `json:"date"`
	Metadata     chart.Metadata `json:"metadata"`
	Mode         string         `json:"mode"` // live, auto, replay
	Completed    bool           `json:"completed"`
	Practice     bool           `json:"practice"`        // Playback rate changed from 1x.
	Rate         float64        `json:"rate"`            // Initial playback rate.
	Pitch        *bool          `json:"pitch,omitempty"` // Missing in older records: true.
	Score        Score          `json:"score"`
	Accuracy     float64        `json:"accuracy"`
	Replay       Replay         `json:"replay"`
	Warnings     []string       `json:"warnings,omitempty"`
	HitError     *HitError      `json:"hitError,omitempty"`     // Missing in older records.
	Verification string         `json:"verification,omitempty"` // Recomputed at each trust boundary; never trusted from disk.
}

// HitError is the mean signed timing error of hit judgements; positive is late.
// Fast and Slow count Cool and Good hits before and after the note; Kool and
// exact hits are in neither.
// They are recomputed from the replay, so older files without them still verify.
type HitError struct {
	MeanMS float64 `json:"meanMS"`
	Count  int     `json:"count"`
	Fast   int     `json:"fast"`
	Slow   int     `json:"slow"`
}

func (r Record) PitchEnabled() bool { return r.Pitch == nil || *r.Pitch }

// Retained distinguishes scored completion/health failure from a canceled run.
func (r Record) Retained() bool {
	return r.Completed && r.Score.Points > 0 || r.Health != nil && r.Health.Failed
}

func (r Record) Validate() error {
	if h := r.Health; h != nil {
		if !h.valid(r.Replay) || h.At < -chart.MaxTime || h.At > r.Replay.Duration || h.Failed && r.Completed {
			return fmt.Errorf("record: invalid health result")
		}
	}
	// Keep records from the former .25..4 range readable in history.
	// Playback entry points enforce the current .5..2 range.
	if r.Version != 1 && r.Version != RecordVersion || r.ID == "" || r.Date.IsZero() || math.IsNaN(r.Rate) || math.IsInf(r.Rate, 0) || r.Rate < .25 || r.Rate > 4 {
		return fmt.Errorf("record: invalid metadata")
	}
	if r.Mode != "live" && r.Mode != "auto" && r.Mode != "replay" {
		return fmt.Errorf("record: invalid play mode")
	}
	s := r.Score
	if r.Version == 1 {
		// Keep old totals readable; input replays are scored anew by the game.
		accuracy := 1.0
		if s.Total > 0 {
			accuracy = float64(s.Points) / (float64(s.Total) * 1000)
		}
		if s.Kool < 0 || s.Cool < 0 || s.Good < 0 || s.Miss < 0 || s.Total < 0 ||
			int64(s.Total) != int64(s.Kool)+int64(s.Cool)+int64(s.Good)+int64(s.Miss) ||
			int64(s.Points) != int64(s.Kool)*1000+int64(s.Cool)*700+int64(s.Good)*300 ||
			s.Combo < 0 || s.MaxCombo < s.Combo || s.MaxCombo > s.Total || r.Accuracy != accuracy ||
			s.Possible != 0 || s.Flow != 0 || s.Precision != 0 || s.Earned != 0 {
			return fmt.Errorf("record: invalid legacy score")
		}
	} else {
		if err := s.Validate(); err != nil {
			return err
		}
		if r.Accuracy != s.Accuracy() {
			return fmt.Errorf("record: accuracy does not match judgements")
		}
		if r.Completed && s.Total != s.Possible {
			return fmt.Errorf("record: completed play has unjudged notes")
		}
	}
	return r.Replay.Validate()
}

func ReadRecord(path string) (Record, error) {
	f, err := os.Open(path)
	if err != nil {
		return Record{}, err
	}
	r, err := DecodeRecord(f)
	if closeErr := f.Close(); closeErr != nil {
		return Record{}, errors.Join(err, closeErr)
	}
	if err != nil {
		var pathErr *os.PathError
		if errors.As(err, &pathErr) {
			return Record{}, err
		}
		return r, fmt.Errorf("%w: %w", ErrRecord, err)
	}
	return r, nil
}

// Decode reads a result from a file, browser storage or imported JSON.
func DecodeRecord(reader io.Reader) (Record, error) {
	var r Record
	d := json.NewDecoder(io.LimitReader(reader, 128<<20))
	d.DisallowUnknownFields()
	err := d.Decode(&r)
	if err == nil {
		var extra any
		if d.Decode(&extra) != io.EOF {
			err = fmt.Errorf("record: expected one result")
		}
	}
	if err == nil {
		if e := r.Validate(); e != nil {
			err = fmt.Errorf("%w: %w", ErrRecordState, e)
		}
	}
	return r, err
}

// Save publishes the result only after the whole replay has been written.
func SaveRecord(path string, r Record) (err error) {
	if err := r.Validate(); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".record-*")
	if err != nil {
		return err
	}
	defer func() {
		if e := os.Remove(f.Name()); e != nil && !os.IsNotExist(e) {
			err = errors.Join(err, e)
		}
	}()
	writeErr := json.NewEncoder(f).Encode(r)
	if writeErr == nil {
		writeErr = f.Sync()
	}
	if err := errors.Join(writeErr, f.Close()); err != nil {
		return err
	}
	if err := os.Rename(f.Name(), path); err != nil {
		return fmt.Errorf("save result: %w", err)
	}
	return nil
}
