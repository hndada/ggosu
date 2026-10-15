package chart_test

import (
	"os"
	"path/filepath"
	"testing"

	"ggosu/internal/chart"
)

func TestScanDemoFallback(t *testing.T) {
	root := t.TempDir()
	demo := filepath.Join(root, "demo", "test.ggc")
	if err := os.MkdirAll(filepath.Dir(demo), 0755); err != nil {
		t.Fatal(err)
	}
	c := chart.New(4)
	c.Metadata.Title = "Demo"
	c.Timing = []chart.Timing{{BPM: 120, Scroll: 1, Meter: 4}}
	c.Notes = []chart.Note{{At: 1, End: 1, Kind: "tap"}}
	f, err := os.Create(demo)
	if err != nil {
		t.Fatal(err)
	}
	writeErr := chart.Write(f, c)
	closeErr := f.Close()
	if writeErr != nil || closeErr != nil {
		t.Fatal(writeErr, closeErr)
	}
	check := func(path string, errors int) {
		t.Helper()
		catalog, err := chart.Scan(root, nil)
		if err != nil || len(catalog.Entries) != 1 || len(catalog.Errors) != errors {
			t.Fatalf("catalog: %+v, %v", catalog, err)
		}
		if catalog.Entries[0].Path != path {
			t.Fatalf("chart: %s, want %s", catalog.Entries[0].Path, path)
		}
	}
	check(demo, 0)
	// Unrelated files and broken charts must not suppress the fallback.
	if err := os.WriteFile(filepath.Join(root, "readme.txt"), []byte("song folder"), 0644); err != nil {
		t.Fatal(err)
	}
	broken := filepath.Join(root, "broken.ggc")
	if err := os.WriteFile(broken, []byte("invalid"), 0644); err != nil {
		t.Fatal(err)
	}
	check(demo, 1)
	data, err := os.ReadFile(demo)
	if err != nil {
		t.Fatal(err)
	}
	song := filepath.Join(root, "song.ggc")
	if err := os.WriteFile(song, data, 0644); err != nil {
		t.Fatal(err)
	}
	check(song, 1)
	if err := os.Remove(song); err != nil {
		t.Fatal(err)
	}
	check(demo, 1)
	if err := os.Remove(broken); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(demo); err != nil {
		t.Fatal(err)
	}
	catalog, err := chart.Scan(root, nil)
	if err != nil || len(catalog.Entries) != 0 || len(catalog.Errors) != 0 {
		t.Fatalf("empty catalog: %+v, %v", catalog, err)
	}
}
