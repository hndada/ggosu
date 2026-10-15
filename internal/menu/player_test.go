package menu

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"ggosu/internal/chart"
	"ggosu/internal/piano"
)

func TestPlayerNameAndKeysToday(t *testing.T) {
	a := &App{dataDir: filepath.Join(t.TempDir(), "records")}
	p, err := a.Player()
	if err != nil || p.Name != "Player" || p.PP != 0 || p.KeysToday != 0 {
		t.Fatalf("empty player: %+v, %v", p, err)
	}
	if name, err := a.SaveName("  고수  "); err != nil || name != "고수" {
		t.Fatalf("save name: %q, %v", name, err)
	}
	if _, err := a.SaveName(strings.Repeat("가", 33)); err == nil {
		t.Fatal("accepted a 33 character nickname")
	}
	c := chart.New(4)
	c.Timing = []chart.Timing{{BPM: 120, Scroll: 1, Meter: 4}}
	c.Notes = []chart.Note{{At: time.Second, End: time.Second, Kind: "tap"}, {At: 2 * time.Second, End: 2 * time.Second, Kind: "tap"}}
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
	now := time.Now()
	for _, r := range []piano.Record{
		{ID: "today", Date: now.UTC(), Mode: "live"},
		{ID: "yesterday", Date: now.AddDate(0, 0, -1).UTC(), Mode: "live"},
		{ID: "auto", Date: now.UTC(), Mode: "auto"},
	} {
		r.Version, r.Rate, r.Replay, r.Completed = piano.RecordVersion, 1, f, true
		r.Score, r.Accuracy = s.Score, s.Score.Accuracy()
		if err := piano.SaveRecord(filepath.Join(a.dataDir, r.ID+".json"), r); err != nil {
			t.Fatal(err)
		}
	}
	p, err = a.Player()
	if err != nil || p.Name != "고수" || p.KeysToday != 2 || p.PP <= 0 {
		t.Fatalf("player: %+v, %v", p, err)
	}
}
