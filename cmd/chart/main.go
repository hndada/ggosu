// Command chart inspects, exports and verifies charts without opening a window.
package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"ggosu/internal/chart"
	_ "ggosu/internal/chart/bms"
	_ "ggosu/internal/chart/ojn"
	_ "ggosu/internal/chart/osu"
	_ "ggosu/internal/chart/sm"
	"ggosu/internal/piano"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
func run() error {
	path := flag.String("input", "", "source chart")
	index := flag.Int("index", 0, "difficulty index for export")
	out := flag.String("out", "", "export universal .ggc beside the source")
	verify := flag.Bool("verify", false, "autoplay each chart using the real judgement path")
	flag.Parse()
	if *path == "" {
		return fmt.Errorf("-input is required")
	}
	cs, err := chart.Load(*path)
	if err != nil {
		return err
	}
	for i, c := range cs {
		hash, err := c.Digest()
		if err != nil {
			return err
		}
		row := struct {
			Index        int            `json:"index"`
			Metadata     chart.Metadata `json:"metadata"`
			Lanes, Notes int
			DurationNS   int64        `json:"duration_ns"`
			Hash         string       `json:"hash"`
			Warnings     []string     `json:"warnings,omitempty"`
			Fixes        []string     `json:"fixes,omitempty"`
			Score        *piano.Score `json:"score,omitempty"`
		}{Index: i, Metadata: c.Metadata, Lanes: c.Lanes, Notes: len(c.Notes), DurationNS: int64(c.Duration()), Hash: hash, Warnings: c.Warnings, Fixes: c.Fixes}
		if *verify {
			f, err := piano.Auto(c, piano.Rules)
			if err != nil {
				return err
			}
			s, err := piano.NewSession(c)
			if err != nil {
				return err
			}
			for _, e := range f.Events {
				if err := s.Input(e.At, int(e.Key), e.Down); err != nil {
					return err
				}
			}
			if err := s.Advance(f.Duration); err != nil {
				return err
			}
			row.Score = &s.Score
			if !s.Done() || s.Score.Miss > 0 {
				return fmt.Errorf("difficulty %d: autoplay failed: %+v", i, s.Score)
			}
		}
		if err := json.NewEncoder(os.Stdout).Encode(row); err != nil {
			return err
		}
	}
	if *out != "" {
		if !strings.EqualFold(filepath.Ext(*out), ".ggc") {
			return fmt.Errorf("export requires a .ggc extension")
		}
		srcPath, err := filepath.Abs(*path)
		if err != nil {
			return err
		}
		outPath, err := filepath.Abs(*out)
		if err != nil {
			return err
		}
		if strings.EqualFold(srcPath, outPath) {
			return fmt.Errorf("export cannot replace the source chart")
		}
		if *index < 0 || *index >= len(cs) {
			return fmt.Errorf("index out of range")
		}
		srcDir, err := filepath.Abs(filepath.Dir(*path))
		if err != nil {
			return err
		}
		outDir, err := filepath.Abs(filepath.Dir(*out))
		if err != nil {
			return err
		}
		if srcDir != outDir && len(cs[*index].Assets) > 0 {
			return fmt.Errorf("export with assets must stay beside source (relative asset paths)")
		}
		return export(*out, cs[*index])
	}
	return nil
}

func export(path string, c *chart.Chart) (err error) {
	f, err := os.CreateTemp(filepath.Dir(path), ".chart-*")
	if err != nil {
		return err
	}
	temp := f.Name()
	defer func() {
		if e := os.Remove(temp); e != nil && !os.IsNotExist(e) {
			err = errors.Join(err, e)
		}
	}()
	writeErr := chart.Write(f, c)
	if writeErr == nil {
		writeErr = f.Sync()
	}
	if err := errors.Join(writeErr, f.Close()); err != nil {
		return err
	}
	return os.Rename(temp, path)
}
