package osu_test

import (
	"bytes"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"ggosu/internal/chart"
	_ "ggosu/internal/chart/bms"
	"ggosu/internal/chart/osu"
)

func reparse(t *testing.T, data []byte) *chart.Chart {
	t.Helper()
	cs, err := chart.Decode("export.osu", bytes.NewReader(data))
	if err != nil {
		t.Fatalf("export does not parse: %v\n%s", err, data)
	}
	return cs[0]
}

func TestExportRoundTripsOsu(t *testing.T) {
	cs, err := chart.Load(filepath.Join("..", "..", "..", "songs", "demo", "demo.osu"))
	if err != nil {
		t.Fatal(err)
	}
	c := cs[0]
	data, dropped, err := osu.Export(c, &c.Source)
	if err != nil || len(dropped) != 0 {
		t.Fatal(err, dropped)
	}
	got := reparse(t, data)
	if !reflect.DeepEqual(got.Notes, c.Notes) || !reflect.DeepEqual(got.Timing, c.Timing) || got.Metadata != c.Metadata || got.Lanes != c.Lanes {
		t.Fatalf("round trip changed the chart\n%s", data)
	}
}

func TestExportConvertsWhatOsuCannotHold(t *testing.T) {
	c := chart.New(7)
	c.Metadata = chart.Metadata{Title: "T", Difficulty: "D"}
	c.Assets["bank"] = chart.Asset{Path: "song.ojm", Kind: "bank", Sample: 3}
	c.Assets["kick"] = chart.Asset{Path: "kick.wav", Kind: "audio"}
	c.Timing = []chart.Timing{{BPM: 120, Scroll: 1, Meter: 4}, {At: time.Second, BPM: 120, Scroll: 2, Meter: 4}, {At: 2 * time.Second, BPM: 120, Scroll: 2, Meter: 4, Stop: 250 * time.Millisecond}}
	c.Audio = []chart.Audio{{At: 500 * time.Millisecond, Sound: chart.Sound{Asset: "kick", Volume: .5}}, {At: 600 * time.Millisecond, Sound: chart.Sound{Asset: "bank", Volume: 1}}}
	c.Notes = []chart.Note{
		{Lane: 0, At: 1500 * time.Microsecond, End: 1500 * time.Microsecond, Kind: "tap", Sounds: []chart.Sound{{Asset: "kick", Volume: 1}}},
		{Lane: 6, At: time.Second, End: 3 * time.Second, Kind: "roll"},
		{Lane: 3, At: time.Second, End: time.Second, Kind: "mine"},
		{Lane: 2, At: 4 * time.Second, End: 4 * time.Second, Kind: "lift"},
	}
	data, dropped, err := osu.Export(c, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(dropped, []string{"keysound 1", "mine 1"}) {
		t.Fatal(dropped)
	}
	got := reparse(t, data)
	if len(got.Notes) != 3 || got.Notes[0].At != 2*time.Millisecond || got.Notes[0].Sounds[0].Asset != "kick.wav" || got.Notes[1].Lane != 6 || got.Notes[1].Kind != "hold" || got.Notes[2].Lane != 2 {
		t.Fatalf("notes %+v\n%s", got.Notes, data)
	}
	if len(got.Audio) != 1 || got.Audio[0].At != 500*time.Millisecond || got.Audio[0].Sound.Volume != .5 {
		t.Fatalf("samples %+v", got.Audio)
	}
	// The stop holds the field still, then the beat restarts after it with the scroll restored.
	last := got.Timing[len(got.Timing)-1]
	if last.At != 2250*time.Millisecond || last.Scroll != 2 || last.BPM != 120 {
		t.Fatalf("timing %+v\n%s", got.Timing, data)
	}
}

func TestSaveEditWritesNewOsuBesideSource(t *testing.T) {
	dir := t.TempDir()
	source := filepath.Join(dir, "song.bms")
	bms := "#PLAYER 1\n#TITLE Song\n#BPM 120\n#WAV01 kick.wav\n#00111:01010000\n#00101:01\n"
	if err := os.WriteFile(source, []byte(bms), 0o644); err != nil {
		t.Fatal(err)
	}
	cs, err := chart.Load(source)
	if err != nil {
		t.Fatal(err)
	}
	path, _, err := osu.SaveEdit(source, cs[0], cs[0])
	if err != nil || !strings.HasPrefix(filepath.Base(path), "song-edit-") || filepath.Ext(path) != ".osu" {
		t.Fatal(path, err)
	}
	got, err := chart.Load(path)
	if err != nil || len(got[0].Notes) != len(cs[0].Notes) || len(got[0].Audio) != len(cs[0].Audio) {
		t.Fatal("exported copy differs", err)
	}
}
