package chart

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// Copy accepts only editable content. Asset paths, audio and storyboard come from
// the loaded original, never from an untrusted draft.
func CopyEdit(original, draft *Chart) (*Chart, error) {
	if original == nil || draft == nil || draft.Lanes != original.Lanes {
		return nil, fmt.Errorf("edit: invalid chart or lane count")
	}
	c := *original
	c.Metadata = draft.Metadata
	c.Notes = draft.Notes
	c.Timing = draft.Timing
	format := original.Source.OriginalFormat
	if format == "" && original.Source.Format != "ggc" {
		format = original.Source.Format
	}
	c.Source = Source{Format: "ggc", OriginalFormat: format}
	c.Normalize()
	if err := c.Validate(); err != nil {
		return nil, fmt.Errorf("edit: %w", err)
	}
	return &c, nil
}

// SaveEdit writes a new .ggc; importing or saving again never overwrites a source.
// Keeping the copy in the same directory preserves relative asset references.
func SaveEdit(source string, original, draft *Chart) (string, error) {
	c, err := CopyEdit(original, draft)
	if err != nil {
		return "", err
	}
	return CreateEdit(source, ".ggc", func(path string, w io.Writer) error {
		c.Source.Path = path
		return Write(w, c)
	})
}

// CreateEdit writes a new <source>-edit-<n><ext> beside source, removing it if write fails.
func CreateEdit(source, ext string, write func(path string, w io.Writer) error) (string, error) {
	stem := strings.TrimSuffix(filepath.Base(source), filepath.Ext(source))
	f, err := os.CreateTemp(filepath.Dir(source), stem+"-edit-*"+ext)
	if err != nil {
		return "", fmt.Errorf("edit: create copy: %w", err)
	}
	path := f.Name()
	writeErr := write(path, f)
	closeErr := f.Close()
	if writeErr == nil {
		writeErr = closeErr
	}
	if writeErr != nil {
		if err := os.Remove(path); err != nil {
			return "", fmt.Errorf("edit: write copy: %v; cleanup: %w", writeErr, err)
		}
		return "", fmt.Errorf("edit: write copy: %w", writeErr)
	}
	return path, nil
}
