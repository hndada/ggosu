package chart

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/hajimehoshi/go-mp3"
	"github.com/jfreymuth/oggvorbis"
)

// AudioDuration is when the last audio event stops, from time zero like
// Duration, so it includes each event's start, not only its AudioLength.
// It covers BGM and playable keysounds after the final note. Lengths come from
// WAV headers, the last Ogg page and MP3 frame headers, so no audio is decoded. An unreadable asset
// ends where it starts. lengths may be nil; a shared cache skips files that
// have not changed.
func (c *Chart) AudioDuration(lengths *AudioLengths) time.Duration {
	if lengths == nil {
		lengths = NewAudioLengths()
	}
	// Catalogue and multiplayer callers use a best-effort end. The song info
	// uses Duration directly so it can report unavailable measurements.
	end, _ := lengths.Duration(c)
	return end
}

// Duration measures the end of BGM and playable note sounds on the original
// timeline. Missing or invalid audio returns an error instead of a trusted length.
// Empty slots in a valid O2Jam bank are silence, not errors.
func (l *AudioLengths) Duration(c *Chart) (time.Duration, error) {
	var end time.Duration
	var errs []error
	paths := map[string]string{}
	measured := map[string]struct {
		length time.Duration
		err    error
	}{}
	add := func(at time.Duration, s Sound) {
		if s.Volume <= 0 {
			return
		}
		m, ok := measured[s.Asset]
		if !ok {
			m.length, m.err = l.asset(c, s.Asset, paths)
			measured[s.Asset] = m
			if m.err != nil {
				errs = append(errs, fmt.Errorf("audio %s: %w", s.Asset, m.err))
			}
		}
		if m.length > 0 || m.err != nil {
			length := max(0, m.length-s.From)
			if s.Length > 0 {
				length = min(length, s.Length)
			}
			end = max(end, at+length)
		}
	}
	for _, e := range c.Audio {
		add(e.At, e.Sound)
	}
	// osu!/StepMania use skin hitsounds; missing defaults are not song audio.
	format := c.Source.Format
	if c.Source.OriginalFormat != "" {
		format = c.Source.OriginalFormat
	}
	if format == "osu" || format == "sm" {
		return end, errors.Join(errs...)
	}
	for _, n := range c.Notes {
		if n.Kind == "fake" || n.Kind == "mine" {
			continue
		}
		for _, s := range n.Sounds {
			add(n.At, s)
		}
	}
	return end, errors.Join(errs...)
}

// lengthsVersion changes when AudioLength would measure a file differently.
const lengthsVersion = 3

// AudioLengths caches audio file lengths by path, size and modification time.
// One value is not safe for concurrent use.
type AudioLengths struct {
	files map[string]audioFile
	used  map[string]bool // Checked against the file since this value was made.
}

type audioFile struct {
	Size     int64                 `json:"size"`
	Modified int64                 `json:"modified_ns"`
	Length   time.Duration         `json:"length_ns,omitempty"`
	Samples  map[int]time.Duration `json:"samples_ns,omitempty"` // Bank sample lengths by sample number.
	Errors   map[int]string        `json:"sample_errors,omitempty"`
	Error    string                `json:"error,omitempty"`
}

type lengthsFile struct {
	Version int                  `json:"version"`
	Files   map[string]audioFile `json:"files"`
}

func NewAudioLengths() *AudioLengths {
	return &AudioLengths{files: map[string]audioFile{}, used: map[string]bool{}}
}

// LoadAudioLengths reads a cache saved by Save. A missing file is an empty
// cache; an unreadable or older one returns an empty cache and an error.
func LoadAudioLengths(path string) (*AudioLengths, error) {
	l := NewAudioLengths()
	b, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return l, nil
	}
	if err != nil {
		return l, err
	}
	var f lengthsFile
	if err := json.Unmarshal(b, &f); err != nil {
		return l, fmt.Errorf("audio length cache %s: %w", path, err)
	}
	if f.Version == lengthsVersion && f.Files != nil {
		l.files = f.Files
	}
	return l, nil
}

// Save atomically writes the cache, retaining files measured by earlier selections.
func (l *AudioLengths) Save(path string) (err error) {
	f := lengthsFile{Version: lengthsVersion, Files: l.files}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".audio-lengths-*")
	if err != nil {
		return err
	}
	defer func() {
		if e := os.Remove(tmp.Name()); e != nil && !os.IsNotExist(e) {
			err = errors.Join(err, e)
		}
	}()
	writeErr := json.NewEncoder(tmp).Encode(f)
	if writeErr == nil {
		writeErr = tmp.Sync()
	}
	if err := errors.Join(writeErr, tmp.Close()); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}

func (l *AudioLengths) asset(c *Chart, id string, paths map[string]string) (time.Duration, error) {
	a, ok := c.Assets[id]
	if !ok || (a.Kind != "audio" && a.Kind != "bank") {
		return 0, fmt.Errorf("missing audio asset")
	}
	source := c.Source.Path
	if a.Source != "" {
		source = a.Source
	}
	path, ok := paths[source+"|"+a.Path]
	if !ok {
		var err error
		path, err = AssetPath(source, a.Path)
		if err != nil {
			return 0, err
		}
		paths[source+"|"+a.Path] = path
	}
	f, err := l.file(path, a.Kind == "bank")
	if err != nil {
		return 0, err
	}
	if a.Kind == "bank" {
		if msg := f.Errors[a.Sample]; msg != "" {
			return 0, fmt.Errorf("%s sample %d: %s", a.Path, a.Sample, msg)
		}
		if length, ok := f.Samples[a.Sample]; ok {
			return length, nil
		}
		if f.Error != "" {
			return 0, fmt.Errorf("%s sample %d: %s", a.Path, a.Sample, f.Error)
		}
		if a.Sample > 0 && a.Sample <= 2000 {
			return 0, nil
		}
		return 0, fmt.Errorf("%s: invalid sample %d", a.Path, a.Sample)
	}
	if f.Error != "" {
		return 0, errors.New(f.Error)
	}
	return f.Length, nil
}

// file measures path unless the cache holds it at the same size and time.
// Unreadable files are not cached; undecodable ones are cached as zero length.
func (l *AudioLengths) file(path string, bank bool) (audioFile, error) {
	if l.used[path] {
		return l.files[path], nil
	}
	info, err := os.Stat(path)
	if err != nil {
		return audioFile{}, err
	}
	if !info.Mode().IsRegular() || info.Size() > 256<<20 {
		return audioFile{}, fmt.Errorf("audio asset too large or not a file: %s", path)
	}
	if f, ok := l.files[path]; ok && f.Size == info.Size() && f.Modified == info.ModTime().UnixNano() {
		l.used[path] = true
		return f, nil
	}
	f := audioFile{Size: info.Size(), Modified: info.ModTime().UnixNano()}
	if bank {
		data, err := readAsset(path)
		if err != nil {
			return audioFile{}, err
		}
		samples, err := ReadBank(path, data)
		if err != nil {
			f.Error = err.Error()
		}
		f.Samples = make(map[int]time.Duration, len(samples))
		f.Errors = map[int]string{}
		for n, s := range samples {
			if len(s.Data) == 0 {
				f.Samples[n] = 0
				continue
			}
			length, err := AudioLength(s.Extension, bytes.NewReader(s.Data))
			if err != nil {
				f.Errors[n] = err.Error()
				continue
			}
			f.Samples[n] = length
		}
	} else {
		r, err := os.Open(path)
		if err != nil {
			return audioFile{}, err
		}
		f.Length, err = AudioLength(filepath.Ext(path), r)
		if err = errors.Join(err, r.Close()); err != nil {
			f.Error = err.Error()
		}
	}
	l.files[path], l.used[path] = f, true
	return f, nil
}

func readAsset(path string) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	b, err := io.ReadAll(io.LimitReader(f, 256<<20+1))
	if err != nil {
		return nil, err
	}
	if len(b) > 256<<20 {
		return nil, fmt.Errorf("audio asset too large: %s", path)
	}
	return b, nil
}

// AudioLength reads the playing time of encoded audio; ext selects the format.
func AudioLength(ext string, r io.ReadSeeker) (time.Duration, error) {
	switch strings.ToLower(ext) {
	case ".wav":
		return wavLength(r)
	case ".ogg":
		samples, format, err := oggvorbis.GetLength(r)
		if err != nil {
			return 0, err
		}
		if format.SampleRate <= 0 {
			return 0, fmt.Errorf("ogg: invalid sample rate")
		}
		return time.Duration(samples) * time.Second / time.Duration(format.SampleRate), nil
	case ".mp3":
		d, err := mp3.NewDecoder(r)
		if err != nil {
			return 0, err
		}
		// go-mp3 counts 16-bit stereo output bytes at the source rate.
		return time.Duration(d.Length()/4) * time.Second / time.Duration(d.SampleRate()), nil
	}
	return 0, fmt.Errorf("unsupported audio %s", ext)
}

func wavLength(r io.ReadSeeker) (time.Duration, error) {
	var riff [12]byte
	if _, err := io.ReadFull(r, riff[:]); err != nil {
		return 0, err
	}
	if string(riff[:4]) != "RIFF" || string(riff[8:]) != "WAVE" {
		return 0, fmt.Errorf("wav: missing RIFF header")
	}
	var rate, align uint32
	for {
		var chunk [8]byte
		if _, err := io.ReadFull(r, chunk[:]); err != nil {
			return 0, err
		}
		size := int64(binary.LittleEndian.Uint32(chunk[4:]))
		switch string(chunk[:4]) {
		case "fmt ":
			var f [16]byte
			if size < 16 {
				return 0, fmt.Errorf("wav: short fmt chunk")
			}
			if _, err := io.ReadFull(r, f[:]); err != nil {
				return 0, err
			}
			rate, align = binary.LittleEndian.Uint32(f[4:]), uint32(binary.LittleEndian.Uint16(f[12:]))
			size -= 16
		case "data":
			if rate == 0 || align == 0 {
				return 0, fmt.Errorf("wav: data before fmt chunk")
			}
			// Streaming writers leave the size unset; count only bytes present.
			at, err := r.Seek(0, io.SeekCurrent)
			if err != nil {
				return 0, err
			}
			last, err := r.Seek(0, io.SeekEnd)
			if err != nil {
				return 0, err
			}
			frames := min(size, last-at) / int64(align)
			return time.Duration(frames) * time.Second / time.Duration(rate), nil
		}
		if _, err := r.Seek(size+size&1, io.SeekCurrent); err != nil {
			return 0, err
		}
	}
}
