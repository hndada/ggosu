package chart

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestCatalogMetadata(t *testing.T) {
	root := t.TempDir()
	c := New(4)
	c.Metadata = Metadata{Title: "Variable", Artist: "Artist", Creator: "Mapper", Difficulty: "Hard", Level: "12"}
	c.Timing = []Timing{{BPM: 120, Scroll: 1, Meter: 4}, {At: time.Second, BPM: 180, Scroll: 0.5, Meter: 4}, {At: 2 * time.Second, BPM: 90, Scroll: 2, Meter: 4}}
	c.Notes = []Note{{At: -time.Second, End: -time.Second, Kind: "tap"}, {At: time.Second, End: 4 * time.Second, Kind: "hold", Lane: 1}, {At: 2 * time.Second, End: 2 * time.Second, Kind: "tap", Lane: 2}}
	path := filepath.Join(root, "test.ggc")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	writeErr := Write(f, c)
	closeErr := f.Close()
	if writeErr != nil || closeErr != nil {
		t.Fatal(writeErr, closeErr)
	}
	modified := time.Date(2026, 10, 8, 12, 0, 0, 0, time.UTC)
	if err := os.Chtimes(path, modified, modified); err != nil {
		t.Fatal(err)
	}
	got, err := Scan(root, nil)
	if err != nil || len(got.Entries) != 1 || len(got.Errors) != 0 {
		t.Fatalf("scan: %+v, %v", got, err)
	}
	e := got.Entries[0]
	if e.Artist != "Artist" || e.Creator != "Mapper" || e.Level != "12" || e.Lanes != 4 || e.Notes != 3 {
		t.Fatalf("metadata: %+v", e)
	}
	if e.BPMMin != 90 || e.BPMMax != 180 || e.Length != 5 {
		t.Fatalf("timing range ignores scroll and includes negative lead-in and hold tail: %+v", e)
	}
	if e.Modified != modified.Format(time.RFC3339Nano) {
		t.Fatalf("file modification time: %s", e.Modified)
	}
}
