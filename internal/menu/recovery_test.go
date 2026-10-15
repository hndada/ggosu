package menu

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"ggosu/internal/chart"
	"ggosu/internal/piano"
)

func registerChart(t *testing.T, a *App, c *chart.Chart) {
	t.Helper()
	folder := t.TempDir()
	f, err := os.Create(filepath.Join(folder, "silent.ggc"))
	if err != nil {
		t.Fatal(err)
	}
	if err := chart.Write(f, c); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := a.songs.Scan(filepath.Join(t.TempDir(), "lengths.json"), []string{folder}); err != nil {
		t.Fatal(err)
	}
}

func TestRecordRepairAndIsolation(t *testing.T) {
	a := &App{dataDir: t.TempDir()}
	c := chart.New(4)
	c.Timing = []chart.Timing{{BPM: 120, Scroll: 1, Meter: 4}}
	c.Notes = []chart.Note{{At: time.Second, End: time.Second, Kind: "tap"}}
	registerChart(t, a, c)
	f, err := piano.Auto(c, piano.Rules)
	if err != nil {
		t.Fatal(err)
	}
	score, hit, err := piano.ReplayScore(c, f)
	if err != nil {
		t.Fatal(err)
	}
	r := piano.Record{Version: piano.RecordVersion, ID: "valid", Date: time.Now().UTC(), Mode: "live", Completed: true, Rate: 1, Score: score, Accuracy: score.Accuracy(), Replay: f, HitError: hit}
	if err := piano.SaveRecord(filepath.Join(a.dataDir, "valid.json"), r); err != nil {
		t.Fatal(err)
	}
	r.ID, r.Replay.Events = "forged", nil
	path := filepath.Join(a.dataDir, "forged.json")
	if err := piano.SaveRecord(path, r); err != nil {
		t.Fatal(err)
	}
	r.Score.Points++
	b, err := json.Marshal(r)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, b, 0600); err != nil {
		t.Fatal(err)
	}
	original, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(a.dataDir, "broken.json"), []byte("{broken"), 0600); err != nil {
		t.Fatal(err)
	}
	rows, err := a.records()
	if err != nil || len(rows) != 2 {
		t.Fatalf("records: %+v %v", rows, err)
	}
	for _, row := range rows {
		if row.ID == "forged" && (row.Score.Points != 0 || row.Verification != "repaired") {
			t.Fatalf("forgery escaped: %+v", row)
		}
	}
	kept, err := filepath.Glob(path + ".invalid-*")
	if err != nil || len(kept) != 1 {
		t.Fatal("original not preserved", err)
	}
	b, err = os.ReadFile(kept[0])
	if err != nil || !bytes.Equal(b, original) {
		t.Fatal("preserved bytes changed", err)
	}
	if len(a.State().Notices) != 2 {
		t.Fatal("missing recovery notices", a.State().Notices)
	}
	if _, err := a.records(); err != nil {
		t.Fatal("repaired list failed on reload", err)
	}
}

func TestUnavailableChartCannotEarnAchievements(t *testing.T) {
	a := &App{dataDir: t.TempDir()}
	c := chart.New(4)
	c.Timing = []chart.Timing{{BPM: 120, Scroll: 1, Meter: 4}}
	c.Notes = []chart.Note{{At: time.Second, End: time.Second, Kind: "tap"}}
	f, err := piano.Auto(c, piano.Rules)
	if err != nil {
		t.Fatal(err)
	}
	s, hit, err := piano.ReplayScore(c, f)
	if err != nil {
		t.Fatal(err)
	}
	r := piano.Record{Version: piano.RecordVersion, ID: "missing", Date: time.Now().UTC(), Mode: "live", Completed: true, Rate: 1, Score: s, Accuracy: s.Accuracy(), Replay: f, HitError: hit, Verification: "replayed"}
	if err := piano.SaveRecord(filepath.Join(a.dataDir, "missing.json"), r); err != nil {
		t.Fatal(err)
	}
	rows, err := a.Progress()
	if err != nil || len(rows) != 1 || rows[0].Perfect || rows[0].BestPoints != nil {
		t.Fatalf("unverified achievement: %+v %v", rows, err)
	}
}
