package chart

import (
	"bytes"
	"crypto/md5"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"path/filepath"
	"strings"
)

// Format converts one source chart format. A format package registers its
// Format from init, so callers import it for that effect, as with image/png.
type Format interface {
	// Extensions lists the lower-case file extensions, with the dot, it reads.
	Extensions() []string
	// Parse returns every difficulty in data; Decode normalizes and validates them.
	Parse(data []byte) ([]*Chart, error)
}

// Merger is a Format whose charts take in sibling files, such as osu! .osb storyboards.
type Merger interface {
	Format
	MergeExtensions() []string
	Merge(c *Chart, data []byte) error
}

// BankFormat is a Format that also decodes sample banks, such as O2Jam .ojm files.
type BankFormat interface {
	Format
	BankExtensions() []string
	ReadBank(data []byte) (map[int]Sample, error)
}

// Sample is an encoded WAV or Ogg payload from a sample bank.
type Sample struct {
	Data      []byte
	Extension string
}

var (
	formats = map[string]Format{}
	banks   = map[string]BankFormat{}
)

// Register makes f available to Decode, Load and ReadBank. Call it from init.
func Register(f Format) {
	for _, ext := range f.Extensions() {
		if _, dup := formats[ext]; dup || ext == ".ggc" {
			panic("chart: format registered twice for " + ext)
		}
		formats[ext] = f
	}
	if b, ok := f.(BankFormat); ok {
		for _, ext := range b.BankExtensions() {
			banks[ext] = b
		}
	}
}

func Supported(name string) bool {
	ext := strings.ToLower(filepath.Ext(name))
	return ext == ".ggc" || formats[ext] != nil
}

// Decode returns every difficulty. Source identity includes the original bytes
// and difficulty index. It never opens asset paths supplied by the source.
func Decode(name string, r io.Reader) ([]*Chart, error) {
	b, err := io.ReadAll(io.LimitReader(r, MaxSize+1))
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", name, err)
	}
	if len(b) > MaxSize {
		return nil, fmt.Errorf("%s: file exceeds %d bytes", name, MaxSize)
	}
	ext := strings.ToLower(filepath.Ext(name))
	var charts []*Chart
	if f, ok := formats[ext]; ok {
		charts, err = f.Parse(b)
	} else if ext == ".ggc" {
		var c *Chart
		c, err = Read(bytes.NewReader(b))
		if err == nil {
			charts = []*Chart{c}
		}
	} else {
		err = fmt.Errorf("unsupported chart extension %q", filepath.Ext(name))
	}
	if err != nil {
		return nil, fmt.Errorf("%s: %w", name, err)
	}
	sha := sha256.Sum256(b)
	md := md5.Sum(b)
	for i, c := range charts {
		if ext != ".ggc" {
			c.Source = Source{Format: strings.TrimPrefix(ext, "."), Path: name, Index: i, SHA256: hex.EncodeToString(sha[:]), MD5: hex.EncodeToString(md[:])}
		} else {
			c.Source.Path = name
		}
		// Parsers see only bytes; an embedded asset with no path lives in the chart file itself.
		for id, a := range c.Assets {
			if a.Size > 0 && a.Path == "" {
				a.Path = filepath.Base(name)
				c.Assets[id] = a
			}
		}
		c.Normalize()
		if err := c.Validate(); err != nil {
			return nil, fmt.Errorf("%s difficulty %d: %w", name, i, err)
		}
	}
	return charts, nil
}

// ReadBank decodes the sample bank file name using its registered format.
// A malformed record can return complete earlier samples alongside an error.
func ReadBank(name string, data []byte) (map[int]Sample, error) {
	b, ok := banks[strings.ToLower(filepath.Ext(name))]
	if !ok {
		return nil, fmt.Errorf("unsupported sample bank %q", filepath.Ext(name))
	}
	return b.ReadBank(data)
}
