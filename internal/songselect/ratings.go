package songselect

import (
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"runtime"
	"sync"

	"ggosu/internal/chart"
	"ggosu/internal/difficulty"
)

// ratingsFile caches difficulty.Rate by chart digest, and in Rates the other
// difficulty.Presets by digest and rate key. A chart that cannot be rated
// (a scratch lane, say) is stored as 0 so it is not analysed again on every scan.
type ratingsFile struct {
	Model   string                        `json:"model"`
	Ratings map[string]float64            `json:"ratings"`
	Rates   map[string]map[string]float64 `json:"rates,omitempty"`
}

// complete reports whether hash has a cached level at every preset.
func (f ratingsFile) complete(hash string) bool {
	if _, ok := f.Ratings[hash]; !ok {
		return false
	}
	for _, r := range difficulty.Presets[1:] {
		if _, ok := f.Rates[hash][difficulty.RateKey(r)]; !ok {
			return false
		}
	}
	return true
}

// Rate fills each entry's Rating and Rates, analysing only charts missing from the cache
// at path, and saves the cache with exactly the listed charts. Each preset is a full
// analysis, so a large library's first scan takes several times as long as at 1x alone.
func Rate(entries []chart.Entry, path string) error {
	var cache ratingsFile
	var loadErr error
	if b, err := os.ReadFile(path); err == nil {
		if err := json.Unmarshal(b, &cache); err != nil {
			loadErr = fmt.Errorf("난이도 캐시 %s: %w", path, err)
		}
	} else if !os.IsNotExist(err) {
		loadErr = err
	}
	if cache.Model != difficulty.Model || cache.Ratings == nil {
		cache = ratingsFile{Model: difficulty.Model, Ratings: map[string]float64{}}
	}
	if cache.Rates == nil {
		cache.Rates = map[string]map[string]float64{}
	}
	// One parse per file covers all of its difficulties.
	missing := map[string][]int{}
	var files []string
	for i, e := range entries {
		if cache.complete(e.Hash) {
			continue
		}
		if missing[e.Path] == nil {
			files = append(files, e.Path)
		}
		missing[e.Path] = append(missing[e.Path], i)
	}
	var mu sync.Mutex
	var wg sync.WaitGroup
	next := make(chan string)
	for range min(runtime.GOMAXPROCS(0), max(len(files), 1)) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for path := range next {
				cs, err := chart.Load(path)
				for _, i := range missing[path] {
					e := entries[i]
					var c *chart.Chart
					if err == nil && e.Index < len(cs) {
						if hash, err := cs[e.Index].Digest(); err == nil && hash == e.Hash {
							c = cs[e.Index]
						}
					}
					mu.Lock()
					have, ok := cache.Ratings[e.Hash]
					rates := maps.Clone(cache.Rates[e.Hash])
					mu.Unlock()
					// A 1x level cached by an earlier version is kept; only the missing presets are analysed.
					if !ok {
						have = safeRate(c, 1)
					}
					if rates == nil {
						rates = map[string]float64{}
					}
					for _, r := range difficulty.Presets[1:] {
						if _, ok := rates[difficulty.RateKey(r)]; !ok {
							rates[difficulty.RateKey(r)] = safeRate(c, r)
						}
					}
					mu.Lock()
					cache.Ratings[e.Hash] = have
					cache.Rates[e.Hash] = rates
					mu.Unlock()
				}
			}
		}()
	}
	for _, f := range files {
		next <- f
	}
	close(next)
	wg.Wait()
	kept := ratingsFile{Model: cache.Model, Ratings: map[string]float64{}, Rates: map[string]map[string]float64{}}
	for i := range entries {
		hash := entries[i].Hash
		entries[i].Rating = cache.Ratings[hash]
		entries[i].Rates = cache.Rates[hash]
		kept.Ratings[hash] = entries[i].Rating
		kept.Rates[hash] = cache.Rates[hash]
	}
	return errors.Join(loadErr, save(path, kept))
}

func save(path string, v any) (err error) {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".ratings-*")
	if err != nil {
		return err
	}
	defer func() {
		if e := os.Remove(tmp.Name()); e != nil && !os.IsNotExist(e) {
			err = errors.Join(err, e)
		}
	}()
	writeErr := json.NewEncoder(tmp).Encode(v)
	if writeErr == nil {
		writeErr = tmp.Sync()
	}
	if err := errors.Join(writeErr, tmp.Close()); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}

// safeRate keeps one chart the model cannot handle from taking the scan down with it.
// A nil chart (missing or changed since the scan) rates 0.
func safeRate(c *chart.Chart, rate float64) (value float64) {
	if c == nil {
		return 0
	}
	defer func() {
		if recover() != nil {
			value = 0
		}
	}()
	value, _ = difficulty.RateAt(c, rate)
	return value
}
