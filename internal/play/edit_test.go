package play

import (
	"image"
	"testing"
	"time"

	"ggosu/internal/chart"
	"ggosu/internal/piano"
	"ggosu/internal/settings"
)

func editView(t *testing.T, lanes int) *view {
	t.Helper()
	c := chart.New(lanes)
	c.Timing = []chart.Timing{{BPM: 120, Scroll: 1, Meter: 4}}
	c.Notes = []chart.Note{{At: time.Second, End: 3 * time.Second, Kind: "hold"}}
	s, err := piano.NewSession(c)
	if err != nil {
		t.Fatal(err)
	}
	p, err := newView(c, s, nil, viewConfig{Keys: make([]string, lanes)})
	if err != nil {
		t.Fatal(err)
	}
	p.Update(viewState{Position: time.Second, Rate: 1, Scroll: 110, Paused: true})
	return p
}

func TestHUDDragRetainsGrabOffsetAndClamps(t *testing.T) {
	p := editView(t, 4)
	var e hudEditor
	grab := p.defaultBounds(scoreHUD).Min.Add(image.Pt(20, 40))
	e.begin(p, grab, noHUD)
	if e.item != scoreHUD {
		t.Fatalf("selected %v", e.item)
	}
	e.drag(p, grab.Add(image.Pt(-300, 100)))
	if p.layout.Score != (settings.Point{X: -300, Y: 100}) {
		t.Fatal(p.layout.Score)
	}
	e.drag(p, image.Pt(9999, -9999))
	b := p.hudBounds(scoreHUD)
	if b.Max.X != Width || b.Min.Y != 0 {
		t.Fatal(b)
	}
	// Grabbing again must start from the displayed, clamped position.
	e.begin(p, b.Min.Add(image.Pt(20, 40)), noHUD)
	e.drag(p, e.start.Sub(image.Pt(100, -100)))
	if got := p.hudBounds(scoreHUD); got.Min != b.Min.Add(image.Pt(-100, 100)) {
		t.Fatal(got)
	}
}

func TestScaledHUDDragKeepsScaleAndFitsFrame(t *testing.T) {
	p := editView(t, 7)
	p.layout.Score = settings.Point{Scale: 150}
	b, d := p.hudBounds(scoreHUD), p.defaultBounds(scoreHUD)
	if b.Dx() != d.Dx()*3/2 || b.Dy() != d.Dy()*3/2 || b.Max.X != Width {
		t.Fatal(b)
	}
	var e hudEditor
	e.begin(p, b.Min.Add(image.Pt(20, 40)), noHUD)
	e.drag(p, e.start.Add(image.Pt(-100, 80)))
	if p.layout.Score.Scale != 150 || p.hudBounds(scoreHUD).Min != b.Min.Add(image.Pt(-100, 80)) {
		t.Fatal(p.layout.Score)
	}
	e.drag(p, image.Pt(9999, 9999))
	if got := p.hudBounds(scoreHUD); got.Max != image.Pt(Width, Height) {
		t.Fatal(got)
	}
}

func TestScoreAndCountsMoveIndependently(t *testing.T) {
	for _, item := range []hudItem{scoreHUD, countsHUD} {
		t.Run(hudNames[item], func(t *testing.T) {
			p := editView(t, 7)
			other := scoreHUD
			if item == scoreHUD {
				other = countsHUD
			}
			before := p.hudBounds(other)
			q := hudPoint(&p.layout, item)
			q.Scale = 150
			b := p.hudBounds(item)
			var e hudEditor
			e.begin(p, b.Min.Add(image.Pt(20, 20)), noHUD)
			if e.item != item {
				t.Fatalf("selected %s, want %s", hudNames[e.item], hudNames[item])
			}
			// Counts sits bottom-left, so it moves up and right instead.
			move := image.Pt(-250, 220)
			if item == countsHUD {
				move = image.Pt(250, -220)
			}
			e.drag(p, e.start.Add(move))
			if got := p.hudBounds(item); got.Min != b.Min.Add(move) || q.Scale != 150 {
				t.Fatal("drag lost the position or scale", got, *q)
			}
			if p.hudBounds(other) != before || *hudPoint(&p.layout, other) != (settings.Point{}) {
				t.Fatal("editing moved or scaled the other panel")
			}
		})
	}
}

func TestHitPositionDragKeepsJudgementDataAndNoteAlignment(t *testing.T) {
	p := editView(t, 7)
	if err := p.session.Input(time.Second, 0, true); err != nil {
		t.Fatal(err)
	}
	score, results := p.session.Score, len(p.session.Results)
	hash, err := p.session.Chart.Digest()
	if err != nil {
		t.Fatal(err)
	}
	var e hudEditor
	start := image.Pt(int(p.field.layout.left+10), int(p.field.layout.line))
	e.begin(p, start, noHUD)
	if e.item != hitHUD {
		t.Fatal(e.item)
	}
	var before [hitHUD]image.Rectangle
	for item := songHUD; item < hitHUD; item++ {
		before[item] = p.hudBounds(item)
	}
	e.drag(p, start.Sub(image.Pt(300, 116)))
	if p.layout.HitPosition != 500 || p.field.layout.line != 500 {
		t.Fatal(p.layout, p.field.layout)
	}
	// Dragging the hit position moves only the hit position.
	for item := songHUD; item < hitHUD; item++ {
		if p.hudBounds(item) != before[item] {
			t.Fatal(hudNames[item], "moved with the hit position")
		}
	}
	if p.field.y(p.field.scroll.position(time.Second), 110) != 500 {
		t.Fatal("note no longer lands on the moved line")
	}
	w, h := p.field.layout.surfaceSize()
	if p.notes.Bounds().Size() != image.Pt(w, h) {
		t.Fatal("note surface was not resized")
	}
	for _, y := range []int{-9999, 9999} {
		e.drag(p, image.Pt(start.X, y))
		if p.field.layout.line < settings.MinHitPosition || p.field.layout.line > settings.MaxHitPosition {
			t.Fatal(p.field.layout)
		}
	}
	p.applyLayout(settings.Layout{})
	if p.field.layout.line != float32(Height)-argonHitTarget {
		t.Fatal("reset did not restore skin hit position")
	}
	after, err := p.session.Chart.Digest()
	if err != nil || after != hash || p.session.Score != score || len(p.session.Results) != results {
		t.Fatal("layout editing changed gameplay data")
	}
}

func TestHUDLayoutFitsEveryKeyCountAndFocus(t *testing.T) {
	for lanes := 1; lanes <= 18; lanes++ {
		p := editView(t, lanes)
		p.layout = settings.Layout{Combo: settings.Point{X: 1280, Y: -720}, Gauges: settings.Point{X: 1280, Y: 720}}
		for item := songHUD; item < hitHUD; item++ {
			if b := p.hudBounds(item); !b.In(image.Rect(0, 0, Width, Height)) {
				t.Fatalf("%dK %v: %v", lanes, item, b)
			}
		}
		p.focus = true
		if p.hudAt(image.Pt(50, 500)) != noHUD {
			t.Fatal("hidden score handle is active")
		}
	}
}

// Cancel shows the HUD from before the editor opened, presets included.
func TestHUDEditorCancelRestoresPresets(t *testing.T) {
	p := editView(t, 4)
	g := &scene{view: p, display: (settings.Config{}).DisplaySettings()}
	g.openEditor()
	g.cyclePreset(2)
	g.cyclePreset(4)
	if p.timeDisplay != "bar" || p.judgementPosition != "below" {
		t.Fatal(p.timeDisplay, p.judgementPosition)
	}
	g.cancelHUD()
	if g.editing || p.editing || p.timeDisplay != "pie" || p.judgementPosition != "center" || g.stopRequested {
		t.Fatal("cancel did not restore the HUD")
	}
}

func TestHealthBarCanBeMovedInFocusHUD(t *testing.T) {
	p := editView(t, 4)
	p.session.Health = piano.NewHealth(2, 0)
	p.SetFocus(true)
	b := p.hudBounds(healthHUD)
	var e hudEditor
	e.begin(p, b.Min.Add(image.Pt(20, 20)), noHUD)
	if e.item != healthHUD {
		t.Fatal("visible health bar cannot be grabbed", e.item)
	}
	e.drag(p, e.start.Add(image.Pt(40, 20)))
	if p.layout.Health != (settings.Point{X: 40}) {
		t.Fatal(p.layout.Health)
	}
	p.session.Health = piano.NewHealth(0, 0)
	if p.movable(healthHUD) {
		t.Fatal("hidden health bar has a drag handle")
	}
}

// A corner handle scales about the opposite corner, as lazer's selection box does.
func TestHUDCornerHandleScalesAboutOppositeCorner(t *testing.T) {
	p := editView(t, 7)
	var e hudEditor
	d := p.defaultBounds(scoreHUD)
	e.begin(p, image.Pt(d.Min.X, d.Max.Y), scoreHUD)
	if e.item != scoreHUD || e.corner != 3 {
		t.Fatalf("grabbed %v corner %d", e.item, e.corner)
	}
	e.drag(p, image.Pt(d.Min.X-d.Dx()/2, d.Max.Y+d.Dy()/2))
	if p.layout.Score != (settings.Point{X: -d.Dx() / 2, Scale: 150}) {
		t.Fatal(p.layout.Score)
	}
	if b := p.hudBounds(scoreHUD); b.Max.X != d.Max.X || b.Min.Y != d.Min.Y {
		t.Fatal(b)
	}
	e.drag(p, image.Pt(1260, 20))
	if p.layout.Score.Scale != 25 {
		t.Fatal(p.layout.Score)
	}
	// Without a selection the same point only moves the item under it.
	e.begin(p, p.hudBounds(scoreHUD).Min.Add(image.Pt(2, 2)), noHUD)
	if e.corner != 0 {
		t.Fatal(e.corner)
	}
}
