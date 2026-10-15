package play

import (
	"time"
	"image"
	"image/color"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
	"github.com/hajimehoshi/ebiten/v2/vector"

	"ggosu/internal/settings"
)

// The game draws a fixed Width×Height frame. Draw fits it into the letterbox area
// (osu!lazer screen scaling) at the window's device pixels and draws the in-window
// cursor on top, so the cursor can also move over the bars around the area.

// Layout renders at device pixels so the frame is scaled once, by Draw.
func (g *scene) Layout(w, h int) (int, int) {
	g.deviceScale = 1
	if m := ebiten.Monitor(); m != nil {
		g.deviceScale = m.DeviceScaleFactor()
	}
	return max(1, int(float64(w)*g.deviceScale)), max(1, int(float64(h)*g.deviceScale))
}

func (g *scene) Draw(dst *ebiten.Image) {
	defer func(t time.Time) { zzDraw(t, time.Since(t)) }(time.Now())
	if g.frame == nil {
		g.frame = ebiten.NewImage(Width, Height)
	}
	g.frame.Clear()
	g.view.Draw(g.frame)
	if g.editing {
		g.drawEditor(g.frame)
	}
	g.drawScroll(g.frame)
	g.drawVolume(g.frame)
	g.drawToasts(g.frame)
	g.drawn(g.frame)
	g.area = fit(dst.Bounds().Dx(), dst.Bounds().Dy(), g.display)
	op := &ebiten.DrawImageOptions{Filter: ebiten.FilterLinear}
	op.GeoM.Scale(float64(g.area.Dx())/Width, float64(g.area.Dy())/Height)
	op.GeoM.Translate(float64(g.area.Min.X), float64(g.area.Min.Y))
	dst.DrawImage(g.frame, op)
	if a := g.retryShade(); a > 0 {
		vector.FillRect(dst, 0, 0, float32(dst.Bounds().Dx()), float32(dst.Bounds().Dy()), color.NRGBA{A: uint8(a * 255)}, false)
	}
	if a := g.endShade(); a > 0 {
		vector.FillRect(dst, 0, 0, float32(dst.Bounds().Dx()), float32(dst.Bounds().Dy()), color.NRGBA{A: uint8(a * 255)}, false)
	}
	if g.screenshot {
		g.screenshot = false
		g.saveScreenshot(dst)
	}
	if g.cursorShown {
		drawCursor(dst, g.mouse, float32(g.deviceScale)*float32(g.display.Cursor())/100)
	}
}

// fit returns where the 16:9 frame lands in a sw×sh screen: inside the letterbox
// box, which is centred and then moved by the position percent of the free space.
func fit(sw, sh int, d settings.Display) image.Rectangle {
	pw, ph, px, py := d.Area()
	bw, bh := sw*pw/100, sh*ph/100
	bx := (sw-bw)/2 + (sw-bw)*px/100
	by := (sh-bh)/2 + (sh-bh)*py/100
	w, h := bw, bw*Height/Width
	if h > bh {
		w, h = bh*Width/Height, bh
	}
	x, y := bx+(bw-w)/2, by+(bh-h)/2
	return image.Rect(x, y, x+w, y+h)
}

// cursor returns the mouse position in frame coordinates.
func (g *scene) cursor() image.Point {
	x, y := ebiten.CursorPosition()
	a := g.area
	if a.Empty() {
		return image.Pt(x, y)
	}
	return image.Pt((x-a.Min.X)*Width/a.Dx(), (y-a.Min.Y)*Height/a.Dy())
}

// trackCursor shows the cursor after mouse input and hides it after a key press,
// as osu!lazer's menu cursor does with HideCursorOnNonMouseInput during play.
func (g *scene) trackCursor() {
	p := image.Pt(ebiten.CursorPosition())
	moved := g.mouseSeen && p != g.mouse
	clicked := inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonLeft) || inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonRight)
	if moved || clicked {
		g.cursorShown = true
	} else if g.keysPressed = inpututil.AppendJustPressedKeys(g.keysPressed[:0]); len(g.keysPressed) > 0 {
		g.cursorShown = false
	}
	if g.editing {
		g.cursorShown = true
	}
	g.mouse, g.mouseSeen = p, true
}

var (
	cursorBody = color.NRGBA{255, 255, 255, 255}
	cursorEdge = color.NRGBA{13, 13, 14, 255}
)

// cursorShape is a plain arrow, 20 units tall with its tip at the origin. The menu
// draws the same points in scenes/cursor.js.
var cursorShape = []float32{0, 0, 0, 16, 4, 12.5, 6.8, 18.6, 9.3, 17.5, 6.6, 11.6, 11.6, 11.6}

func drawCursor(dst *ebiten.Image, at image.Point, scale float32) {
	var path vector.Path
	for i := 0; i < len(cursorShape); i += 2 {
		x, y := float32(at.X)+cursorShape[i]*scale, float32(at.Y)+cursorShape[i+1]*scale
		if i == 0 {
			path.MoveTo(x, y)
		} else {
			path.LineTo(x, y)
		}
	}
	path.Close()
	op := &vector.DrawPathOptions{AntiAlias: true}
	op.ColorScale.ScaleWithColor(cursorEdge)
	vector.StrokePath(dst, &path, &vector.StrokeOptions{Width: 2 * scale, LineJoin: vector.LineJoinRound}, op)
	op.ColorScale.Reset()
	op.ColorScale.ScaleWithColor(cursorBody)
	vector.FillPath(dst, &path, nil, op)
}
