package play

import (
	"cmp"
	"fmt"
	"image/color"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"
	"golang.org/x/image/font"

	"ggosu/internal/chart"
	"ggosu/internal/piano"
	"ggosu/internal/settings"
	"ggosu/internal/ui"
)

const (
	small   = ui.Small
	regular = ui.Regular
	label   = ui.Label
	large   = ui.Large
)

type hud struct {
	exposureText string
	exposure     int
	cover        settings.Cover
	coverText    string
	ui.Text
	flowText, precisionText            string
	title                              string
	chartText, songText                string
	score                              piano.Score
	scoreText, accuracyText, comboText string
	counts                             [4]string
	bucket                             time.Duration
	clockText, scrollText, meterText   string
	judgementText                      string
	rate, scroll                       float64
	results                            int
	scoreShown, scoreTarget            int
	scoreFrom                          float64
	scoreAt, scoreTime                 time.Duration
	scoreReady                         bool
	comboAt                            time.Duration
}

func newHUD(c *chart.Chart, data []byte) (hud, error) {
	var h hud
	var err error
	h.Text, err = ui.NewText(data)
	if err != nil {
		return h, err
	}
	// "Artist - Title" over "[7K] Difficulty (1.0x)".
	title := c.Metadata.Title
	if c.Metadata.Artist != "" {
		title = c.Metadata.Artist + " - " + title
	}
	h.title = title
	h.chartText = fmt.Sprintf("[%dK]", c.Lanes)
	if c.Metadata.Difficulty != "" {
		h.chartText += " " + c.Metadata.Difficulty
	}
	h.bucket = -chart.MaxTime
	h.results = -1
	h.comboAt = -chart.MaxTime
	h.scoreText, h.comboText, h.accuracyText = "0000000", "0", "--"
	return h, nil
}

// comboLift follows Quaver's combo bump: 5 px up, back in 370 ms (EaseOutExpo).
func comboLift(age time.Duration) float32 {
	const d = 370 * time.Millisecond
	if age < 0 || age >= d {
		return 0
	}
	return -5 * float32(math.Pow(2, -10*float64(age)/float64(d)))
}

type pose struct {
	scale, fall, angle float64
	alpha              float64
}

// judgementPose follows hndada/gosu (a short 1.15x pulse, 275 ms) for hits and
// osu!mania's default piece for MISS (shrinks from 1.6x, then drops 100 px and tilts 40° over 800 ms).
func judgementPose(g piano.Grade, age time.Duration) (pose, bool) {
	ms := float64(age) / float64(time.Millisecond)
	if age < 0 {
		return pose{}, false
	}
	if g == piano.Miss {
		if ms >= 800 {
			return pose{}, false
		}
		k := math.Pow(ms/800, 5)
		return pose{scale: 1.6 - 0.6*math.Pow(min(1, ms/100), 2), fall: 100 * k, angle: 40 * math.Pi / 180 * k, alpha: 1 - ms/800}, true
	}
	switch {
	case ms >= 275:
		return pose{}, false
	case ms < 25:
		return pose{scale: 1 + 0.15*ms/25, alpha: 1}, true
	case ms < 50:
		return pose{scale: 1.15 - 0.15*(ms-25)/25, alpha: 1}, true
	case ms < 250:
		return pose{scale: 1, alpha: 1}, true
	default:
		return pose{scale: 1 - 0.25*(ms-250)/25, alpha: 1}, true
	}
}

func clockText(t time.Duration) string {
	seconds := int(max(time.Duration(0), t) / time.Second)
	return fmt.Sprintf("%02d:%02d", seconds/60, seconds%60)
}

// updateScore retargets from the current interpolated value, so dense chords
// keep counting smoothly. Rewinds, resets and the final frame show the exact score.
func (h *hud) updateScore(target int, at time.Duration, final bool) {
	value := tween(at-h.scoreAt, 180*time.Millisecond, h.scoreFrom, float64(h.scoreTarget))
	if !h.scoreReady || at < h.scoreTime || target < h.scoreTarget || final {
		value = float64(target)
		h.scoreFrom, h.scoreTarget, h.scoreAt = value, target, at
		h.scoreReady = true
	} else if target != h.scoreTarget {
		h.scoreFrom, h.scoreTarget, h.scoreAt = value, target, at
	}
	h.scoreTime = at
	shown := int(math.Round(value))
	if h.scoreText == "" || shown != h.scoreShown {
		h.scoreShown, h.scoreText = shown, fmt.Sprintf("%07d", shown)
	}
}

func (h *hud) update(p *view) {
	s, at := p.session.Score, p.state.Position

	if s.Combo > h.score.Combo {
		h.comboAt = at
	}
	h.updateScore(s.Points, at, p.state.Finished)
	if h.results < 0 || h.score != s {
		h.score = s
		h.comboText = fmt.Sprint(s.Combo)
		h.accuracyText = "--"
		if s.Total > 0 {
			h.accuracyText = fmt.Sprintf("%.2f%%", s.Accuracy()*100)
		}
		for i, n := range []int{s.Kool, s.Cool, s.Good, s.Miss} {
			h.counts[i] = fmt.Sprint(n)
		}
		h.flowText, h.precisionText = fmt.Sprint(s.Flow), fmt.Sprint(s.Precision)
	}
	if h.results != p.results {
		h.results = p.results
		h.meterText = "No timing samples"
		if p.meter.count > 0 {
			h.meterText = fmt.Sprintf("Last %+.1f ms    Mean %+.1f ms", p.meter.last().Seconds()*1000, p.meter.mean().Seconds()*1000)
		}
		if p.hasJudgement {
			h.judgementText = strings.ToUpper(string(p.judgement.Grade))
		}
	}
	bucket := p.state.Position / (100 * time.Millisecond)
	if bucket != h.bucket {
		h.bucket = bucket
		h.clockText = clockText(min(p.state.Position, p.duration)) + " / " + clockText(p.duration)
	}
	if h.rate != p.state.Rate {
		h.rate = p.state.Rate
		h.songText = h.chartText + " (" + rateText(h.rate) + ")"
	}
	ms := int(math.Round(p.exposureMS()))
	var cover settings.Cover
	if p.config.Profile != nil {
		cover = p.config.Profile.LaneCover()
	}
	if h.scrollText == "" || h.scroll != p.state.Scroll || h.exposure != ms || h.cover != cover {
		h.scroll, h.exposure, h.cover = p.state.Scroll, ms, cover
		h.exposureText = "—"
		if ms > 0 {
			h.exposureText = fmt.Sprintf("%dms", ms)
		}
		h.scrollText = fmt.Sprintf("%.1f (%s)", h.scroll/settings.PixelsPerScroll, h.exposureText)
		h.coverText = coverText(cover)
	}
}

// rateText keeps at least one decimal: 1.0x, 1.25x, 1.5x.
func rateText(rate float64) string {
	s := strconv.FormatFloat(rate, 'f', -1, 64)
	if !strings.Contains(s, ".") {
		s += ".0"
	}
	return s + "x"
}

var ink = color.NRGBA{227, 234, 245, 255}
var muted = color.NRGBA{147, 161, 180, 255}
var accent = color.NRGBA{83, 214, 195, 255}

func rect(dst *ebiten.Image, x, y, w, h float32, c color.NRGBA) {
	fill(dst, x, y, w, h, c, false)
}

func (p *view) drawBase(dst *ebiten.Image) { p.drawColumns(dst) }

func (p *view) drawHUD(dst *ebiten.Image) {
	if !p.focus {
		p.drawItem(dst, songHUD, p.drawSong)
		p.drawItem(dst, scoreHUD, p.drawScore)
		p.drawItem(dst, countsHUD, p.drawCounts)
	}
	if p.ghost {
		p.drawItem(dst, ghostHUD, p.drawGhost)
	}
	p.drawItem(dst, healthHUD, p.drawHealth)
	p.drawItem(dst, meterHUD, p.drawMeter)
	p.drawItem(dst, timeHUD, p.drawTime)
	p.drawItem(dst, comboHUD, p.drawCombo)
	p.drawItem(dst, judgementHUD, p.drawJudgement)
	if p.config.Debug {
		p.drawGauges(dst)
	}
}

func (p *view) drawItem(dst *ebiten.Image, item hudItem, draw func(*ebiten.Image)) {
	scale := p.hudScale(item)
	if scale == 1 {
		draw(dst)
		return
	}
	if p.hudLayer == nil {
		p.hudLayer = ebiten.NewImage(Width, Height)
	}
	p.hudLayer.Clear()
	draw(p.hudLayer)
	b := p.hudBounds(item)
	op := &ebiten.DrawImageOptions{Filter: ebiten.FilterLinear}
	op.GeoM.Translate(-float64(b.Min.X), -float64(b.Min.Y))
	op.GeoM.Scale(scale, scale)
	op.GeoM.Translate(float64(b.Min.X), float64(b.Min.Y))
	dst.DrawImage(p.hudLayer, op)
}

// drawSong writes the title over the chart in the top-left corner, each on one line
// and without a panel, keeping clear of the field.
func (p *view) drawSong(dst *ebiten.Image) {
	h := p.hud
	x, y := p.hudOffset(songHUD)
	title, chart := p.songLines()
	h.Left(dst, title, label, hudEdge+x, 36+y, ink)
	h.Left(dst, chart, small, hudEdge+x, 58+y, muted)
}

// songLines fits the title and the chart into the space left of the field.
func (p *view) songLines() (title, chart string) {
	h := p.hud
	room := max(int(p.field.layout.left)-hudEdge-16, 40)
	return ui.Fit(h.title, h.Faces[label], room), ui.Fit(h.songText, h.Faces[small], room)
}

func textWidth(h hud, s string, size int) int {
	return font.MeasureString(h.Faces[size], s).Ceil()
}

// exposureMS matches the exposure scroll mode: real time at scroll multiplier 1,
// including the current playback rate and the actual field reach.
func (p *view) exposureMS() float64 {
	if p.state.Scroll <= 0 || p.state.Rate <= 0 {
		return 0
	}
	return float64(p.field.layout.line-p.field.layout.top) * p.visibleShare() * 1000 / (p.state.Scroll * p.state.Rate)
}

// drawScroll keeps "speed (exposure)" in the bottom-left corner; the focus HUD
// shows it only for 1.5 s after PageUp/Down.
func (g *scene) drawScroll(dst *ebiten.Image) {
	if g.view == nil || g.clock == nil || g.view.focus && g.clock.Now() >= g.scrollUntil {
		return
	}
	g.view.hud.Left(dst, g.view.hud.scrollText, regular, 20, Height-20, ink)
	if t := g.view.hud.coverText; t != "" {
		g.view.hud.Left(dst, t, regular, 20, Height-48, ink)
	}
}

func (p *view) ghostPoints() int {
	s := p.session.Score
	if s.Possible == 0 {
		return 0
	}
	return int(int64(p.ghostScore) * int64(s.Total) / int64(s.Possible))
}

// ghostDiff is the pace lead over the target, or in max mode how far the best
// score still reachable (all remaining notes KOOL) stays above the target.
func (p *view) ghostDiff() int {
	if p.ghostMax {
		return p.session.Score.MaxPoints() - p.ghostScore
	}
	return p.session.Score.Points - p.ghostPoints()
}

func (p *view) ghostAnchor() (x, y float32) {
	l := p.field.layout
	return l.left + l.width/2, (l.top+skinLine(p.skin))/2 + 56
}

func (p *view) drawGhost(dst *ebiten.Image) {
	x, y := p.ghostAnchor()
	dx, dy := p.hudOffset(ghostHUD)
	x, y = x+dx, y+dy
	diff := p.ghostDiff()
	c := accent
	if diff < 0 {
		c = gradeColor(piano.Miss)
	}
	text := fmt.Sprintf("%+d", diff)
	p.hud.Center(dst, text, label, x+1, y+1, color.NRGBA{0, 0, 0, 220})
	p.hud.Center(dst, text, label, x, y, c)
	target := ghostTarget(p.ghostScore)
	p.hud.Posed(dst, target, small, x+1, y+19, 0.8, 0, color.NRGBA{0, 0, 0, 220})
	p.hud.Posed(dst, target, small, x, y+18, 0.8, 0, muted)
}

// ghostTarget rounds the target to thousands so it reads at a glance.
func ghostTarget(score int) string {
	if score < 1000 {
		return strconv.Itoa(score)
	}
	return strconv.Itoa((score+500)/1000) + "K"
}

func (p *view) drawHealth(dst *ebiten.Image) {
	h := p.session.Health
	if h.Level == 0 {
		return
	}
	x, y := p.hudOffset(healthHUD)
	l := p.field.layout
	height := (l.bottom - l.top) * healthHeightRatio
	x += p.healthEdge()
	y += l.bottom - height
	c, tip := accent, mix(accent, white, 0.65)
	switch h.Level {
	case 2:
		c, tip = color.NRGBA{255, 190, 66, 255}, color.NRGBA{255, 245, 198, 255}
	case 3:
		c, tip = color.NRGBA{176, 111, 255, 255}, color.NRGBA{218, 244, 255, 255}
	}
	filled := height * p.health.value / 10000
	if filled > 0 {
		// The fill thumps on the beat; the number below stays at the exact value.
		filled = min(height, filled+healthBeat(p.session.Chart.Timing, p.state.Position))
	}
	front, bottom := y+height-filled, y+height
	seconds := p.state.Position.Seconds()
	gain := healthPulse(p.state.Position-p.health.gain, 420*time.Millisecond)
	loss := healthPulse(p.state.Position-p.health.loss, 560*time.Millisecond)
	danger := 0.0
	red := gradeColor(piano.Miss)
	if h.Value < 2500 {
		danger = (1 + math.Sin(seconds*math.Pi*4)) / 2
		c = mix(c, red, 0.65+0.25*danger)
		tip = mix(red, white, 0.35+0.35*danger)
	}
	c = mix(c, red, loss*0.85)
	tip = mix(tip, white, gain*0.75)
	rect(dst, x, y, 12, height, darken(c, 8))
	// Lost health hangs above the live fill, then drains down to it.
	trail := y + height - height*p.health.trail/10000
	if front > trail {
		rect(dst, x, trail, 12, front-trail, withAlpha(red, 0.25+0.55*loss))
		rect(dst, x+9, trail, 3, front-trail, withAlpha(white, 0.2*loss))
	}
	// The glow grows outward from the rail, away from the notes.
	for i := 1; i <= 9; i++ {
		strength := (0.1 + gain*0.45 + loss*0.35 + danger*0.16) * (1 - float64(i)/10)
		rect(dst, x+12+float32(i-1), min(front, trail), 1, bottom-min(front, trail), withAlpha(c, strength))
	}
	// Argon: the glow climbs ahead of the fill on a rise.
	if glowTop := y + height - height*p.health.glow/10000; glowTop < front {
		rect(dst, x, glowTop, 16, front-glowTop, withAlpha(c, 0.35))
	}
	if filled > 0 {
		rect(dst, x, front, 12, filled, c)
		if flash := healthFlash(p.state.Position - p.health.gain); flash > 0 {
			rect(dst, x, front, 12, filled, withAlpha(white, 0.8*flash))
		}
		rect(dst, x, front, 1, filled, withAlpha(tip, 0.65))
		// Narrow light streaks rise at different speeds, even on the easy rail.
		for i := 0; i < 3; i++ {
			phase := math.Mod(seconds*(0.18+float64(i)*0.07)+float64(i)*0.31, 1)
			if phase < 0 {
				phase++
			}
			top := bottom - float32(phase)*height
			end := min(bottom, top+18+float32(i)*8)
			top = max(front, top)
			if end > top {
				rect(dst, x+2+float32(i)*3, top, 1, end-top, withAlpha(tip, 0.4+gain*0.5))
			}
		}
		if h.Level >= 2 {
			// A bright core and a rising sheen give the gold rail a metallic finish.
			rect(dst, x+4, front, 3, filled, withAlpha(tip, 0.6))
			sweep := bottom + 48 - float32(math.Mod(seconds*90, float64(height+96)))
			for i := -8; i < 8; i++ {
				top := max(front, sweep+float32(i*6))
				end := min(bottom, sweep+float32((i+1)*6))
				if end > top {
					a := 0.55 * (1 - math.Abs(float64(i)+0.5)/8)
					rect(dst, x, top, 12, end-top, withAlpha(tip, a))
				}
			}
			vector.StrokeLine(dst, x+5.5, front, x+5.5, bottom, 1, withAlpha(tip, 0.6), true)
		}
		if h.Level == 3 {
			// Cyan and violet facets move inside the rail, clipped to current health.
			cyan := color.NRGBA{105, 232, 255, 255}
			rect(dst, x+8, front, 2, filled, withAlpha(cyan, 0.8))
			shift := float32(math.Mod(seconds*36, 72))
			for yy := y + shift; yy < bottom; yy += 72 {
				if yy-10 < front || yy+10 > bottom {
					continue
				}
				vector.StrokeLine(dst, x+2, yy+10, x+10, yy-10, 1, withAlpha(tip, 0.5), true)
			}
		}
		rect(dst, x, front, 12, min(3, filled), tip)
		disc(dst, x+6, front+min(1.5, filled/2), 2+float32(gain)*2, withAlpha(tip, 0.8))
		if gain > 0 {
			age := (p.state.Position - p.health.gain).Seconds()
			for i := 0; i < 5; i++ {
				dx := 15 + float32(age*float64(18+i*7))
				dy := float32(math.Sin(float64(i)*2.4)*9 - age*float64(30+i*20))
				disc(dst, x+dx, max(y+2, front+dy), 1+float32(i%2), withAlpha(tip, gain*0.85))
			}
		}
	}
	if loss > 0 {
		rect(dst, x, y, 12, height, withAlpha(red, loss*0.24))
		vector.StrokeLine(dst, x+12, front, x+12+float32(loss)*16, front, 2, withAlpha(red, loss), true)
	}
	level := fmt.Sprintf("LV %d", h.Level)
	if p.session.O2Jam() {
		level = [...]string{"EASY", "NORMAL", "HARD"}[h.Level-1]
		// O2Jam's pill slots sit above the bar: five, lit for each pill in stock.
		if pills := p.session.Pills(); pills >= 0 {
			for i := 0; i < 5; i++ {
				slot := withAlpha(c, 0.25)
				if i < pills {
					slot = tip
				}
				rect(dst, x, y-10-float32(i)*8, 12, 5, slot)
			}
		}
	}
	p.hud.Left(dst, level, small, x+30, bottom-12, c)
	// The exact value sits at its own height, not the easing fill, clear of the level below.
	exact := bottom - height*float32(h.Value)/10000
	p.hud.Left(dst, fmt.Sprintf("%.2f%%", float64(h.Value)/100), small, x+30, max(y+10, min(exact+4, bottom-30)), c)
}

// hudEdge is the margin of the HUD items held to the frame's corners.
const hudEdge = 12

// scoreWidth is the widest score block, so the progress beside it never shifts.
// It is a whole, even number of pixels so the editor scales the block exactly.
func (p *view) scoreWidth() float32 {
	w := math.Ceil(float64(max(p.hud.CounterWidth("0000000", 25), float32(textWidth(p.hud, "100.00%", label)))))
	return float32(w + math.Mod(w, 2))
}

// drawScore holds score and accuracy against the top-right corner.
func (p *view) drawScore(dst *ebiten.Image) {
	h := p.hud
	x, y := p.hudOffset(scoreHUD)
	right := Width - hudEdge + x
	h.Counter(dst, h.scoreText, right-h.CounterWidth(h.scoreText, 25), hudEdge+2+y, 25, ink)
	h.Right(dst, h.accuracyText, label, right, 66+y, ink)
}

// timeRight is the right edge of the progress display, just left of the score.
func (p *view) timeRight() float32 {
	return Width - hudEdge - p.scoreWidth() - 14
}

func (p *view) drawCounts(dst *ebiten.Image) {
	h := p.hud
	d := p.defaultBounds(countsHUD)
	x, y := p.hudOffset(countsHUD)
	x, y = x+float32(d.Min.X), y+float32(d.Min.Y)
	// Minimal like lazer's judgement counter: a grade name and its count, no panel.
	for i, name := range []string{"KOOL", "COOL", "GOOD", "MISS"} {
		yy := float32(16+i*19) + y
		c := gradeColor(piano.Grade(strings.ToLower(name)))
		h.Left(dst, name, ui.CounterSmall, 96+x, yy, c)
		h.Right(dst, ui.Fit(h.counts[i], h.Faces[small], 60), small, 180+x, yy, ink)
	}
}

// FLOW/PREC are scoring internals: only a -debug run shows them, as one small line in the bottom-right corner.
func (p *view) drawGauges(dst *ebiten.Image) {
	p.hud.Right(dst, fmt.Sprintf("TPS %.0f  FLOW %s  PREC %s", ebiten.ActualTPS(), p.hud.flowText, p.hud.precisionText), small, Width-12, Height-52, muted)
	if p.strain.err != "" {
		p.hud.Right(dst, "STRAIN "+p.strain.err, small, Width-12, Height-72, muted)
	}
}

func (p *view) drawTime(dst *ebiten.Image) {
	progress := float32(0)
	if p.duration > 0 {
		progress = float32(min(1, max(0, float64(p.state.Position)/float64(p.duration))))
	}
	dx, dy := p.hudOffset(timeHUD)
	if p.timeDisplay != "bar" {
		// osu!stable-style progress: a filled clockwise sector with a thin rim,
		// without a panel, label, clock text or the old donut cutout.
		x, y, radius := p.timeRight()-16+dx, float32(40)+dy, float32(14)
		disc(dst, x, y, radius, withAlpha(ink, 0.18))
		if progress > 0 {
			var path vector.Path
			path.MoveTo(x, y)
			path.LineTo(x, y-radius)
			steps := max(1, int(64*progress))
			for i := 1; i <= steps; i++ {
				a := -math.Pi/2 + 2*math.Pi*float64(progress)*float64(i)/float64(steps)
				path.LineTo(x+radius*float32(math.Cos(a)), y+radius*float32(math.Sin(a)))
			}
			path.Close()
			op := &vector.DrawPathOptions{AntiAlias: true}
			op.ColorScale.ScaleWithColor(ink)
			vector.FillPath(dst, &path, nil, op)
		}
		ring(dst, x, y, 15, 1.5, withAlpha(ink, 0.7))
		return
	}
	w := float32(184)
	left := p.timeRight() - w - 4 + dx
	panel(dst, left-4, 16+dy, w+8, 62)
	p.hud.Left(dst, "TRACK TIME", small, left+10, 34+dy, muted)
	gauge(dst, left+10, 62+dy, w-20, 6, progress, accent)
	p.hud.Right(dst, p.hud.clockText, small, left+w-10, 55+dy, ink)
}

// Combo and judgement follow the skin's hit position, not a dragged one.
func (p *view) comboAnchor() (x, y, height float32) {
	l := p.field.layout
	x, y, height = l.left+l.width/2, (l.top+skinLine(p.skin))/2-130-60, 60
	switch p.comboPosition {
	case "left":
		x, y, height = 112, 322, 30
	case "below":
		x, y, height = l.left+l.width/2, 673, 26
		if p.judgementPosition == "below" {
			x -= 100
		}
	}
	return
}

func (p *view) drawCombo(dst *ebiten.Image) {
	if !p.editing && (p.hideCombo || p.comboPosition == "center" && p.phase != Playing && p.phase != Outro) {
		return
	}
	h := p.hud
	x, y, height := p.comboAnchor()
	dx, dy := p.hudOffset(comboHUD)
	x, y = x+dx, y+dy
	if p.comboPosition == "left" {
		panel(dst, 16+dx, 300+dy, 192, 56)
		h.Left(dst, "COMBO", ui.CounterSmall, 28+dx, 317+dy, accent)
	} else {
		labelY := y - 7
		if p.comboPosition == "center" {
			labelY = y - 10
		}
		h.Center(dst, "COMBO", ui.CounterSmall, x, labelY, accent)
	}
	if p.session.Score.Combo >= 100000 && height == 60 {
		height = 34
	}
	if w := h.CounterWidth(h.comboText, height); w > 200 {
		height *= 200 / w
	}
	if p.comboPosition == "left" {
		x = 196 + dx - h.CounterWidth(h.comboText, height)/2
	}
	h.Counter(dst, h.comboText, x-h.CounterWidth(h.comboText, height)/2, y+comboLift(p.state.Position-h.comboAt), height, ink)
}

func (p *view) judgementAnchor() (x, y float32, size int) {
	l := p.field.layout
	x, y, size = l.left+l.width/2, (l.top+skinLine(p.skin))/2+90, large
	switch p.judgementPosition {
	case "left":
		x, y, size = 112, 451, regular
	case "below":
		x, y, size = l.left+l.width/2, 685, label
		if p.comboPosition == "below" {
			x += 100
		}
	}
	return
}

func (p *view) drawJudgement(dst *ebiten.Image) {
	dx, dy := p.hudOffset(judgementHUD)
	if p.judgementPosition == "left" {
		panel(dst, 16+dx, 432+dy, 192, 24)
	}
	if !p.editing && p.judgementPosition == "center" && p.phase != Playing && p.phase != Outro {
		return
	}
	if !p.editing {
		p.drawTiming(dst, dx, dy)
	}
	if !p.editing && (!p.hasJudgement || !judgementShown(p.judgement.Grade, p.judgementShow)) {
		return
	}
	m, ok := judgementPose(p.judgement.Grade, p.state.Position-p.judgement.At)
	text, c := p.hud.judgementText, gradeColor(p.judgement.Grade)
	if p.editing {
		m, ok, text, c = pose{scale: 1, alpha: 1}, true, "KOOL", accent
	}
	if !ok {
		return
	}
	x, y, size := p.judgementAnchor()
	size += ui.CounterSmall
	x, y = x+dx, y+dy
	c.A = uint8(255 * m.alpha)
	p.hud.Posed(dst, text, size, x+2, y+2+float32(m.fall), m.scale, m.angle, color.NRGBA{0, 0, 0, c.A})
	p.hud.Posed(dst, text, size, x, y+float32(m.fall), m.scale, m.angle, c)
}

// timingHold keeps Early/Late on screen past the judgement's own fade, until the next one replaces it.
const timingHold, timingFade = 1000 * time.Millisecond, 150 * time.Millisecond

func (p *view) drawTiming(dst *ebiten.Image, dx, dy float32) {
	age := p.state.Position - p.timingShown.At
	if !p.hasTiming || age < 0 || age >= timingHold {
		return
	}
	alpha := min(1, float64(timingHold-age)/float64(timingFade))
	x, y, _ := p.judgementAnchor()
	size, offset := small, float32(18)
	if p.judgementPosition == "center" {
		size, offset = regular, 28
	}
	p.hud.Center(dst, timingText(p.timingShown, p.timingThreshold), size, x+dx, y+dy+offset, withAlpha(timingColor(p.timingShown.Delta), alpha))
}

// judgementShown follows ONGEKI's 判定表示設定: only the chosen grade or worse, or none.
func judgementShown(g piano.Grade, show string) bool {
	switch show {
	case "":
		return true
	case "off":
		return false
	}
	return gradeRank(g) >= gradeRank(piano.Grade(show))
}

func timingText(r piano.Result, threshold string) string {
	if threshold == "off" || r.Grade == piano.Miss || r.Delta == 0 || gradeRank(r.Grade) < gradeRank(piano.Grade(cmp.Or(threshold, "cool"))) {
		return ""
	}
	if r.Delta < 0 {
		return "Early"
	}
	return "Late"
}

func timingColor(delta time.Duration) color.NRGBA {
	if delta < 0 {
		return color.NRGBA{117, 178, 242, 255}
	}
	if delta > 0 {
		return color.NRGBA{242, 193, 95, 255}
	}
	return ink
}

func gradeColor(g piano.Grade) color.NRGBA {
	switch g {
	case piano.Kool:
		return accent
	case piano.Cool:
		return color.NRGBA{117, 178, 242, 255}
	case piano.Good:
		return color.NRGBA{242, 193, 95, 255}
	default:
		return color.NRGBA{245, 112, 125, 255}
	}
}

func (p *view) meterAnchor() (center, width, y float32) {
	l := p.field.layout
	return l.left + l.width/2, 240, (l.top + skinLine(p.skin)) / 2
}

func (p *view) drawMeter(dst *ebiten.Image) {
	dx, dy := p.hudOffset(meterHUD)
	x, width, top := p.meterAnchor()
	// Composite the whole meter once: even overlapping markers cannot make it
	// opaque over notes. Coordinates match the existing 280x64 drag handle.
	layer := p.timing
	layer.Clear()
	center, y := float32(140), float32(30)
	rect(layer, center-width/2, y-2, width, 4, color.NRGBA{104, 51, 65, 190})
	// O2Jam windows follow the beat, so the meter spans the current GOOD window.
	scale, bands := float32(piano.Window), [3]time.Duration{90 * time.Millisecond, 45 * time.Millisecond, 22 * time.Millisecond}
	if p.session.O2Jam() {
		w := p.session.O2JamWindows(p.state.Position)
		scale, bands = float32(w[2]), [3]time.Duration{w[2], w[1], w[0]}
	}
	for i, c := range []color.NRGBA{gradeColor(piano.Good), gradeColor(piano.Cool), gradeColor(piano.Kool)} {
		half := float32(bands[i]) / scale * width / 2
		c.A = 190
		rect(layer, center-half, y-2, half*2, 4, c)
	}
	rect(layer, center-1, y-7, 2, 14, ink)
	for _, h := range p.meter.trails {
		a := h.opacity(p.state.Position)
		if a == 0 {
			continue
		}
		x := center + max(-1, min(1, float32(h.delta)/scale))*width/2
		c := timingColor(h.delta)
		c.A = uint8(255 * a)
		rect(layer, x-1, y-9, 2, 18, c)
	}
	p.hud.Left(layer, "EARLY", small, center-width/2, y-14, timingColor(-time.Millisecond))
	p.hud.Right(layer, "LATE", small, center+width/2, y-14, timingColor(time.Millisecond))
	if !p.focus {
		p.hud.Center(layer, p.hud.meterText, small, center, y+29, muted)
	}
	op := &ebiten.DrawImageOptions{}
	op.GeoM.Translate(float64(x+dx-center), float64(top+dy-y))
	op.ColorScale.ScaleAlpha(0.4)
	dst.DrawImage(layer, op)
}

func (p *view) drawPhase(dst *ebiten.Image) {
	if p.phase != Paused {
		p.pause.Close()
	}
	var title, detail string
	switch p.phase {
	case Ready:
		title = "READY"
		detail = fmt.Sprintf("Starts in %.1f s", (p.begins - p.state.Position).Seconds())
		if p.state.Online != "" {
			title = "MULTIPLAYER READY"
			detail = ui.Fit(p.state.Online, p.hud.Faces[small], int(p.field.layout.width)-20)
		}
	case Paused:
		if p.state.Peek {
			return
		}
		progress := 0
		if p.duration > 0 {
			progress = int(100 * min(1, max(0, float64(p.state.Position)/float64(p.duration))))
		}
		info := fmt.Sprintf("Retry %d  ·  Progress %d%%  ·  Accuracy %s", p.state.Retries, progress, p.hud.accuracyText)
		ui.DrawPause(dst, p.hud.Text, &p.pause, p.state.Countdown, p.state.PauseChoice, info)
		return
	case Outro:
		title = "OUTRO"
		detail = "ESC to results"
	case Failed:
		title, detail = "FAILED", "Health depleted"
		if p.state.Aborted {
			detail = "Target out of reach"
		}
	case Finished:
		title = "COMPLETE"
		detail = "Returning to results"
	default:
		return
	}
	l := p.field.layout
	rect(dst, l.left, 286, l.width, 100, color.NRGBA{12, 18, 29, 238})
	p.hud.Center(dst, title, label, Width/2, 327, ink)
	p.hud.Center(dst, detail, small, Width/2, 358, muted)
}
