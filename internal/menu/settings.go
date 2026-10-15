package menu

import (
	"fmt"
	"math"
	"path/filepath"
	"strings"

	"ggosu/internal/chart"
	"ggosu/internal/piano"
	"ggosu/internal/settings"
)

func (a *App) PlaybackSettings() (settings.Playback, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	path, _, err := a.settingsKey(Selection{})
	if err != nil {
		return settings.Playback{}, err
	}
	c, err := a.loadSettings(path)
	return c.PlaybackSettings(), err
}

func (a *App) SavePlayback(p settings.Playback) (settings.Playback, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.closed || a.state.Running {
		return p, fmt.Errorf("플레이 종료 후 설정을 변경해 주세요")
	}
	if err := p.Validate(); err != nil {
		return p, err
	}
	path, _, err := a.settingsKey(Selection{})
	if err != nil {
		return p, err
	}
	c, err := a.loadSettings(path)
	if err != nil {
		return p, err
	}
	c.Playback = &p
	return p, settings.Save(path, c)
}

// Called with a.mu held; an empty selection addresses the global setting only.
func (a *App) settingsKey(s Selection) (string, string, error) {
	dir, err := a.recordsDir()
	if err != nil {
		return "", "", err
	}
	path := filepath.Join(filepath.Dir(dir), "settings.json")
	if s.ChartID == "" {
		return path, "", nil
	}
	e, err := a.songs.Lookup(s)
	if err != nil {
		return "", "", err
	}
	cs, err := chart.Load(e.Path)
	if err != nil {
		return "", "", err
	}
	if s.Index < 0 || s.Index >= len(cs) {
		return "", "", fmt.Errorf("채보가 변경됐습니다")
	}
	hash, err := cs[s.Index].Digest()
	return path, hash, err
}

func (a *App) Offsets(s Selection) (settings.Offsets, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.offsets(s)
}

func (a *App) offsets(s Selection) (settings.Offsets, error) {
	path, hash, err := a.settingsKey(s)
	if err != nil {
		return settings.Offsets{}, err
	}
	c, err := a.loadSettings(path)
	if err != nil {
		return settings.Offsets{}, fmt.Errorf("설정 읽기: %w", err)
	}
	return c.Offsets(hash), nil
}

func (a *App) SaveOffsets(s Selection, o settings.Offsets) (settings.Offsets, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.closed || a.state.Running {
		return settings.Offsets{}, fmt.Errorf("플레이 종료 후 설정을 변경해 주세요")
	}
	if err := o.Validate(); err != nil {
		return settings.Offsets{}, err
	}
	path, hash, err := a.settingsKey(s)
	if err != nil {
		return settings.Offsets{}, err
	}
	if hash == "" && o.ChartMS != 0 {
		return settings.Offsets{}, fmt.Errorf("채보를 선택해 주세요")
	}
	c, err := a.loadSettings(path)
	if err != nil {
		return settings.Offsets{}, err
	}
	c.GlobalMS, c.JudgeMS = o.GlobalMS, o.JudgeMS
	if hash != "" {
		if c.Charts == nil {
			c.Charts = make(map[string]int)
		}
		if o.ChartMS == 0 {
			delete(c.Charts, hash)
		} else {
			c.Charts[hash] = o.ChartMS
		}
	}
	if err := settings.Save(path, c); err != nil {
		return settings.Offsets{}, fmt.Errorf("설정 저장: %w", err)
	}
	return c.Offsets(hash), nil
}

// Calibration is one automatic global offset change made from a play's mean hit
// error, like osu!lazer's "use average of last play" but applied after every play.
type Calibration struct {
	MeanMS float64 `json:"meanMS"`
	FromMS int     `json:"fromMS"`
	ToMS   int     `json:"toMS"`
}

// A short play's mean error is mostly noise.
const calibrationHits = 50

// calibrate moves the global offset by a normal-speed live play's mean hit error
// when auto offset is on. Notes were already shifted by the old offset, so the
// error adds to it. a.mu must be held.
func (a *App) calibrate(r piano.Record) (*Calibration, error) {
	if r.Mode != "live" || r.Practice || r.HitError == nil || r.HitError.Count < calibrationHits {
		return nil, nil
	}
	shift := int(math.Round(r.HitError.MeanMS))
	if shift == 0 {
		return nil, nil
	}
	path, _, err := a.settingsKey(Selection{})
	if err != nil {
		return nil, err
	}
	c, err := a.loadSettings(path)
	if err != nil || !c.AutoOffset {
		return nil, err
	}
	cal := &Calibration{MeanMS: r.HitError.MeanMS, FromMS: c.GlobalMS, ToMS: max(-1000, min(1000, c.GlobalMS+shift))}
	c.GlobalMS = cal.ToMS
	if err := settings.Save(path, c); err != nil {
		return nil, err
	}
	return cal, nil
}

func (a *App) AutoOffset() (bool, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	path, _, err := a.settingsKey(Selection{})
	if err != nil {
		return false, err
	}
	c, err := a.loadSettings(path)
	return c.AutoOffset, err
}

func (a *App) SaveAutoOffset(on bool) (bool, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.closed || a.state.Running {
		return !on, fmt.Errorf("플레이 종료 후 설정을 변경해 주세요")
	}
	path, _, err := a.settingsKey(Selection{})
	if err != nil {
		return !on, err
	}
	c, err := a.loadSettings(path)
	if err != nil {
		return !on, err
	}
	c.AutoOffset = on
	if err := settings.Save(path, c); err != nil {
		return !on, err
	}
	return on, nil
}

func (a *App) Profile(lanes int) (settings.Profile, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.profile(lanes)
}

func (a *App) profile(lanes int) (settings.Profile, error) {
	path, _, err := a.settingsKey(Selection{})
	if err != nil {
		return settings.Profile{}, err
	}
	c, err := a.loadSettings(path)
	if err != nil {
		return settings.Profile{}, err
	}
	return c.Profile(lanes)
}

func (a *App) SaveProfile(lanes int, p settings.Profile) (settings.Profile, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.closed || a.state.Running {
		return settings.Profile{}, fmt.Errorf("플레이 종료 후 설정을 변경해 주세요")
	}
	for i, key := range p.Keys {
		p.Keys[i] = strings.ToUpper(strings.TrimSpace(key))
	}
	if p.Scroll == 0 {
		p.Scroll = settings.DefaultScroll
	}
	p = p.Normalized()
	if err := p.Validate(lanes); err != nil {
		return settings.Profile{}, err
	}
	path, _, err := a.settingsKey(Selection{})
	if err != nil {
		return settings.Profile{}, err
	}
	c, err := a.loadSettings(path)
	if err != nil {
		return settings.Profile{}, err
	}
	c.SetProfile(lanes, p)
	if err := settings.Save(path, c); err != nil {
		return settings.Profile{}, err
	}
	return p, nil
}

func (a *App) ResetProfile(lanes int) (settings.Profile, error) {
	keys, err := settings.DefaultKeys(lanes)
	if err != nil {
		return settings.Profile{}, err
	}
	// Scroll is shared by all key counts, so resetting one clears only its keys and offsets.
	cur, err := a.Profile(lanes)
	if err != nil {
		return settings.Profile{}, err
	}
	return a.SaveProfile(lanes, settings.Profile{Keys: keys, Scroll: cur.Scroll, ScrollMode: cur.ScrollMode, ExposureMS: cur.ExposureMS, SpeedChange: cur.SpeedChange, Cover: cur.Cover})
}

func (a *App) AudioSettings() (settings.Audio, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	path, _, err := a.settingsKey(Selection{})
	if err != nil {
		return settings.Audio{}, err
	}
	c, err := a.loadSettings(path)
	return c.AudioSettings(), err
}
func (a *App) SaveAudio(v settings.Audio) (settings.Audio, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.closed || a.state.Running {
		return v, fmt.Errorf("플레이 종료 후 설정을 변경해 주세요")
	}
	if err := v.Validate(); err != nil {
		return v, err
	}
	path, _, err := a.settingsKey(Selection{})
	if err != nil {
		return v, err
	}
	c, err := a.loadSettings(path)
	if err != nil {
		return v, err
	}
	c.Audio = &v
	return v, settings.Save(path, c)
}

func (a *App) DisplaySettings() (settings.Display, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	path, _, err := a.settingsKey(Selection{})
	if err != nil {
		return settings.Display{}, err
	}
	c, err := a.loadSettings(path)
	return c.DisplaySettings(), err
}
func (a *App) SaveDisplay(v settings.Display) (settings.Display, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.closed {
		return v, fmt.Errorf("플레이 종료 후 설정을 변경해 주세요")
	}
	if err := v.Validate(); err != nil {
		return v, err
	}
	path, _, err := a.settingsKey(Selection{})
	if err != nil {
		return v, err
	}
	c, err := a.loadSettings(path)
	if err != nil {
		return v, err
	}
	if a.state.Running {
		d := c.DisplaySettings()
		d.Layout, d.HUD, d.Skin, d.TimeDisplay = v.Layout, v.HUD, v.Skin, v.TimeDisplay
		d.ComboPosition, d.JudgementPosition, d.LaneLines = v.ComboPosition, v.JudgementPosition, v.LaneLines
		d.NoteThickness = v.NoteThickness
		d.TimingThreshold = v.TimingThreshold
		v = d
	}
	old := c.DisplaySettings()
	c.Display = &v
	if err := settings.Save(path, c); err != nil {
		return v, err
	}
	if a.window != nil && !a.state.Running && (old.WindowMode != v.WindowMode || old.Resolution != v.Resolution) {
		a.window(v)
	}
	return v, nil
}
