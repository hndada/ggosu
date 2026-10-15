package play

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"math"
	"os"
	"strings"
	"syscall/js"
	"time"

	"github.com/hajimehoshi/ebiten/v2"

	"ggosu/internal/chart"
	"ggosu/internal/clock"
	"ggosu/internal/keyboard"
	"ggosu/internal/piano"
	"ggosu/internal/settings"
)

type network struct{}

func (g *scene) updateChat() error     { return nil }
func (g *scene) chatTyping() bool      { return false }
func (g *scene) windowControls() error { return nil }

func (g *scene) networkUpdate(time.Duration) (bool, error)      { return false, nil }
func (g *scene) networkFinished() (bool, error)                 { return true, nil }
func (g *scene) gameStart(now time.Duration) time.Duration      { return now }
func (g *scene) networkAdvance() error                          { return nil }
func (g *scene) networkState(*viewState)                        {}
func (g *scene) networkEnabled() bool                           { return false }
func (g *scene) networkFinishReady() bool                       { return true }
func (g *scene) networkPosition(at time.Duration) time.Duration { return at }

var web struct {
	controls        map[ebiten.Key]bool
	scrollFine      map[ebiten.Key]bool // Alt at the time of PageUp/Down, even if released before Update.
	suspended       bool
	at              time.Duration
	listeners       []js.Func
	pauseRequest    int
	restartRequest  bool
	retryHeld       bool // Backquote is down.
	retryChord      bool // R was pressed with Ctrl or Command.
	ctrlHeld        bool
	shiftHeld       bool
	hudToggle       bool
	coverToggle     bool
	peekHeld        bool // Tab is down.
	lastPause       bool
	lastCountdown   int
	volumeKeys      []ebiten.Key
	display         *settings.Display
	displayRevision int
}

func post(kind string, value any) {
	b, err := json.Marshal(value)
	if err != nil {
		js.Global().Get("console").Call("error", err.Error())
		return
	}
	js.Global().Get("parent").Call("postMessage", map[string]any{"type": "gosu:" + kind, "data": string(b)}, js.Global().Get("location").Get("origin"))
}

func (g *scene) persistHUD(d settings.Display) (err error) {
	if err := d.Validate(); err != nil {
		return err
	}
	b, err := json.Marshal(d)
	if err != nil {
		return err
	}
	// Storage denial is a JavaScript exception; keep the editor open for retry.
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("browser preferences: %v", r)
		}
	}()
	storage := js.Global().Get("localStorage")
	storage.Call("setItem", "ggosu:display:backup", string(b))
	storage.Call("setItem", "ggosu:display", string(b))
	post("display", d)
	return nil
}

// RunBrowser runs the game in the embedding page until it removes the iframe.
func RunBrowser() error {
	err := runBrowser()
	if err != nil {
		post("error", err.Error())
	}
	// Audio/browser callbacks can arrive after RunGame ends. The parent removes
	// this iframe after the result/error message, which owns runtime teardown.
	// Keep Go alive until then rather than letting a late callback resume an
	// exited program. runBrowser has already stopped audio and input listeners.
	select {}
}

func runBrowser() (err error) {
	config := js.Global().Get("gosuConfig")
	if config.IsUndefined() {
		return fmt.Errorf("missing browser configuration")
	}
	c, err := chart.Read(strings.NewReader(config.Get("chart").String()))
	if err != nil {
		return err
	}
	g := &scene{chart: c, scroll: settings.DefaultScroll, speed: config.Get("speed").Float(), result: "browser", mode: config.Get("mode").String()}
	g.pitch = config.Get("pitch").IsUndefined() || config.Get("pitch").Bool()
	if math.IsNaN(g.speed) || math.IsInf(g.speed, 0) || g.speed < .5 || g.speed > 2 {
		return fmt.Errorf("invalid browser rate")
	}
	g.initialRate, g.practice = g.speed, g.speed != 1
	if !config.Get("mod").IsUndefined() {
		seed := config.Get("seed").Float()
		if math.IsNaN(seed) || math.IsInf(seed, 0) || seed < 0 || seed > math.MaxUint32 || math.Trunc(seed) != seed {
			return fmt.Errorf("invalid browser random seed")
		}
		g.mods, err = chart.NewMod(c, config.Get("mod").String(), uint32(seed))
		if err != nil {
			return err
		}
	}
	g.audio = (settings.Config{}).AudioSettings()
	if v := config.Get("visibility"); !v.IsUndefined() {
		g.mods.Visibility = v.String()
	}
	if v := config.Get("o2jam"); !v.IsUndefined() {
		g.mods.O2Jam = v.String()
	}
	if v := config.Get("pills"); !v.IsUndefined() {
		g.mods.Pills = v.Truthy()
	}
	if err := g.mods.Validate(c.Lanes); err != nil {
		return err
	}
	if !config.Get("audio").IsUndefined() {
		if err := json.Unmarshal([]byte(config.Get("audio").String()), &g.audio); err != nil {
			return fmt.Errorf("browser audio settings: %w", err)
		}
		if err := g.audio.Validate(); err != nil {
			return err
		}
	}
	defer func() { err = errors.Join(err, g.saveVolume()) }()
	g.display = (settings.Config{}).DisplaySettings()
	if !config.Get("display").IsUndefined() {
		if err := json.Unmarshal([]byte(config.Get("display").String()), &g.display); err != nil {
			return fmt.Errorf("browser display settings: %w", err)
		}
		if err := g.display.Validate(); err != nil {
			return err
		}
	}
	g.bindings, err = bindings(c.Lanes, "")
	if err != nil {
		return err
	}
	if v := config.Get("profile"); !v.IsUndefined() {
		if err := json.Unmarshal([]byte(v.String()), &g.profile); err != nil {
			return fmt.Errorf("browser key and scroll settings: %w", err)
		}
	}
	if v := config.Get("offsets"); !v.IsUndefined() {
		if err := json.Unmarshal([]byte(v.String()), &g.offsets); err != nil {
			return fmt.Errorf("browser timing settings: %w", err)
		}
	}
	if v := config.Get("name"); !v.IsUndefined() {
		g.playerName = v.String()
	}
	if v := config.Get("section"); !v.IsUndefined() {
		var section struct{ Start, End float64 }
		if err := json.Unmarshal([]byte(v.String()), &section); err != nil {
			return fmt.Errorf("browser practice section: %w", err)
		}
		if g.mode != "live" && g.mode != "auto" || math.IsNaN(section.Start) || math.IsNaN(section.End) || math.IsInf(section.Start, 0) || math.IsInf(section.End, 0) || section.Start >= section.End || math.Abs(section.Start) > chart.MaxTime.Seconds() || math.Abs(section.End) > chart.MaxTime.Seconds() {
			return fmt.Errorf("invalid browser practice section")
		}
		g.sectionStart, g.sectionEnd = time.Duration(section.Start*float64(time.Second)), time.Duration(section.End*float64(time.Second))
		g.practice = true
	}
	g.physical = make([]bool, c.Lanes)
	g.warnings = append(g.warnings, c.Warnings...)
	g.health = 3
	if v := config.Get("health"); !v.IsUndefined() {
		g.health = v.Int()
	}
	if v := config.Get("healthFail"); !v.IsUndefined() {
		g.healthFail = v.Int()
	}
	switch g.mode {
	case "live":
	case "auto":
		g.autoplay = true
	case "hud":
		// The settings window's HUD editor: looping Auto play that leaves no record.
		g.autoplay, g.hudEdit, g.mode = true, true, "auto"
		g.preview = !config.Get("preview").IsUndefined() && config.Get("preview").Bool()
	case "replay":
		r, err := piano.DecodeRecord(strings.NewReader(config.Get("replay").String()))
		if err != nil {
			return err
		}
		g.pitch = r.PitchEnabled()
		if err := r.Replay.Match(c, piano.Rules); err != nil {
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
	default:
		return fmt.Errorf("invalid browser play mode")
	}
	if err := g.prepare(); err != nil {
		return err
	}
	if g.preview {
		g.scroll, g.profile.Scroll = 11.4, 11.4
	}
	g.bindings, err = bindings(c.Lanes, strings.Join(g.profile.Keys, ","))
	if err != nil {
		return err
	}
	assets := map[string][]byte{}
	values := config.Get("assets")
	keys := js.Global().Get("Object").Call("keys", values)
	for i := 0; i < keys.Length(); i++ {
		path := keys.Index(i).String()
		v := values.Get(path)
		data := make([]byte, v.Get("byteLength").Int())
		js.CopyBytesToGo(data, v)
		assets[path] = data
	}
	g.bank, err = loadBankAssets(c, func(a chart.Asset) ([]byte, error) {
		b, ok := assets[a.Path]
		if !ok {
			return nil, fmt.Errorf("%s: %w", a.Path, os.ErrNotExist)
		}
		return b, nil
	})
	if err != nil {
		return err
	}
	defer g.bank.Close()
	g.bank.SetAudio(g.audio)
	g.warnings = append(g.warnings, g.bank.warnings...)
	if err := g.bank.SetPitch(g.pitch); err != nil {
		return err
	}
	if err := g.bank.SetRate(g.speed); err != nil {
		return err
	}
	if err := g.prepareSectionMusic(); err != nil {
		return err
	}
	g.images = map[string]*ebiten.Image{}
	pixels := 0
	for id, a := range c.Assets {
		if a.Kind != "image" {
			continue
		}
		data, ok := assets[a.Path]
		if !ok {
			g.warnings = append(g.warnings, "missing image: "+a.Path)
			continue
		}
		if a.Size > 0 {
			if a.Offset < 0 || a.Offset+a.Size > int64(len(data)) {
				g.warnings = append(g.warnings, "missing image: "+a.Path)
				continue
			}
			data = data[a.Offset : a.Offset+a.Size]
		}
		rd := bytes.NewReader(data)
		cfg, _, err := image.DecodeConfig(rd)
		if err != nil {
			g.warnings = append(g.warnings, err.Error())
			continue
		}
		if cfg.Width <= 0 || cfg.Height <= 0 || cfg.Width > 8192 || cfg.Height > 8192 || cfg.Width*cfg.Height > 32<<20-pixels {
			return fmt.Errorf("image memory limit exceeded")
		}
		img, _, err := image.Decode(bytes.NewReader(data))
		if err != nil {
			g.warnings = append(g.warnings, err.Error())
			continue
		}
		pixels += cfg.Width * cfg.Height
		g.images[id] = ebiten.NewImageFromImage(img)
	}
	if err := g.initView(); err != nil {
		return err
	}
	g.clock, err = clock.New()
	if err != nil {
		return err
	}
	web.controls = map[ebiten.Key]bool{}
	web.scrollFine = map[ebiten.Key]bool{}
	web.pauseRequest = -1
	g.listen()
	defer unlisten()
	setFrameRate(g.display)
	ebiten.SetWindowSize(800, 600)
	ebiten.SetCursorMode(ebiten.CursorModeHidden)
	ebiten.SetRunnableOnUnfocused(true)
	post("ready", true)
	if err := ebiten.RunGame(g); err != nil {
		return err
	}
	if err := g.saveVolume(); err != nil {
		return err
	}
	// Keep scroll and lane-cover changes even when a run leaves no record.
	if !g.preview && g.mode != "replay" {
		post("profile", map[string]any{"lanes": c.Lanes, "profile": g.profile})
	}
	if g.hudEdit || g.sectionEnd != 0 {
		post("closed", true)
		return nil
	}
	if err := g.endRecording(); err != nil {
		return err
	}
	var id [16]byte
	if _, err := rand.Read(id[:]); err != nil {
		return err
	}
	r, err := g.resultRecord(hex.EncodeToString(id[:]))
	if err != nil {
		return err
	}
	if !r.Retained() {
		post("closed", true)
		return nil
	}
	post("result", r)
	return nil
}

func (g *scene) eventInput() bool { return true }
func sceneFont() []byte           { return nil }
func (g *scene) pressed(key ebiten.Key) bool {
	down := web.controls[key]
	delete(web.controls, key)
	return down
}
func (g *scene) held(key ebiten.Key) bool {
	switch key {
	case ebiten.KeyBackquote:
		return web.retryHeld
	case ebiten.KeyR:
		return web.retryChord
	case ebiten.KeyControl:
		return web.ctrlHeld
	case ebiten.KeyShift:
		return web.shiftHeld
	case ebiten.KeyTab:
		return web.peekHeld
	}
	return false
}

func (g *scene) hudShortcut() bool {
	down := web.hudToggle
	web.hudToggle = false
	return down
}

func (g *scene) coverShortcut() bool {
	down := web.coverToggle
	web.coverToggle = false
	return down
}

func (g *scene) scrollStep(key ebiten.Key) float64 {
	if web.scrollFine[key] {
		return .1
	}
	return 1
}
func (g *scene) drawn(*ebiten.Image) {
	g.frames++
	if g.preview && (g.frames == 1 || g.clock.Now()-g.previewAt >= 100*time.Millisecond) {
		g.previewAt = g.clock.Now()
		d := g.view.hudDisplay(g.display)
		geometry := map[string]any{}
		for item := songHUD; item <= hitHUD; item++ {
			b, base := g.view.hudBounds(item), g.view.defaultBounds(item)
			geometry[strings.ToLower(hudNames[item])] = map[string]any{"x": b.Min.X, "y": b.Min.Y, "width": b.Dx(), "height": b.Dy(), "baseX": base.Min.X, "baseY": base.Min.Y, "baseWidth": base.Dx(), "baseHeight": base.Dy(), "movable": g.view.movable(item)}
		}
		post("hud-preview", map[string]any{"display": d, "geometry": geometry, "revision": g.previewRevision, "selection": strings.ToLower(hudNames[g.previewItem])})
	}
	if web.lastPause != g.paused {
		web.lastPause = g.paused
		post("paused", g.paused)
	}
	n := g.countdown(g.clock.Now())
	if web.lastCountdown != n {
		web.lastCountdown = n
		post("countdown", n)
	}
}

// Visibility events carry their observation time. Freeze at that time even
// when the next frame arrives much later due to browser background throttling.
func (g *scene) beforeUpdate(now time.Duration) error {
	if web.display != nil {
		d := *web.display
		web.display = nil
		g.view.setHUD(d)
		g.display = d
		g.previewRevision = web.displayRevision
	}
	if g.editing {
		web.restartRequest, web.pauseRequest = false, -1
	}
	if web.restartRequest {
		web.restartRequest = false
		web.suspended, web.pauseRequest = false, -1
		return g.restart(now)
	}
	if web.suspended {
		web.suspended = false
		// Like the native editor, the HUD editor keeps playing without focus.
		if g.hudEdit {
			return nil
		}
		if err := g.pauseFor(web.at, "visibility"); err != nil {
			return err
		}
	}
	if web.pauseRequest >= 0 {
		requested := web.pauseRequest == 1
		web.pauseRequest = -1
		if requested {
			return g.pauseFor(now, "browser-control")
		}
		g.resumeFor(now, "browser-control")
	}
	return nil
}

func (g *scene) listen() {
	window := js.Global()
	document := window.Get("document")
	listen := func(target js.Value, event string, fn func(js.Value)) {
		f := js.FuncOf(func(_ js.Value, args []js.Value) any { fn(args[0]); return nil })
		target.Call("addEventListener", event, f)
		web.listeners = append(web.listeners, f)
	}
	key := func(e js.Value, down bool) {
		if g.preview {
			return
		}
		code := e.Get("code").String()
		web.ctrlHeld = e.Get("ctrlKey").Bool() || e.Get("metaKey").Bool()
		web.shiftHeld = e.Get("shiftKey").Bool()
		if code == "KeyR" && (web.ctrlHeld || web.retryChord) {
			e.Call("preventDefault")
			web.retryChord = down
			return
		}
		arrows := map[string]ebiten.Key{"ArrowUp": ebiten.KeyArrowUp, "ArrowDown": ebiten.KeyArrowDown, "ArrowLeft": ebiten.KeyArrowLeft, "ArrowRight": ebiten.KeyArrowRight}
		if k, ok := arrows[code]; ok && e.Get("altKey").Bool() {
			e.Call("preventDefault")
			e.Call("stopImmediatePropagation")
			if down {
				web.volumeKeys = append(web.volumeKeys, k)
			}
			return
		}
		if code == "Backquote" {
			e.Call("preventDefault")
			web.retryHeld = down
			return
		}
		if code == "Tab" {
			e.Call("preventDefault")
			web.peekHeld = down
			if down && !e.Get("repeat").Bool() {
				web.hudToggle = web.shiftHeld
			}
			return
		}
		if code == "KeyH" && g.mode != "live" {
			e.Call("preventDefault")
			if down && !e.Get("repeat").Bool() {
				web.hudToggle = true
			}
			return
		}
		if web.ctrlHeld && (code == "Equal" || code == "NumpadAdd" || code == "Minus" || code == "NumpadSubtract") {
			e.Call("preventDefault")
			if down && !e.Get("repeat").Bool() {
				k := ebiten.KeyPageUp
				if code == "Minus" || code == "NumpadSubtract" {
					k = ebiten.KeyPageDown
				}
				web.controls[k], web.scrollFine[k] = true, false
			}
			return
		}
		if down && (web.ctrlHeld && code != "F3" && code != "F4" || e.Get("altKey").Bool() && code != "PageUp" && code != "PageDown") {
			return
		}
		if down && e.Get("repeat").Bool() {
			return
		}
		if code == "F3" || code == "F4" {
			e.Call("preventDefault")
			if down {
				if code == "F4" && web.shiftHeld {
					web.coverToggle = true
				} else if !web.shiftHeld {
					k := ebiten.KeyPageUp
					if code == "F4" {
						k = ebiten.KeyPageDown
					}
					web.controls[k], web.scrollFine[k] = true, false
				}
			}
			return
		}
		controls := map[string]ebiten.Key{"KeyP": ebiten.KeyP, "F3": ebiten.KeyF3, "F4": ebiten.KeyF4, "F6": ebiten.KeyF6, "F7": ebiten.KeyF7, "F8": ebiten.KeyF8, "F9": ebiten.KeyF9, "PageUp": ebiten.KeyPageUp, "PageDown": ebiten.KeyPageDown, "Escape": ebiten.KeyEscape, "ArrowUp": ebiten.KeyArrowUp, "ArrowDown": ebiten.KeyArrowDown, "Enter": ebiten.KeyEnter, "NumpadEnter": ebiten.KeyNumpadEnter}
		if k, ok := controls[code]; ok {
			e.Call("preventDefault")
			if down {
				web.controls[k] = true
				if k == ebiten.KeyPageUp || k == ebiten.KeyPageDown {
					web.scrollFine[k] = e.Get("altKey").Bool()
				}
			}
			return
		}
		for _, b := range g.bindings {
			want := "Key" + b.name
			if b.code == 32 {
				want = "Space"
			}
			if b.code == 186 {
				want = "Semicolon"
			}
			if b.code >= '0' && b.code <= '9' {
				want = "Digit" + b.name
			}
			if code == want {
				e.Call("preventDefault")
				if g.player == nil {
					g.box.Push(keyboard.Event{At: g.clock.Now(), Key: b.code, Down: down})
				}
				return
			}
		}
	}
	listen(window, "keydown", func(e js.Value) { key(e, true) })
	listen(window, "keyup", func(e js.Value) { key(e, false) })
	suspend := func(js.Value) {
		web.peekHeld = false // Tab's keyup is lost once focus leaves.
		web.retryHeld, web.retryChord, web.ctrlHeld, web.shiftHeld = false, false, false, false
		clear(web.controls)
		web.hudToggle, web.coverToggle = false, false
		if (!g.paused || g.resumeAt != 0) && !web.suspended {
			web.at = g.clock.Now()
			web.suspended = true
		}
	}
	listen(window, "blur", suspend)
	listen(document, "visibilitychange", func(e js.Value) {
		if document.Get("hidden").Bool() {
			suspend(e)
		}
	})
	listen(window, "message", func(e js.Value) {
		if !e.Get("source").Equal(window.Get("parent")) {
			return
		}
		if e.Get("origin").String() != window.Get("location").Get("origin").String() {
			return
		}
		v := e.Get("data")
		if v.Type() != js.TypeObject {
			return
		}
		switch v.Get("type").String() {
		case "gosu:display-settings":
			var d settings.Display
			if err := json.Unmarshal([]byte(v.Get("data").String()), &d); err != nil || d.Validate() != nil {
				return
			}
			web.display = &d
			if !v.Get("revision").IsUndefined() {
				web.displayRevision = v.Get("revision").Int()
			}
			if g.preview && !v.Get("selection").IsUndefined() {
				name := strings.ToUpper(v.Get("selection").String())
				for item := songHUD; item <= hitHUD; item++ {
					if hudNames[item] == name {
						g.previewItem = item
					}
				}
			}
		case "gosu:pause":
			web.pauseRequest = 1
		case "gosu:resume":
			web.pauseRequest = 0
		case "gosu:stop":
			g.stopRequested = true
		case "gosu:restart":
			web.restartRequest = true
		}
	})
}

// Ebitengine can retain the document after termination; release Go callbacks.
func unlisten() {
	window := js.Global()
	doc := window.Get("document")
	names := []string{"keydown", "keyup", "blur", "visibilitychange", "message"}
	for i, f := range web.listeners {
		target := window
		if names[i] == "visibilitychange" {
			target = doc
		}
		target.Call("removeEventListener", names[i], f)
		f.Release()
	}
	web.listeners = nil
	web.controls = nil
}
