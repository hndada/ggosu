// Package chart defines the engine-independent keyboard chart format.
package chart

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"slices"
	"time"
)

const Version = 1

// All times are signed nanoseconds on the song's unscaled audio timeline.
// Negative positions allow lead-ins. Lanes are zero-based, left to right.
type Chart struct {
	Version    int              `json:"version"`
	Metadata   Metadata         `json:"metadata"`
	Source     Source           `json:"source"`
	Lanes      int              `json:"lanes"`
	Layout     []Lane           `json:"layout,omitempty"`
	Notes      []Note           `json:"notes"`
	Timing     []Timing         `json:"timing"`
	Audio      []Audio          `json:"audio,omitempty"`
	Storyboard []StoryEvent     `json:"storyboard,omitempty"`
	Assets     map[string]Asset `json:"assets,omitempty"`
	Warnings   []string         `json:"warnings,omitempty"`
	// Fixes are source errors repaired on load. They go to the load log, not to play results.
	Fixes []string `json:"fixes,omitempty"`
}

// Lane records logical controls; physical keyboard bindings belong to the game.
type Lane struct {
	Name  string `json:"name"`
	Role  string `json:"role"`
	Group int    `json:"group,omitempty"`
}

type Metadata struct {
	Title      string        `json:"title"`
	Artist     string        `json:"artist,omitempty"`
	Creator    string        `json:"creator,omitempty"`
	Difficulty string        `json:"difficulty"`
	Level      string        `json:"level,omitempty"`
	Preview    time.Duration `json:"preview_ns,omitempty"`
	LeadIn     time.Duration `json:"lead_in_ns,omitempty"`
}

type Source struct {
	OriginalFormat string `json:"originalFormat,omitempty"` // Format before saving an editable .ggc copy.
	Format         string `json:"format"`
	Path           string `json:"path,omitempty"`
	Index          int    `json:"index"` // Difficulty index inside a multi-chart source.
	SHA256         string `json:"sha256,omitempty"`
	MD5            string `json:"md5,omitempty"` // Compatibility with osu! replay identity.
}

type Note struct {
	At         time.Duration `json:"at_ns"`
	End        time.Duration `json:"end_ns"` // Tap: End == At; hold/roll: End > At.
	Lane       int           `json:"lane"`
	Kind       string        `json:"kind"` // tap, hold, roll, mine, lift, fake
	Sounds     []Sound       `json:"sounds,omitempty"`
	TailSounds []Sound       `json:"tail_sounds,omitempty"` // O2Jam release mark (head sample again); never played, kept for chart hashes.
}

type Sound struct {
	Asset  string  `json:"asset"`
	Volume float64 `json:"volume"`        // 0..1, explicit (zero means silent).
	Pan    float64 `json:"pan,omitempty"` // -1..1
	// A medley clip plays Length (zero: to the end) from From into the asset,
	// fading in and out over the first FadeIn and the last FadeOut.
	From    time.Duration `json:"from_ns,omitempty"`
	Length  time.Duration `json:"length_ns,omitempty"`
	FadeIn  time.Duration `json:"fade_in_ns,omitempty"`
	FadeOut time.Duration `json:"fade_out_ns,omitempty"`
}

type Asset struct {
	Path   string `json:"path"`             // Relative to Source.Path's directory.
	Kind   string `json:"kind"`             // audio, image, video, bank
	Sample int    `json:"sample,omitempty"` // One-based OJN bank reference.
	Offset int64  `json:"offset,omitempty"` // Embedded asset: byte range inside Path (an OJN cover).
	Size   int64  `json:"size,omitempty"`
	Source string `json:"source,omitempty"` // A medley asset's own chart path, replacing Source.Path.
}

// Timing is a complete state, not a delta. Multiple points at one time keep
// source order. Stop is a duration during which scroll position is frozen.
type Timing struct {
	At        time.Duration `json:"at_ns"`
	BPM       float64       `json:"bpm"`
	Scroll    float64       `json:"scroll"`
	Meter     int           `json:"meter"`
	Stop      time.Duration `json:"stop_ns,omitempty"`
	Highlight bool          `json:"highlight,omitempty"`
}

type Audio struct {
	At    time.Duration `json:"at_ns"`
	Sound Sound         `json:"sound"`
}

// StoryEvent stores a background, video, break or osu sprite declaration.
// Commands retain their source syntax; unsupported commands are diagnosed.
type StoryEvent struct {
	At          time.Duration `json:"at_ns"`
	End         time.Duration `json:"end_ns"`
	Kind        string        `json:"kind"`
	Asset       string        `json:"asset,omitempty"`
	X           float64       `json:"x,omitempty"`
	Y           float64       `json:"y,omitempty"`
	Layer       string        `json:"layer,omitempty"`
	Origin      string        `json:"origin,omitempty"`
	Commands    []Command     `json:"commands,omitempty"`
	Unsupported []string      `json:"unsupported,omitempty"`
}

type Command struct {
	Kind   string        `json:"kind"`
	Easing int           `json:"easing"`
	At     time.Duration `json:"at_ns"`
	End    time.Duration `json:"end_ns"`
	Values []float64     `json:"values"`
}

// MaxSize is the largest source chart or storyboard file accepted, in bytes.
const MaxSize = 64 << 20

func New(lanes int) *Chart {
	return &Chart{Version: Version, Lanes: lanes, Assets: make(map[string]Asset)}
}

func (c *Chart) Normalize() {
	slices.SortStableFunc(c.Notes, func(a, b Note) int {
		if a.At < b.At {
			return -1
		}
		if a.At > b.At {
			return 1
		}
		return a.Lane - b.Lane
	})
	slices.SortStableFunc(c.Timing, func(a, b Timing) int { return cmpTime(a.At, b.At) })
	slices.SortStableFunc(c.Audio, func(a, b Audio) int { return cmpTime(a.At, b.At) })
	slices.SortStableFunc(c.Storyboard, func(a, b StoryEvent) int { return cmpTime(a.At, b.At) })
}

func cmpTime(a, b time.Duration) int {
	if a < b {
		return -1
	}
	if a > b {
		return 1
	}
	return 0
}
func finite(v float64) bool { return !math.IsNaN(v) && !math.IsInf(v, 0) }

const MaxTime = 24 * time.Hour

func validTime(t time.Duration) bool { return t >= -MaxTime && t <= MaxTime }

func (c *Chart) Validate() error {
	if c == nil || c.Version != Version || c.Lanes < 1 || c.Lanes > 18 {
		return fmt.Errorf("chart: invalid version or lane count")
	}
	if !validTime(c.Metadata.Preview) || !validTime(c.Metadata.LeadIn) || c.Metadata.LeadIn < 0 {
		return fmt.Errorf("chart: invalid metadata time")
	}
	if len(c.Timing) == 0 {
		return fmt.Errorf("chart: no timing points")
	}
	if len(c.Layout) != 0 && len(c.Layout) != c.Lanes {
		return fmt.Errorf("chart: layout does not match lane count")
	}
	for _, l := range c.Layout {
		if l.Role != "key" && l.Role != "scratch" || l.Group < 0 {
			return fmt.Errorf("chart: invalid lane role or group")
		}
	}
	for i, t := range c.Timing {
		if !validTime(t.At) || t.Stop > MaxTime || !finite(t.BPM) || t.BPM <= 0 || !finite(t.Scroll) || t.Scroll < 0 || t.Meter < 1 || t.Stop < 0 || (i > 0 && t.At < c.Timing[i-1].At) {
			return fmt.Errorf("chart: invalid timing point %d", i)
		}
	}
	checkSound := func(s Sound) error {
		if _, ok := c.Assets[s.Asset]; !ok || !finite(s.Volume) || s.Volume < 0 || s.Volume > 1 || !finite(s.Pan) || s.Pan < -1 || s.Pan > 1 ||
			s.From < 0 || s.Length < 0 || s.FadeIn < 0 || s.FadeOut < 0 || !validTime(s.From) || !validTime(s.Length) || s.Length > 0 && s.FadeIn+s.FadeOut > s.Length {
			return fmt.Errorf("chart: invalid sound %q", s.Asset)
		}
		return nil
	}
	ends := make([]time.Duration, c.Lanes)
	starts := make([]time.Duration, c.Lanes)
	seen := make([]bool, c.Lanes)
	for i, n := range c.Notes {
		if !validTime(n.At) || !validTime(n.End) || n.Lane < 0 || n.Lane >= c.Lanes || n.End < n.At || (i > 0 && n.At < c.Notes[i-1].At) {
			return fmt.Errorf("chart: invalid note %d", i)
		}
		switch n.Kind {
		case "tap", "mine", "lift", "fake":
			if n.End != n.At {
				return fmt.Errorf("chart: non-hold note %d has duration", i)
			}
		case "hold", "roll":
			if n.End <= n.At {
				return fmt.Errorf("chart: empty hold %d", i)
			}
		default:
			return fmt.Errorf("chart: unknown note kind %q", n.Kind)
		}
		if n.Kind != "fake" {
			if seen[n.Lane] && (n.At < ends[n.Lane] || n.At == starts[n.Lane]) {
				return fmt.Errorf("chart: overlapping notes at lane %d", n.Lane)
			}
			ends[n.Lane] = n.End
			starts[n.Lane] = n.At
			seen[n.Lane] = true
		}
		for _, s := range append(slices.Clone(n.Sounds), n.TailSounds...) {
			if err := checkSound(s); err != nil {
				return err
			}
		}
	}
	for i, a := range c.Audio {
		if !validTime(a.At) {
			return fmt.Errorf("chart: audio time out of range")
		}
		if i > 0 && a.At < c.Audio[i-1].At {
			return fmt.Errorf("chart: unordered audio")
		}
		if err := checkSound(a.Sound); err != nil {
			return err
		}
	}
	for i, s := range c.Storyboard {
		if !validTime(s.At) || !validTime(s.End) || s.End < s.At || !finite(s.X) || !finite(s.Y) || (i > 0 && s.At < c.Storyboard[i-1].At) {
			return fmt.Errorf("chart: invalid storyboard event %d", i)
		}
		if s.Asset != "" {
			if _, ok := c.Assets[s.Asset]; !ok {
				return fmt.Errorf("chart: missing storyboard asset %q", s.Asset)
			}
		}
		for _, cmd := range s.Commands {
			if !validTime(cmd.At) || !validTime(cmd.End) || cmd.End < cmd.At {
				return fmt.Errorf("chart: reversed command")
			}
			dim, ok := map[string]int{"F": 1, "M": 2, "MX": 1, "MY": 1, "S": 1, "V": 2, "R": 1, "C": 3}[cmd.Kind]
			if !ok || len(cmd.Values) != dim && len(cmd.Values) != dim*2 {
				return fmt.Errorf("chart: invalid command %q", cmd.Kind)
			}
			for _, v := range cmd.Values {
				if !finite(v) {
					return fmt.Errorf("chart: invalid command value")
				}
			}
		}
	}
	return nil
}

func (c *Chart) Duration() time.Duration {
	var end time.Duration
	for _, n := range c.Notes {
		end = max(end, n.End)
	}
	for _, a := range c.Audio {
		end = max(end, a.At)
	}
	for _, s := range c.Storyboard {
		if s.Kind == "break" || s.Kind == "sprite" {
			end = max(end, s.End)
		}
	}
	return end
}

// Digest identifies playable content and assets, independent of disk location.
func (c *Chart) Digest() (string, error) {
	if err := c.Validate(); err != nil {
		return "", err
	}
	x := *c
	x.Source.Path = ""
	x.Warnings, x.Fixes = nil, nil
	b, err := json.Marshal(x)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:]), nil
}

func Read(r io.Reader) (*Chart, error) {
	var c Chart
	d := json.NewDecoder(io.LimitReader(r, 64<<20))
	d.DisallowUnknownFields()
	if err := d.Decode(&c); err != nil {
		return nil, fmt.Errorf("read chart: %w", err)
	}
	var extra any
	if err := d.Decode(&extra); err != io.EOF {
		return nil, fmt.Errorf("chart: expected one JSON document")
	}
	if err := c.Validate(); err != nil {
		return nil, err
	}
	return &c, nil
}

func Write(w io.Writer, c *Chart) error {
	if err := c.Validate(); err != nil {
		return err
	}
	e := json.NewEncoder(w)
	e.SetIndent("", "  ")
	if err := e.Encode(c); err != nil {
		return fmt.Errorf("write chart: %w", err)
	}
	return nil
}
