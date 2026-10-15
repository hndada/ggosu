// Package scores verifies submitted replays and stores accepted plays on disk.
package scores

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"ggosu/internal/chart"
	"ggosu/internal/score"
)

type Store struct {
	dir    string
	lookup func(string) (*chart.Chart, bool)
	mu     sync.Mutex
}

type entry struct {
	Request score.Request `json:"play"`
	Receipt score.Receipt `json:"receipt"`
}

func New(dir string, lookup func(string) (*chart.Chart, bool)) (*Store, error) {
	if dir == "" || lookup == nil {
		return nil, fmt.Errorf("scores: storage directory and chart lookup required")
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(abs, 0755); err != nil {
		return nil, fmt.Errorf("scores: create storage: %w", err)
	}
	return &Store{dir: abs, lookup: lookup}, nil
}

func (s *Store) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		http.Error(w, "POST required", http.StatusMethodNotAllowed)
		return
	}
	if r.Header.Get("X-Gosu-Score") != "1" || r.Header.Get("Sec-Fetch-Site") != "" && r.Header.Get("Sec-Fetch-Site") != "same-origin" {
		http.Error(w, "score submission request required", http.StatusForbidden)
		return
	}
	body := http.MaxBytesReader(w, r.Body, 32<<20)
	d := json.NewDecoder(body)
	d.DisallowUnknownFields()
	var q score.Request
	if err := d.Decode(&q); err != nil {
		badBody(w, err)
		return
	}
	var extra any
	if err := d.Decode(&extra); err != io.EOF {
		if err == nil {
			err = fmt.Errorf("expected one score submission")
		}
		badBody(w, err)
		return
	}
	if len(q.Record.ID) == 0 || len(q.Record.ID) > 64 || !score.ValidPlayer(q.Player) || !score.Eligible(q.Record) || q.Record.Rate < .5 || q.Record.Rate > 2 || q.Record.Validate() != nil {
		http.Error(w, "retained current-rule live play required", http.StatusUnprocessableEntity)
		return
	}
	c, ok := s.lookup(q.Record.Replay.ChartHash)
	if !ok {
		http.Error(w, "chart is not in the server catalogue", http.StatusNotFound)
		return
	}
	verified, changed, err := q.Record.Recalculate(c)
	if err != nil || changed {
		http.Error(w, "submitted result does not match its replay", http.StatusUnprocessableEntity)
		return
	}
	// Keep only canonical result data. Client warnings and verification labels
	// must not alter duplicate detection or claim server trust.
	verified.Warnings = nil
	verified.Replay.Player = q.Player
	verified.Date = verified.Date.UTC()
	q.Record = verified
	if r.Context().Err() != nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	key := sha256.Sum256([]byte(q.Record.ID))
	path := filepath.Join(s.dir, hex.EncodeToString(key[:])+".json")
	var old entry
	b, err := os.ReadFile(path)
	if err == nil {
		if err := json.Unmarshal(b, &old); err != nil {
			http.Error(w, "cannot read stored score", http.StatusInternalServerError)
			return
		}
		before, e1 := json.Marshal(old.Request)
		after, e2 := json.Marshal(q)
		if e1 != nil || e2 != nil || string(before) != string(after) {
			http.Error(w, "record ID already contains another play", http.StatusConflict)
			return
		}
		respond(w, http.StatusOK, old.Receipt)
		return
	}
	if !os.IsNotExist(err) {
		http.Error(w, "cannot read score storage", http.StatusInternalServerError)
		return
	}
	x := entry{Request: q, Receipt: score.Receipt{ID: q.Record.ID, Received: time.Now().UTC(), Verified: true}}
	if err := save(path, x); err != nil {
		http.Error(w, "cannot save score", http.StatusInternalServerError)
		return
	}
	respond(w, http.StatusCreated, x.Receipt)
}

func badBody(w http.ResponseWriter, err error) {
	var large *http.MaxBytesError
	if errors.As(err, &large) {
		http.Error(w, "submission exceeds 32 MiB", http.StatusRequestEntityTooLarge)
		return
	}
	http.Error(w, "invalid score submission JSON", http.StatusBadRequest)
}

func respond(w http.ResponseWriter, code int, receipt score.Receipt) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(code)
	if err := json.NewEncoder(w).Encode(receipt); err != nil {
		return
	}
}

func save(path string, x entry) (err error) {
	f, err := os.CreateTemp(filepath.Dir(path), ".score-*")
	if err != nil {
		return err
	}
	defer func() {
		if e := os.Remove(f.Name()); e != nil && !os.IsNotExist(e) {
			err = errors.Join(err, e)
		}
	}()
	err = json.NewEncoder(f).Encode(x)
	if err == nil {
		err = f.Sync()
	}
	if err = errors.Join(err, f.Close()); err != nil {
		return err
	}
	return os.Rename(f.Name(), path)
}
