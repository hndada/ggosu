// Package settings persists playback preferences separately from source charts.
package settings

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"ggosu/internal/chart"
	"ggosu/internal/download"
)

// Positive offsets delay notes and judgements relative to unchanged music.
// Values and replay input times use the original song's 1x timeline.
// JudgeMS is ONGEKI's 判定タイミングB: it moves only the judgement, so notes
// keep their place on screen and a positive value judges later.
type Offsets struct {
	GlobalMS int `json:"globalMS"`
	ChartMS  int `json:"chartMS"`
	JudgeMS  int `json:"judgeMS,omitempty"`
}

func (o Offsets) Validate() error {
	if o.GlobalMS < -1000 || o.GlobalMS > 1000 || o.ChartMS < -1000 || o.ChartMS > 1000 || o.JudgeMS < -1000 || o.JudgeMS > 1000 {
		return fmt.Errorf("offsets must be between -1000 and 1000 ms")
	}
	return nil
}

// Total is the shift of the judged chart, the judgement offset included.
func (o Offsets) Total() time.Duration {
	return time.Duration(o.GlobalMS+o.ChartMS+o.JudgeMS) * time.Millisecond
}

// Judge is how far the drawing clock runs ahead so notes stay where Total-Judge puts them.
func (o Offsets) Judge() time.Duration {
	return time.Duration(o.JudgeMS) * time.Millisecond
}

// Apply copies only the note and scroll timelines. Audio, storyboard, source files
// and the original chart digest remain unchanged.
func (o Offsets) Apply(c *chart.Chart) (*chart.Chart, error) {
	if err := o.Validate(); err != nil {
		return nil, err
	}
	if err := c.Validate(); err != nil {
		return nil, err
	}
	x := *c
	x.Notes = slices.Clone(c.Notes)
	x.Timing = slices.Clone(c.Timing)
	for i := range x.Notes {
		x.Notes[i].At += o.Total()
		x.Notes[i].End += o.Total()
	}
	for i := range x.Timing {
		x.Timing[i].At += o.Total()
	}
	if err := x.Validate(); err != nil {
		return nil, fmt.Errorf("apply offsets: %w", err)
	}
	return &x, nil
}

// The zero value is the default: zero global offset and no chart overrides.
type Config struct {
	Language      string `json:"language,omitempty"`
	ArchiveServer string `json:"archiveServer,omitempty"`
	// ScoreServer is the submission server. Nil uses the local web server; empty disables submission.
	ScoreServer *string         `json:"scoreServer,omitempty"`
	Playback    *Playback       `json:"playback,omitempty"`
	Display     *Display        `json:"display,omitempty"`
	GlobalMS    int             `json:"globalMS"`
	JudgeMS     int             `json:"judgeMS,omitempty"`
	Charts      map[string]int  `json:"charts,omitempty"`
	Profiles    map[int]Profile `json:"profiles,omitempty"`
	// Scroll is shared by every key count; each profile only adds its own offset.
	// Nil keeps older settings, which stored scroll per profile.
	Scroll *Scroll `json:"scroll,omitempty"`
	Audio  *Audio  `json:"audio,omitempty"`
	Chat   *Chat   `json:"chat,omitempty"`
	// AutoOffset moves GlobalMS by each live play's mean hit error. Off by default.
	AutoOffset bool `json:"autoOffset,omitempty"`
	// Name is the player's nickname, also the multiplayer default. Empty means "Player".
	Name string `json:"name,omitempty"`
	// SongFolders are extra local folders scanned for charts beside the songs folder, as absolute paths.
	SongFolders []string `json:"songFolders,omitempty"`
}

const NameLimit = 32

func (c Config) PlayerName() string {
	if c.Name == "" {
		return "Player"
	}
	return c.Name
}

func (c Config) Offsets(hash string) Offsets {
	return Offsets{GlobalMS: c.GlobalMS, ChartMS: c.Charts[hash], JudgeMS: c.JudgeMS}
}

func (c Config) Validate() error {
	if c.Language != "" && c.Language != "ko" && c.Language != "en" {
		return fmt.Errorf("unsupported language %q", c.Language)
	}
	if c.ArchiveServer != "" {
		if err := download.ValidateServer(c.ArchiveServer); err != nil {
			return err
		}
	}
	if err := ValidateScoreServer(c.ScoreURL()); err != nil {
		return err
	}
	if err := c.PlaybackSettings().Validate(); err != nil {
		return err
	}
	if err := c.DisplaySettings().Validate(); err != nil {
		return err
	}
	if err := c.AudioSettings().Validate(); err != nil {
		return err
	}
	for lanes, p := range c.Profiles {
		if err := p.Validate(lanes); err != nil {
			return err
		}
	}
	if s := c.Scroll; s != nil {
		if err := (Profile{Keys: []string{"A"}, Scroll: s.Speed, ScrollMode: s.Mode, ExposureMS: s.ExposureMS, SpeedChange: s.SpeedChange}).Validate(1); err != nil {
			return err
		}
	}
	for i, f := range c.SongFolders {
		if !filepath.IsAbs(f) || slices.Contains(c.SongFolders[:i], f) {
			return fmt.Errorf("song folders must be distinct absolute paths")
		}
	}
	if utf8.RuneCountInString(c.Name) > NameLimit || strings.TrimSpace(c.Name) != c.Name {
		return fmt.Errorf("nickname must be at most %d characters without outer spaces", NameLimit)
	}
	if err := c.Offsets("").Validate(); err != nil {
		return err
	}
	for hash, ms := range c.Charts {
		if _, err := hex.DecodeString(hash); err != nil || len(hash) != 64 {
			return fmt.Errorf("invalid chart identity in settings")
		}
		if err := (Offsets{ChartMS: ms}).Validate(); err != nil {
			return err
		}
	}
	return nil
}

func (c Config) ScoreURL() string {
	if c.ScoreServer == nil {
		return "http://127.0.0.1:8080"
	}
	return *c.ScoreServer
}

func ValidateScoreServer(server string) error {
	if server == "" {
		return nil
	}
	u, err := url.Parse(server)
	if err != nil || u == nil || (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || strings.TrimSpace(server) != server {
		return fmt.Errorf("점수 서버는 http:// 또는 https:// 주소여야 합니다")
	}
	return nil
}

func Load(path string) (Config, error) {
	f, err := os.Open(path)
	if os.IsNotExist(err) {
		return Config{}, nil
	}
	if err != nil {
		return Config{}, err
	}
	var c *Config
	d := json.NewDecoder(io.LimitReader(f, 8<<20))
	d.DisallowUnknownFields()
	err = d.Decode(&c)
	if err == nil {
		var extra any
		if d.Decode(&extra) != io.EOF {
			err = fmt.Errorf("settings: expected one object")
		}
	}
	if closeErr := f.Close(); closeErr != nil {
		return Config{}, errors.Join(err, closeErr)
	}
	if err == nil && c == nil {
		err = fmt.Errorf("settings: expected an object")
	}
	if err == nil {
		err = c.Validate()
	}
	if err != nil {
		var pathErr *os.PathError
		if errors.As(err, &pathErr) {
			return Config{}, err
		}
		return Config{}, fmt.Errorf("%w: %v", ErrInvalid, err)
	}
	return *c, nil
}

// Save keeps a validated copy for recovery. Both files use atomic replacement.
func Save(path string, c Config) error {
	if err := write(path+".bak", c); err != nil {
		return fmt.Errorf("save settings backup: %w", err)
	}
	return write(path, c)
}

func write(path string, c Config) (err error) {
	if err := c.Validate(); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".settings-*")
	if err != nil {
		return err
	}
	defer func() {
		if e := os.Remove(f.Name()); e != nil && !os.IsNotExist(e) {
			err = errors.Join(err, e)
		}
	}()
	writeErr := json.NewEncoder(f).Encode(c)
	if writeErr == nil {
		writeErr = f.Sync()
	}
	if err := errors.Join(writeErr, f.Close()); err != nil {
		return err
	}
	return os.Rename(f.Name(), path)
}
