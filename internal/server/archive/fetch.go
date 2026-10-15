package archive

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// FetchOptions bounds one download run.
type FetchOptions struct {
	Limit   int    // items to attempt, 0 for no limit
	Budget  int64  // stop after this many new bytes, 0 for no limit
	MaxSize int64  // skip files above this size, 0 for no limit
	Keys    []int  // keep items with any of these key counts
	Status  string // keep items with this status
	Query   string // keep items whose title, artist or creator contains this
	Table   string // BMS: keep items listed in a table whose symbol+level starts with this, e.g. sl
	Tier    string // keep items at this tier or higher
	Top     int    // keep only the N most played of the remaining items, most played first
	Retry   bool   // retry items whose last attempt failed
	DryRun  bool   // count what would be downloaded without touching the network
	Reserve int64  // stop before the disk holding the source's files would drop below this many free bytes
}

// freeSpace is a variable so tests can simulate a filling disk.
var freeSpace = diskFree

type FetchStats struct {
	Done, Failed, Skipped int
	Bytes                 int64
	Unknown               int // dry run: items whose size is not known in advance
}

// Fetch downloads indexed items that are not yet stored. Interrupted files resume on the next run.
func Fetch(ctx context.Context, c *Client, st Store, source string, opt FetchOptions) (FetchStats, error) {
	var stats FetchStats
	items, err := st.ReadIndex(source)
	if err != nil {
		return stats, err
	}
	results, err := st.Results(source)
	if err != nil {
		return stats, err
	}
	var picked []Item
	for _, it := range items {
		if opt.match(it) {
			picked = append(picked, it)
		}
	}
	if opt.Top > 0 {
		// Stored items count toward the top N so a rerun keeps the same selection.
		sort.SliceStable(picked, func(i, j int) bool { return picked[i].Plays > picked[j].Plays })
		picked = picked[:min(opt.Top, len(picked))]
	}
	// Measured on the source's own directory, which may be a junction or mount on another disk.
	filesDir := filepath.Join(st.Root, "files", source)
	if opt.Reserve > 0 && !opt.DryRun {
		if err := os.MkdirAll(filesDir, 0o755); err != nil {
			return stats, err
		}
	}
	attempts := 0
	for _, it := range picked {
		if ctx.Err() != nil {
			return stats, ctx.Err()
		}
		dir, err := st.Dir(source, it.ID)
		if err != nil {
			return stats, err
		}
		if r, ok := results[it.ID]; ok {
			if r.File != "" && exists(filepath.Join(dir, r.File)) {
				continue
			}
			if r.Error != "" && !opt.Retry {
				stats.Skipped++
				continue
			}
		}
		if opt.MaxSize > 0 && it.Size > opt.MaxSize {
			stats.Skipped++
			continue
		}
		if opt.Budget > 0 && stats.Bytes+it.Size > opt.Budget {
			c.Log("budget reached")
			return stats, nil
		}
		if opt.Limit > 0 && attempts >= opt.Limit {
			return stats, nil
		}
		attempts++
		if opt.DryRun {
			stats.Done++
			stats.Bytes += it.Size
			if it.Size == 0 {
				stats.Unknown++
			}
			continue
		}
		if opt.Reserve > 0 {
			free, err := freeSpace(filesDir)
			if err != nil {
				return stats, err
			}
			// An item of unknown size may be as large as the per-file limit.
			need := it.Size
			if need == 0 {
				need = opt.MaxSize
			}
			if free-need < opt.Reserve {
				c.Log("disk reserve reached: %d bytes free", free)
				return stats, nil
			}
		}
		r := fetchItem(ctx, c, source, it, dir, opt.MaxSize)
		if ctx.Err() != nil {
			return stats, ctx.Err()
		}
		if err := st.Record(source, r); err != nil {
			return stats, err
		}
		if r.Error != "" {
			stats.Failed++
			c.Log("%s/%s %s: %s", source, it.ID, it.Title, r.Error)
			continue
		}
		stats.Done++
		stats.Bytes += r.Size
		c.Log("%s/%s %s: %s (%d bytes)", source, it.ID, it.Title, r.File, r.Size)
		if opt.Budget > 0 && stats.Bytes >= opt.Budget {
			c.Log("budget reached")
			return stats, nil
		}
	}
	return stats, nil
}

func fetchItem(ctx context.Context, c *Client, source string, it Item, dir string, maxSize int64) Result {
	r := Result{ID: it.ID, At: time.Now().UTC()}
	var errs []string
	for _, link := range it.Links {
		urls := []string{link}
		if source == SourceBMS {
			var err error
			if urls, err = Resolve(ctx, c, link); err != nil {
				errs = append(errs, err.Error())
				continue
			}
		}
		for _, u := range urls {
			name, size, sum, err := c.Download(ctx, u, dir, maxSize, it.Kind == "diff")
			if err == nil {
				r.File, r.Size, r.SHA256, r.URL = name, size, sum, u
				return r
			}
			if ctx.Err() != nil {
				r.Error = ctx.Err().Error()
				return r
			}
			errs = append(errs, err.Error())
			if errors.Is(err, ErrTooLarge) {
				r.Error = strings.Join(errs, "; ")
				return r
			}
		}
	}
	if len(errs) == 0 {
		errs = append(errs, "no links")
	}
	r.Error = strings.Join(errs, "; ")
	return r
}

func (o FetchOptions) match(it Item) bool {
	if o.Status != "" && !strings.EqualFold(it.Status, o.Status) {
		return false
	}
	if !atLeast(it.Tier, o.Tier) {
		return false
	}
	if len(o.Keys) > 0 && !anyKey(it.Keys, o.Keys) {
		return false
	}
	if o.Query != "" && !matchQuery(it, o.Query) {
		return false
	}
	if o.Table != "" && !anyPrefix(it.Tables, o.Table) {
		return false
	}
	return true
}

func anyKey(have, want []int) bool {
	for _, h := range have {
		for _, w := range want {
			if h == w {
				return true
			}
		}
	}
	return false
}

func anyPrefix(list []string, prefix string) bool {
	for _, v := range list {
		if strings.HasPrefix(v, prefix) {
			return true
		}
	}
	return false
}

func exists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}
