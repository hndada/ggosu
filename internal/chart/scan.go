package chart

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"
)

type Entry struct {
	Path       string             `json:"path"`
	Index      int                `json:"index"`
	Folder     string             `json:"folder"`
	Title      string             `json:"title"`
	Artist     string             `json:"artist"`
	Creator    string             `json:"creator"`
	Format     string             `json:"format"`
	Difficulty string             `json:"difficulty"`
	Level      string             `json:"level"`
	Rating     float64            `json:"rating,omitempty"` // Calculated level (internal/difficulty); zero when not rated.
	Rates      map[string]float64 `json:"rates,omitempty"`  // Calculated level at other preset Time Rates, by rate ("1.5").
	Lanes      int                `json:"lanes"`
	Notes      int                `json:"notes"`
	BPMMin     float64            `json:"bpmMin"`
	BPMMax     float64            `json:"bpmMax"`
	BaseBPM    float64            `json:"baseBPM"`              // First timing point, used for scroll exposure.
	Length     float64            `json:"length"`               // First note start to last note end, seconds.
	Modified   string             `json:"modified"`             // Source file modification time, RFC3339.
	Hash       string             `json:"hash"`                 // Chart digest; records refer to charts by it.
	Background string             `json:"background,omitempty"` // Asset id of the first background image, for the song select.
	// AudioDuration is Chart.AudioDuration, in nanoseconds; zero when Scan had no length cache.
	AudioDuration time.Duration `json:"audioDuration,omitempty"`
}
type Catalog struct {
	Entries []Entry  `json:"entries"`
	Errors  []string `json:"errors"`
	// Fixes lists the source errors Load repaired, as "path: fix", for the load log.
	Fixes []string `json:"fixes,omitempty"`
}

// Scan keeps per-file diagnostics so one bad chart cannot hide the collection.
// With lengths, each entry also records its AudioDuration.
// The root's demo subdirectory is used only when no other charts are found.
// Files are parsed in parallel; entries and errors come out in the same order as a serial scan.
func Scan(root string, lengths *AudioLengths) (Catalog, error) {
	root = filepath.Clean(root)
	out := Catalog{Entries: []Entry{}, Errors: []string{}}
	demo := filepath.Join(root, "demo")
	hasDemo := false
	var files []string
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if path == demo {
				hasDemo = true
				return filepath.SkipDir
			}
			if strings.HasPrefix(d.Name(), ".") {
				return filepath.SkipDir
			}
			return nil
		}
		if Supported(path) {
			files = append(files, path)
		}
		return nil
	})
	if err == nil {
		scanFiles(files, lengths, &out)
	}
	if err == nil && len(out.Entries) == 0 && hasDemo {
		files = files[:0]
		err = filepath.WalkDir(demo, func(path string, d fs.DirEntry, err error) error {
			if err == nil && !d.IsDir() && Supported(path) {
				files = append(files, path)
			}
			return err
		})
		if err == nil {
			scanFiles(files, lengths, &out)
		}
	}
	if err != nil {
		return out, fmt.Errorf("scan songs: %w", err)
	}
	sort.Slice(out.Entries, func(i, j int) bool {
		a, b := out.Entries[i], out.Entries[j]
		if a.Title != b.Title {
			return a.Title < b.Title
		}
		if a.Path != b.Path {
			return a.Path < b.Path
		}
		return a.Index < b.Index
	})
	return out, nil
}

// scanFiles parses files on all CPUs. AudioLengths is not safe for concurrent use, so measuring
// audio takes a lock; parsing and digests, the bulk of the work, run in parallel.
func scanFiles(files []string, lengths *AudioLengths, out *Catalog) {
	type result struct {
		entries []Entry
		errors  []string
		fixes   []string
	}
	results := make([]result, len(files))
	next := make(chan int)
	var measure sync.Mutex
	var wg sync.WaitGroup
	for range min(runtime.GOMAXPROCS(0), max(len(files), 1)) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range next {
				results[i].entries, results[i].errors, results[i].fixes = scanFile(files[i], lengths, &measure)
			}
		}()
	}
	for i := range files {
		next <- i
	}
	close(next)
	wg.Wait()
	for _, r := range results {
		out.Entries = append(out.Entries, r.entries...)
		out.Errors = append(out.Errors, r.errors...)
		out.Fixes = append(out.Fixes, r.fixes...)
	}
}

func scanFile(path string, lengths *AudioLengths, measure *sync.Mutex) ([]Entry, []string, []string) {
	cs, err := Load(path)
	if err != nil {
		return nil, []string{err.Error()}, nil
	}
	info, err := os.Stat(path)
	if err != nil {
		return nil, []string{fmt.Sprintf("%s: %v", path, err)}, nil
	}
	var entries []Entry
	var errs, fixes []string
	for i, c := range cs {
		// A file-level fix (a skipped difficulty) repeats on every chart of the file.
		for _, f := range c.Fixes {
			if f = path + ": " + f; !slices.Contains(fixes, f) {
				fixes = append(fixes, f)
			}
		}
		if len(c.Notes) == 0 && !strings.EqualFold(filepath.Ext(path), ".ggc") {
			continue
		}
		hash, err := c.Digest()
		if err != nil {
			errs = append(errs, fmt.Sprintf("%s #%d: %v", path, i, err))
			continue
		}
		e := Entry{Path: path, Index: i, Folder: filepath.Dir(path), Title: c.Metadata.Title,
			Artist: c.Metadata.Artist, Creator: c.Metadata.Creator, Format: c.Source.Format,
			Difficulty: c.Metadata.Difficulty, Level: c.Metadata.Level, Lanes: c.Lanes,
			Notes: len(c.Notes), BaseBPM: c.Timing[0].BPM, Modified: info.ModTime().UTC().Format(time.RFC3339Nano), Hash: hash, Background: background(c)}
		for _, t := range c.Timing {
			if t.BPM > 0 {
				if e.BPMMin == 0 || t.BPM < e.BPMMin {
					e.BPMMin = t.BPM
				}
				e.BPMMax = max(e.BPMMax, t.BPM)
			}
		}
		var start, end time.Duration
		if len(c.Notes) > 0 {
			start, end = c.Notes[0].At, c.Notes[0].End
		}
		for _, n := range c.Notes {
			start = min(start, n.At)
			end = max(end, n.At, n.End)
		}
		e.Length = (end - start).Seconds()
		if lengths != nil {
			measure.Lock()
			e.AudioDuration = c.AudioDuration(lengths)
			measure.Unlock()
		}
		entries = append(entries, e)
	}
	return entries, errs, fixes
}

// background picks the earliest background scene that points at an image asset.
func background(c *Chart) string {
	id, at := "", time.Duration(0)
	for _, s := range c.Storyboard {
		if s.Kind != "background" || c.Assets[s.Asset].Kind != "image" {
			continue
		}
		if id == "" || s.At < at {
			id, at = s.Asset, s.At
		}
	}
	return id
}
