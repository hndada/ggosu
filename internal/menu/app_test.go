package menu

import (
	"fmt"
	"math"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"ggosu/internal/chart"
	"ggosu/internal/piano"
)

func TestMain(m *testing.M) {
	if err := os.Chdir("../.."); err != nil {
		fmt.Fprintln(os.Stderr, "menu test fixtures:", err)
		os.Exit(1)
	}
	os.Exit(m.Run())
}

func TestPlayedWithoutRecords(t *testing.T) {
	a := &App{dataDir: t.TempDir()}
	hashes, err := a.Played()
	if err != nil || len(hashes) != 0 {
		t.Fatalf("played: %v, %v", hashes, err)
	}
}

func TestProgressFromSavedRecord(t *testing.T) {
	a := &App{dataDir: t.TempDir()}
	rows, err := a.Progress()
	if err != nil || rows == nil || len(rows) != 0 {
		t.Fatalf("empty progress: %v, %v", rows, err)
	}
	c := chart.New(4)
	c.Timing = []chart.Timing{{BPM: 120, Scroll: 1, Meter: 4}}
	c.Notes = []chart.Note{{At: time.Second, End: time.Second, Kind: "tap"}}
	registerChart(t, a, c)
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
	if err := s.Advance(f.Duration); err != nil {
		t.Fatal(err)
	}
	r := piano.Record{Version: piano.RecordVersion, ID: "summary", Date: time.Now().UTC(), Mode: "live", Completed: true, Rate: 1, Score: s.Score, Accuracy: s.Score.Accuracy(), Replay: f}
	if err := piano.SaveRecord(filepath.Join(a.dataDir, "summary.json"), r); err != nil {
		t.Fatal(err)
	}
	rows, err = a.Progress()
	if err != nil || len(rows) != 1 || rows[0].Hash != f.ChartHash || !rows[0].Perfect || rows[0].BestPoints == nil || *rows[0].BestPoints != piano.MaxScore {
		t.Fatalf("saved progress: %+v, %v", rows, err)
	}
}

func TestProgressFromSavedLaneMods(t *testing.T) {
	for _, kind := range []string{"mirror", "random"} {
		t.Run(kind, func(t *testing.T) {
			a := &App{dataDir: t.TempDir()}
			c := chart.New(4)
			c.Timing = []chart.Timing{{BPM: 120, Scroll: 1, Meter: 4}}
			c.Notes = []chart.Note{{At: time.Second, End: time.Second, Kind: "tap"}}
			registerChart(t, a, c)
			m, err := chart.NewMod(c, kind, 42)
			if err != nil {
				t.Fatal(err)
			}
			modified, err := m.Apply(c)
			if err != nil {
				t.Fatal(err)
			}
			f, err := piano.Auto(modified, piano.Rules)
			if err != nil {
				t.Fatal(err)
			}
			f.ChartHash, err = c.Digest()
			if err != nil {
				t.Fatal(err)
			}
			f.Mods = m
			score, hit, err := piano.ReplayScore(c, f)
			if err != nil {
				t.Fatal(err)
			}
			r := piano.Record{Version: piano.RecordVersion, ID: kind, Date: time.Now().UTC(), Mode: "live", Completed: true, Rate: 1, Score: score, Accuracy: score.Accuracy(), Replay: f, HitError: hit, Metadata: c.Metadata}
			if err := piano.SaveRecord(filepath.Join(a.dataDir, kind+".json"), r); err != nil {
				t.Fatal(err)
			}
			for range 2 {
				rows, err := a.Progress()
				if err != nil || len(rows) != 1 || rows[0].Hash != f.ChartHash || rows[0].BestPoints == nil || *rows[0].BestPoints != piano.MaxScore {
					t.Fatalf("saved %s progress: %+v, %v", kind, rows, err)
				}
			}
		})
	}
}

func TestDistributionAndInvalidPlaybackOptions(t *testing.T) {
	a := &App{}
	c, err := a.Catalog()
	if err != nil || len(c.Entries) == 0 {
		t.Fatal(err)
	}
	e := c.Entries[0]
	s := Selection{ChartID: e.Path, Folder: e.Folder, Index: e.Index}
	r, err := a.Distribution(s)
	if err != nil || r.Total == 0 || len(r.Lanes) != e.Lanes || len(r.Bins) > 600 {
		t.Fatalf("distribution: %+v, %v", r, err)
	}
	for _, rate := range []float64{.25, .49, 2.01, 4, math.NaN(), math.Inf(1)} {
		s.Rate = rate
		if _, err := a.Play(s); err == nil {
			t.Fatalf("accepted rate %v", rate)
		}
	}
	s.Rate, s.Mod = 1, "invalid"
	if _, err := a.Play(s); err == nil {
		t.Fatal("accepted unsupported mod")
	}
	if a.State().Running {
		t.Fatal("invalid options launched a game")
	}
}

func TestPreviewBackgroundServesOnlyImages(t *testing.T) {
	a := &App{}
	c, err := a.Catalog()
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range c.Entries {
		if e.Background == "" {
			continue
		}
		q := url.Values{"chart": {e.Path}, "folder": {e.Folder}, "index": {fmt.Sprint(e.Index)}, "id": {e.Background}}
		w := httptest.NewRecorder()
		a.serveAudio(w, httptest.NewRequest(http.MethodGet, "/preview/background?"+q.Encode(), nil))
		if w.Code != http.StatusOK || !strings.HasPrefix(w.Header().Get("Content-Type"), "image/") {
			t.Fatalf("%s: background %d %q", e.Path, w.Code, w.Header().Get("Content-Type"))
		}
		q.Set("id", "missing")
		w = httptest.NewRecorder()
		a.serveAudio(w, httptest.NewRequest(http.MethodGet, "/preview/background?"+q.Encode(), nil))
		if w.Code != http.StatusNotFound {
			t.Fatalf("missing asset served: %d", w.Code)
		}
		return
	}
	t.Fatal("no demo chart has a background")
}
