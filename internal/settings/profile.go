package settings

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"slices"
	"strings"

	"ggosu/internal/chart"
)

// Profile is one key count's settings. In the preferences file and the options window, Scroll,
// ScrollMode, ExposureMS and SpeedChange are the values shared by every key count, and the offsets
// are this key count's own; Effective gives the values play uses.
type Profile struct {
	Keys           []string `json:"keys"`
	Scroll         float64  `json:"scrollSpeed"` // osu!lazer mania scroll speed (1..40)
	ScrollMode     string   `json:"scrollMode"`
	ExposureMS     int      `json:"exposureMS"`
	SpeedChange    float64  `json:"speedChange"`
	ScrollOffset   float64  `json:"scrollSpeedOffset,omitempty"` // added to the shared scroll speed
	ExposureOffset int      `json:"exposureOffset,omitempty"`    // ms added to the shared exposure time
	Cover          *Cover   `json:"cover,omitempty"`             // shared by every key count, like the scroll values
}

// Scroll is the scroll setting shared by every key count.
type Scroll struct {
	Mode        string  `json:"mode"`
	Speed       float64 `json:"scrollSpeed"`
	ExposureMS  int     `json:"exposureMS"`
	SpeedChange float64 `json:"speedChange"`
	Cover       *Cover  `json:"cover,omitempty"`
}

// Scroll speed follows osu!lazer mania: 1..40 in steps of 0.1, default 8, and a
// note takes 11485 ms / speed to fall the default stage (ManiaRulesetConfigManager,
// DrawableManiaRuleset.MAX_TIME_RANGE). The stage runs from the top of the screen
// to lazer's default hit position, 124.8 of 768 above the bottom.
const MinScroll, MaxScroll, DefaultScroll = 1, 40, 8

// PixelsPerScroll is px/s on the 1280x720 field for one unit of scroll speed.
const PixelsPerScroll = (768 - 124.8) * 720 / 768 * 1000 / 11485

// legacyScroll converts a former px/s value to scroll speed.
func legacyScroll(px, lo, hi float64) float64 {
	return min(hi, max(lo, math.Round(px/PixelsPerScroll*10)/10))
}

// Effective applies the key count's offsets within the play limits.
func (p Profile) Effective() Profile {
	p.Scroll = min(MaxScroll, max(MinScroll, p.Scroll+p.ScrollOffset))
	p.ExposureMS = min(5000, max(100, p.ExposureMS+p.ExposureOffset))
	p.ScrollOffset, p.ExposureOffset = 0, 0
	return p
}

// UnmarshalJSON still reads "svScale", the former name of speedChange, and the
// former px/s "scroll" and "scrollOffset", so older settings and record replays
// stay readable.
func (p *Profile) UnmarshalJSON(b []byte) error {
	type profile Profile
	var v struct {
		profile
		SVScale     *float64 `json:"svScale"`
		PixelScroll *float64 `json:"scroll"`
		PixelOffset *float64 `json:"scrollOffset"`
	}
	d := json.NewDecoder(bytes.NewReader(b))
	d.DisallowUnknownFields()
	if err := d.Decode(&v); err != nil {
		return err
	}
	*p = Profile(v.profile)
	if v.SVScale != nil && !bytes.Contains(b, []byte(`"speedChange"`)) {
		p.SpeedChange = *v.SVScale
	}
	if v.PixelScroll != nil && !bytes.Contains(b, []byte(`"scrollSpeed"`)) {
		p.Scroll = legacyScroll(*v.PixelScroll, MinScroll, MaxScroll)
	}
	if v.PixelOffset != nil && !bytes.Contains(b, []byte(`"scrollSpeedOffset"`)) {
		p.ScrollOffset = legacyScroll(*v.PixelOffset, MinScroll-MaxScroll, MaxScroll-MinScroll)
	}
	return nil
}

// UnmarshalJSON still reads the former px/s "speed".
func (s *Scroll) UnmarshalJSON(b []byte) error {
	type scroll Scroll
	var v struct {
		scroll
		PixelSpeed *float64 `json:"speed"`
	}
	d := json.NewDecoder(bytes.NewReader(b))
	d.DisallowUnknownFields()
	if err := d.Decode(&v); err != nil {
		return err
	}
	*s = Scroll(v.scroll)
	if v.PixelSpeed != nil && !bytes.Contains(b, []byte(`"scrollSpeed"`)) {
		s.Speed = legacyScroll(*v.PixelSpeed, MinScroll, MaxScroll)
	}
	return nil
}

func DefaultKeys(lanes int) ([]string, error) {
	if lanes < 1 || lanes > 18 {
		return nil, fmt.Errorf("key count must be between 1 and 18")
	}
	// hndada/gosu piano.Options.KeyMappings. With 8K read as 7K plus a left scratch (KeyScratchModes),
	// the scratch lane lands on the edge key: A for 8K, S for BMS 5+1.
	defaults := map[int]string{1: "Space", 2: "F,J", 3: "F,Space,J", 4: "D,F,J,K", 5: "D,F,Space,J,K", 6: "S,D,F,J,K,L", 7: "S,D,F,Space,J,K,L", 8: "A,S,D,F,Space,J,K,L", 9: "A,S,D,F,Space,J,K,L,Semicolon", 10: "A,S,D,F,V,N,J,K,L,Semicolon"}
	s := defaults[lanes]
	if s == "" {
		s = strings.Join(strings.Split("A,S,D,F,G,H,J,K,L,Semicolon,Q,W,E,R,T,Y,U,I", ",")[:lanes], ",")
	}
	return strings.Split(strings.ToUpper(s), ","), nil
}

func KeyCode(name string) (uint16, error) {
	switch name {
	case "SPACE":
		return 32, nil
	case "SEMICOLON":
		return 186, nil
	case "P", "ESCAPE", "F6", "F7", "PAGEUP", "PAGEDOWN":
		return 0, fmt.Errorf("%s is reserved for playback controls", name)
	}
	if len(name) == 1 && (name[0] >= 'A' && name[0] <= 'Z' || name[0] >= '0' && name[0] <= '9') {
		return uint16(name[0]), nil
	}
	return 0, fmt.Errorf("unsupported lane key %q (A-Z, 0-9, SPACE, SEMICOLON)", name)
}

func (p Profile) Validate(lanes int) error {
	p = p.Normalized()
	if p.ScrollMode != "speed" && p.ScrollMode != "exposure" {
		return fmt.Errorf("invalid scroll mode")
	}
	if p.ExposureMS < 100 || p.ExposureMS > 5000 || math.IsNaN(p.SpeedChange) || math.IsInf(p.SpeedChange, 0) || p.SpeedChange < 0 || p.SpeedChange > 2 {
		return fmt.Errorf("exposure must be 100..5000 ms and speed change 0..2")
	}
	if math.IsNaN(p.Scroll) || math.IsInf(p.Scroll, 0) || p.Scroll < MinScroll || p.Scroll > MaxScroll {
		return fmt.Errorf("scroll speed must be between %d and %d", MinScroll, MaxScroll)
	}
	if math.IsNaN(p.ScrollOffset) || math.IsInf(p.ScrollOffset, 0) || p.ScrollOffset < MinScroll-MaxScroll || p.ScrollOffset > MaxScroll-MinScroll || p.ExposureOffset < -4900 || p.ExposureOffset > 4900 {
		return fmt.Errorf("scroll speed offset must be within -%d..%d and exposure offset within -4900..4900 ms", MaxScroll-MinScroll, MaxScroll-MinScroll)
	}
	if err := p.LaneCover().Validate(); err != nil {
		return err
	}
	if lanes < 1 || lanes > 18 || len(p.Keys) != lanes {
		return fmt.Errorf("expected %d lane keys", lanes)
	}
	seen := map[uint16]bool{}
	for _, key := range p.Keys {
		code, err := KeyCode(key)
		if err != nil {
			return err
		}
		if seen[code] {
			return fmt.Errorf("duplicate lane key %s", key)
		}
		seen[code] = true
	}
	return nil
}

func (c Config) Profile(lanes int) (Profile, error) {
	if p, ok := c.Profiles[lanes]; ok {
		p.Keys = append([]string(nil), p.Keys...)
		if p.Scroll == 0 {
			p.Scroll = DefaultScroll
		}
		p = c.shared(p.Normalized())
		return p, p.Validate(lanes)
	}
	keys, err := DefaultKeys(lanes)
	return c.shared((Profile{Keys: keys, Scroll: DefaultScroll}).Normalized()), err
}

// SetProfile stores a key count's keys and offsets; its scroll values become the shared ones.
func (c *Config) SetProfile(lanes int, p Profile) {
	if c.Profiles == nil {
		c.Profiles = make(map[int]Profile)
	}
	c.Profiles[lanes] = p
	c.Scroll = &Scroll{Mode: p.ScrollMode, Speed: p.Scroll, ExposureMS: p.ExposureMS, SpeedChange: p.SpeedChange, Cover: p.Cover}
}

func (c Config) shared(p Profile) Profile {
	if s := c.Scroll; s != nil {
		p.ScrollMode, p.Scroll, p.ExposureMS, p.SpeedChange = s.Mode, s.Speed, s.ExposureMS, s.SpeedChange
		if s.Cover != nil {
			p.Cover = s.Cover
		}
	}
	return p
}

func (p Profile) Normalized() Profile {
	if p.Scroll == 0 {
		p.Scroll = DefaultScroll
	}
	if p.ScrollMode == "" {
		p.ScrollMode = "speed"
		p.SpeedChange = 1
	}
	if p.ExposureMS == 0 {
		p.ExposureMS = 750
	}
	cover := p.LaneCover()
	p.Cover = &cover
	return p
}

// PixelsPerSecond returns the speed on the chart timeline. Exposure mode
// compensates for playback rate so the real exposure time stays constant.
func (p Profile) PixelsPerSecond(rate, reach float64) float64 {
	if p.ScrollMode == "exposure" {
		return reach * 1000 / (float64(p.ExposureMS) * rate)
	}
	return p.Scroll * PixelsPerScroll
}

// Visual projects seconds with the chart's speed changes: each BPM relative to
// the main BPM times its scroll multiplier, scaled by SpeedChange. The base speed
// stays independent of BPM. Judgement, audio and source identity are unchanged;
// stops still freeze scrolling. One position unit is one second at multiplier 1.
func (p Profile) Visual(c *chart.Chart) *chart.Chart {
	x := *c
	x.Timing = slices.Clone(c.Timing)
	main := c.MainBPM()
	for i, t := range c.Timing {
		x.Timing[i].BPM = 60
		x.Timing[i].Scroll = max(0, 1+p.SpeedChange*(t.BPM/main*t.Scroll-1))
	}
	return &x
}
