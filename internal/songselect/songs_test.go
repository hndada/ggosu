package songselect

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"ggosu/internal/chart"
)

func TestMain(m *testing.M) {
	if err := os.Chdir("../.."); err != nil {
		fmt.Fprintln(os.Stderr, "songselect test fixtures:", err)
		os.Exit(1)
	}
	os.Exit(m.Run())
}

func TestCatalogAndSelection(t *testing.T) {
	var a Songs
	lengths, err := chart.LoadAudioLengths(filepath.Join(t.TempDir(), "audio-lengths.json"))
	if err != nil {
		t.Fatal(err)
	}
	catalog, err := chart.Scan("songs/demo", lengths)
	if err != nil {
		t.Fatal(err)
	}
	a.entries = catalog.Entries
	if len(catalog.Entries) < 7 {
		t.Fatalf("demo catalogue: %d entries, %v", len(catalog.Entries), catalog.Errors)
	}
	if _, err := a.Lookup(Selection{Folder: "fake", ChartID: "outside.osu"}); err == nil {
		t.Fatal("allowed file outside catalogue")
	}
	seen := map[string]int{}
	for _, e := range catalog.Entries {
		seen[e.Title]++
		if len(e.Hash) != 64 {
			t.Fatalf("%s #%d: missing chart digest", e.Path, e.Index)
		}
		if e.AudioDuration <= 0 {
			t.Fatalf("%s #%d: missing audio duration", e.Path, e.Index)
		}
	}
	if seen["Gosu O2Jam Demo"] < 3 || seen["Gosu StepMania Demo"] < 2 {
		t.Fatalf("missing difficulties: %v", seen)
	}
}

func TestScanKeepsGoodCharts(t *testing.T) {
	var a Songs
	catalog, err := a.Scan(filepath.Join(t.TempDir(), "audio-lengths.json"), nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(catalog.Entries) == 0 {
		t.Fatal("no playable entries")
	}
}

// Extra song folders (an O2Mania Music folder, say) join the catalogue; a missing one is only reported.
func TestScanSongFolders(t *testing.T) {
	folder := t.TempDir()
	for _, name := range []string{"demo.ojn", "demo.ojm"} {
		b, err := os.ReadFile(filepath.Join("songs", "demo", name))
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(folder, "o2ma100"+filepath.Ext(name)), b, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	missing := filepath.Join(folder, "gone")
	var a Songs
	catalog, err := a.Scan(filepath.Join(t.TempDir(), "audio-lengths.json"), []string{folder, missing})
	if err != nil {
		t.Fatal(err)
	}
	found := 0
	for _, e := range catalog.Entries {
		if e.Folder == folder {
			found++
			if _, err := a.Lookup(Selection{Folder: e.Folder, ChartID: e.Path, Index: e.Index}); err != nil {
				t.Fatal(err)
			}
		}
	}
	if found != 3 {
		t.Fatalf("extra folder charts: %d, %v", found, catalog.Errors)
	}
	reported := false
	for _, e := range catalog.Errors {
		reported = reported || strings.Contains(e, missing)
	}
	if !reported {
		t.Fatal("missing folder not reported", catalog.Errors)
	}
}
