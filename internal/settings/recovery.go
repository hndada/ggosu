package settings

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// ErrInvalid distinguishes damaged content from filesystem failures.
var ErrInvalid = errors.New("invalid settings")

type Recovery struct {
	Path   string // Preserved original, or empty if the primary file was missing.
	Backup bool
	Cause  error
}

// Recover restores invalid or missing settings from a valid backup, otherwise
// defaults. Permission and I/O errors are returned without resetting anything.
// The caller serializes access and presents the recovery notice to the user.
func Recover(path string) (Config, *Recovery, error) {
	c, err := Load(path)
	if err == nil {
		if _, statErr := os.Stat(path); !os.IsNotExist(statErr) {
			return c, nil, statErr
		}
		if _, statErr := os.Stat(path + ".bak"); os.IsNotExist(statErr) {
			return c, nil, nil
		} else if statErr != nil {
			return Config{}, nil, statErr
		}
	} else if !errors.Is(err, ErrInvalid) {
		return Config{}, nil, err
	}
	r := &Recovery{Cause: err}
	backup, backupErr := Load(path + ".bak")
	if backupErr == nil {
		if _, statErr := os.Stat(path + ".bak"); statErr == nil {
			c, r.Backup = backup, true
		} else if !os.IsNotExist(statErr) {
			return Config{}, nil, statErr
		}
	} else if !errors.Is(backupErr, ErrInvalid) {
		return Config{}, nil, backupErr
	}
	if !r.Backup {
		c = Config{}
	}
	if err != nil {
		f, e := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+".invalid-*")
		if e != nil {
			return Config{}, nil, fmt.Errorf("preserve invalid settings: %w", e)
		}
		if e = f.Close(); e != nil {
			return Config{}, nil, e
		}
		r.Path = f.Name()
		if e = os.Rename(path, r.Path); e != nil {
			return Config{}, nil, errors.Join(e, os.Remove(r.Path))
		}
	}
	// Do not overwrite a damaged backup; it can still be inspected separately.
	if err := write(path, c); err != nil {
		return Config{}, r, fmt.Errorf("restore settings: %w", err)
	}
	return c, r, nil
}
