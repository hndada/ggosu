package play

import (
	"fmt"
	"image"
	"image/color"
	"math"
	"os"
	"path/filepath"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

func TestVisibilityBoundsFollowComboAndHitArea(t *testing.T) {
	for _, h := range []float32{240, 600} {
		_, high0, _ := visibilityBounds("hidden", h, 0)
		_, high480, _ := visibilityBounds("hidden", h, 480)
		_, high1000, _ := visibilityBounds("hidden", h, 1000)
		low0, _, _ := visibilityBounds("fadein", h, 0)
		low480, _, _ := visibilityBounds("fadein", h, 480)
		if high0 <= high480 || high480 != high1000 || low0 >= low480 || math.Abs(float64(high480+low480-h)) > .001 {
			t.Fatal("combo coverage did not grow and cap", h)
		}
		low, high, fade := visibilityBounds("cover", h, 1000)
		if low != h/2 || high < h || fade != 0 {
			t.Fatal("cover must hide the top half")
		}
		low, high, fade = visibilityBounds("flashlight", h, 0)
		if math.Abs(float64((low+high)/2-h/2)) > .001 || high-low >= h/4 || fade != 0 {
			t.Fatal("flashlight must show a narrow central strip")
		}
	}
}

// A silent, synthetic fixture checks the real GPU mask at every row and saves
// each effect on every built-in skin, including a held long note.
func TestVisibilityRender(t *testing.T) {
	dir := os.Getenv("GGOSU_VISIBILITY_CAPTURE")
	if dir == "" {
		t.Skip("native GPU validation requires GGOSU_VISIBILITY_CAPTURE")
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	g := &visibilityGame{dir: dir}
	for _, skin := range skinNames {
		for _, kind := range []string{"", "hidden", "fadein", "cover", "flashlight"} {
			p := thumbView(t, skin)
			p.config.Visibility = kind
			if kind != "" {
				var err error
				p.visibilityShader, err = ebiten.NewShader([]byte(visibilityShader))
				if err != nil {
					t.Fatal(err)
				}
			}
			g.views = append(g.views, p)
		}
	}
	ebiten.SetWindowSize(Width, Height)
	ebiten.SetWindowTitle("Ggosu Visibility validation")
	ebiten.SetRunnableOnUnfocused(true)
	if err := ebiten.RunGame(g); err != nil {
		t.Fatal(err)
	}
	if g.err != nil {
		t.Fatal(g.err)
	}
}

type visibilityGame struct {
	dir    string
	views  []*view
	frame  *ebiten.Image
	frames int
	err    error
}

func (g *visibilityGame) Layout(int, int) (int, int) { return Width, Height }
func (g *visibilityGame) Update() error {
	if g.err != nil {
		return g.err
	}
	if g.frames > 4 {
		return ebiten.Termination
	}
	return nil
}

func (g *visibilityGame) Draw(dst *ebiten.Image) {
	g.frames++
	if g.frames != 4 {
		return
	}
	g.frame = ebiten.NewImage(Width, Height)
	for _, p := range g.views {
		for _, combo := range []int{0, 480} {
			p.session.Score.Combo = combo
			g.frame.Clear()
			p.notes.Fill(color.White)
			p.drawNotes(g.frame)
			pixels := make([]byte, Width*Height*4)
			g.frame.ReadPixels(pixels)
			l := p.field.layout
			h := l.line - l.top
			low, high, fade := visibilityBounds(p.config.Visibility, h, combo)
			for y := 0; y < int(h); y++ {
				want := float64(1)
				at := float32(y) + .5
				// Fractional skin geometry can rasterise a hard boundary into either
				// adjacent row. Check the interior and every gradient row instead.
				if fade == 0 && (math.Abs(float64(at-low)) < 1 || math.Abs(float64(at-high)) < 1) {
					continue
				}
				if at < low || at > high {
					want = 0
				} else if fade > 0 {
					want = float64(min(float32(1), (at-low)/fade, (high-at)/fade))
				}
				i := (y*Width+Width/2)*4 + 3
				if math.Abs(float64(pixels[i])/255-want) > .035 {
					g.err = fmt.Errorf("%s/%s combo %d row %d: alpha %d, want %.3f", p.skin, p.config.Visibility, combo, y, pixels[i], want)
					return
				}
			}
		}
		p.session.Score.Combo = 0
		g.frame.Clear()
		g.frame.DrawImage(p.base, nil)
		p.drawField(g.frame)
		img := image.NewRGBA(image.Rect(0, 0, Width, Height))
		g.frame.ReadPixels(img.Pix)
		kind := p.config.Visibility
		if kind == "" {
			kind = "off"
		}
		if err := writePNG(filepath.Join(g.dir, p.skin+"-"+kind+".png"), img); err != nil {
			g.err = err
			return
		}
	}
	dst.DrawImage(g.frame, nil)
}
