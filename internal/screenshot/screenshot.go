// Package screenshot saves F12 captures as PNG files beside the program, like
// osu!'s screenshots folder, and shows a saved one in the file manager.
package screenshot

import (
	"errors"
	"fmt"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"time"
)

// Dir is data/screenshots next to the running executable.
func Dir() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	return filepath.Join(filepath.Dir(exe), "data", "screenshots"), nil
}

// Save writes img as <date>-<time>.png in Dir and returns its path.
// Captures in the same second get a numeric suffix instead of replacing an earlier file.
func Save(img image.Image) (string, error) {
	dir, err := Dir()
	if err != nil {
		return "", err
	}
	return save(dir, img, time.Now())
}

func save(dir string, img image.Image, now time.Time) (string, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("screenshot directory: %w", err)
	}
	base := now.Format("20060102-150405")
	for n := 1; ; n++ {
		name := base
		if n > 1 {
			name += fmt.Sprintf("-%d", n)
		}
		path := filepath.Join(dir, name+".png")
		f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
		if errors.Is(err, os.ErrExist) {
			continue
		}
		if err != nil {
			return "", fmt.Errorf("create screenshot: %w", err)
		}
		err = png.Encode(f, img)
		err = errors.Join(err, f.Close())
		if err != nil {
			return "", errors.Join(fmt.Errorf("write screenshot: %w", err), os.Remove(path))
		}
		return path, nil
	}
}
