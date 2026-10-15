package play

import (
	"cmp"
	"fmt"
	"image"
	"math"
	"slices"
	"strings"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
	"github.com/hajimehoshi/ebiten/v2/vector"

	"ggosu/internal/settings"
	"ggosu/internal/ui"
)

type hudItem uint8

const (
	noHUD hudItem = iota
	songHUD
	scoreHUD
	countsHUD
	timeHUD
	gaugesHUD
	meterHUD
	comboHUD
	judgementHUD
	healthHUD
	ghostHUD
	hitHUD
)

var hudNames = [...]string{"", "SONG", "SCORE", "COUNTS", "TIME", "GAUGES", "TIMING", "COMBO", "JUDGEMENT", "HEALTH", "GHOST", "HIT POSITION"}

func hudPoint(l *settings.Layout, item hudItem) *settings.Point {
	switch item {
	case healthHUD:
		return &l.Health
	case songHUD:
		return &l.Song
	case ghostHUD:
		return &l.Ghost
	case scoreHUD:
		return &l.Score
	case countsHUD:
		return &l.Counts
	case timeHUD:
		return &l.Time
	case gaugesHUD:
		return &l.Gauges
	case meterHUD:
		return &l.Meter
	case comboHUD:
		return &l.Combo
	case judgementHUD:
		return &l.Judgement
	default:
		return nil
	}
}

func (p *view) hudOffset(item hudItem) (float32, float32) {
	b := p.hudBounds(item)
	d := p.defaultBounds(item)
	return float32(b.Min.X - d.Min.X), float32(b.Min.Y - d.Min.Y)
}

func box(x, y, w, h float32) image.Rectangle {
	return image.Rect(int(math.Floor(float64(x))), int(math.Floor(float64(y))), int(math.Ceil(float64(x+w))), int(math.Ceil(float64(y+h))))
}

func (p *view) defaultBounds(item hudItem) image.Rectangle {
	l := p.field.layout
	switch item {
	case songHUD:
		title, chart := p.songLines()
		return box(hudEdge, 14, float32(max(textWidth(p.hud, title, label), textWidth(p.hud, chart, small))), 50)
	case ghostHUD:
		x, y := p.ghostAnchor()
		return box(x-110, y-26, 220, 46)
	case scoreHUD:
		return box(Width-hudEdge-p.scoreWidth()-4, 10, p.scoreWidth()+8, 64)
	case countsHUD:
		// Bottom-left, against the field's left edge.
		return box(max(16, l.left-12-192), Height-16-78, 192, 78)
	case healthHUD:
		h := (l.bottom - l.top) * healthHeightRatio
		return box(p.healthEdge(), l.bottom-h, 96, h)
	case timeHUD:
		r := p.timeRight()
		if p.timeDisplay != "bar" {
			return box(r-32, 24, 32, 32)
		}
		return box(r-192, 16, 192, 62)
	case gaugesHUD:
		return box(l.left+l.width+14, 90, 76, 538)
	case meterHUD:
		x, w, y := p.meterAnchor()
		return box(x-w/2-20, y-30, w+40, 64)
	case comboHUD:
		if p.comboPosition == "left" {
			return image.Rect(16, 300, 208, 356)
		}
		x, y, h := p.comboAnchor()
		return box(x-110, y-24, 220, h+28)
	case judgementHUD:
		if p.judgementPosition == "left" {
			return image.Rect(16, 432, 208, 480)
		}
		x, y, _ := p.judgementAnchor()
		if p.judgementPosition == "below" {
			return box(x-96, y-24, 192, 50)
		}
		return box(x-110, y-40, 220, 80)
	case hitHUD:
		return box(l.left, l.line-12, l.width, 24)
	}
	return image.Rectangle{}
}

// Clamp the whole handle into the frame, including when another key count or
// hit position changes the preset's anchor. Saved offsets remain independent.
func (p *view) hudBounds(item hudItem) image.Rectangle {
	b := p.defaultBounds(item)
	if q := hudPoint(&p.layout, item); q != nil {
		scale := p.hudScale(item)
		b.Max = b.Min.Add(image.Pt(int(math.Ceil(float64(b.Dx())*scale)), int(math.Ceil(float64(b.Dy())*scale))))
		x := min(max(b.Min.X+q.X, 0), Width-b.Dx())
		y := min(max(b.Min.Y+q.Y, 0), Height-b.Dy())
		b = b.Add(image.Pt(x-b.Min.X, y-b.Min.Y))
	}
	return b
}

func (p *view) hudScale(item hudItem) float64 {
	if q := hudPoint(&p.layout, item); q != nil && q.Scale != 0 {
		return float64(q.Scale) / 100
	}
	return 1
}

func (p *view) movable(item hudItem) bool {
	// The gauges are no longer a panel; their debug line stays in the corner.
	if item == gaugesHUD || item == healthHUD && p.session.Health.Level == 0 || item == ghostHUD && !p.ghost {
		return false
	}
	return !p.focus || item == comboHUD || item == judgementHUD || item == timeHUD || item == meterHUD || item == healthHUD || item == ghostHUD || item == hitHUD
}

func (p *view) hudAt(at image.Point) hudItem {
	for item := hitHUD; item > noHUD; item-- {
		if p.movable(item) && at.In(p.hudBounds(item)) {
			return item
		}
	}
	return noHUD
}

type hudEditor struct {
	before settings.Display
	item   hudItem
	start  image.Point
	origin image.Rectangle
	status string
	corner int // 1-4 while a scale handle of the selection is dragged.
	scale  float64
}

// handleSize is the side of the corner squares that scale the selection, as on
// osu!lazer's skin editor selection box.
const handleSize = 8

// corners returns b's corner handles: top-left, top-right, bottom-left, bottom-right.
func corners(b image.Rectangle) [4]image.Rectangle {
	h := handleSize / 2
	var out [4]image.Rectangle
	for i, c := range [4]image.Point{b.Min, {b.Max.X, b.Min.Y}, {b.Min.X, b.Max.Y}, b.Max} {
		out[i] = image.Rect(c.X-h, c.Y-h, c.X+h, c.Y+h)
	}
	return out
}

// begin grabs a corner handle of selected first, then whichever item is under at.
func (e *hudEditor) begin(p *view, at image.Point, selected hudItem) {
	e.corner = 0
	if selected != noHUD && selected != hitHUD && p.movable(selected) {
		for i, c := range corners(p.hudBounds(selected)) {
			if at.In(c.Inset(-2)) {
				e.item, e.corner, e.scale = selected, i+1, p.hudScale(selected)
				e.start, e.origin = at, p.hudBounds(selected)
				return
			}
		}
	}
	e.item = p.hudAt(at)
	e.start, e.origin = at, p.hudBounds(e.item)
}

// resize scales the item uniformly about the corner opposite the grabbed one.
func (e *hudEditor) resize(p *view, at image.Point) {
	o, left, top := e.origin, e.corner == 1 || e.corner == 3, e.corner <= 2
	fx, fy := o.Min.X, o.Min.Y
	if left {
		fx = o.Max.X
	}
	if top {
		fy = o.Max.Y
	}
	// Project the pointer onto the box diagonal so both axes drive the size.
	w, h := float64(o.Dx()), float64(o.Dy())
	dx, dy := math.Abs(float64(at.X-fx)), math.Abs(float64(at.Y-fy))
	f := (dx*w + dy*h) / (w*w + h*h)
	s := min(max(int(math.Round(e.scale*f*100)), 25), 200)
	b := p.defaultBounds(e.item)
	nw, nh := int(math.Ceil(float64(b.Dx())*float64(s)/100)), int(math.Ceil(float64(b.Dy())*float64(s)/100))
	x, y := fx, fy
	if left {
		x = fx - nw
	}
	if top {
		y = fy - nh
	}
	x, y = min(max(x, 0), Width-nw), min(max(y, 0), Height-nh)
	q := hudPoint(&p.layout, e.item)
	q.X, q.Y, q.Scale = x-b.Min.X, y-b.Min.Y, s
	if s == 100 {
		q.Scale = 0
	}
	p.applyLayout(p.layout)
}

func (e *hudEditor) drag(p *view, at image.Point) {
	if e.item == noHUD {
		return
	}
	if e.corner != 0 {
		e.resize(p, at)
		return
	}
	d := at.Sub(e.start)
	if e.item == hitHUD {
		p.layout.HitPosition = min(max(e.origin.Min.Y+12+d.Y, settings.MinHitPosition), settings.MaxHitPosition)
	} else {
		b := p.defaultBounds(e.item)
		x := min(max(e.origin.Min.X+d.X, 0), Width-e.origin.Dx())
		y := min(max(e.origin.Min.Y+d.Y, 0), Height-e.origin.Dy())
		q := hudPoint(&p.layout, e.item)
		q.X, q.Y = x-b.Min.X, y-b.Min.Y
	}
	p.applyLayout(p.layout)
}

func (p *view) applyLayout(l settings.Layout) {
	p.layout = l
	line := skinLine(p.skin)
	if l.HitPosition != 0 {
		line = float32(l.HitPosition)
	}
	line = p.liftLine(line)
	if line == p.field.layout.line {
		return
	}
	p.field.layout.line = line
	p.base.Clear()
	p.drawBase(p.base)
	w, h := p.field.layout.surfaceSize()
	p.notes.Deallocate()
	p.notes = ebiten.NewImage(w, h)
	p.field.update(p.state.Position+p.config.Judge, p.state.Scroll)
}

// hudDisplay returns d with the HUD choices the view currently shows.
func (p *view) hudDisplay(d settings.Display) settings.Display {
	d.Layout = p.layout
	d.HUD = "standard"
	if p.focus {
		d.HUD = "focus"
	}
	d.ComboPosition, d.JudgementPosition, d.TimeDisplay = p.comboPosition, p.judgementPosition, cmp.Or(p.timeDisplay, "pie")
	d.TimingThreshold = p.timingThreshold
	d.Skin = p.skin
	d.LaneLines = p.laneLines
	d.NoteThickness = p.noteThickness
	d.LaneDim, d.NoteColour, d.JudgementShow, d.HideCombo = p.laneDim, p.noteColour, p.judgementShow, p.hideCombo
	return d
}

// setHUD shows d's HUD choices at once; play keeps running underneath.
func (p *view) setHUD(d settings.Display) {
	p.ghost, p.ghostMax, p.ghostScore = d.Ghost != "off", d.Ghost == "max", d.TargetScore()
	p.laneLines = d.LaneLines
	p.noteThickness = d.NoteThickness
	p.focus = d.HUD == "focus"
	p.comboPosition, p.judgementPosition = cmp.Or(d.ComboPosition, "center"), cmp.Or(d.JudgementPosition, "center")
	p.timingThreshold = cmp.Or(d.TimingThreshold, "cool")
	p.timeDisplay = cmp.Or(d.TimeDisplay, "pie")
	p.laneDim, p.judgementShow, p.hideCombo = d.LaneDim, d.JudgementShow, d.HideCombo
	if skin := cmp.Or(d.Skin, "argon"); skin != p.skin || d.NoteColour != p.noteColour {
		// A new skin brings its own columns; a zero line makes applyLayout rebuild the field.
		p.skin = skin
		p.field.layout = newLayout(p.session.Chart, skin)
		p.field.layout.line = 0
		p.setNoteColour(d.NoteColour)
	}
	p.base.Clear()
	p.drawBase(p.base)
	p.applyLayout(d.Layout)
	p.noteMargin()
	p.field.update(p.state.Position+p.config.Judge, p.state.Scroll)
}

// Keep thick heads and tail trails visible until their pixels leave the field.
func (p *view) noteMargin() {
	p.field.margin = 16
	for _, lane := range p.field.layout.lanes {
		p.field.margin = max(p.field.margin, float64(p.noteHeight(lane.width))+56)
	}
}

var (
	presetButtons = [...]image.Rectangle{image.Rect(8, 2, 164, 30), image.Rect(172, 2, 328, 30), image.Rect(336, 2, 492, 30), image.Rect(500, 2, 656, 30), image.Rect(664, 2, 820, 30)}
	editButtons   = [...]image.Rectangle{image.Rect(956, 2, 1056, 30), image.Rect(1064, 2, 1164, 30), image.Rect(1172, 2, 1272, 30)}
	hudModes      = []string{"standard", "focus"}
	timeDisplays  = []string{"pie", "bar"}
	hudPositions  = []string{"center", "below", "left"}
)

func cycle(values []string, v string) string {
	return values[(slices.Index(values, v)+1)%len(values)]
}

func (p *view) presetLabels() [5]string {
	d := p.hudDisplay(settings.Display{})
	return [5]string{"SKIN " + d.Skin, "HUD " + d.HUD, "TIME " + d.TimeDisplay, "COMBO " + d.ComboPosition, "JUDGE " + d.JudgementPosition}
}

// A preset change drops that item's drag offset, as in the settings window.
func (g *scene) cyclePreset(i int) {
	d := g.view.hudDisplay(g.display)
	switch i {
	case 0:
		d.Skin = cycle(skinNames, d.Skin)
	case 1:
		d.HUD = cycle(hudModes, d.HUD)
	case 2:
		d.TimeDisplay = cycle(timeDisplays, d.TimeDisplay)
	case 3:
		d.ComboPosition, d.Layout.Combo = cycle(hudPositions, d.ComboPosition), settings.Point{}
	case 4:
		d.JudgementPosition, d.Layout.Judgement = cycle(hudPositions, d.JudgementPosition), settings.Point{}
	}
	g.view.setHUD(d)
}

// Like osu!lazer's skin editor, editing does not stop play: every change shows
// on the running HUD.
func (g *scene) openEditor() {
	g.editing, g.view.editing = true, true
	g.cursorShown = true
	g.editor = hudEditor{before: g.view.hudDisplay(g.display), status: "Drag to move"}
}

// The settings window's HUD editor is its own game; closing the editor ends it.
func (g *scene) closeEditor() {
	g.editing, g.view.editing = false, false
	if g.hudEdit {
		g.stopRequested = true
	}
}

func (g *scene) cancelHUD() {
	g.view.setHUD(g.editor.before)
	g.closeEditor()
}

func (g *scene) editHUD(focused bool) {
	if !g.preview && g.pressed(ebiten.KeyEscape) {
		g.cancelHUD()
		return
	}
	if !focused {
		g.editor.item = noHUD
		return
	}
	point := g.cursor()
	if inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonLeft) {
		if !g.preview {
			for i, b := range presetButtons {
				if point.In(b) {
					g.cyclePreset(i)
					return
				}
			}
			for i, b := range editButtons {
				if !point.In(b) {
					continue
				}
				switch i {
				case 0:
					d := g.view.hudDisplay(g.display)
					d.Layout = settings.Layout{}
					g.view.setHUD(d)
				case 1:
					g.cancelHUD()
				case 2:
					g.commitHUD()
				}
				return
			}
		}
		selected := noHUD
		if g.preview {
			selected = g.previewItem
		}
		g.editor.begin(g.view, point, selected)
		if g.preview && g.editor.item != noHUD {
			g.previewItem = g.editor.item
		}
	}
	// Apply the release position as well, so quick drags retain their final point.
	if ebiten.IsMouseButtonPressed(ebiten.MouseButtonLeft) || inpututil.IsMouseButtonJustReleased(ebiten.MouseButtonLeft) {
		g.editor.drag(g.view, point)
	}
	if !ebiten.IsMouseButtonPressed(ebiten.MouseButtonLeft) {
		g.editor.item = noHUD
	}
}

func (g *scene) commitHUD() {
	d := g.view.hudDisplay(g.display)
	if err := g.persistHUD(d); err != nil {
		g.editor.status = "Save failed: " + err.Error()
		return
	}
	g.display = d
	g.closeEditor()
}

func (g *scene) drawEditor(dst *ebiten.Image) {
	p := g.view
	hover := p.hudAt(g.cursor())
	if g.preview {
		selected := g.previewItem
		if g.editor.item != noHUD {
			selected, hover = g.editor.item, g.editor.item
		}
		g.drawBlueprints(dst, selected, hover)
		return
	}
	rect(dst, 0, Height-26, Width, 26, panelFill)
	p.hud.Center(dst, "DRAG HUD / HIT POSITION     F9 or SAVE: keep changes     ESC or CANCEL: restore", small, Width/2, Height-8, ink)
	for item := songHUD; item <= hitHUD; item++ {
		if !p.movable(item) {
			continue
		}
		b, c := p.hudBounds(item), muted
		if item == hover || item == g.editor.item {
			c = accent
		}
		vector.StrokeRect(dst, float32(b.Min.X), float32(b.Min.Y), float32(b.Dx()), float32(b.Dy()), 1, c, false)
		if item >= comboHUD {
			y := float32(b.Min.Y - 4)
			rect(dst, float32(b.Min.X), y-13, 100, 16, panelFill)
			p.hud.Left(dst, hudNames[item], small, float32(b.Min.X+4), y, c)
		}
		if item == hitHUD {
			p.hud.Right(dst, fmt.Sprintf("Y %d", int(p.field.layout.line)), small, float32(b.Max.X-4), float32(b.Min.Y+13), c)
		}
	}
	rect(dst, 0, 0, Width, 32, panelFill)
	for i, name := range p.presetLabels() {
		b := presetButtons[i]
		c := muted
		if g.cursor().In(b) {
			c = accent
		}
		framed(dst, float32(b.Min.X), float32(b.Min.Y), float32(b.Dx()), float32(b.Dy()), c)
		p.hud.Center(dst, strings.ToUpper(name), small, float32(b.Min.X+b.Dx()/2), 21, ink)
	}
	p.hud.Left(dst, ui.Fit(g.editor.status, p.hud.Faces[small], 120), small, 828, 21, ink)
	for i, name := range []string{"RESET", "CANCEL", "SAVE (F9)"} {
		b := editButtons[i]
		panel(dst, float32(b.Min.X), float32(b.Min.Y), float32(b.Dx()), float32(b.Dy()))
		p.hud.Center(dst, name, small, float32(b.Min.X+b.Dx()/2), 21, ink)
	}
}

// drawBlueprints marks every movable item like osu!lazer's skin editor: a faint
// box on each, the name on hover or selection, and corner handles to scale the
// selection.
func (g *scene) drawBlueprints(dst *ebiten.Image, selected, hover hudItem) {
	p := g.view
	for item := songHUD; item <= hitHUD; item++ {
		if !p.movable(item) {
			continue
		}
		b, c := p.hudBounds(item), withAlpha(muted, .5)
		x, y, w, h := float32(b.Min.X), float32(b.Min.Y), float32(b.Dx()), float32(b.Dy())
		switch item {
		case selected:
			c = accent
			rect(dst, x, y, w, h, withAlpha(accent, .12))
		case hover:
			c = withAlpha(accent, .7)
		}
		vector.StrokeRect(dst, x, y, w, h, 1+float32(boolInt(item == selected)), c, false)
		if item != selected && item != hover {
			continue
		}
		// The name sits under the box's right edge, inside when the box meets the bottom.
		label := hudNames[item]
		if item == hitHUD {
			label = fmt.Sprintf("%s %d", label, int(p.field.layout.line))
		} else if s := p.hudScale(item); s != 1 {
			label = fmt.Sprintf("%s %d%%", label, int(math.Round(s*100)))
		}
		ly := y + h + 14
		if ly > Height-2 {
			ly = y + h - 4
		}
		p.hud.Right(dst, label, small, x+w, ly, c)
		if item == selected && item != hitHUD {
			for _, k := range corners(b) {
				rect(dst, float32(k.Min.X), float32(k.Min.Y), handleSize, handleSize, accent)
			}
		}
	}
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
