package archive

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"ggosu/internal/chart"
)

// PackRef names a pack that contains a chart. Source "event" marks a BMS event that is searchable but
// not itself a download; other sources name an Item.
type PackRef struct {
	Source string `json:"source"`
	ID     string `json:"id"`
	Title  string `json:"title"`
	Path   string `json:"path,omitempty"` // chart file inside the pack, known after a scan
}

// ChartRecord is one sighting of a chart. IDs are identities with a prefix:
//
//	fp:<notes and timing fingerprint>  from a scan, matches across formats' packs
//	md5:<chart file md5>               osu! checksum, BMS table and BMS Search md5
//	eo:<EtternaOnline chart id>
//	pk:<source>/<pack>/<title>/<difficulty>  links remote metadata to a scan of the same pack
//
// Records that share any ID describe the same chart.
type ChartRecord struct {
	IDs        []string  `json:"ids"`
	Format     string    `json:"format,omitempty"`
	Title      string    `json:"title"`
	Artist     string    `json:"artist,omitempty"`
	Creator    string    `json:"creator,omitempty"`
	Difficulty string    `json:"difficulty,omitempty"`
	Level      string    `json:"level,omitempty"`
	Lanes      int       `json:"lanes,omitempty"` // playable columns including scratch
	Notes      int       `json:"notes,omitempty"`
	Packs      []PackRef `json:"packs"`
	Tables     []string  `json:"tables,omitempty"`
	Scanned    bool      `json:"scanned,omitempty"`
	Tier       string    `json:"tier,omitempty"`
	Plays      int       `json:"plays,omitempty"`
}

// Chart is every sighting of one chart merged. Key is its most stable ID.
type Chart struct {
	Key        string    `json:"key"`
	IDs        []string  `json:"ids"`
	Format     string    `json:"format,omitempty"`
	Title      string    `json:"title"`
	Artist     string    `json:"artist,omitempty"`
	Creator    string    `json:"creator,omitempty"`
	Difficulty string    `json:"difficulty,omitempty"`
	Level      string    `json:"level,omitempty"`
	Lanes      int       `json:"lanes,omitempty"`
	Notes      int       `json:"notes,omitempty"`
	Packs      []PackRef `json:"packs"`
	Tables     []string  `json:"tables,omitempty"`
	Scanned    bool      `json:"scanned,omitempty"`
	Tier       string    `json:"tier,omitempty"`  // highest tier of any sighting
	Plays      int       `json:"plays,omitempty"` // highest play count of any sighting
}

// ScanResult is the chart list read from one stored file.
type ScanResult struct {
	ID     string        `json:"id"`
	SHA256 string        `json:"sha256"`
	Charts []ChartRecord `json:"charts,omitempty"`
	Error  string        `json:"error,omitempty"`
	At     time.Time     `json:"at"`
}

// Fingerprint identifies a chart by what is played: lanes, timing and notes, rounded to milliseconds.
// Titles, sounds and assets are left out so a chart repacked or retagged in another pack still matches.
func Fingerprint(c *chart.Chart) string {
	ms := func(d time.Duration) int64 { return d.Round(time.Millisecond).Milliseconds() }
	h := sha256.New()
	fmt.Fprintf(h, "lanes %d\n", c.Lanes)
	for _, t := range c.Timing {
		fmt.Fprintf(h, "t %d %.3f %.3f %d\n", ms(t.At), t.BPM, t.Scroll, ms(t.Stop))
	}
	for _, n := range c.Notes {
		fmt.Fprintf(h, "n %d %d %d %s\n", ms(n.At), ms(n.End), n.Lane, n.Kind)
	}
	return hex.EncodeToString(h.Sum(nil))[:32]
}

func packKey(source, pack, title, difficulty string) string {
	return "pk:" + source + "/" + pack + "/" + normalize(title) + "/" + normalize(difficulty)
}

// MergeCharts joins records that share an ID. Scanned records win metadata conflicts.
func MergeCharts(recs []ChartRecord) []Chart {
	parent := make([]int, len(recs))
	for i := range parent {
		parent[i] = i
	}
	var find func(int) int
	find = func(i int) int {
		for parent[i] != i {
			parent[i] = parent[parent[i]]
			i = parent[i]
		}
		return i
	}
	// A pack key only bridges metadata and a scan of the same pack. A pack holding two charts with
	// one title and difficulty (remixes, alternate versions) makes the key ambiguous, so it is dropped.
	type sides struct{ remote, scanned int }
	pk := map[string]*sides{}
	for _, r := range recs {
		for _, id := range r.IDs {
			if strings.HasPrefix(id, "pk:") {
				s := pk[id]
				if s == nil {
					s = &sides{}
					pk[id] = s
				}
				if r.Scanned {
					s.scanned++
				} else {
					s.remote++
				}
			}
		}
	}
	ambiguous := func(id string) bool {
		s := pk[id]
		return s != nil && (s.remote > 1 || s.scanned > 1)
	}
	owner := map[string]int{}
	for i, r := range recs {
		for _, id := range r.IDs {
			if ambiguous(id) {
				continue
			}
			if j, ok := owner[id]; ok {
				parent[find(i)] = find(j)
			} else {
				owner[id] = i
			}
		}
	}
	groups := map[int][]int{}
	var roots []int
	for i := range recs {
		root := find(i)
		if _, ok := groups[root]; !ok {
			roots = append(roots, root)
		}
		groups[root] = append(groups[root], i)
	}
	out := make([]Chart, 0, len(roots))
	for _, root := range roots {
		members := groups[root]
		sort.SliceStable(members, func(a, b int) bool { return recs[members[a]].Scanned && !recs[members[b]].Scanned })
		var c Chart
		for _, i := range members {
			r := recs[i]
			for _, id := range r.IDs {
				if !ambiguous(id) {
					c.IDs = appendUnique(c.IDs, id)
				}
			}
			for _, t := range r.Tables {
				c.Tables = appendUnique(c.Tables, t)
			}
			for _, p := range r.Packs {
				c.Packs = addPack(c.Packs, p)
			}
			first(&c.Format, r.Format)
			first(&c.Title, r.Title)
			first(&c.Artist, r.Artist)
			first(&c.Creator, r.Creator)
			first(&c.Difficulty, r.Difficulty)
			first(&c.Level, r.Level)
			if c.Lanes == 0 {
				c.Lanes = r.Lanes
			}
			if c.Notes == 0 {
				c.Notes = r.Notes
			}
			c.Scanned = c.Scanned || r.Scanned
			c.Tier = maxTier(c.Tier, r.Tier)
			c.Plays = max(c.Plays, r.Plays)
		}
		sort.Strings(c.IDs)
		c.Key = chartKey(c.IDs)
		out = append(out, c)
	}
	return out
}

// chartKey prefers the fingerprint, then file md5, so a key survives new sightings.
func chartKey(ids []string) string {
	for _, prefix := range []string{"fp:", "md5:", "eo:"} {
		for _, id := range ids {
			if strings.HasPrefix(id, prefix) {
				return id
			}
		}
	}
	if len(ids) == 0 {
		return ""
	}
	return ids[0]
}

func addPack(list []PackRef, p PackRef) []PackRef {
	for i, q := range list {
		if q.Source == p.Source && q.ID == p.ID {
			if q.Path == "" {
				list[i].Path = p.Path
			}
			if q.Title == "" {
				list[i].Title = p.Title
			}
			return list
		}
	}
	return append(list, p)
}

func first(dst *string, v string) {
	if *dst == "" {
		*dst = v
	}
}

func (s Store) WriteCharts(source string, recs []ChartRecord) error {
	return writeJSONL(filepath.Join(s.Root, "charts"), source, recs)
}

func (s Store) ReadCharts(source string) ([]ChartRecord, error) {
	var out []ChartRecord
	err := readLines(filepath.Join(s.Root, "charts", source+".jsonl"), func(b []byte) error {
		var r ChartRecord
		if err := json.Unmarshal(b, &r); err != nil {
			return err
		}
		out = append(out, r)
		return nil
	})
	return out, err
}

func (s Store) Scans(source string) (map[string]ScanResult, error) {
	out := map[string]ScanResult{}
	err := readLines(filepath.Join(s.Root, "scan", source+".jsonl"), func(b []byte) error {
		var r ScanResult
		if err := json.Unmarshal(b, &r); err != nil {
			return err
		}
		out[r.ID] = r
		return nil
	})
	return out, err
}

func (s Store) RecordScan(source string, r ScanResult) error {
	return appendJSONL(filepath.Join(s.Root, "scan"), source, r)
}

// AllCharts merges remote records and scans of every source.
func (s Store) AllCharts() ([]Chart, error) {
	var recs []ChartRecord
	for _, src := range Sources {
		r, err := s.ReadCharts(src)
		if err != nil {
			return nil, err
		}
		recs = append(recs, r...)
		scans, err := s.Scans(src)
		if err != nil {
			return nil, err
		}
		ids := make([]string, 0, len(scans))
		for id := range scans {
			ids = append(ids, id)
		}
		sort.Strings(ids)
		for _, id := range ids {
			recs = append(recs, scans[id].Charts...)
		}
	}
	return MergeCharts(recs), nil
}
