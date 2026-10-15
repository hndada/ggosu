// Command archive indexes remote chart sources, downloads them and serves the result.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"ggosu/internal/server/archive"
)

const usage = `usage: archive <index|fetch|scan|serve|stats> [flags]

  index  -source all            rebuild the pack list and chart records of each source
  fetch  -source osu -budget 10GB  plan downloads; add -download to really fetch
  scan                          read charts inside downloaded packs
  serve  -addr :8090            serve pack/chart search API and stored files
  stats                         print index, download and chart counts`

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, usage)
		os.Exit(2)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	if err := run(ctx, os.Args[1], os.Args[2:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(ctx context.Context, cmd string, args []string) error {
	fs := flag.NewFlagSet(cmd, flag.ExitOnError)
	root := fs.String("root", "data/archive", "archive storage root")
	source := fs.String("source", "all", "comma list of "+strings.Join(archive.Sources, ",")+" or all")
	ua := fs.String("ua", "ggosu-archive/0.1", "User-Agent sent to sources; add contact info")
	interval := fs.Duration("interval", time.Second, "minimum gap between requests to one host")
	tags := fs.String("bms-tags", "General", "DARKSABUN table tags to index, comma list, empty for all")
	limit := fs.Int("limit", 0, "fetch: items to attempt, 0 for no limit")
	budget := fs.String("budget", "10GB", "fetch: stop after this many new bytes, 0 for no limit")
	maxSize := fs.String("max-size", "2GB", "fetch: skip files above this size, 0 for no limit")
	keys := fs.String("keys", "", "fetch: key counts, e.g. 4,7")
	status := fs.String("status", "", "fetch: item status, e.g. ranked")
	query := fs.String("q", "", "fetch: title/artist/creator words")
	tier := fs.String("tier", "", "fetch: minimum tier: official, candidate or unvetted")
	top := fs.Int("top", 0, "fetch: only the N most played packs that pass the other filters")
	table := fs.String("table", "", "fetch: BMS table symbol+level prefix, e.g. sl or st3")
	retry := fs.Bool("retry", false, "fetch: retry failed items")
	download := fs.Bool("download", false, "fetch: really download; without it fetch only reports the plan")
	reserve := fs.String("reserve", "20GB", "fetch: stop before the disk holding the files drops below this much free space, 0 for no check")
	charts := fs.Bool("charts", true, "index: also crawl chart lists (Etterna songs, BMS Search); false updates packs only")
	sevenZip := fs.String("7z", "", "scan: 7z executable for rar/7z/lzh, default from PATH or Program Files")
	addr := fs.String("addr", ":8090", "serve: listen address")
	reload := fs.Duration("reload", 5*time.Minute, "serve: index reload interval")
	if err := fs.Parse(args); err != nil {
		return err
	}
	st := archive.Store{Root: *root}
	c := archive.NewClient(*ua, *interval)
	c.Log = log.Printf
	sources, err := pickSources(*source)
	if err != nil {
		return err
	}
	switch cmd {
	case "index":
		var failed []error
		for _, src := range sources {
			cat, err := archive.Index(ctx, c, src, archive.Options{BMSTags: splitList(*tags), Charts: *charts})
			if err != nil {
				failed = append(failed, fmt.Errorf("%s: %w", src, err))
				continue
			}
			if err := st.WriteIndex(src, cat.Items); err != nil {
				return err
			}
			// Without -charts the chart crawl is partial, so the previous chart records stay.
			if *charts {
				if err := st.WriteCharts(src, cat.Charts); err != nil {
					return err
				}
			}
			log.Printf("%s: %d packs, %d chart records", src, len(cat.Items), len(cat.Charts))
		}
		return errors.Join(failed...)
	case "scan":
		exe := *sevenZip
		if exe == "" {
			exe = findSevenZip()
		}
		if exe == "" {
			log.Printf("7z not found: rar, 7z and lzh archives stay unscanned (set -7z)")
		}
		for _, src := range sources {
			s, err := archive.Scan(ctx, st, src, archive.ScanOptions{SevenZip: exe, Retry: *retry}, log.Printf)
			log.Printf("%s: %d packs scanned, %d charts, %d failed, %d unchanged", src, s.Packs, s.Charts, s.Failed, s.Skipped)
			if err != nil {
				return err
			}
		}
		return nil
	case "fetch":
		b, err := parseBytes(*budget)
		if err != nil {
			return err
		}
		m, err := parseBytes(*maxSize)
		if err != nil {
			return err
		}
		minTier, err := archive.ParseTier(*tier)
		if err != nil {
			return err
		}
		res, err := parseBytes(*reserve)
		if err != nil {
			return err
		}
		opt := archive.FetchOptions{Limit: *limit, Budget: b, MaxSize: m, Status: *status, Query: *query, Table: *table,
			Tier: minTier, Top: *top, Retry: *retry, DryRun: !*download, Reserve: res}
		for _, k := range splitList(*keys) {
			n, err := strconv.Atoi(k)
			if err != nil {
				return fmt.Errorf("bad -keys: %w", err)
			}
			opt.Keys = append(opt.Keys, n)
		}
		for _, src := range sources {
			s, err := archive.Fetch(ctx, c, st, src, opt)
			if opt.DryRun {
				log.Printf("%s (dry run, add -download to start): %d items, %d known bytes, %d of unknown size, %d skipped", src, s.Done, s.Bytes, s.Unknown, s.Skipped)
			} else {
				log.Printf("%s: %d stored, %d failed, %d skipped, %d bytes", src, s.Done, s.Failed, s.Skipped, s.Bytes)
			}
			if err != nil {
				return err
			}
			if opt.Budget > 0 {
				if opt.Budget -= s.Bytes; opt.Budget <= 0 {
					return nil
				}
			}
		}
		return nil
	case "serve":
		srv, err := archive.NewServer(st)
		if err != nil {
			return err
		}
		go func() {
			t := time.NewTicker(*reload)
			defer t.Stop()
			for {
				select {
				case <-ctx.Done():
					return
				case <-t.C:
					if err := srv.Reload(); err != nil {
						log.Printf("reload: %v", err)
					}
				}
			}
		}()
		hs := &http.Server{Addr: *addr, Handler: srv.Handler(), ReadHeaderTimeout: 10 * time.Second}
		go func() {
			<-ctx.Done()
			shut, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if err := hs.Shutdown(shut); err != nil {
				log.Printf("shutdown: %v", err)
			}
		}()
		log.Printf("serving %s on %s", *root, *addr)
		if err := hs.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
			return err
		}
		return nil
	case "stats":
		out := map[string]any{}
		for _, src := range sources {
			items, err := st.ReadIndex(src)
			if err != nil {
				return err
			}
			results, err := st.Results(src)
			if err != nil {
				return err
			}
			var stored, failed int
			var size, bytes int64
			type tierCount struct {
				Packs int   `json:"packs"`
				Bytes int64 `json:"indexed_bytes"`
			}
			tiers := map[string]*tierCount{}
			for _, it := range items {
				size += it.Size
				t := it.Tier
				if t == "" {
					t = archive.TierUnvetted
				}
				if tiers[t] == nil {
					tiers[t] = &tierCount{}
				}
				tiers[t].Packs++
				tiers[t].Bytes += it.Size
				if r, ok := results[it.ID]; ok && r.File != "" {
					stored++
					bytes += r.Size
				} else if ok {
					failed++
				}
			}
			recs, err := st.ReadCharts(src)
			if err != nil {
				return err
			}
			out[src] = map[string]any{"packs": len(items), "indexed_bytes": size, "stored": stored, "stored_bytes": bytes,
				"failed": failed, "chart_records": len(recs), "tiers": tiers}
		}
		charts, err := st.AllCharts()
		if err != nil {
			return err
		}
		counts := map[string]int{"merged": len(charts)}
		for _, c := range charts {
			if len(c.Packs) > 1 {
				counts["in_several_packs"]++
			}
			t := c.Tier
			if t == "" {
				t = archive.TierUnvetted
			}
			counts[t]++
		}
		out["charts"] = counts
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(out)
	}
	return fmt.Errorf("unknown command %q\n%s", cmd, usage)
}

func findSevenZip() string {
	for _, name := range []string{"7z", "7za"} {
		if p, err := exec.LookPath(name); err == nil {
			return p
		}
	}
	p := filepath.Join(os.Getenv("ProgramFiles"), "7-Zip", "7z.exe")
	if _, err := os.Stat(p); err == nil {
		return p
	}
	return ""
}

func pickSources(s string) ([]string, error) {
	if s == "all" {
		return archive.Sources, nil
	}
	var out []string
	for _, v := range splitList(s) {
		ok := false
		for _, known := range archive.Sources {
			ok = ok || v == known
		}
		if !ok {
			return nil, fmt.Errorf("unknown source %q", v)
		}
		out = append(out, v)
	}
	return out, nil
}

func splitList(s string) []string {
	var out []string
	for _, f := range strings.Split(s, ",") {
		if f = strings.TrimSpace(f); f != "" {
			out = append(out, f)
		}
	}
	return out
}

// parseBytes reads sizes like 500MB, 10GB or 0.
func parseBytes(s string) (int64, error) {
	s = strings.ToUpper(strings.TrimSpace(s))
	mul := int64(1)
	for _, u := range []struct {
		suffix string
		n      int64
	}{{"TB", 1 << 40}, {"GB", 1 << 30}, {"MB", 1 << 20}, {"KB", 1 << 10}, {"B", 1}} {
		if strings.HasSuffix(s, u.suffix) {
			s, mul = strings.TrimSpace(strings.TrimSuffix(s, u.suffix)), u.n
			break
		}
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil || f < 0 {
		return 0, fmt.Errorf("bad size %q", s)
	}
	return int64(f * float64(mul)), nil
}
