package chart

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// Load decodes the chart file at path and merges sibling files its format asks for.
func Load(path string) ([]*Chart, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	charts, readErr := Decode(path, f)
	closeErr := f.Close()
	if readErr != nil {
		return nil, readErr
	}
	if closeErr != nil {
		return nil, closeErr
	}
	m, ok := formats[strings.ToLower(filepath.Ext(path))].(Merger)
	if !ok {
		return charts, nil
	}
	files, err := os.ReadDir(filepath.Dir(path))
	if err != nil {
		return nil, err
	}
	for _, file := range files {
		if file.IsDir() || !slices.Contains(m.MergeExtensions(), strings.ToLower(filepath.Ext(file.Name()))) {
			continue
		}
		p, err := AssetPath(path, file.Name())
		if err != nil {
			return nil, err
		}
		f, err := os.Open(p)
		if err != nil {
			return nil, err
		}
		b, err := io.ReadAll(io.LimitReader(f, MaxSize+1))
		closeErr := f.Close()
		if err == nil {
			err = closeErr
		}
		if err != nil {
			return nil, err
		}
		for _, c := range charts {
			if err := m.Merge(c, b); err != nil {
				return nil, fmt.Errorf("%s: %w", p, err)
			}
		}
	}
	return charts, nil
}
