package chart_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"ggosu/internal/chart"
)

func TestSaveCopy(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "original.ggc")
	c := chart.New(4)
	c.Metadata.Title = "Test"
	c.Timing = []chart.Timing{{BPM: 120, Scroll: 1, Meter: 4}}
	c.Notes = []chart.Note{{Lane: 1, At: time.Second, End: time.Second, Kind: "tap"}}
	c.Assets["music"] = chart.Asset{Path: "music.wav", Kind: "audio"}
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := chart.Write(f, c); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	draft := *c
	draft.Notes = []chart.Note{{Lane: 2, At: 2 * time.Second, End: 3 * time.Second, Kind: "hold"}}
	draft.Assets = map[string]chart.Asset{"escape": {Path: "../private.wav", Kind: "audio"}}
	saved, err := chart.SaveEdit(path, c, &draft)
	if err != nil {
		t.Fatal(err)
	}
	loaded, err := chart.Load(saved)
	if err != nil {
		t.Fatal(err)
	}
	if loaded[0].Notes[0].End != 3*time.Second || loaded[0].Assets["music"].Path != "music.wav" || len(loaded[0].Assets) != 1 {
		t.Fatal("copy lost edits or trusted draft assets")
	}
	after, err := os.ReadFile(path)
	if err != nil || string(before) != string(after) {
		t.Fatal("source changed", err)
	}
	second, err := chart.SaveEdit(path, c, &draft)
	if err != nil || saved == second {
		t.Fatal("saved copy overwritten", err)
	}
	draft.Notes = nil
	empty, err := chart.SaveEdit(path, c, &draft)
	if err != nil {
		t.Fatal(err)
	}
	catalog, err := chart.Scan(dir, nil)
	if err != nil || len(catalog.Entries) != 4 {
		t.Fatalf("empty draft %s missing: %+v, %v", empty, catalog, err)
	}
}

func TestInvalidDraftDoesNotCreateCopy(t *testing.T) {
	c := chart.New(4)
	c.Timing = []chart.Timing{{BPM: 120, Scroll: 1, Meter: 4}}
	c.Notes = []chart.Note{{Lane: 1, At: time.Second, End: 2 * time.Second, Kind: "hold"}, {Lane: 1, At: time.Second, End: time.Second, Kind: "tap"}}
	dir := t.TempDir()
	if _, err := chart.SaveEdit(filepath.Join(dir, "original.osu"), c, c); err == nil {
		t.Fatal("saved overlapping notes")
	}
	files, err := os.ReadDir(dir)
	if err != nil || len(files) != 0 {
		t.Fatal("invalid save left a copy", err)
	}
}

func TestEditedCopyKeepsOriginalFormat(t *testing.T) {
	for _, format := range []string{"ojn", "bms", "bme", "bml", "osu", "sm"} {
		t.Run(format, func(t *testing.T) {
			c := chart.New(1)
			c.Source = chart.Source{Format: format, Index: 2, SHA256: "original", MD5: "original"}
			c.Timing = []chart.Timing{{BPM: 120, Scroll: 1, Meter: 4}}
			c.Notes = []chart.Note{{At: time.Second, End: time.Second, Kind: "tap"}}
			path := filepath.Join(t.TempDir(), "song."+format)
			for range 2 {
				var err error
				path, err = chart.SaveEdit(path, c, c)
				if err != nil {
					t.Fatal(err)
				}
				cs, err := chart.Load(path)
				if err != nil {
					t.Fatal(err)
				}
				c = cs[0]
				if c.Source.Format != "ggc" || c.Source.OriginalFormat != format || c.Source.Index != 0 || c.Source.SHA256 != "" || c.Source.MD5 != "" {
					t.Fatalf("copy lost its format or retained source identity: %+v", c.Source)
				}
			}
		})
	}
}
