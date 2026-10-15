package settings

import "fmt"

type Display struct {
	NoteThickness     int    `json:"noteThickness,omitempty"` // Percent of the skin's height; zero means 100.
	LaneLines         bool   `json:"laneLines,omitempty"`     // Lane separators are off by default.
	Ghost             string `json:"ghost,omitempty"`         // Empty means target (pace); max compares the best reachable score; off hides it.
	GhostScore        int    `json:"ghostScore,omitempty"`    // Zero uses 900,000.
	TPS               int    `json:"tps,omitempty"`           // Zero uses 240; -1 follows FPS.
	FrameLimit        string `json:"frameLimit,omitempty"`    // Empty uses monitor VSync.
	Layout            Layout `json:"hudLayout,omitzero"`
	Dim               int    `json:"dim"`
	Background        bool   `json:"background"`
	Storyboard        bool   `json:"storyboard"`
	HUD               string `json:"hud,omitempty"`           // Empty means standard for older settings.
	ComboPosition     string `json:"comboPosition,omitempty"` // Empty means center.
	JudgementPosition string `json:"judgementPosition,omitempty"`
	TimingThreshold   string `json:"timingThreshold,omitempty"` // Early/Late for this grade or worse; empty means cool, off hides it.
	TimeDisplay       string `json:"timeDisplay,omitempty"`     // Empty means pie.
	// Skin selects the play field. Empty means argon.
	Skin string `json:"skin,omitempty"`
	// Window modes follow osu!lazer: windowed, borderless (a frameless window covering
	// the monitor) and fullscreen. Empty means windowed.
	WindowMode string `json:"windowMode,omitempty"`
	// Resolution is the window size in logical pixels. Empty keeps 1600x900.
	// Fullscreen and borderless use the monitor size instead.
	Resolution string `json:"resolution,omitempty"`
	// Letterbox shrinks the game area like lazer's screen scaling. Sizes are percent
	// of the window (20..100, zero means lazer's default 80); positions move the area
	// from the centre in percent of the free space (-50..50).
	Letterbox       bool `json:"letterbox,omitempty"`
	LetterboxWidth  int  `json:"letterboxWidth,omitempty"`
	LetterboxHeight int  `json:"letterboxHeight,omitempty"`
	LetterboxX      int  `json:"letterboxX,omitempty"`
	LetterboxY      int  `json:"letterboxY,omitempty"`
	CursorSize      int  `json:"cursorSize,omitempty"` // Percent, 50..200; zero means 100 as in lazer.
	// The rest follow ONGEKI's options. LaneDim darkens the lane background like
	// レーンの明るさ; 100 hides it like ステルスフィールド.
	LaneDim int `json:"laneDim,omitempty"`
	// NoteColour: empty keeps the skin's column colours, snap colours each note by its
	// beat like lazer's timing-based note colouring, assist uses a colour-blind safe pair.
	NoteColour string `json:"noteColour,omitempty"`
	// JudgementShow shows this grade or worse (cool, good, miss); empty shows every grade, off none.
	JudgementShow string `json:"judgementShow,omitempty"`
	HideCombo     bool   `json:"hideCombo,omitempty"`
	// Abort ends a live play once the best reachable score falls below the ghost target,
	// like ONGEKI's バトル中断設定.
	Abort bool `json:"abort,omitempty"`
}

var resolutions = map[string][2]int{
	"1024x640":  {1024, 640},
	"1280x720":  {1280, 720},
	"1366x768":  {1366, 768},
	"1600x900":  {1600, 900},
	"1920x1080": {1920, 1080},
	"2560x1440": {2560, 1440},
	"3840x2160": {3840, 2160},
}

func (d Display) WindowSize() (width, height int) {
	if size, ok := resolutions[d.Resolution]; ok {
		return size[0], size[1]
	}
	return 1600, 900
}

func (d Display) TickRate() int {
	if d.TPS == 0 {
		return 240
	}
	return d.TPS
}

func (d Display) Thickness() int {
	if d.NoteThickness == 0 {
		return 100
	}
	return d.NoteThickness
}

// Area returns the letterbox width, height, x and y with defaults filled in.
func (d Display) Area() (w, h, x, y int) {
	if !d.Letterbox {
		return 100, 100, 0, 0
	}
	w, h = d.LetterboxWidth, d.LetterboxHeight
	if w == 0 {
		w = 80
	}
	if h == 0 {
		h = 80
	}
	return w, h, d.LetterboxX, d.LetterboxY
}

// Cursor returns the cursor scale in percent.
func (d Display) Cursor() int {
	if d.CursorSize == 0 {
		return 100
	}
	return d.CursorSize
}

func (d Display) TargetScore() int {
	if d.GhostScore == 0 {
		return 900000
	}
	return d.GhostScore
}

func (d Display) Validate() error {
	if d.NoteThickness != 0 && (d.NoteThickness < 50 || d.NoteThickness > 200) {
		return fmt.Errorf("note thickness must be 50..200 percent")
	}
	if d.Ghost != "" && d.Ghost != "target" && d.Ghost != "max" && d.Ghost != "off" || d.GhostScore < 0 || d.GhostScore > 100000000 {
		return fmt.Errorf("invalid ghost mode or target score")
	}
	if d.TPS != 0 && d.TPS != -1 && (d.TPS < 30 || d.TPS > 1000) {
		return fmt.Errorf("TPS must be 30..1000 or -1 to follow FPS")
	}
	if d.FrameLimit != "" && d.FrameLimit != "vsync" && d.FrameLimit != "unlimited" {
		return fmt.Errorf("frame limit must be vsync or unlimited")
	}
	if err := d.Layout.Validate(); err != nil {
		return err
	}
	if d.Dim < 0 || d.Dim > 100 {
		return fmt.Errorf("background dim must be between 0 and 100")
	}
	if d.HUD != "" && d.HUD != "standard" && d.HUD != "focus" {
		return fmt.Errorf("HUD must be standard or focus")
	}
	if d.Skin != "" && d.Skin != "argon" && d.Skin != "triangles" && d.Skin != "classic" && d.Skin != "circles" && d.Skin != "o2jam" {
		return fmt.Errorf("skin must be argon, triangles, classic, circles or o2jam")
	}
	if d.TimeDisplay != "" && d.TimeDisplay != "bar" && d.TimeDisplay != "pie" {
		return fmt.Errorf("time display must be bar or pie")
	}
	if d.TimingThreshold != "" && d.TimingThreshold != "kool" && d.TimingThreshold != "cool" && d.TimingThreshold != "good" && d.TimingThreshold != "off" {
		return fmt.Errorf("timing threshold must be kool, cool, good or off")
	}
	for _, p := range []string{d.ComboPosition, d.JudgementPosition} {
		if p != "" && p != "center" && p != "below" && p != "left" {
			return fmt.Errorf("combo and judgement positions must be center, below or left")
		}
	}
	if d.WindowMode != "" && d.WindowMode != "windowed" && d.WindowMode != "borderless" && d.WindowMode != "fullscreen" {
		return fmt.Errorf("window mode must be windowed, borderless or fullscreen")
	}
	if _, ok := resolutions[d.Resolution]; d.Resolution != "" && !ok {
		return fmt.Errorf("unsupported window resolution: %q", d.Resolution)
	}
	for _, v := range []int{d.LetterboxWidth, d.LetterboxHeight} {
		if v != 0 && (v < 20 || v > 100) {
			return fmt.Errorf("letterbox size must be between 20 and 100 percent")
		}
	}
	for _, v := range []int{d.LetterboxX, d.LetterboxY} {
		if v < -50 || v > 50 {
			return fmt.Errorf("letterbox position must be between -50 and 50 percent")
		}
	}
	if d.CursorSize != 0 && (d.CursorSize < 50 || d.CursorSize > 200) {
		return fmt.Errorf("cursor size must be between 50 and 200 percent")
	}
	if d.LaneDim < 0 || d.LaneDim > 100 {
		return fmt.Errorf("lane dim must be between 0 and 100")
	}
	if d.NoteColour != "" && d.NoteColour != "snap" && d.NoteColour != "assist" {
		return fmt.Errorf("note colour must be snap or assist")
	}
	if d.JudgementShow != "" && d.JudgementShow != "cool" && d.JudgementShow != "good" && d.JudgementShow != "miss" && d.JudgementShow != "off" {
		return fmt.Errorf("judgement display must be cool, good, miss or off")
	}
	return nil
}
func (c Config) DisplaySettings() Display {
	if c.Display != nil {
		return *c.Display
	}
	return Display{Dim: 75, Background: true, Storyboard: true}
}
