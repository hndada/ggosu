package archive

import (
	"archive/zip"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	"golang.org/x/text/encoding/japanese"

	"ggosu/internal/chart"
	_ "ggosu/internal/chart/bms"
	_ "ggosu/internal/chart/ojn"
	_ "ggosu/internal/chart/osu"
	_ "ggosu/internal/chart/sm"
)

// ScanOptions configures archive reading.
type ScanOptions struct {
	SevenZip string // 7z executable for rar, 7z and lzh; empty leaves those unscanned
	Retry    bool   // rescan files whose last scan failed
}

type ScanStats struct {
	Packs, Charts, Failed, Skipped int
}

// chartMasks lists what 7z extracts; everything else in an archive stays packed.
var chartMasks = []string{"*.bms", "*.bme", "*.bml", "*.sm", "*.osu", "*.ojn"}

// Scan reads the charts inside every stored file of source that changed since its last scan.
func Scan(ctx context.Context, st Store, source string, opt ScanOptions, log func(string, ...any)) (ScanStats, error) {
	var stats ScanStats
	items, err := st.ReadIndex(source)
	if err != nil {
		return stats, err
	}
	results, err := st.Results(source)
	if err != nil {
		return stats, err
	}
	scans, err := st.Scans(source)
	if err != nil {
		return stats, err
	}
	for _, it := range items {
		if ctx.Err() != nil {
			return stats, ctx.Err()
		}
		r, ok := results[it.ID]
		if !ok || r.File == "" {
			continue
		}
		if old, ok := scans[it.ID]; ok && old.SHA256 == r.SHA256 && (old.Error == "" || !opt.Retry) {
			stats.Skipped++
			continue
		}
		dir, err := st.Dir(source, it.ID)
		if err != nil {
			return stats, err
		}
		ref := PackRef{Source: source, ID: it.ID, Title: it.Title}
		recs, err := scanFile(ctx, filepath.Join(dir, r.File), ref, opt.SevenZip)
		// A scanned chart is as vetted as its pack; merging with metadata can raise it further.
		for i := range recs {
			recs[i].Tier = it.Tier
		}
		res := ScanResult{ID: it.ID, SHA256: r.SHA256, Charts: recs, At: time.Now().UTC()}
		if err != nil {
			res.Error = err.Error()
			stats.Failed++
			log("scan %s/%s %s: %v", source, it.ID, it.Title, err)
		} else {
			stats.Packs++
			stats.Charts += len(recs)
		}
		if err := st.RecordScan(source, res); err != nil {
			return stats, err
		}
	}
	return stats, nil
}

func scanFile(ctx context.Context, path string, ref PackRef, sevenZip string) ([]ChartRecord, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	head := make([]byte, 8)
	n, _ := io.ReadFull(f, head)
	if err := f.Close(); err != nil {
		return nil, err
	}
	head = head[:n]
	switch {
	case strings.HasPrefix(string(head), "PK\x03\x04"):
		return scanZip(path, ref)
	case isArchive(head):
		if sevenZip == "" {
			return nil, fmt.Errorf("%s archive needs -7z", archiveName(head))
		}
		return scan7z(ctx, sevenZip, path, ref)
	case chart.Supported(path):
		b, err := os.Open(path)
		if err != nil {
			return nil, err
		}
		defer b.Close()
		return readCharts(filepath.Base(path), b, ref)
	}
	return nil, fmt.Errorf("unknown file type")
}

func scanZip(path string, ref PackRef) ([]ChartRecord, error) {
	zr, err := zip.OpenReader(path)
	if err != nil {
		return nil, err
	}
	defer zr.Close()
	var recs []ChartRecord
	var errs []error
	for _, f := range zr.File {
		name := f.Name
		if f.NonUTF8 || !utf8.ValidString(name) {
			if d, err := japanese.ShiftJIS.NewDecoder().String(name); err == nil {
				name = d
			}
		}
		if f.FileInfo().IsDir() || !chart.Supported(name) || f.UncompressedSize64 > chart.MaxSize {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			errs = append(errs, err)
			continue
		}
		r, err := readCharts(name, rc, ref)
		rc.Close()
		if err != nil {
			errs = append(errs, err)
			continue
		}
		recs = append(recs, r...)
	}
	return recs, partial(recs, errs)
}

// scan7z extracts only chart files into a temporary directory, then reads them.
func scan7z(ctx context.Context, exe, path string, ref PackRef) ([]ChartRecord, error) {
	tmp, err := os.MkdirTemp("", "ggosu-scan-*")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(tmp)
	args := []string{"x", "-y", "-bd", "-o" + tmp, path}
	for _, m := range chartMasks {
		args = append(args, "-ir!"+m)
	}
	if out, err := exec.CommandContext(ctx, exe, args...).CombinedOutput(); err != nil {
		return nil, fmt.Errorf("7z: %v: %s", err, lastLine(string(out)))
	}
	var recs []ChartRecord
	var errs []error
	err = filepath.WalkDir(tmp, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !chart.Supported(p) {
			return err
		}
		f, err := os.Open(p)
		if err != nil {
			return err
		}
		defer f.Close()
		rel, _ := filepath.Rel(tmp, p)
		r, err := readCharts(filepath.ToSlash(rel), f, ref)
		if err != nil {
			errs = append(errs, err)
			return nil
		}
		recs = append(recs, r...)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return recs, partial(recs, errs)
}

func readCharts(name string, r io.Reader, ref PackRef) ([]ChartRecord, error) {
	charts, err := chart.Decode(name, r)
	if err != nil {
		return nil, err
	}
	ref.Path = name
	recs := make([]ChartRecord, 0, len(charts))
	for _, c := range charts {
		recs = append(recs, record(c, ref))
	}
	return recs, nil
}

func record(c *chart.Chart, ref PackRef) ChartRecord {
	m := c.Metadata
	ids := []string{"fp:" + Fingerprint(c), packKey(ref.Source, ref.ID, m.Title, m.Difficulty)}
	// The file md5 names a chart only where a file holds one chart; .sm and .ojn hold several.
	switch c.Source.Format {
	case "osu", "bms", "bme", "bml":
		ids = append(ids, "md5:"+c.Source.MD5)
	}
	return ChartRecord{
		IDs:    ids,
		Format: c.Source.Format, Title: m.Title, Artist: m.Artist, Creator: m.Creator,
		Difficulty: m.Difficulty, Level: m.Level, Lanes: c.Lanes, Notes: len(c.Notes),
		Packs: []PackRef{ref}, Scanned: true,
	}
}

// partial keeps charts read from a pack even when some files in it fail; it only fails a pack
// that has unreadable charts and nothing else.
func partial(recs []ChartRecord, errs []error) error {
	if len(recs) == 0 && len(errs) > 0 {
		return errors.Join(errs...)
	}
	if len(recs) == 0 {
		return fmt.Errorf("no charts in archive")
	}
	return nil
}

func archiveName(head []byte) string {
	switch {
	case strings.HasPrefix(string(head), "Rar!"):
		return "rar"
	case strings.HasPrefix(string(head), "7z"):
		return "7z"
	}
	return "lzh"
}

func lastLine(s string) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	return strings.TrimSpace(lines[len(lines)-1])
}
