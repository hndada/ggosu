package scores

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"ggosu/internal/chart"
	"ggosu/internal/piano"
	"ggosu/internal/score"
)

func testPlay(t *testing.T) (*chart.Chart, score.Request) {
	t.Helper()
	c := chart.New(4)
	c.Metadata.Title = "Silent submission fixture"
	c.Timing = []chart.Timing{{BPM: 120, Scroll: 1, Meter: 4}}
	c.Notes = []chart.Note{{At: time.Second, End: time.Second, Kind: "tap"}, {At: 2 * time.Second, End: 3 * time.Second, Lane: 1, Kind: "hold"}}
	f, err := piano.Auto(c, piano.Rules)
	if err != nil {
		t.Fatal(err)
	}
	f.Mods.Visibility = "hidden"
	s, h, err := piano.ReplayScore(c, f)
	if err != nil {
		t.Fatal(err)
	}
	r := piano.Record{Version: piano.RecordVersion, ID: "0123456789abcdef0123456789abcdef", Date: time.Now().UTC(), Metadata: c.Metadata, Mode: "live", Completed: true, Rate: 1, Replay: f, Score: s, Accuracy: s.Accuracy(), HitError: h}
	return c, score.Request{Player: "고수", Record: r}
}

func testStore(t *testing.T, dir string, c *chart.Chart) *Store {
	t.Helper()
	hash, err := c.Digest()
	if err != nil {
		t.Fatal(err)
	}
	s, err := New(dir, func(id string) (*chart.Chart, bool) { return c, id == hash })
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func post(t *testing.T, s *Store, q score.Request) *httptest.ResponseRecorder {
	t.Helper()
	b, err := json.Marshal(q)
	if err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRequest("POST", "/api/scores", bytes.NewReader(b))
	r.Header.Set("X-Gosu-Score", "1")
	w := httptest.NewRecorder()
	s.ServeHTTP(w, r)
	return w
}

func TestSubmissionPersistsAndDeduplicatesAcrossRestart(t *testing.T) {
	c, q := testPlay(t)
	dir := t.TempDir()
	s := testStore(t, dir, c)
	w := post(t, s, q)
	if w.Code != 201 {
		t.Fatal(w.Code, w.Body.String())
	}
	var first score.Receipt
	if err := json.Unmarshal(w.Body.Bytes(), &first); err != nil {
		t.Fatal(err)
	}
	if first.ID != q.Record.ID || !first.Verified || first.Received.IsZero() {
		t.Fatal(first)
	}
	s = testStore(t, dir, c)
	server := httptest.NewServer(s)
	defer server.Close()
	second, err := score.Submit(context.Background(), server.URL, q)
	if err != nil || second != first {
		t.Fatal("duplicate changed the receipt", second, err)
	}
	files, err := os.ReadDir(dir)
	if err != nil || len(files) != 1 {
		t.Fatal("duplicate file", files, err)
	}
	b, err := os.ReadFile(dir + string(os.PathSeparator) + files[0].Name())
	if err != nil {
		t.Fatal(err)
	}
	var stored entry
	if err := json.Unmarshal(b, &stored); err != nil {
		t.Fatal(err)
	}
	if stored.Request.Player != q.Player || stored.Request.Record.Score.Points != 1100000 || stored.Request.Record.Replay.Mods.Visibility != "hidden" || stored.Request.Record.Verification != "replayed" {
		t.Fatal(stored)
	}
	q.Record.Date = q.Record.Date.Add(time.Second)
	if w := post(t, s, q); w.Code != 409 {
		t.Fatal("overwrote another play", w.Code, w.Body.String())
	}
}

func TestSubmissionRejectsForgeryUnsupportedModesAndUnknownCharts(t *testing.T) {
	c, good := testPlay(t)
	s := testStore(t, t.TempDir(), c)
	for _, mode := range []string{"auto", "replay"} {
		q := good
		q.Record.Mode = mode
		if w := post(t, s, q); w.Code != 422 {
			t.Fatal("accepted", mode, w.Code)
		}
	}
	q := good
	q.Record.Replay.Events = nil
	q.Record.Verification = "replayed"
	if w := post(t, s, q); w.Code != 422 {
		t.Fatal("trusted forged score", w.Code, w.Body.String())
	}
	q = good
	q.Record.Replay.ChartHash = strings.Repeat("f", 64)
	if w := post(t, s, q); w.Code != 404 {
		t.Fatal("accepted absent chart", w.Code)
	}
	q = good
	q.Player = "../\nadmin"
	if w := post(t, s, q); w.Code != 422 {
		t.Fatal("accepted invalid player", w.Code)
	}
	q = good
	q.Record.Replay.Mods.Visibility = "hidden+fadein"
	if w := post(t, s, q); w.Code != 422 {
		t.Fatal("accepted invalid visibility", w.Code)
	}
	files, err := os.ReadDir(s.dir)
	if err != nil || len(files) != 0 {
		t.Fatal("rejected score reached storage", files, err)
	}
}

func TestSubmissionKeepsRateModsAndHealthFailure(t *testing.T) {
	c, q := testPlay(t)
	s := testStore(t, t.TempDir(), c)
	q.Record.Rate, q.Record.Practice = 1.5, true
	if w := post(t, s, q); w.Code != 201 {
		t.Fatal("lost practice conditions", w.Code, w.Body.String())
	}
	q.Record.ID = "failed"
	q.Record.Replay.Events = nil
	q.Record.Replay.Health, q.Record.Replay.HealthFail, q.Record.Replay.HealthStages = 3, 3, true
	// Nine misses empty the hard gauge. The server retains a 0-point failure.
	c.Notes = nil
	for i := 0; i < 12; i++ {
		at := time.Duration(i+1) * time.Second
		c.Notes = append(c.Notes, chart.Note{At: at, End: at, Kind: "tap"})
	}
	q.Record.Replay.ChartHash, _ = c.Digest()
	q.Record.Replay.Duration = 15 * time.Second
	x, _, err := q.Record.Recalculate(c)
	if err != nil {
		t.Fatal(err)
	}
	q.Record = x
	s = testStore(t, t.TempDir(), c)
	if w := post(t, s, q); w.Code != 201 || !x.Health.Failed || x.Score.Points != 0 {
		t.Fatal(w.Code, w.Body.String(), x)
	}
}

func TestSubmissionBoundsAndCrossSiteGuard(t *testing.T) {
	c, q := testPlay(t)
	s := testStore(t, t.TempDir(), c)
	b, err := json.Marshal(q)
	if err != nil {
		t.Fatal(err)
	}
	for _, tt := range []struct {
		body, header, site string
		code               int
	}{
		{string(b), "", "", 403}, {string(b), "1", "cross-site", 403},
		{string(b) + "{}", "1", "", 400}, {"{\"unknown\":1}", "1", "", 400},
		{strings.Repeat(" ", 32<<20) + "{}", "1", "", 413},
	} {
		r := httptest.NewRequest("POST", "/api/scores", strings.NewReader(tt.body))
		r.Header.Set("X-Gosu-Score", tt.header)
		r.Header.Set("Sec-Fetch-Site", tt.site)
		w := httptest.NewRecorder()
		s.ServeHTTP(w, r)
		if w.Code != tt.code {
			t.Fatal("unexpected rejection", w.Code, tt.code)
		}
	}
}
