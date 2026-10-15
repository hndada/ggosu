//go:build !js

package play

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"

	"ggosu/build"
	"ggosu/internal/chart"
	_ "ggosu/internal/chart/bms"
	_ "ggosu/internal/chart/ojn"
	_ "ggosu/internal/chart/osu"
	_ "ggosu/internal/chart/sm"
	"ggosu/internal/clock"
	"ggosu/internal/keyboard"
	"ggosu/internal/piano"
	"ggosu/internal/settings"
)

func (g *scene) eventInput() bool { return g.source != nil }
func (g *scene) pressed(key ebiten.Key) bool {
	if g.network != nil && (g.network.chatFocus || g.network.chatHandled) {
		return false
	}
	return inpututil.IsKeyJustPressed(key)
}

// held reports a key that is down now; chat typing keeps it from gameplay.
func (g *scene) held(key ebiten.Key) bool {
	if g.network != nil && (g.network.chatFocus || g.network.chatHandled) {
		return false
	}
	return ebiten.IsKeyPressed(key)
}

func (g *scene) hudShortcut() bool {
	return g.pressed(ebiten.KeyTab) && g.held(ebiten.KeyShift) || g.mode != "live" && g.pressed(ebiten.KeyH)
}

func (g *scene) coverShortcut() bool {
	return g.held(ebiten.KeyShift) && g.pressed(ebiten.KeyF4)
}

func (g *scene) scrollStep(ebiten.Key) float64 {
	if g.held(ebiten.KeyAltLeft) || g.held(ebiten.KeyAltRight) {
		return .1
	}
	return 1
}
func (g *scene) beforeUpdate(now time.Duration) error {
	if g.settingsPath == "" || g.editing || now-g.hudReadAt < 100*time.Millisecond {
		return nil
	}
	g.hudReadAt = now
	info, err := os.Stat(g.settingsPath)
	if err != nil {
		return nil
	} // An unavailable preferences file does not stop play.
	if info.ModTime().Equal(g.hudStamp) {
		return nil
	}
	g.hudStamp = info.ModTime()
	prefs, err := settings.Load(g.settingsPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "live HUD preferences:", err)
		return nil
	}
	d := g.display
	g.chatPrefs = prefs.ChatSettings()
	v := prefs.DisplaySettings()
	d.Layout, d.HUD, d.Skin, d.TimeDisplay = v.Layout, v.HUD, v.Skin, v.TimeDisplay
	d.ComboPosition, d.JudgementPosition, d.LaneLines = v.ComboPosition, v.JudgementPosition, v.LaneLines
	d.NoteThickness = v.NoteThickness
	if d != g.display {
		g.display = d
		g.view.setHUD(d)
	}
	return nil
}
func (g *scene) drawn(screen *ebiten.Image) {
	g.frames++
	if !g.announced {
		g.announced = true
		for _, w := range g.warnings {
			fmt.Fprintln(os.Stderr, "warning:", w)
		}
		if g.ready {
			if _, err := fmt.Fprintln(os.Stdout, "ready"); err != nil {
				g.err = err
			}
		}
	}
	if g.capture != "" && (g.finishing || g.frames == g.limit) {
		g.err = writeFile(g.capture, func(f *os.File) error { return png.Encode(f, screen) })
		g.captured = true
	}
}

// Run owns a native game's resources until the game exits.
func Run(c Config) (err error) {
	g, err := newScene(c)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, g.close()) }()
	if err := windowIcon(); err != nil {
		return err
	}
	g.window()
	if err := ebiten.RunGame(g); err != nil {
		return err
	}
	return errors.Join(g.save(), g.saveNetwork())
}

func newScene(c Config) (*scene, error) {
	g := &scene{}
	if err := g.load(c); err != nil {
		return nil, errors.Join(err, g.close())
	}
	return g, nil
}

func (g *scene) load(c Config) error {
	chatPrefs := (settings.Config{}).ChatSettings()
	if c.Settings != "" {
		prefs, recovered, err := settings.Recover(c.Settings)
		if err != nil {
			return fmt.Errorf("load HUD layout: %w", err)
		}
		c.Display.Layout = prefs.DisplaySettings().Layout
		chatPrefs = prefs.ChatSettings()
		g.english = prefs.Language == "en"
		if recovered != nil {
			fmt.Fprintln(os.Stderr, "warning: settings restored; original:", recovered.Path)
		}
	}
	*g = scene{health: c.Health, healthFail: c.HealthFail,
		scoreServer: c.ScoreServer, playerName: c.Name, roomPassword: c.RoomPassword,
		settingsPath: c.Settings, hudEdit: c.HUDEdit,
		chatPrefs: chatPrefs,
		scroll:    c.Profile.Scroll,
		autoplay:  c.Auto, speed: c.Rate, pitch: c.Pitch,
		result: c.Result, record: c.Record, report: c.Report,
		profile: c.Profile, audio: c.Audio, display: c.Display, offsets: c.Offsets,
		ready: c.Ready, limit: c.Frames, runFor: c.Duration, capture: c.Capture,
		debug: c.Debug, sectionStart: c.SectionStart, sectionEnd: c.SectionEnd,
	}
	path, index, input := c.Chart, c.Index, c.Input
	keys := strings.Join(c.Profile.Keys, ",")
	// bindings normalizes the configured keys before storing the replay profile.
	g.profile.Keys = nil
	replayPath, replayRecord := c.Replay, c.ReplayRecord
	mute, kind, seed := c.Mute, c.Mod, c.Seed
	server, name, roomID, roomName := c.Server, c.Name, c.Room, c.RoomName
	multiReport, multiToken, multiRevision := c.MultiReport, c.MultiToken, c.MultiRevision
	autoStart, chatOpen := c.MultiStart, c.ChatOpen
	if g.limit < 0 || g.runFor < 0 || (g.capture != "" && g.limit == 0 && g.runFor == 0) || math.IsNaN(g.speed) || math.IsInf(g.speed, 0) || g.speed < .5 || g.speed > 2 || input != "native" && input != "ebiten" || g.autoplay && (replayPath != "" || replayRecord != "") || g.hudEdit && !g.autoplay || replayPath != "" && replayRecord != "" || g.sectionEnd != 0 && (g.sectionStart >= g.sectionEnd || replayPath != "" || replayRecord != "" || c.Server != "" || g.hudEdit) {
		return fmt.Errorf("invalid playback options")
	}
	g.initialRate, g.practice, g.mode = g.speed, g.speed != 1, "live"
	if err := g.audio.Validate(); err != nil {
		return err
	}
	if err := g.display.Validate(); err != nil {
		return err
	}
	if err := settings.ValidateScoreServer(c.ScoreServer); err != nil {
		return err
	}
	if math.IsNaN(g.scroll) || math.IsInf(g.scroll, 0) || g.scroll < settings.MinScroll || g.scroll > settings.MaxScroll {
		return fmt.Errorf("invalid scroll speed")
	}
	charts, err := chart.Load(path)
	if err != nil {
		return err
	}
	if index < 0 || index >= len(charts) {
		return fmt.Errorf("difficulty index %d out of range", index)
	}
	g.chart = charts[index]
	if seed > math.MaxUint32 {
		return fmt.Errorf("random seed is outside uint32 range")
	}
	g.mods, err = chart.NewMod(g.chart, kind, uint32(seed))
	if err != nil {
		return err
	}
	g.mods.Visibility = c.Visibility
	g.mods.O2Jam, g.mods.Pills = c.O2Jam, c.Pills
	if err := g.mods.Validate(g.chart.Lanes); err != nil {
		return err
	}
	g.bindings, err = bindings(g.chart.Lanes, keys)
	if err != nil {
		return err
	}
	g.physical = make([]bool, len(g.bindings))
	for _, b := range g.bindings {
		g.profile.Keys = append(g.profile.Keys, b.name)
	}
	g.profile.Scroll = g.scroll
	g.warnings = append(g.warnings, g.chart.Warnings...)
	if g.autoplay {
		g.mode = "auto"
	} else if replayRecord != "" {
		g.mode = "replay"
		r, err := piano.ReadRecord(replayRecord)
		if err != nil {
			return err
		}
		g.pitch = r.PitchEnabled()
		if err := r.Replay.Match(g.chart, piano.Rules); err != nil {
			return err
		}
		g.replayIncomplete, g.replayEnd = !r.Completed, r.Replay.Duration
		g.offsets = r.Replay.Offsets
		g.mods = r.Replay.Mods
		if r.Replay.Profile != nil {
			g.profile = *r.Replay.Profile
		}
		g.player, err = piano.NewPlayer(r.Replay)
		if err != nil {
			return err
		}
	} else if replayPath != "" {
		g.mode = "replay"
		f, err := os.Open(replayPath)
		if err != nil {
			return err
		}
		var r piano.Replay
		var readErr error
		if strings.EqualFold(filepath.Ext(replayPath), ".osr") {
			r, readErr = piano.ReadOSR(f, g.chart, piano.Rules)
		} else {
			r, readErr = piano.ReadReplay(f)
		}
		closeErr := f.Close()
		if readErr != nil {
			return readErr
		}
		if closeErr != nil {
			return closeErr
		}
		if err := r.Match(g.chart, piano.Rules); err != nil {
			return err
		}
		g.offsets = r.Offsets
		g.mods = r.Mods
		if r.Profile != nil {
			g.profile = *r.Profile
		}
		g.player, err = piano.NewPlayer(r)
		if err != nil {
			return err
		}
	}
	if err := g.prepare(); err != nil {
		return err
	}
	g.bindings, err = bindings(g.chart.Lanes, strings.Join(g.profile.Keys, ","))
	if err != nil {
		return err
	}
	if !mute || server != "" {
		g.bank, err = loadCachedBank(g.chart, charts, c.Cache)
		if err != nil {
			return err
		}
		g.warnings = append(g.warnings, g.bank.warnings...)
		if err := g.bank.SetPitch(g.pitch); err != nil {
			return err
		}
		if err := g.bank.SetRate(g.speed); err != nil {
			return err
		}
		if mute {
			g.audio.Music, g.audio.Keys, g.audio.Effects, g.audio.Hit, g.audio.Guide = 0, 0, 0, 0, 0
		}
		g.bank.SetAudio(g.audio)
		if err := g.prepareSectionMusic(); err != nil {
			return err
		}
	}
	if err := g.loadImages(); err != nil {
		return err
	}
	g.clock, err = clock.New()
	if err != nil {
		return err
	}
	if server != "" {
		if err := g.connect(server, name, roomID, roomName, multiReport, multiToken, multiRevision, autoStart); err != nil {
			return err
		}
		if chatOpen {
			g.network.chat.Show()
		}
	}
	if err := g.initView(); err != nil {
		return err
	}
	if input == "native" && g.player == nil {
		g.source, err = keyboard.Start(g.clock, g.box.Push)
		if err != nil {
			return err
		}
	}
	return nil
}

func windowIcon() error {
	icon, err := png.Decode(bytes.NewReader(build.Icon))
	if err != nil {
		return fmt.Errorf("decode window icon: %w", err)
	}
	ebiten.SetWindowIcon([]image.Image{icon})
	return nil
}

func (g *scene) window() {
	setFrameRate(g.display)
	ebiten.SetFullscreen(false)
	ebiten.SetWindowDecorated(true)
	ebiten.SetWindowSize(g.display.WindowSize())
	ebiten.SetWindowResizingMode(ebiten.WindowResizingModeEnabled)
	switch g.display.WindowMode {
	case "fullscreen":
		ebiten.SetFullscreen(true)
	case "borderless":
		if m := ebiten.Monitor(); m != nil {
			ebiten.SetWindowDecorated(false)
			ebiten.SetWindowSize(m.Size())
			ebiten.SetWindowPosition(0, 0)
		}
	}
	ebiten.SetCursorMode(ebiten.CursorModeHidden)
	parts := []string{"gosu"}
	for _, text := range []string{g.chart.Metadata.Title, g.chart.Metadata.Artist, g.chart.Metadata.Difficulty} {
		if text != "" {
			parts = append(parts, text)
		}
	}
	ebiten.SetWindowTitle(strings.Join(parts, " — "))
	ebiten.SetRunnableOnUnfocused(true)
}

func (g *scene) windowControls() error {
	if !ebiten.IsFocused() || !ebiten.IsKeyPressed(ebiten.KeyAlt) ||
		ebiten.IsKeyPressed(ebiten.KeyControl) || ebiten.IsKeyPressed(ebiten.KeyMeta) || ebiten.IsKeyPressed(ebiten.KeyShift) ||
		(!inpututil.IsKeyJustPressed(ebiten.KeyEnter) && !inpututil.IsKeyJustPressed(ebiten.KeyNumpadEnter)) {
		return nil
	}
	mode := "fullscreen"
	if ebiten.IsFullscreen() || g.display.WindowMode == "borderless" {
		mode = "windowed"
	}
	if g.settingsPath != "" {
		c, recovered, err := settings.Recover(g.settingsPath)
		if err != nil {
			return fmt.Errorf("read window preferences: %w", err)
		}
		if recovered != nil {
			g.warnings = append(g.warnings, "화면 설정 저장 전에 손상된 설정을 복구했습니다.")
		}
		d := c.DisplaySettings()
		d.WindowMode = mode
		c.Display = &d
		if err := settings.Save(g.settingsPath, c); err != nil {
			return fmt.Errorf("write window preferences: %w", err)
		}
	}
	g.display.WindowMode = mode
	restoreWindow()
	g.window()
	return nil
}

// close releases per-song resources; the process keeps the graphics driver,
// window and audio context for the next song. It is safe after partial loading.
func (g *scene) close() error {
	err := errors.Join(g.saveVolume(), g.saveProfile())
	if g.source != nil {
		g.source.SetActive(false)
		err = errors.Join(err, g.source.Close())
		g.source = nil
	}
	if g.network != nil {
		g.network.chatInput.Cancel()
		err = errors.Join(err, g.network.client.Close())
		g.network = nil
	}
	if g.bank != nil {
		g.bank.Close()
		g.bank = nil
	}
	for _, img := range g.images {
		img.Deallocate()
	}
	g.images = nil
	if g.frame != nil {
		g.frame.Deallocate()
		g.frame = nil
	}
	if g.view != nil {
		for _, img := range []*ebiten.Image{g.view.base, g.view.notes, g.view.timing, g.view.hudLayer} {
			if img != nil {
				img.Deallocate()
			}
		}
		for _, img := range g.view.sprites.cache {
			img.Deallocate()
		}
		g.view = nil
	}
	return err
}
