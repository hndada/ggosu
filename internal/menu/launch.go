package menu

import (
	"crypto/rand"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"time"

	"ggosu/internal/chart"
	"ggosu/internal/piano"
)

// launch is a validated selection and the arguments for its game process.
type launch struct {
	selection  Selection
	args       []string
	resultPath string
	message    string
	title      string
	lanes      int
}

// prepare is called with a.mu held.
func (a *App) prepare(s Selection, networkArgs []string) (launch, error) {
	var c *chart.Chart
	var chartPath string
	index := s.Index
	if len(s.Medley) > 0 {
		var err error
		if c, chartPath, err = a.medleyChart(s.Medley); err != nil {
			return launch{}, err
		}
		index = 0
	} else {
		selected, err := a.songs.Lookup(s)
		if err != nil {
			return launch{}, err
		}
		charts, err := chart.Load(selected.Path)
		if err != nil {
			return launch{}, err
		}
		if s.Index < 0 || s.Index >= len(charts) {
			return launch{}, fmt.Errorf("난이도가 변경됐습니다. 목록을 새로 읽어 주세요")
		}
		c, chartPath = charts[s.Index], selected.Path
	}
	parts := []string{"gosu"}
	for _, text := range []string{c.Metadata.Title, c.Metadata.Artist, c.Metadata.Difficulty} {
		if text != "" {
			parts = append(parts, text)
		}
	}
	title := strings.Join(parts, " — ")
	if s.Mode == "" {
		s.Mode = "live"
	}
	if s.Rate == 0 {
		s.Rate = 1
	}
	if s.Mode != "replay" {
		if err := chart.ValidateVisibility(s.Visibility); err != nil {
			return launch{}, err
		}
		if err := chart.ValidateO2Jam(s.O2Jam); err != nil {
			return launch{}, err
		}
		if math.IsNaN(s.Rate) || math.IsInf(s.Rate, 0) || s.Rate < .5 || s.Rate > 2 {
			return launch{}, fmt.Errorf("Time Rate는 0.5~2.0배여야 합니다")
		}
		if (s.Mod == "random" || s.Mod == "mirror+random") && s.Seed == nil {
			var seed [4]byte
			if _, err := rand.Read(seed[:]); err != nil {
				return launch{}, err
			}
			n := binary.LittleEndian.Uint32(seed[:])
			s.Seed = &n
		}
		seed := uint32(0)
		if (s.Mod == "random" || s.Mod == "mirror+random") && s.Seed != nil {
			seed = *s.Seed
		} else {
			s.Seed = nil
		}
		if _, err := chart.NewMod(c, s.Mod, seed); err != nil {
			return launch{}, err
		}
	}
	if len(s.Medley) > 0 && (len(networkArgs) > 0 || s.Mode != "live" && s.Mode != "auto" || s.Section != nil) {
		return launch{}, fmt.Errorf("메들리는 싱글 플레이만 됩니다")
	}
	args := []string{"-chart", chartPath, "-index", fmt.Sprint(index), "-ready"}
	args = append(args, networkArgs...)
	p, err := a.profile(c.Lanes)
	if err != nil {
		return launch{}, err
	}
	p = p.Effective()
	args = append(args, "-keys", strings.Join(p.Keys, ","))
	args = append(args, "-scroll", fmt.Sprint(p.Scroll))
	args = append(args, "-scroll-mode", p.ScrollMode, "-exposure", fmt.Sprint(p.ExposureMS), "-speed-change", fmt.Sprint(p.SpeedChange))
	args = append(args, "-cover", p.LaneCover().Flag())
	settingsPath, _, err := a.settingsKey(Selection{})
	if err != nil {
		return launch{}, err
	}
	config, err := a.loadSettings(settingsPath)
	if err != nil {
		return launch{}, err
	}
	audio := config.AudioSettings()
	pitch := s.Pitch == nil || *s.Pitch
	display := config.DisplaySettings()
	args = append(args, "-settings", settingsPath)
	args = append(args, "-cache", filepath.Join(filepath.Dir(settingsPath), "cache", "pcm"))
	args = append(args, "-lane-lines="+fmt.Sprint(display.LaneLines))
	args = append(args, "-dim", fmt.Sprint(display.Dim), "-background="+fmt.Sprint(display.Background), "-storyboard="+fmt.Sprint(display.Storyboard))
	args = append(args, "-hud", display.HUD)
	args = append(args, "-note-thickness", fmt.Sprint(display.Thickness()))
	args = append(args, "-time-display", display.TimeDisplay)
	args = append(args, "-ghost", display.Ghost, "-ghost-score", fmt.Sprint(display.TargetScore()))
	args = append(args, "-tps", fmt.Sprint(display.TickRate()), "-frame-limit", display.FrameLimit)
	if display.Skin != "" {
		args = append(args, "-skin", display.Skin)
	}
	args = append(args, "-combo-position", display.ComboPosition, "-judgement-position", display.JudgementPosition)
	args = append(args, "-timing-threshold", display.TimingThreshold)
	args = append(args, "-lane-dim", fmt.Sprint(display.LaneDim), "-note-colour", display.NoteColour, "-judgement-show", display.JudgementShow, "-hide-combo="+fmt.Sprint(display.HideCombo), "-abort="+fmt.Sprint(display.Abort))
	if display.WindowMode != "" {
		args = append(args, "-window-mode", display.WindowMode)
	}
	if display.Resolution != "" {
		args = append(args, "-resolution", display.Resolution)
	}
	w, h, x, y := display.Area()
	args = append(args, "-letterbox="+fmt.Sprint(display.Letterbox), "-letterbox-width", fmt.Sprint(w), "-letterbox-height", fmt.Sprint(h), "-letterbox-x", fmt.Sprint(x), "-letterbox-y", fmt.Sprint(y))
	args = append(args, "-cursor-size", fmt.Sprint(display.Cursor()))
	args = append(args, "-master-volume", fmt.Sprint(audio.Master), "-music-volume", fmt.Sprint(audio.Music), "-key-volume", fmt.Sprint(audio.Keys), "-effect-volume", fmt.Sprint(audio.Effects))
	args = append(args, "-hit-volume", fmt.Sprint(audio.Hit), "-hit-sound", audio.HitSound, "-guide-volume", fmt.Sprint(audio.Guide))
	if s.Mode != "replay" {
		args = append(args, "-speed", fmt.Sprint(s.Rate), "-mod", s.Mod, "-visibility", s.Visibility)
		if s.O2Jam != "" && len(networkArgs) == 0 { // The event mod stays out of multiplayer.
			args = append(args, "-o2jam", s.O2Jam, "-pills="+fmt.Sprint(s.Pills))
		}
		if s.Seed != nil {
			args = append(args, "-seed", fmt.Sprint(*s.Seed))
		}
		offsetKey := s
		if len(s.Medley) > 0 {
			offsetKey = Selection{} // Chart offsets belong to the source songs; keep only the global one.
		}
		o, err := a.offsets(offsetKey)
		if err != nil {
			return launch{}, err
		}
		args = append(args, "-offset", fmt.Sprint(o.GlobalMS), "-chart-offset", fmt.Sprint(o.ChartMS), "-judge-offset", fmt.Sprint(o.JudgeMS))
	}
	if s.Health < 0 || s.Health > 3 || s.HealthFail < 0 || s.HealthFail > 3 || s.Health > 0 && s.HealthFail > s.Health {
		return launch{}, fmt.Errorf("invalid health level")
	}
	args = append(args, "-health", fmt.Sprint(s.Health), "-health-fail", fmt.Sprint(s.HealthFail))
	if s.Section != nil && (s.Mode == "live" || s.Mode == "auto") && len(networkArgs) == 0 {
		r := *s.Section
		if math.IsNaN(r.Start) || math.IsNaN(r.End) || math.IsInf(r.Start, 0) || math.IsInf(r.End, 0) || r.Start >= r.End {
			return launch{}, fmt.Errorf("연습 구간이 올바르지 않습니다")
		}
		seconds := func(v float64) string { return time.Duration(v * float64(time.Second)).String() }
		if _, err := c.Section(time.Duration(r.Start*float64(time.Second)), time.Duration(r.End*float64(time.Second))); err != nil {
			return launch{}, fmt.Errorf("연습 구간에 노트가 없습니다")
		}
		args = append(args, "-section-start", seconds(r.Start), "-section-end", seconds(r.End), "-pitch="+fmt.Sprint(pitch))
		if s.Mode == "auto" {
			args = append(args, "-auto")
		}
		// The section loops until the player leaves and, like the HUD editor, leaves no record.
		return launch{selection: s, args: args, lanes: c.Lanes, message: "연습 구간", title: title}, nil
	}
	s.Section = nil
	if len(s.Medley) > 0 {
		// Each run is a new chart, so like a practice section it leaves no record.
		if s.Mode == "auto" {
			args = append(args, "-auto")
		}
		return launch{selection: s, args: append(args, "-pitch="+fmt.Sprint(pitch)), lanes: c.Lanes, message: "메들리", title: title}, nil
	}
	switch s.Mode {
	case "live":
	case "auto":
		args = append(args, "-auto")
	case "hud":
		// The HUD editor loops Auto play and leaves no record.
		return launch{selection: s, args: append(args, "-auto", "-hud-edit", "-pitch="+fmt.Sprint(pitch)), lanes: c.Lanes, message: "HUD 편집", title: title}, nil
	case "replay":
		path, err := a.replayPath(s.ReplayID)
		if err != nil {
			return launch{}, err
		}
		r, err := a.readRecord(path)
		if err != nil {
			return launch{}, err
		}
		if err := r.Replay.Match(c, piano.Rules); err != nil {
			return launch{}, err
		}
		s.Mod, s.Rate = r.Replay.Mods.Kind, r.Rate
		s.Visibility = r.Replay.Mods.Visibility
		s.O2Jam, s.Pills = r.Replay.Mods.O2Jam, r.Replay.Mods.Pills
		if s.Rate < .5 || s.Rate > 2 {
			return launch{}, fmt.Errorf("Time Rate는 0.5~2.0배여야 합니다")
		}
		pitch = r.PitchEnabled()
		s.Pitch = &pitch
		if s.Mod == "random" || s.Mod == "mirror+random" {
			n := r.Replay.Mods.Seed
			s.Seed = &n
		} else {
			s.Seed = nil
		}
		args = append(args, "-replay-record", path, "-speed", fmt.Sprint(r.Rate))
	default:
		return launch{}, fmt.Errorf("잘못된 플레이 모드입니다")
	}
	args = append(args, "-pitch="+fmt.Sprint(pitch))
	var id [16]byte
	if _, err := rand.Read(id[:]); err != nil {
		return launch{}, err
	}
	resultPath, err := a.replayPath(hex.EncodeToString(id[:]))
	if err != nil {
		return launch{}, err
	}
	if err := os.MkdirAll(filepath.Dir(resultPath), 0755); err != nil {
		return launch{}, fmt.Errorf("기록 폴더 준비: %w", err)
	}
	args = append(args, "-result", resultPath)
	return launch{
		selection:  s,
		args:       args,
		resultPath: resultPath,
		lanes:      c.Lanes,
		message:    fmt.Sprintf("플레이: %s / %s / %dK", c.Metadata.Title, c.Metadata.Difficulty, c.Lanes),
		title:      title,
	}, nil
}
