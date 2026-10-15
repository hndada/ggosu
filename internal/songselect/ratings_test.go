package songselect

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"ggosu/internal/chart"
	"ggosu/internal/difficulty"
)

// Scan rates the demo charts once and reuses the cache on the next scan.
func TestScanRatesChartsAndCachesThem(t *testing.T) {
	dir := t.TempDir()
	var a Songs
	first, err := a.Scan(filepath.Join(dir, "audio-lengths.json"), nil)
	if err != nil {
		t.Fatal(err)
	}
	rated := 0
	for _, e := range first.Entries {
		if e.Rating > 0 {
			rated++
		}
	}
	if rated == 0 {
		t.Fatalf("no demo chart was rated: %v", first.Errors)
	}
	b, err := os.ReadFile(filepath.Join(dir, "ratings.json"))
	if err != nil {
		t.Fatal(err)
	}
	var cache ratingsFile
	if err := json.Unmarshal(b, &cache); err != nil || cache.Model != difficulty.Model || len(cache.Ratings) != len(first.Entries) {
		t.Fatalf("cache = %+v, %v", cache, err)
	}
	// A cached value is used as is, without analysing the chart again.
	e := first.Entries[0]
	cache.Ratings[e.Hash] = 42
	if err := save(filepath.Join(dir, "ratings.json"), cache); err != nil {
		t.Fatal(err)
	}
	second, err := a.Scan(filepath.Join(dir, "audio-lengths.json"), nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, x := range second.Entries {
		if x.Hash == e.Hash && x.Rating != 42 {
			t.Fatalf("rating %v, want the cached 42", x.Rating)
		}
	}
	// The previous model's 1x and preset values must all be replaced.
	cache.Model = "finger-motion-v3-poc"
	for key := range cache.Rates[e.Hash] {
		cache.Rates[e.Hash][key] = 42
	}
	if err := save(filepath.Join(dir, "ratings.json"), cache); err != nil {
		t.Fatal(err)
	}
	third, err := a.Scan(filepath.Join(dir, "audio-lengths.json"), nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, x := range third.Entries {
		if x.Hash == e.Hash {
			if x.Rating != e.Rating {
				t.Fatalf("old model rating %v, want recalculated %v", x.Rating, e.Rating)
			}
			for key, want := range e.Rates {
				if x.Rates[key] != want {
					t.Fatalf("old model rate %s: %v, want recalculated %v", key, x.Rates[key], want)
				}
			}
		}
	}
}

// Every preset Time Rate is rated during the scan; a cache from before presets keeps its 1x
// levels and only gains the missing rates. Other rates are rated on request.
func TestScanRatesPresetsAndLevelRatesTheRest(t *testing.T) {
	dir := t.TempDir()
	var a Songs
	first, err := a.Scan(filepath.Join(dir, "audio-lengths.json"), nil)
	if err != nil {
		t.Fatal(err)
	}
	var e chart.Entry
	for _, x := range first.Entries {
		if x.Rating > 0 {
			e = x
			break
		}
	}
	if e.Hash == "" {
		t.Fatalf("no demo chart was rated: %v", first.Errors)
	}
	for _, r := range difficulty.Presets[1:] {
		if e.Rates[difficulty.RateKey(r)] <= 0 {
			t.Fatalf("rates %v miss %v", e.Rates, r)
		}
	}
	if e.Rates["2"] <= e.Rating {
		t.Fatalf("2x level %v is not above 1x %v", e.Rates["2"], e.Rating)
	}
	// An older cache: 1x only, with a marker value that must survive.
	old := ratingsFile{Model: difficulty.Model, Ratings: map[string]float64{e.Hash: 42}}
	if err := save(filepath.Join(dir, "ratings.json"), old); err != nil {
		t.Fatal(err)
	}
	second, err := a.Scan(filepath.Join(dir, "audio-lengths.json"), nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, x := range second.Entries {
		if x.Hash == e.Hash && (x.Rating != 42 || x.Rates["1.5"] != e.Rates["1.5"]) {
			t.Fatalf("rating %v rates %v, want the cached 42 and %v", x.Rating, x.Rates, e.Rates)
		}
	}
	s := Selection{Folder: e.Folder, ChartID: e.Path, Index: e.Index}
	for rate, want := range map[float64]float64{1.5: e.Rates["1.5"], 1: 42} {
		s.Rate = rate
		if v, err := a.Level(s); err != nil || v != want {
			t.Fatalf("Level(%v) = %v, %v; want %v", rate, v, err, want)
		}
	}
	s.Rate = 1.3
	v, err := a.Level(s)
	if err != nil || v <= e.Rates["1.25"] || v >= e.Rates["1.5"] {
		t.Fatalf("Level(1.3) = %v, %v; want between %v and %v", v, err, e.Rates["1.25"], e.Rates["1.5"])
	}
	s.Rate = 3
	if _, err := a.Level(s); err == nil {
		t.Fatal("Level accepted a 3x rate")
	}
}
