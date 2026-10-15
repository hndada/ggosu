package chart

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// AssetPath rejects absolute, parent, drive and symlink escapes, on every OS.
// It returns a real existing path; callers never trust parser strings directly.
func AssetPath(source, name string) (string, error) {
	name = strings.ReplaceAll(name, "\\", "/")
	if name == "" || strings.HasPrefix(name, "/") || strings.Contains(name, ":") {
		return "", fmt.Errorf("unsafe asset path %q", name)
	}
	for _, p := range strings.Split(name, "/") {
		if p == ".." {
			return "", fmt.Errorf("asset escapes chart directory: %q", name)
		}
	}
	base, err := filepath.Abs(filepath.Dir(source))
	if err != nil {
		return "", err
	}
	base, err = filepath.EvalSymlinks(base)
	if err != nil {
		return "", err
	}
	path, err := filepath.EvalSymlinks(filepath.Join(base, filepath.FromSlash(name)))
	if err != nil {
		return "", err
	}
	rel, err := filepath.Rel(base, path)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("asset escapes chart directory: %q", name)
	}
	return path, nil
}

// ErrSilentAudio means a bank slot has no sound, as opposed to a missing file.
// O2Jam banks are sparse; charts can reference slots with no stored sample.
var ErrSilentAudio = errors.New("empty bank sample")

// OpenAudio opens audio asset id of c, or its sample inside a bank, for previews.
// Missing, unsafe or non-audio assets report fs.ErrNotExist. The caller closes r.
func OpenAudio(c *Chart, id string) (name string, mod time.Time, r io.ReadSeekCloser, err error) {
	a, ok := c.Assets[id]
	if !ok || (a.Kind != "audio" && a.Kind != "bank") {
		return "", time.Time{}, nil, fmt.Errorf("audio asset %q: %w", id, fs.ErrNotExist)
	}
	path, err := AssetPath(c.Source.Path, a.Path)
	if err != nil {
		return "", time.Time{}, nil, fmt.Errorf("%w: %w", fs.ErrNotExist, err)
	}
	f, err := os.Open(path)
	if err != nil {
		return "", time.Time{}, nil, fmt.Errorf("%w: %w", fs.ErrNotExist, err)
	}
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() > 256<<20 {
		f.Close()
		return "", time.Time{}, nil, fmt.Errorf("audio asset %q: %w", id, fs.ErrNotExist)
	}
	if a.Kind == "audio" {
		return info.Name(), info.ModTime(), f, nil
	}
	b, err := io.ReadAll(io.LimitReader(f, (256<<20)+1))
	f.Close()
	if err != nil || len(b) > 256<<20 {
		return "", time.Time{}, nil, errors.New("프리뷰 오디오 읽기 실패")
	}
	samples, bankErr := ReadBank(path, b)
	sample, ok := samples[a.Sample]
	// A damaged final record does not prevent serving earlier complete samples.
	if !ok && bankErr != nil {
		return "", time.Time{}, nil, bankErr
	}
	if (!ok || len(sample.Data) == 0) && a.Sample > 0 && a.Sample <= 2000 {
		return "", time.Time{}, nil, ErrSilentAudio
	}
	if !ok {
		return "", time.Time{}, nil, fmt.Errorf("bank sample %d: %w", a.Sample, fs.ErrNotExist)
	}
	return filepath.Base(path) + sample.Extension, info.ModTime(), nopCloser{bytes.NewReader(sample.Data)}, nil
}

// OpenImage opens image asset id of c, or its byte range inside a file.
// Missing, unsafe or non-image assets report fs.ErrNotExist. The caller closes r.
func OpenImage(c *Chart, id string) (name string, mod time.Time, r io.ReadSeekCloser, err error) {
	a, ok := c.Assets[id]
	if !ok || a.Kind != "image" {
		return "", time.Time{}, nil, fmt.Errorf("image asset %q: %w", id, fs.ErrNotExist)
	}
	path, err := AssetPath(c.Source.Path, a.Path)
	if err != nil {
		return "", time.Time{}, nil, fmt.Errorf("%w: %w", fs.ErrNotExist, err)
	}
	f, err := os.Open(path)
	if err != nil {
		return "", time.Time{}, nil, fmt.Errorf("%w: %w", fs.ErrNotExist, err)
	}
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() > 256<<20 || a.Offset < 0 || a.Size < 0 || a.Offset+a.Size > info.Size() {
		f.Close()
		return "", time.Time{}, nil, fmt.Errorf("image asset %q: %w", id, fs.ErrNotExist)
	}
	if a.Size == 0 {
		return info.Name(), info.ModTime(), f, nil
	}
	return info.Name() + "#" + id, info.ModTime(), section{io.NewSectionReader(f, a.Offset, a.Size), f}, nil
}

type section struct {
	io.ReadSeeker
	io.Closer
}

type nopCloser struct{ io.ReadSeeker }

func (nopCloser) Close() error { return nil }
