//go:build !js

package play

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"ggosu/internal/chart"
	"ggosu/internal/piano"
	"ggosu/internal/score"
)

func TestStandaloneSubmissionKeepsLocalResultWhenServerFails(t *testing.T) {
	c := chart.New(1)
	c.Timing = []chart.Timing{{BPM: 120, Scroll: 1, Meter: 4}}
	c.Notes = []chart.Note{{At: time.Second, End: time.Second, Kind: "tap"}}
	f, err := piano.Auto(c, piano.Rules)
	if err != nil {
		t.Fatal(err)
	}
	s, err := piano.NewSession(c)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range f.Events {
		if err := s.Input(e.At, int(e.Key), e.Down); err != nil {
			t.Fatal(err)
		}
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		var q score.Request
		if err := json.NewDecoder(req.Body).Decode(&q); err != nil {
			t.Error(err)
		}
		if q.Player != "tester" || q.Record.Score.Points != 1100000 {
			t.Error("submitted wrong play", q)
		}
		http.Error(w, "offline", 503)
	}))
	defer server.Close()
	g := scene{chart: c, session: s, recording: f, position: 2 * time.Second, initialRate: 1, mode: "live", completed: true, result: filepath.Join(t.TempDir(), "result.json"), scoreServer: server.URL, playerName: "tester"}
	if err := g.save(); err == nil {
		t.Fatal("submission failure was hidden")
	}
	r, err := piano.ReadRecord(g.result)
	if err != nil || r.Score.Points != 1100000 || !r.Completed {
		t.Fatal("submission failure lost the local score", r, err)
	}
}

func TestCanceledRunDoesNotStoreButHealthFailureDoes(t *testing.T) {
	c := chart.New(1)
	c.Timing = []chart.Timing{{BPM: 120, Scroll: 1, Meter: 4}}
	for i := 1; i <= 40; i++ {
		at := time.Duration(i) * time.Second
		c.Notes = append(c.Notes, chart.Note{At: at, End: at, Kind: "tap"})
	}
	for _, health := range []int{0, 3} {
		s, err := piano.NewSession(c)
		if err != nil {
			t.Fatal(err)
		}
		s.Health = piano.NewHealth(health, health)
		r, err := piano.NewChartReplay(c, piano.Rules)
		if err != nil {
			t.Fatal(err)
		}
		r.Health = health
		r.HealthStages, r.HealthFail, r.HealthGauge = true, health, s.Health.Gauge
		g := scene{chart: c, session: s, recording: r, position: 20 * time.Second, initialRate: 1, mode: "live", result: filepath.Join(t.TempDir(), "result.json")}
		if err := s.Advance(g.position); err != nil {
			t.Fatal(err)
		}
		if err := g.save(); err != nil {
			t.Fatal(err)
		}
		got, err := piano.ReadRecord(g.result)
		if health == 0 {
			if !os.IsNotExist(err) {
				t.Fatal("canceled play stored", err)
			}
		} else if err != nil || got.Completed || got.Health == nil || !got.Health.Failed || got.Score.Points != 0 || len(got.Warnings) != 0 {
			t.Fatal("zero-point health failure not stored", got, err)
		}
	}
}
