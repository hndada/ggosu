//go:build !js

package play

import (
	"bufio"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"ggosu/internal/chart"
)

// Bump the magic when codec/resampling output changes. Cached data is always
// 48kHz stereo signed 16-bit PCM, independent of rate, pitch and judgement.
const pcmMagic = "GOSUPCM1"
const pcmQuota = int64(2 << 30)

var errCacheMiss = errors.New("PCM cache incomplete")

func soundChart(charts []*chart.Chart) *chart.Chart {
	c := *charts[0]
	c.Assets = make(map[string]chart.Asset)
	for _, sibling := range charts {
		for _, a := range sibling.Assets {
			if a.Kind == "audio" || a.Kind == "bank" {
				c.Assets[soundKey(a)] = a
			}
		}
	}
	return &c
}

func soundKey(a chart.Asset) string {
	if a.Source != "" {
		return fmt.Sprintf("%s\x00%s\x00%s\x00%d", a.Kind, a.Source, a.Path, a.Sample)
	}
	return fmt.Sprintf("%s\x00%s\x00%d", a.Kind, a.Path, a.Sample)
}

func pcmPath(c *chart.Chart, dir string) (string, error) {
	if dir == "" {
		base, err := os.UserCacheDir()
		if err != nil {
			return "", err
		}
		dir = filepath.Join(base, "ggosu", "pcm")
	}
	paths := make(map[string]bool)
	seen := make(map[string]bool)
	for _, a := range c.Assets {
		source := c.Source.Path
		if a.Source != "" {
			source = a.Source
		}
		if seen[source+"|"+a.Path] {
			continue
		}
		seen[source+"|"+a.Path] = true
		p, err := chart.AssetPath(source, a.Path)
		if err != nil {
			return "", err
		}
		p, err = filepath.Abs(p)
		if err != nil {
			return "", err
		}
		paths[p] = true
	}
	names := make([]string, 0, len(paths))
	for p := range paths {
		names = append(names, p)
	}
	slices.Sort(names)
	h := sha256.New()
	fmt.Fprintln(h, pcmMagic, sampleRate)
	for _, p := range names {
		f, err := os.Stat(p)
		if err != nil {
			return "", err
		}
		fmt.Fprintf(h, "%q %d %d\n", p, f.Size(), f.ModTime().UnixNano())
	}
	return filepath.Join(dir, fmt.Sprintf("%x.pcm", h.Sum(nil))), nil
}

// readPCM skips unneeded sibling samples and validates checksums before use.
func readPCM(path string, c *chart.Chart) (pcm map[string][]byte, err error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { err = errors.Join(err, f.Close()) }()
	r := bufio.NewReaderSize(f, 128<<10)
	var magic [8]byte
	if _, err := io.ReadFull(r, magic[:]); err != nil || string(magic[:]) != pcmMagic {
		return nil, errCacheMiss
	}
	var count uint32
	if err := binary.Read(r, binary.LittleEndian, &count); err != nil || count > 100000 {
		return nil, errCacheMiss
	}
	needed := make(map[string][]string)
	for id, a := range c.Assets {
		if a.Kind == "audio" || a.Kind == "bank" {
			key := fmt.Sprintf("%x", sha256.Sum256([]byte(soundKey(a))))
			needed[key] = append(needed[key], id)
		}
	}
	out := make(map[string][]byte)
	used := 0
	for range count {
		var key, sum [32]byte
		var size uint32
		if _, err := io.ReadFull(r, key[:]); err != nil {
			return nil, err
		}
		if err := binary.Read(r, binary.LittleEndian, &size); err != nil {
			return nil, err
		}
		if _, err := io.ReadFull(r, sum[:]); err != nil {
			return nil, err
		}
		if size > maxPCM || size%4 != 0 {
			return nil, errCacheMiss
		}
		k := fmt.Sprintf("%x", key)
		ids := needed[k]
		if len(ids) == 0 {
			if _, err := r.Discard(int(size)); err != nil {
				return nil, err
			}
			continue
		}
		if int(size) > maxPCM-used {
			return nil, errPCM
		}
		data := make([]byte, size)
		if _, err := io.ReadFull(r, data); err != nil {
			return nil, err
		}
		if sha256.Sum256(data) != sum {
			return nil, errCacheMiss
		}
		used += len(data)
		if size > 0 {
			for _, id := range ids {
				out[id] = data
			}
		}
		delete(needed, k)
	}
	if len(needed) != 0 {
		return nil, errCacheMiss
	}
	return out, nil
}

func writePCM(path string, c *chart.Chart, pcm map[string][]byte) (err error) {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	f, err := os.CreateTemp(dir, fmt.Sprintf(".pcm-%d-*", os.Getpid()))
	if err != nil {
		return err
	}
	closed := false
	defer func() {
		if !closed {
			err = errors.Join(err, f.Close())
		}
		if e := os.Remove(f.Name()); e != nil && !os.IsNotExist(e) {
			err = errors.Join(err, e)
		}
	}()
	w := bufio.NewWriterSize(f, 128<<10)
	if _, err := io.WriteString(w, pcmMagic); err != nil {
		return err
	}
	if err := binary.Write(w, binary.LittleEndian, uint32(len(c.Assets))); err != nil {
		return err
	}
	for id := range c.Assets {
		data := pcm[id]
		key := sha256.Sum256([]byte(id))
		sum := sha256.Sum256(data)
		if _, err := w.Write(key[:]); err != nil {
			return err
		}
		if err := binary.Write(w, binary.LittleEndian, uint32(len(data))); err != nil {
			return err
		}
		if _, err := w.Write(sum[:]); err != nil {
			return err
		}
		if _, err := w.Write(data); err != nil {
			return err
		}
	}
	if err := w.Flush(); err != nil {
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	closed = true
	return os.Rename(f.Name(), path)
}

func trimPCM(dir string, keep string, quota int64) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	type file struct {
		path string
		size int64
		at   time.Time
	}
	var files []file
	var total int64
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".pcm") && !strings.HasPrefix(e.Name(), ".pcm-") {
			continue
		}
		info, err := e.Info()
		if err != nil {
			return err
		}
		p := filepath.Join(dir, e.Name())
		if strings.HasPrefix(e.Name(), ".pcm-") {
			if time.Since(info.ModTime()) > 24*time.Hour {
				if err := os.Remove(p); err != nil && !os.IsNotExist(err) {
					return err
				}
			}
			continue
		}
		total += info.Size()
		files = append(files, file{p, info.Size(), info.ModTime()})
	}
	slices.SortFunc(files, func(a, b file) int { return a.at.Compare(b.at) })
	for _, f := range files {
		if total <= quota {
			break
		}
		if f.path == keep {
			continue
		}
		if err := os.Remove(f.path); err != nil && !os.IsNotExist(err) {
			return err
		}
		total -= f.size
	}
	return nil
}

func loadCachedBank(c *chart.Chart, charts []*chart.Chart, dir string) (*bank, error) {
	start := time.Now()
	all := soundChart(charts)
	path, pathErr := pcmPath(all, dir)
	if pathErr == nil {
		if pcm, err := readPCM(path, c); err == nil {
			now := time.Now()
			if err := os.Chtimes(path, now, now); err != nil {
				fmt.Fprintln(os.Stderr, "PCM cache touch:", err)
			}
			fmt.Fprintf(os.Stderr, "keysounds: cache hit, %s\n", time.Since(start))
			return &bank{pcm: pcm, rate: 1}, nil
		} else if !os.IsNotExist(err) && !errors.Is(err, errCacheMiss) {
			fmt.Fprintln(os.Stderr, "PCM cache read:", err)
		}
	}
	b, err := loadBank(c)
	if err != nil {
		return nil, err
	}
	if pathErr == nil && len(b.warnings) == 0 {
		stored := soundChart([]*chart.Chart{c})
		values := make(map[string][]byte)
		for id, a := range c.Assets {
			values[soundKey(a)] = b.pcm[id]
		}
		if err := writePCM(path, stored, values); err != nil {
			fmt.Fprintln(os.Stderr, "PCM cache write:", err)
		} else if err := trimPCM(filepath.Dir(path), path, pcmQuota); err != nil {
			fmt.Fprintln(os.Stderr, "PCM cache quota:", err)
		}
	}
	fmt.Fprintf(os.Stderr, "keysounds: decoded, %s\n", time.Since(start))
	return b, nil
}

// Warm prepares all sibling difficulties without opening a window or playing audio.
func Warm(path, dir string) error {
	charts, err := chart.Load(path)
	if err != nil {
		return err
	}
	if len(charts) == 0 {
		return fmt.Errorf("no charts")
	}
	b, err := loadCachedBank(soundChart(charts), charts, dir)
	if err != nil {
		return err
	}
	b.Close()
	return nil
}
