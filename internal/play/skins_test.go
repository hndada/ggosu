package play

import (
	"bytes"
	"fmt"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
	"golang.org/x/image/draw"

	"ggosu/internal/chart"
	"ggosu/internal/piano"
	"ggosu/internal/settings"
)

func TestLongTailFadesInRealTime(t *testing.T) {
	if tailOpacity(0, 1) != 1 || tailOpacity(90*time.Millisecond, 1) != 0.25 || tailOpacity(180*time.Millisecond, 1) != 0 {
		t.Fatal("tail fade duration changed")
	}
	if tailOpacity(180*time.Millisecond, 2) != 0.25 || tailOpacity(-time.Millisecond, 1) != 0 {
		t.Fatal("tail fade depends on playback rate or starts early")
	}
}

func TestColumnLightStaysOnUntilRelease(t *testing.T) {
	p := &view{session: &piano.Session{Held: []bool{true}}, flashes: []time.Duration{time.Second}, released: []time.Duration{0}}
	for _, age := range []time.Duration{50 * time.Millisecond, 300 * time.Millisecond, time.Second, time.Minute, 2 * time.Hour} {
		p.state.Position = time.Second + age
		if got := p.columnLight(0); got != 1 {
			t.Fatalf("held for %s: brightness = %g, want 1", age, got)
		}
	}
	p.session.Held[0] = false
	p.released[0] = p.state.Position
	for _, age := range []time.Duration{0, 125 * time.Millisecond, 250 * time.Millisecond} {
		p.state.Position = p.released[0] + age
		got := p.columnLight(0)
		if age == 0 && got != 1 || age > 0 && age < 250*time.Millisecond && (got <= 0 || got >= 1) || age == 250*time.Millisecond && got != 0 {
			t.Fatalf("released for %s: unexpected brightness %g", age, got)
		}
	}
}

func TestColumnLightShortReleaseDoesNotBrighten(t *testing.T) {
	p := &view{session: &piano.Session{Held: []bool{true}}, flashes: []time.Duration{-time.Second}, released: []time.Duration{0}}
	p.state.Position = -time.Second + 10*time.Millisecond
	before := p.columnLight(0)
	p.session.Held[0] = false
	p.released[0] = p.state.Position
	if got := p.columnLight(0); got != before {
		t.Fatalf("release changed brightness from %g to %g", before, got)
	}
	p.state.Position += 20 * time.Millisecond
	if got := p.columnLight(0); got <= 0 || got >= before {
		t.Fatalf("short release brightened from %g to %g", before, got)
	}
}

// The options window shows each skin as a picture of its play field. Regenerate the pictures with
//
//	$env:GGOSU_SKIN_THUMBS = 'scenes/settings/skins'; go test ./internal/play -run TestSkinThumbnails
//
// (the path is relative to internal/play, so use an absolute path).
func TestSkinThumbnails(t *testing.T) {
	dir := os.Getenv("GGOSU_SKIN_THUMBS")
	if dir == "" {
		t.Skip("native rendering requires GGOSU_SKIN_THUMBS")
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	g := &thumbGame{dir: dir}
	for _, skin := range skinNames {
		g.views = append(g.views, thumbView(t, skin))
	}
	ebiten.SetWindowSize(Width, Height)
	ebiten.SetRunnableOnUnfocused(true)
	if err := ebiten.RunGame(g); err != nil {
		t.Fatal(err)
	}
	if g.err != nil {
		t.Fatal(g.err)
	}
}

// thumbView is a 4K field mid-play: taps on every lane, a held long note and a fresh hit.
func thumbView(t *testing.T, skin string) *view {
	t.Helper()
	c := chart.New(4)
	c.Timing = []chart.Timing{{BPM: 120, Scroll: 1, Meter: 4}}
	c.Notes = []chart.Note{
		{At: 1000 * time.Millisecond, End: 1000 * time.Millisecond, Lane: 1, Kind: "tap"},
		{At: 1500 * time.Millisecond, End: 3500 * time.Millisecond, Lane: 0, Kind: "hold"},
		{At: 2250 * time.Millisecond, End: 2250 * time.Millisecond, Lane: 2, Kind: "tap"},
		{At: 2500 * time.Millisecond, End: 2500 * time.Millisecond, Lane: 3, Kind: "tap"},
		{At: 2750 * time.Millisecond, End: 2750 * time.Millisecond, Lane: 1, Kind: "tap"},
		{At: 3000 * time.Millisecond, End: 3000 * time.Millisecond, Lane: 2, Kind: "tap"},
		{At: 3500 * time.Millisecond, End: 3500 * time.Millisecond, Lane: 3, Kind: "tap"},
	}
	s, err := piano.NewSession(c)
	if err != nil {
		t.Fatal(err)
	}
	for _, in := range []struct {
		at   time.Duration
		lane int
		down bool
	}{{1000 * time.Millisecond, 1, true}, {1010 * time.Millisecond, 1, false}, {1500 * time.Millisecond, 0, true}} {
		if err := s.Input(in.at, in.lane, in.down); err != nil {
			t.Fatal(err)
		}
	}
	display := (settings.Config{}).DisplaySettings()
	display.Skin, display.HUD = skin, "focus"
	p, err := newView(c, s, nil, viewConfig{Keys: make([]string, 4), Mode: "live", Display: &display, Font: sceneFont()})
	if err != nil {
		t.Fatal(err)
	}
	p.Press(0, 1500*time.Millisecond)
	p.hits[0] = 1950 * time.Millisecond
	p.Update(viewState{Position: 2000 * time.Millisecond, Rate: 1, Scroll: 160})
	return p
}

type thumbGame struct {
	dir    string
	views  []*view
	frame  *ebiten.Image
	frames int
	err    error
}

func (g *thumbGame) Layout(int, int) (int, int) { return Width, Height }
func (g *thumbGame) Update() error {
	if g.err != nil {
		return g.err
	}
	if g.frames > 30 {
		return ebiten.Termination
	}
	return nil
}

func (g *thumbGame) Draw(dst *ebiten.Image) {
	if g.frame == nil {
		g.frame = ebiten.NewImage(Width, Height)
	}
	g.frames++
	// Let glyph and texture uploads settle before reading back the GPU frames.
	if g.frames != 30 {
		return
	}
	for i, p := range g.views {
		if err := steadyHold(p); err != nil {
			g.err = err
			return
		}
		g.frame.Fill(black)
		g.frame.DrawImage(p.base, nil)
		p.drawField(g.frame)
		full := image.NewRGBA(image.Rect(0, 0, Width, Height))
		g.frame.ReadPixels(full.Pix)
		// The same 340 px wide window on every skin, from the field top to the bottom.
		crop := image.Rect(Width/2-170, 0, Width/2+170, Height)
		thumb := image.NewRGBA(image.Rect(0, 0, 170, 308))
		draw.CatmullRom.Scale(thumb, thumb.Bounds(), full, crop, draw.Src, nil)
		if err := writePNG(filepath.Join(g.dir, skinNames[i]+".png"), thumb); err != nil {
			g.err = err
		}
	}
	dst.DrawImage(g.frame, nil)
}

// Keep geometry fixed while sampling different song positions. A held body's
// pixels must stay identical; moving notes and the one-shot hit effects are
// deliberately outside this comparison.
func steadyHold(p *view) error {
	at := p.state.Position
	defer func() { p.state.Position = at }()
	lane := p.field.layout.lanes[0]
	var first []byte
	for _, ms := range []int{0, 25, 50, 80, 100, 120, 180, 240, 315} {
		p.state.Position = time.Duration(ms) * time.Millisecond
		p.notes.Clear()
		p.drawBody(0, 30, lane.width, 250, lane.color, 1, true)
		p.drawHoldGlow(0, 30, lane.width, 250, lane.color)
		pixels := make([]byte, p.notes.Bounds().Dx()*p.notes.Bounds().Dy()*4)
		p.notes.ReadPixels(pixels)
		if first == nil {
			first = pixels
		} else if !bytes.Equal(first, pixels) {
			return fmt.Errorf("%s hold lighting changes at %dms", p.skin, ms)
		}
	}
	return nil
}

func writePNG(path string, img image.Image) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	if err := png.Encode(f, img); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}
