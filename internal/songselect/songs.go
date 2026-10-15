// Package songselect scans the song folder and resolves song-select choices against that list.
// It has no Wails or game-process dependency; internal/menu binds it to the UI.
package songselect

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"sync"

	"ggosu/internal/chart"
	_ "ggosu/internal/chart/bms"
	_ "ggosu/internal/chart/ojn"
	_ "ggosu/internal/chart/osu"
	_ "ggosu/internal/chart/sm"
	"ggosu/internal/difficulty"
)

// Selection is a chart chosen in the song select with its play options.
type Selection struct {
	Visibility string  `json:"visibility,omitempty"`
	O2Jam      string  `json:"o2jam,omitempty"`
	Pills      bool    `json:"pills,omitempty"`
	Health     int     `json:"health,omitempty"`
	HealthFail int     `json:"healthFail,omitempty"`
	Folder     string  `json:"folder"`
	ChartID    string  `json:"chartID"`
	Song       string  `json:"song"`
	Difficulty string  `json:"difficulty"`
	Level      string  `json:"level"`
	Index      int     `json:"index"`
	Mode       string  `json:"mode"`
	ReplayID   string  `json:"replayID"`
	Mod        string  `json:"mod"`
	Seed       *uint32 `json:"seed"`
	Rate       float64 `json:"rate"`
	Pitch      *bool   `json:"pitch,omitempty"`
	Section    *Range  `json:"section,omitempty"` // Practice section in seconds on the song timeline.
	// Medley lists the three songs of a medley mode run; it replaces this chart.
	Medley []Selection `json:"medley,omitempty"`
}

// Range is a practice section [Start, End) in seconds.
type Range struct {
	Start float64 `json:"start"`
	End   float64 `json:"end"`
}

// Songs holds the last scanned song list. Its zero value is ready to use.
type Songs struct {
	mu      sync.Mutex
	entries []chart.Entry
	levels  map[string]float64 // Level results for rates outside difficulty.Presets, by digest@rate.
}

// Lookup returns a copy of the scanned entry s refers to. Only scanned entries are accepted.
func (l *Songs) Lookup(s Selection) (chart.Entry, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	for _, e := range l.entries {
		if e.Path == s.ChartID && e.Folder == s.Folder && e.Index == s.Index {
			return e, nil
		}
	}
	return chart.Entry{}, fmt.Errorf("곡 목록을 새로 읽고 채보를 선택해 주세요")
}

// Find returns the scanned entry with the given chart digest.
func (l *Songs) Find(hash string) (chart.Entry, bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	for _, e := range l.entries {
		if e.Hash == hash {
			return e, true
		}
	}
	return chart.Entry{}, false
}

// Scan reads real files and replaces the song list. lengthsCache is the audio
// length cache file; the caller owns where data files live. folders are extra
// local folders scanned beside the songs folder; one that is missing or broken
// is reported in the catalog's errors and does not hide the others. Calculated
// levels are cached in ratings.json beside the length cache.
func (l *Songs) Scan(lengthsCache string, folders []string) (chart.Catalog, error) {
	root := "songs"
	if _, err := os.Stat(root); err != nil {
		exe, exeErr := os.Executable()
		if exeErr != nil {
			return chart.Catalog{}, exeErr
		}
		root = filepath.Join(filepath.Dir(exe), "songs")
	}
	root, err := filepath.Abs(root)
	if err != nil {
		return chart.Catalog{}, err
	}
	// A broken cache only costs a rescan of audio headers.
	lengths, lengthsErr := chart.LoadAudioLengths(lengthsCache)
	catalog, err := chart.Scan(root, lengths)
	if err != nil {
		return catalog, err
	}
	demo := filepath.Join(root, "demo")
	seen := map[string]bool{}
	for _, e := range catalog.Entries {
		seen[fmt.Sprint(e.Path, "#", e.Index)] = true
	}
	for _, folder := range folders {
		// Extra folders skip audio lengths: measuring decodes every sound bank (an O2Mania o2jam
		// folder holds gigabytes), and only the multiplayer lobby needs them, per chart.
		extra, err := chart.Scan(folder, nil)
		if err != nil {
			catalog.Errors = append(catalog.Errors, fmt.Sprintf("곡 폴더 %s: %v", folder, err))
			continue
		}
		// The demo stands in only while there is nothing else to play.
		if len(extra.Entries) > 0 {
			catalog.Entries = slices.DeleteFunc(catalog.Entries, func(e chart.Entry) bool { return strings.HasPrefix(e.Path, demo+string(filepath.Separator)) })
		}
		for _, e := range extra.Entries {
			if k := fmt.Sprint(e.Path, "#", e.Index); !seen[k] && !strings.HasPrefix(e.Path, demo+string(filepath.Separator)) {
				seen[k] = true
				catalog.Entries = append(catalog.Entries, e)
			}
		}
		catalog.Errors = append(catalog.Errors, extra.Errors...)
		catalog.Fixes = append(catalog.Fixes, extra.Fixes...)
	}
	sort.SliceStable(catalog.Entries, func(i, j int) bool {
		a, b := catalog.Entries[i], catalog.Entries[j]
		if a.Title != b.Title {
			return a.Title < b.Title
		}
		if a.Path != b.Path {
			return a.Path < b.Path
		}
		return a.Index < b.Index
	})
	if lengthsErr != nil {
		catalog.Errors = append(catalog.Errors, lengthsErr.Error())
	}
	if err := lengths.Save(lengthsCache); err != nil {
		catalog.Errors = append(catalog.Errors, fmt.Sprintf("곡 길이 캐시 저장: %v", err))
	}
	// Charts repaired on load are logged apart from the song list and play results.
	if err := writeFixLog(filepath.Join(filepath.Dir(lengthsCache), "chart-fixes.log"), catalog.Fixes); err != nil {
		catalog.Errors = append(catalog.Errors, fmt.Sprintf("채보 수정 로그: %v", err))
	}
	if err := Rate(catalog.Entries, filepath.Join(filepath.Dir(lengthsCache), "ratings.json")); err != nil {
		catalog.Errors = append(catalog.Errors, fmt.Sprintf("난이도 캐시: %v", err))
	}
	l.mu.Lock()
	l.entries = catalog.Entries
	l.mu.Unlock()
	return catalog, nil
}

// writeFixLog replaces the log with this scan's repairs, so it always describes the current library.
func writeFixLog(path string, fixes []string) error {
	if len(fixes) == 0 {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return err
		}
		return nil
	}
	return os.WriteFile(path, []byte(strings.Join(fixes, "\n")+"\n"), 0644)
}

func (l *Songs) Distribution(s Selection) (chart.Density, error) {
	e, err := l.Lookup(s)
	if err != nil {
		return chart.Density{}, err
	}
	cs, err := chart.Load(e.Path)
	if err != nil {
		return chart.Density{}, err
	}
	if s.Index < 0 || s.Index >= len(cs) {
		return chart.Density{}, fmt.Errorf("채보가 변경됐습니다")
	}
	return chart.AnalyzeDensity(cs[s.Index])
}

// Level is the calculated level of the selected chart at s.Rate: the scan's cached value at a
// preset rate, else one analysis kept in memory. 0 means the chart cannot be rated.
func (l *Songs) Level(s Selection) (float64, error) {
	if math.IsNaN(s.Rate) || s.Rate < .5 || s.Rate > 2 {
		return 0, fmt.Errorf("Time Rate는 0.5~2.0배입니다")
	}
	e, err := l.Lookup(s)
	if err != nil {
		return 0, err
	}
	key := difficulty.RateKey(s.Rate)
	if s.Rate == 1 && e.Rating > 0 {
		return e.Rating, nil
	}
	if v, ok := e.Rates[key]; ok {
		return v, nil
	}
	l.mu.Lock()
	v, ok := l.levels[e.Hash+"@"+key]
	l.mu.Unlock()
	if ok {
		return v, nil
	}
	cs, err := chart.Load(e.Path)
	if err != nil {
		return 0, err
	}
	if s.Index < 0 || s.Index >= len(cs) {
		return 0, fmt.Errorf("채보가 변경됐습니다")
	}
	if hash, err := cs[s.Index].Digest(); err != nil || hash != e.Hash {
		return 0, fmt.Errorf("채보가 변경됐습니다")
	}
	v = safeRate(cs[s.Index], s.Rate)
	l.mu.Lock()
	if l.levels == nil {
		l.levels = map[string]float64{}
	}
	l.levels[e.Hash+"@"+key] = v
	l.mu.Unlock()
	return v, nil
}
