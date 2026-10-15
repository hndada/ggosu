package play

import (
	"image/color"
	"math"
	"math/rand/v2"
	"path/filepath"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
	"github.com/hajimehoshi/ebiten/v2/vector"

	"ggosu/internal/ui"
)

// Screenshot toasts after osu!lazer's SimpleNotification in NotificationOverlayToastTray, in the
// bottom-right corner instead of the top-right, newest nearest the corner (scenes/notification.js
// is the menu's copy). Values from lazer:
//   - 320 px wide with 3 px above and below, 20 px from the frame edges; an icon column of 40 and a
//     check button of 28; text 14 px medium in a 10 px padding.
//     Three separate lines for the message, filename and action need an 80 px box.
//   - Enters by fading in over 200 ms while sliding in from its own width over 500 ms OutQuint; a
//     white layer at 0.8 fades out additively over 2000 ms OutQuart. Others move over 150 ms OutQuart.
//   - After 12 s (12 s more while hovered) it moves 400 px right and fades out over 600 ms OutQuint.
//   - Left click shows the file and fades out over 100 ms; right click or the check button flings
//     it left (DragContainer.FlingLeft) while it fades out over 600 ms Easing.In. A close plays pop-out.
const (
	toastWidth, toastHeight = 320, 80
	toastSlot               = toastHeight + 2*3
	toastEdge               = 20
	toastIcon, toastClose   = 40, 28
	toastShow               = 12 * time.Second
	toastEnter              = 500 * time.Millisecond
	toastFadeIn             = 200 * time.Millisecond
	toastFlash              = 2000 * time.Millisecond
	toastFlow               = 150 * time.Millisecond
	toastHover              = 200 * time.Millisecond
	toastForward            = 600 * time.Millisecond
	toastFade               = 100 * time.Millisecond
	toastFling              = 600 * time.Millisecond
)

// osu!lazer OverlayColourProvider at the accent's hue (192°), as theme.css --toast*.
var (
	toastBackground = color.NRGBA{0x39, 0x44, 0x46, 255} // Background3
	toastHovered    = color.NRGBA{0x45, 0x51, 0x54, 255} // Background2
	toastIconFill   = color.NRGBA{0x22, 0x29, 0x2a, 255} // Background5
	toastCheck      = color.NRGBA{0x8f, 0x9f, 0xa3, 255} // Foreground1
	toastWhite      = color.NRGBA{255, 255, 255, 255}
)

type toastExit int

const (
	toastStays toastExit = iota
	toastForwarded
	toastFaded
	toastFlung
)

type toast struct {
	path            string
	at, due         time.Duration // posted; when it next tries to leave
	exit            toastExit
	exitAt          time.Duration
	out             time.Duration // left the flow; 0 while in it
	hold            float32       // offset from the bottom when it left the flow
	vx              float64       // fling speed, px/ms
	hover, closeHov bool
	hoverAt         time.Duration
	hoverFrom       float64 // hover blend when the hover last changed
}

func (t *toast) exitLength() time.Duration {
	switch t.exit {
	case toastForwarded:
		return toastForward
	case toastFaded:
		return toastFade
	}
	return toastFling
}

func (t *toast) hoverBlend(now time.Duration) float64 {
	to := 0.0
	if t.hover {
		to = 1
	}
	return t.hoverFrom + (to-t.hoverFrom)*outQuint(float64(now-t.hoverAt)/float64(toastHover))
}

// postToasts shows a toast for each screenshot saved since the last frame.
func (g *scene) postToasts(now time.Duration) {
	for {
		select {
		case path := <-g.shots:
			g.toasts = append(g.toasts, &toast{path: path, at: now, due: now + toastShow})
		default:
			return
		}
	}
}

// offset is how far above the bottom slot t sits now: the newer toasts in the flow push it up,
// growing over toastFlow after they arrive and shrinking over toastFlow after they leave.
func (g *scene) offset(t *toast, now time.Duration) float32 {
	if t.out != 0 {
		return t.hold
	}
	var y float32
	for i := len(g.toasts) - 1; i >= 0 && g.toasts[i] != t; i-- {
		n := g.toasts[i]
		grow := outQuart(float64(now-n.at) / float64(toastFlow))
		if n.out != 0 {
			grow *= 1 - outQuart(float64(now-n.out)/float64(toastFlow))
		}
		y += toastSlot * float32(grow)
	}
	return y
}

func outQuart(t float64) float64 { return 1 - math.Pow(1-min(max(t, 0), 1), 4) }

// toastBox is where t's box is drawn in frame coordinates, before any exit motion.
func (g *scene) toastBox(t *toast, now time.Duration) (x, y float32) {
	slide := toastWidth * float32(1-outQuint(float64(now-t.at)/float64(toastEnter)))
	return Width - toastEdge - toastWidth + slide, Height - toastEdge - 3 - toastHeight - g.offset(t, now)
}

func (g *scene) updateToasts(now time.Duration) {
	if g.clock == nil {
		return
	}
	g.postToasts(now)
	p := g.cursor()
	left := inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonLeft)
	right := inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonRight)
	kept := g.toasts[:0]
	for _, t := range g.toasts {
		if t.exit == toastStays {
			x, y := g.toastBox(t, now)
			in := float32(p.X) >= x && float32(p.X) < x+toastWidth && float32(p.Y) >= y && float32(p.Y) < y+toastHeight
			if in != t.hover {
				t.hoverFrom, t.hoverAt, t.hover = t.hoverBlend(now), now, in
			}
			t.closeHov = in && float32(p.X) >= x+toastWidth-toastClose
			switch {
			case in && (right || left && t.closeHov):
				g.closeToast(t, toastFlung, now)
			case in && left:
				g.revealScreenshot(t.path)
				g.closeToast(t, toastFaded, now)
			case now >= t.due && t.hover:
				t.due = now + toastShow
			case now >= t.due:
				t.exit, t.exitAt = toastForwarded, now
				t.hold, t.out = g.offset(t, now), now
			}
		}
		// Faded and flung toasts keep their slot until they are gone, then leave the flow.
		if t.exit != toastStays && t.out == 0 && now >= t.exitAt+t.exitLength() {
			t.hold, t.out = g.offset(t, now), now
		}
		if t.out == 0 || now < t.out+toastFlow || now < t.exitAt+t.exitLength() {
			kept = append(kept, t)
		}
	}
	clear(g.toasts[len(kept):])
	g.toasts = kept
}

func (g *scene) closeToast(t *toast, exit toastExit, now time.Duration) {
	t.exit, t.exitAt = exit, now
	t.vx = -0.3 - 0.5*rand.Float64()
	if g.bank != nil {
		_ = g.bank.Effect("pop-out", 1)
	}
}

func (g *scene) drawToasts(dst *ebiten.Image) {
	if len(g.toasts) == 0 || g.view == nil || g.clock == nil {
		return
	}
	now := g.clock.Now()
	if g.toastImage == nil {
		g.toastImage = ebiten.NewImage(toastWidth, toastHeight)
	}
	for _, t := range g.toasts {
		if t.exit != toastStays && now >= t.exitAt+t.exitLength() {
			continue
		}
		g.paintToast(g.toastImage, t, now)
		x, y := g.toastBox(t, now)
		alpha := min(1, float64(now-t.at)/float64(toastFadeIn))
		op := &ebiten.DrawImageOptions{Filter: ebiten.FilterLinear}
		age := float64(now - t.exitAt)
		switch t.exit {
		case toastForwarded:
			k := outQuint(age / float64(toastForward))
			x += 400 * float32(k)
			alpha *= 1 - k
		case toastFaded:
			alpha *= 1 - min(1, age/float64(toastFade))
		case toastFlung:
			ms := min(age, float64(toastFling)) / float64(time.Millisecond)
			dx := t.vx * ms
			op.GeoM.Rotate(min(0, dx*0.1) * math.Pi / 180)
			op.GeoM.Translate(dx, 0.0025*ms*ms)
			k := ms / float64(toastFling/time.Millisecond)
			alpha *= 1 - k*k
		}
		op.GeoM.Translate(float64(x), float64(y))
		op.ColorScale.ScaleAlpha(float32(alpha))
		dst.DrawImage(g.toastImage, op)
	}
}

// paintToast draws t's box at rest into img.
func (g *scene) paintToast(img *ebiten.Image, t *toast, now time.Duration) {
	img.Clear()
	h := &g.view.hud
	k := float32(t.hoverBlend(now))
	bg := toastBackground
	bg.R += uint8(float32(toastHovered.R-bg.R) * k)
	bg.G += uint8(float32(toastHovered.G-bg.G) * k)
	bg.B += uint8(float32(toastHovered.B-bg.B) * k)
	rect(img, 0, 0, toastWidth, toastHeight, bg)
	// SimpleNotification's info circle, white on Background5.
	rect(img, 0, 0, toastIcon, toastHeight, toastIconFill)
	cx, cy := float32(toastIcon/2), float32(toastHeight/2)
	disc(img, cx, cy, 8, toastWhite)
	disc(img, cx, cy-3.75, 1.25, toastIconFill)
	rect(img, cx-1, cy-1, 2, 5, toastIconFill)
	text, action := "스크린샷이 저장되었어요!", "클릭해서 보기"
	if g.english {
		text, action = "Screenshot saved!", "Click to view"
	}
	room := toastWidth - toastIcon - toastClose - 20
	face := h.Faces[small]
	h.Left(img, ui.Fit(text, face, room), small, toastIcon+10, 25, toastWhite)
	h.Left(img, ui.Fit(filepath.Base(t.path), face, room), small, toastIcon+10, 45, toastWhite)
	h.Left(img, action, small, toastIcon+10, 65, toastCheck)
	// The check button: black at 0.15 behind it and a white check while hovered.
	check := toastCheck
	if t.closeHov {
		rect(img, toastWidth-toastClose, 0, toastClose, toastHeight, color.NRGBA{A: 38})
		check = toastWhite
	}
	ox, oy := float32(toastWidth-toastClose/2-6), float32(toastHeight/2-8)
	vector.StrokeLine(img, ox+1.4, oy+8.3, ox+4.4, oy+11.3, 2, check, true)
	vector.StrokeLine(img, ox+3.7, oy+11.3, ox+11.3, oy+3.7, 2, check, true)
	if a := 0.8 * (1 - outQuart(float64(now-t.at)/float64(toastFlash))); a > 0 {
		fill(img, 0, 0, toastWidth, toastHeight, withAlpha(toastWhite, a), true)
	}
}
