// Package archive indexes remote chart sources, downloads their files and serves them back.
package archive

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"time"
)

const (
	SourceOsu     = "osu"
	SourceEtterna = "etterna"
	SourceSMO     = "smo"
	SourceBMS     = "bms"
)

// Sources lists every source in index order.
var Sources = []string{SourceOsu, SourceEtterna, SourceSMO, SourceBMS}

// Tiers say how far a chart was vetted by its community, highest first:
//
//	official   passed review or a vote: osu! ranked/approved/loved, EtternaOnline packs, main BMS tables
//	candidate  public and under review, or an author's own release: osu! qualified/pending,
//	           BMS sub, vote and personal tables, BMS Search event charts
//	unvetted   nothing reviewed it: osu! wip/graveyard, StepMania Online packs
const (
	TierOfficial  = "official"
	TierCandidate = "candidate"
	TierUnvetted  = "unvetted"
)

// tierRank orders tiers; an unknown or empty tier counts as unvetted.
func tierRank(t string) int {
	switch t {
	case TierOfficial:
		return 3
	case TierCandidate:
		return 2
	}
	return 1
}

// tierName spells an empty tier as unvetted for counts.
func tierName(t string) string {
	if t == "" {
		return TierUnvetted
	}
	return t
}

func maxTier(a, b string) string {
	if a == "" || tierRank(b) > tierRank(a) {
		return b
	}
	return a
}

// ParseTier accepts a tier name; empty means no filter.
func ParseTier(s string) (string, error) {
	switch s {
	case "", TierOfficial, TierCandidate, TierUnvetted:
		return s, nil
	}
	return "", fmt.Errorf("unknown tier %q (official, candidate, unvetted)", s)
}

// atLeast reports whether tier t passes a minimum tier; an empty minimum passes everything.
func atLeast(t, min string) bool {
	return min == "" || tierRank(t) >= tierRank(min)
}

func osuTier(status string) string {
	switch status {
	case "ranked", "approved", "loved":
		return TierOfficial
	case "qualified", "pending":
		return TierCandidate
	}
	return TierUnvetted
}

// Item is one downloadable unit: an osu! beatmapset, a StepMania pack, or a BMS song or sabun.
type Item struct {
	Source  string   `json:"source"`
	ID      string   `json:"id"`
	Kind    string   `json:"kind,omitempty"`   // BMS: song or diff
	Parent  string   `json:"parent,omitempty"` // BMS diff: song item ID
	Title   string   `json:"title"`
	Artist  string   `json:"artist,omitempty"`
	Creator string   `json:"creator,omitempty"`
	Keys    []int    `json:"keys,omitempty"`
	Levels  []string `json:"levels,omitempty"`
	Status  string   `json:"status,omitempty"`
	Tier    string   `json:"tier,omitempty"`  // official, candidate or unvetted
	Plays   int      `json:"plays,omitempty"` // source play count, 0 when unknown
	Size    int64    `json:"size,omitempty"`  // expected bytes, 0 when unknown
	Updated string   `json:"updated,omitempty"`
	Links   []string `json:"links"`            // original download URLs or pages, tried in order
	Charts  []string `json:"charts,omitempty"` // BMS chart md5
	Tables  []string `json:"tables,omitempty"` // BMS table symbol + level
}

// Result records one download attempt. The last line per ID wins.
type Result struct {
	ID     string    `json:"id"`
	File   string    `json:"file,omitempty"` // name inside the item directory
	Size   int64     `json:"size,omitempty"`
	SHA256 string    `json:"sha256,omitempty"`
	URL    string    `json:"url,omitempty"`
	Error  string    `json:"error,omitempty"`
	At     time.Time `json:"at"`
}

var validID = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)

// Store lays out one archive root:
//
//	index/<source>.jsonl   latest item (pack) snapshot
//	charts/<source>.jsonl  chart records from remote metadata
//	files/<source>/<id>/   downloaded file
//	state/<source>.jsonl   download log
//	scan/<source>.jsonl    charts read from downloaded files
type Store struct{ Root string }

func (s Store) Dir(source, id string) (string, error) {
	if !validSource(source) || !validID.MatchString(id) {
		return "", fmt.Errorf("invalid item %s/%s", source, id)
	}
	return filepath.Join(s.Root, "files", source, id), nil
}

func (s Store) WriteIndex(source string, items []Item) error {
	return writeJSONL(filepath.Join(s.Root, "index"), source, items)
}

// ReadIndex returns nil without error when the source was never indexed.
func (s Store) ReadIndex(source string) ([]Item, error) {
	var items []Item
	err := readLines(filepath.Join(s.Root, "index", source+".jsonl"), func(b []byte) error {
		var it Item
		if err := json.Unmarshal(b, &it); err != nil {
			return err
		}
		items = append(items, it)
		return nil
	})
	return items, err
}

func (s Store) Results(source string) (map[string]Result, error) {
	out := map[string]Result{}
	err := readLines(filepath.Join(s.Root, "state", source+".jsonl"), func(b []byte) error {
		var r Result
		if err := json.Unmarshal(b, &r); err != nil {
			return err
		}
		out[r.ID] = r
		return nil
	})
	return out, err
}

func (s Store) Record(source string, r Result) error {
	return appendJSONL(filepath.Join(s.Root, "state"), source, r)
}

// writeJSONL replaces dir/<source>.jsonl atomically.
func writeJSONL[T any](dir, source string, rows []T) (err error) {
	if !validSource(source) {
		return fmt.Errorf("unknown source %q", source)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	f, err := os.CreateTemp(dir, ".write-*")
	if err != nil {
		return err
	}
	temp := f.Name()
	defer func() {
		if e := os.Remove(temp); e != nil && !errors.Is(e, fs.ErrNotExist) {
			err = errors.Join(err, e)
		}
	}()
	w := bufio.NewWriter(f)
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	var writeErr error
	for _, row := range rows {
		if writeErr = enc.Encode(row); writeErr != nil {
			break
		}
	}
	if writeErr == nil {
		writeErr = w.Flush()
	}
	if err := errors.Join(writeErr, f.Close()); err != nil {
		return err
	}
	return os.Rename(temp, filepath.Join(dir, source+".jsonl"))
}

func appendJSONL(dir, source string, row any) error {
	if !validSource(source) {
		return fmt.Errorf("unknown source %q", source)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	b, err := json.Marshal(row)
	if err != nil {
		return err
	}
	f, err := os.OpenFile(filepath.Join(dir, source+".jsonl"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	_, err = f.Write(append(b, '\n'))
	return errors.Join(err, f.Close())
}

func readLines(path string, fn func([]byte) error) error {
	f, err := os.Open(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 64<<10), 16<<20)
	for n := 1; sc.Scan(); n++ {
		if len(sc.Bytes()) == 0 {
			continue
		}
		if err := fn(sc.Bytes()); err != nil {
			return fmt.Errorf("%s:%d: %w", path, n, err)
		}
	}
	return sc.Err()
}

func validSource(s string) bool {
	for _, v := range Sources {
		if v == s {
			return true
		}
	}
	return false
}
