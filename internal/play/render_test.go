//go:build windows

package play

import (
	"bytes"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/hajimehoshi/ebiten/v2"

	"ggosu/internal/chart"
	"ggosu/internal/multi"
	"ggosu/internal/piano"
	"ggosu/internal/settings"
)

// This fixture judges real inputs before rendering, including misses and late hits.
// Each capture runs in its own process because Ebitengine owns the UI thread.
func TestHUDRender(t *testing.T) {
	dir := os.Getenv("GGOSU_HUD_PREVIEW")
	if dir == "" {
		t.Skip("native HUD rendering requires GGOSU_HUD_PREVIEW")
	}
	lanes := 4
	if value := os.Getenv("GGOSU_HUD_LANES"); value != "" {
		var err error
		lanes, err = strconv.Atoi(value)
		if err != nil || lanes < 1 || lanes > 18 {
			t.Fatalf("invalid HUD lane count %q", value)
		}
	}
	c := chart.New(lanes)
	c.Metadata = chart.Metadata{Title: "gosu HUD", Artist: "ggosu", Difficulty: "Preview"}
	if os.Getenv("GGOSU_HUD_CAPTURE") == "long" {
		c.Metadata = chart.Metadata{Title: "밤하늘을 달리는 별들의 이야기 Extended Mix", Artist: "Composer Name & Another Artist", Difficulty: "Hyper / Original Chart by ggosu"}
	}
	c.Timing = []chart.Timing{{BPM: 120, Scroll: 1, Meter: 4}}
	for i := 0; i < 128; i++ {
		at := time.Duration(i+1) * 125 * time.Millisecond
		c.Notes = append(c.Notes, chart.Note{At: at, End: at, Lane: i % lanes, Kind: "tap"})
	}
	s, err := piano.NewSession(c)
	if err != nil {
		t.Fatal(err)
	}
	for i, n := range c.Notes {
		if n.At >= 8*time.Second {
			break
		}
		if i%17 == 0 {
			continue
		}
		delay := []time.Duration{0, 0, 0, 30 * time.Millisecond, 70 * time.Millisecond}[i%5]
		if err := s.Input(n.At+delay, n.Lane, true); err != nil {
			t.Fatal(err)
		}
		if err := s.Input(n.At+delay+time.Millisecond, n.Lane, false); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.Advance(8 * time.Second); err != nil {
		t.Fatal(err)
	}
	display := (settings.Config{}).DisplaySettings()
	mode := os.Getenv("GGOSU_HUD_CAPTURE")
	if mode == "focus" {
		display.HUD = "focus"
	}
	if mode == "below" || mode == "left" {
		display.ComboPosition, display.JudgementPosition = mode, mode
	}
	if mode == "pie" {
		display.TimeDisplay = "pie"
	}
	p, err := newView(c, s, nil, viewConfig{Keys: make([]string, lanes), Mode: "live", Display: &display, Font: sceneFont()})
	if err != nil {
		t.Fatal(err)
	}
	state := viewState{Position: 7500 * time.Millisecond, Rate: 1, Scroll: 110}
	p.Update(state)
	state.Position, state.Paused = 8*time.Second, mode == "paused"
	if mode == "chat" {
		state.Chat = &chat{Open: true, Room: "채팅 · 알림 검증", Messages: []chatLine{{Time: "21:30", Name: "Rival", Text: "Ready!", Colour: chatNameColour("rival")}, {Name: "Player", Text: "Good luck.", Colour: chatNameColour("player")}}}
		state.Rivals = []rival{{Name: "Rival", Difference: 12345, Available: true}}
	}
	if mode == "chat-alerts" {
		state.Chat = &chat{Unread: 2, Popup: &chatAlert{Message: multi.Message{Name: "친구", Text: "@Player 이름 언급 알림입니다."}, Count: 2}, Ticker: &chatAlert{Message: multi.Message{Name: "mint", Text: "다음 곡 같이 해요."}}}
	}
	p.Update(state)
	before := s.Score
	if mode == "split" {
		p.applyLayout(settings.Layout{Score: settings.Point{X: -230, Y: 110, Scale: 125}, Counts: settings.Point{X: -180, Y: 220, Scale: 150}})
	}
	var editor *scene
	if mode == "edit" {
		p.editing = true
		p.applyLayout(settings.Layout{HitPosition: 500, Score: settings.Point{X: 880, Y: -300}, Combo: settings.Point{X: -160, Y: 50}, Judgement: settings.Point{X: 170, Y: 40}})
		editor = &scene{view: p, editing: true, editor: hudEditor{item: hitHUD, status: "Drag HUD / F9 save / Esc cancel"}}
	}
	hash, err := c.Digest()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	name := "hud-" + strconv.Itoa(lanes) + "k-" + mode + ".png"
	g := &renderGame{play: p, path: filepath.Join(dir, name), editor: editor}
	ebiten.SetWindowSize(Width, Height)
	ebiten.SetRunnableOnUnfocused(true)
	if err := ebiten.RunGame(g); err != nil {
		t.Fatal(err)
	}
	if g.err != nil {
		t.Fatal(g.err)
	}
	if !p.focus {
		data, err := os.ReadFile(g.path)
		if err != nil {
			t.Fatal(err)
		}
		img, err := png.Decode(bytes.NewReader(data))
		if err != nil {
			t.Fatal(err)
		}
		// A capture must contain both the score digits and the static judgement tiles.
		threshold := uint32(300 * 257)
		if p.phase == Paused || state.Chat != nil && state.Chat.Open {
			threshold = 90 * 257 // Pause and chat overlays dim the HUD beneath them.
		}
		for _, sample := range []struct {
			item hudItem
			area image.Rectangle
		}{{scoreHUD, image.Rect(12, 12, 180, 44)}, {countsHUD, image.Rect(12, 2, 112, 78)}} {
			origin := p.hudBounds(sample.item).Min
			scale := p.hudScale(sample.item)
			area := image.Rect(int(float64(sample.area.Min.X)*scale), int(float64(sample.area.Min.Y)*scale), int(float64(sample.area.Max.X)*scale), int(float64(sample.area.Max.Y)*scale)).Add(origin)
			bright := 0
			for y := area.Min.Y; y < area.Max.Y; y++ {
				for x := area.Min.X; x < area.Max.X; x++ {
					r, green, blue, _ := img.At(x, y).RGBA()
					if r+green+blue >= threshold {
						bright++
					}
				}
			}
			if bright < 40 {
				t.Fatalf("HUD capture missing score/count text in %v", area)
			}
		}
	}
	after, err := c.Digest()
	if err != nil || after != hash || s.Score != before {
		t.Fatal("HUD rendering changed the chart or score")
	}
}

// Opt-in native rendering exercises the actual pause overlay and checks that
// Draw, not just the CPU presentation update, cannot change score or chart.
func TestPausedRender(t *testing.T) {
	dir := os.Getenv("GGOSU_SCENE_CAPTURE")
	if dir == "" {
		t.Skip("native rendering requires GGOSU_SCENE_CAPTURE")
	}
	c := chart.New(4)
	c.Metadata.Title = "Paused Scene"
	c.Timing = []chart.Timing{{BPM: 120, Scroll: 1, Meter: 4}}
	c.Notes = []chart.Note{{At: time.Second, End: 3 * time.Second, Kind: "hold"}, {At: 2 * time.Second, End: 2 * time.Second, Lane: 1, Kind: "tap"}}
	s, err := piano.NewSession(c)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Input(time.Second, 0, true); err != nil {
		t.Fatal(err)
	}
	before := s.Score
	hash, err := c.Digest()
	if err != nil {
		t.Fatal(err)
	}
	display := (settings.Config{}).DisplaySettings()
	mode := os.Getenv("GGOSU_HUD_CAPTURE")
	if mode == "focus" || mode == "countdown" {
		display.HUD = "focus"
	}
	if mode == "below" || mode == "left" {
		display.ComboPosition, display.JudgementPosition = mode, mode
	}
	if mode == "pie" {
		display.TimeDisplay = "pie"
	}
	p, err := newView(c, s, nil, viewConfig{Keys: []string{"D", "F", "J", "K"}, Mode: "live", Display: &display})
	if err != nil {
		t.Fatal(err)
	}
	state := viewState{Position: time.Second, Rate: 1, Scroll: 110, Paused: true}
	name := "scene-paused.png"
	if mode == "focus" {
		name = "scene-hud-focus.png"
	} else if mode == "countdown" {
		state.Countdown = 3
		name = "scene-countdown.png"
	} else if mode == "peek" {
		state.Peek = true
		name = "scene-paused-peek.png"
	}
	if mode == "center" || mode == "below" || mode == "left" {
		state.Paused = false
		name = "scene-feedback-" + mode + ".png"
	}
	if mode == "playing" || mode == "pie" || mode == "ready" || mode == "chat" {
		state.Paused = false
		name = "scene-" + mode + ".png"
		if mode == "ready" {
			state.Position = -time.Second
		}
		if mode == "chat" {
			state.Chat = &chat{Open: true, Messages: []chatLine{{Time: "21:30", Name: "Rival", Text: "Ready!", Colour: chatNameColour("rival")}, {Name: "Player", Text: "Good luck.", Colour: chatNameColour("player")}}}
			state.Rivals = []rival{{Name: "Rival", Difference: 12345, Available: true}}
		}
	}
	p.Update(state)
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	g := &renderGame{play: p, path: filepath.Join(dir, name)}
	ebiten.SetWindowSize(1280, 720)
	ebiten.SetWindowTitle("gosu - Scene render test")
	ebiten.SetRunnableOnUnfocused(true)
	if err := ebiten.RunGame(g); err != nil {
		t.Fatal(err)
	}
	if g.err != nil {
		t.Fatal(g.err)
	}
	data, err := os.ReadFile(g.path)
	if err != nil {
		t.Fatal(err)
	}
	img, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	// Keep the field visible beneath either overlay (the judgement line); a text-only capture fails.
	lane := p.field.layout.lanes[0]
	r, green, blue, _ := img.At(int(lane.x+lane.width/2), int(p.field.layout.line)+2).RGBA()
	if r < 12*257 || green < 12*257 || blue < 12*257 {
		t.Fatal("pause/countdown capture lost the note field")
	}
	after, err := c.Digest()
	if err != nil || after != hash || s.Score != before || len(s.Results) != 1 {
		t.Fatal("rendering changed source or judgement")
	}
}

type renderGame struct {
	editor *scene
	play   *view
	path   string
	drawn  bool
	frames int
	err    error
}

func (g *renderGame) Layout(int, int) (int, int) { return Width, Height }
func (g *renderGame) Update() error {
	if g.err != nil {
		return g.err
	}
	if g.drawn {
		return ebiten.Termination
	}
	return nil
}
func (g *renderGame) Draw(dst *ebiten.Image) {
	g.play.Draw(dst)
	if g.editor != nil {
		g.editor.drawEditor(dst)
	}
	if g.drawn {
		return
	}
	g.frames++
	// Let initial glyph/texture uploads settle before reading back the GPU frame.
	if g.frames < 30 {
		return
	}
	f, err := os.Create(g.path)
	if err != nil {
		g.err = err
	} else {
		g.err = png.Encode(f, dst)
		if err := f.Close(); g.err == nil {
			g.err = err
		}
	}
	g.drawn = true
}

// Silent, synthetic capture with a held note, failed long note and fresh hit.
func TestFeedbackRender(t *testing.T) {
	dir := os.Getenv("GGOSU_FEEDBACK_CAPTURE")
	if dir == "" {
		t.Skip("set GGOSU_FEEDBACK_CAPTURE for native visual verification")
	}
	c := chart.New(7)
	c.Metadata.Title = "Feedback verification"
	c.Timing = []chart.Timing{{BPM: 120, Scroll: 1, Meter: 4}}
	c.Notes = []chart.Note{
		{At: time.Second, End: 4 * time.Second, Lane: 0, Kind: "hold"},
		{At: 1100 * time.Millisecond, End: 4 * time.Second, Lane: 2, Kind: "hold"},
		{At: 1800 * time.Millisecond, End: 1800 * time.Millisecond, Lane: 4, Kind: "tap"},
		{At: 2500 * time.Millisecond, End: 2500 * time.Millisecond, Lane: 1, Kind: "tap"},
		{At: 2800 * time.Millisecond, End: 2800 * time.Millisecond, Lane: 3, Kind: "tap"},
		{At: 3000 * time.Millisecond, End: 3000 * time.Millisecond, Lane: 5, Kind: "tap"},
	}
	session, err := piano.NewSession(c)
	if level, e := strconv.Atoi(os.Getenv("GGOSU_FEEDBACK_HEALTH")); e == nil && err == nil {
		session.Health = piano.NewHealth(level, 0)
	}
	if err != nil {
		t.Fatal(err)
	}
	for _, in := range []struct {
		at   time.Duration
		lane int
		down bool
	}{
		{980 * time.Millisecond, 0, true}, {1700 * time.Millisecond, 1, true},
		{1820 * time.Millisecond, 4, true}, {1830 * time.Millisecond, 4, false},
	} {
		if err := session.Input(in.at, in.lane, in.down); err != nil {
			t.Fatal(err)
		}
	}
	if err := session.Advance(1900 * time.Millisecond); err != nil {
		t.Fatal(err)
	}
	d := (settings.Config{}).DisplaySettings()
	d.Skin = os.Getenv("GGOSU_FEEDBACK_SKIN")
	p, err := newView(c, session, nil, viewConfig{Keys: make([]string, 7), Font: sceneFont(), Display: &d})
	if err != nil {
		t.Fatal(err)
	}
	p.Press(0, 980*time.Millisecond)
	p.Press(1, 1700*time.Millisecond)
	p.Press(4, 1820*time.Millisecond)
	p.Update(viewState{Position: 1900 * time.Millisecond, Rate: 1, Scroll: 110})
	// Capture the animated rail at a known point without audio or a demo run.
	effect := os.Getenv("GGOSU_FEEDBACK_EFFECT")
	if session.Health.Level > 0 && effect != "" {
		value, next := 7800, 5200
		if effect == "gain" {
			value, next = 5200, 7800
		} else if effect == "danger" {
			value, next = 2200, 2200
		}
		p.health = newHealthMotion(value)
		p.health.level = session.Health.Level
		p.health.update(value, 1800*time.Millisecond)
		session.Health.Value = next
		p.Update(viewState{Position: 1850 * time.Millisecond, Rate: 1, Scroll: 110})
		p.Update(viewState{Position: 1900 * time.Millisecond, Rate: 1, Scroll: 110})
	}
	if !session.Visible(1) || !session.Failed(1) || session.Active(1) {
		t.Fatal("failed hold state")
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	name := "feedback-" + p.skin
	if effect != "" {
		name += "-" + effect
	}
	g := &renderGame{play: p, path: filepath.Join(dir, name+".png")}
	setFrameRate(d)
	ebiten.SetWindowSize(Width, Height)
	ebiten.SetRunnableOnUnfocused(true)
	if err := ebiten.RunGame(g); err != nil {
		t.Fatal(err)
	}
	if g.err != nil {
		t.Fatal(g.err)
	}
}
