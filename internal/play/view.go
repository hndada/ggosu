package play

import (
	"cmp"
	"fmt"
	"image/color"
	"strings"
	"time"

	"github.com/hajimehoshi/ebiten/v2"

	"ggosu/internal/chart"
	"ggosu/internal/piano"
	"ggosu/internal/settings"
	"ggosu/internal/storyboard"
	"ggosu/internal/ui"
)

type Phase uint8

const (
	Ready Phase = iota
	Playing
	Paused
	Outro
	Finished
	Failed
)

type viewState struct {
	Position         time.Duration
	Rate, Scroll     float64
	Paused, Finished bool
	Aborted          bool // The ghost target went out of reach; shown like a failure.
	Countdown        int  // Remaining real seconds before resuming; game time is paused.
	PauseChoice      int  // Selected pause button, an index into ui.PauseButtons.
	Peek             bool // Tab is held while paused: the pause overlay hides to show the field.
	Retries          int  // Quick restarts of this chart so far.
	Online           string
	Rivals           []rival
	Chat             *chat
}

type rival struct {
	Name       string
	At         time.Duration
	Points     int
	Difference int
	Available  bool
	Status     string
}

type viewConfig struct {
	Visibility string
	Keys       []string
	Mode       string
	Offset     time.Duration
	Judge      time.Duration // Judgement-only offset; the field draws this far ahead to keep notes in place.
	Font       []byte        // Optional local font; nil uses the bundled Go font.
	Profile    *settings.Profile
	Display    *settings.Display
	Online     bool
	Debug      bool // FLOW/PREC and per-note strain appear only here.
}

type view struct {
	visibilityShader                 *ebiten.Shader
	cover                            coverMotion
	lifted                           float64 // LIFT the hit line was last placed with.
	speedSudden                      float64 // SUDDEN+ when exposure mode last set the speed.
	laneLines                        bool
	ghost                            bool
	ghostScore                       int
	ghostMax                         bool // Compare the best still reachable score instead of the pace.
	layout                           settings.Layout
	editing                          bool
	session                          *piano.Session
	field                            field
	background                       storyboard.Background
	hud                              hud
	pause                            ui.PauseMenu
	health                           healthMotion
	frames                           frameMeter
	config                           viewConfig
	state                            viewState
	phase                            Phase
	begins, duration                 time.Duration
	results                          int
	meter                            meter
	flashes                          []time.Duration
	tails                            []tailFade
	lit, hits                        []time.Duration // Last moment each lane was pressed / last note hit there.
	held                             []bool          // Key state seen by the last Update.
	released                         []time.Duration // When each lane's key last came up.
	sprites                          argonSprites
	judgement                        piano.Result
	hasJudgement                     bool
	timingShown                      piano.Result // Last judgement with Early/Late, held for timingHold.
	hasTiming                        bool
	base, notes, timing, hudLayer    *ebiten.Image
	chatSource                       []chatLine
	chatLines                        [][]string
	focus                            bool
	hideHUD                          bool // Temporary visibility; saved HUD settings stay separate.
	comboPosition, judgementPosition string
	timingThreshold                  string
	timeDisplay                      string
	skin                             string
	noteThickness                    int
	laneDim                          int
	noteColour                       string
	snap                             []color.NRGBA // Per-note beat colours when noteColour is snap.
	judgementShow                    string
	hideCombo                        bool
	strain                           noteStrain // Debug only.
}

func newView(source *chart.Chart, s *piano.Session, images map[string]*ebiten.Image, c viewConfig) (*view, error) {
	if source == nil || s == nil || s.Chart == nil || source.Lanes != s.Chart.Lanes || len(c.Keys) != source.Lanes {
		return nil, fmt.Errorf("scene: chart, session and lane keys must agree")
	}
	if err := s.Chart.Validate(); err != nil {
		return nil, err
	}
	if err := chart.ValidateVisibility(c.Visibility); err != nil {
		return nil, err
	}
	c.Mode = strings.ToUpper(c.Mode)
	visual := (settings.Profile{SpeedChange: 1}).Visual(s.Chart)
	if c.Profile != nil {
		if err := c.Profile.Validate(s.Chart.Lanes); err != nil {
			return nil, err
		}
		visual = c.Profile.Visual(s.Chart)
	}
	if err := visual.Validate(); err != nil {
		return nil, err
	}
	display := (settings.Config{}).DisplaySettings()
	if c.Display != nil {
		display = *c.Display
	}
	if err := display.Validate(); err != nil {
		return nil, err
	}
	p := &view{session: s, field: newField(visual, s.Chart, display.Skin), skin: cmp.Or(display.Skin, "argon"), background: storyboard.NewBackground(source, images, display), config: c, flashes: make([]time.Duration, source.Lanes), focus: display.HUD == "focus"}
	p.ghost, p.ghostMax, p.ghostScore = display.Ghost != "off", display.Ghost == "max", display.TargetScore()
	p.laneLines = display.LaneLines
	p.layout = display.Layout
	p.noteThickness = display.NoteThickness
	p.laneDim, p.judgementShow, p.hideCombo = display.LaneDim, display.JudgementShow, display.HideCombo
	p.setNoteColour(display.NoteColour)
	p.noteMargin()
	if p.layout.HitPosition != 0 {
		p.field.layout.line = float32(p.layout.HitPosition)
	}
	p.field.layout.line = p.liftLine(p.field.layout.line)
	p.speedSudden = 1 - p.visibleShare()
	p.comboPosition, p.judgementPosition = display.ComboPosition, display.JudgementPosition
	p.timingThreshold = cmp.Or(display.TimingThreshold, "cool")
	p.timeDisplay = cmp.Or(display.TimeDisplay, "pie")
	p.health = newHealthMotion(s.Health.Value)
	p.health.level = s.Health.Level
	if p.comboPosition == "" {
		p.comboPosition = "center"
	}
	if p.judgementPosition == "" {
		p.judgementPosition = "center"
	}
	p.duration = max(source.Duration(), s.Chart.Duration())
	for _, n := range s.Chart.Notes {
		p.begins = min(p.begins, n.At)
	}
	for _, a := range source.Audio {
		p.begins = min(p.begins, a.At)
	}
	p.lit, p.hits = make([]time.Duration, source.Lanes), make([]time.Duration, source.Lanes)
	p.tails = make([]tailFade, source.Lanes)
	p.held, p.released = make([]bool, source.Lanes), make([]time.Duration, source.Lanes)
	for i := range p.flashes {
		p.tails[i].at = -chart.MaxTime
		p.flashes[i], p.lit[i], p.hits[i], p.released[i] = -chart.MaxTime, -chart.MaxTime, -chart.MaxTime, -chart.MaxTime
	}
	var err error
	p.hud, err = newHUD(source, c.Font)
	if err != nil {
		return nil, err
	}
	p.base = ebiten.NewImage(Width, Height)
	p.drawBase(p.base)
	w, h := p.field.layout.surfaceSize()
	p.notes = ebiten.NewImage(w, h)
	// The lane cover can switch on during play, so the mask is always ready.
	p.visibilityShader, err = ebiten.NewShader([]byte(visibilityShader))
	if err != nil {
		return nil, fmt.Errorf("visibility shader: %w", err)
	}
	p.timing = ebiten.NewImage(280, 64)
	return p, nil
}

func (p *view) Phase() Phase { return p.phase }

func (p *view) Focus() bool { return p.focus }

func (p *view) SetFocus(focus bool) {
	if p.focus == focus {
		return
	}
	p.focus = focus
	p.base.Clear()
	p.drawBase(p.base)
}

func phase(at, begins time.Duration, paused, finished, done bool) Phase {
	switch {
	case paused:
		return Paused
	case finished:
		return Finished
	case at < begins:
		return Ready
	case done:
		return Outro
	default:
		return Playing
	}
}

// Update borrows already-judged state. It cannot create hits or play sounds.
func (p *view) Update(s viewState) {
	if p.base != nil && p.lift() != p.lifted {
		p.applyLayout(p.layout) // LIFT changed in play or the HUD editor opened.
	}
	if p.config.Profile != nil {
		p.cover.update(p.config.Profile.LaneCover(), s.Position, s.Rate)
	}
	if p.config.Profile != nil && p.config.Profile.ScrollMode == "exposure" {
		// beatoraja sets the speed so the part below SUDDEN+ takes the exposure time.
		s.Scroll = p.config.Profile.PixelsPerSecond(s.Rate, float64(p.field.layout.line-p.field.layout.top)*max(1-p.speedSudden, .01))
	}
	p.state = s
	if p.config.Debug {
		p.strain.update(p.session.Chart, s.Rate)
	}
	if p.health.level != p.session.Health.Level {
		// A fallback drains from a full bar to its already-calculated fill, as damage.
		from := p.session.Health.Value
		if p.session.Health.Level < p.health.level {
			from = 10000
		}
		p.health = newHealthMotion(from)
		p.health.level, p.health.at = p.session.Health.Level, s.Position
		p.health.fallback = from > p.session.Health.Value
	}
	p.health.update(p.session.Health.Value, s.Position)
	p.phase = phase(s.Position, p.begins, s.Paused, s.Finished, p.session.Done() && s.Position >= p.duration)
	if p.session.Health.Failed || s.Aborted {
		p.phase = Failed
	}
	p.field.update(s.Position+p.config.Judge, s.Scroll)
	for k := range p.lit {
		if p.session.Held[k] && !p.held[k] && p.flashes[k] < 0 {
			p.flashes[k] = s.Position
		}
		if p.session.Held[k] || s.Position-p.flashes[k] < 100*time.Millisecond {
			p.lit[k] = s.Position
		}
		if p.held[k] && !p.session.Held[k] {
			p.released[k] = s.Position
		}
		p.held[k] = p.session.Held[k]
	}
	for _, r := range p.session.Results[p.results:] {
		n := p.session.Chart.Notes[r.Note]
		if p.session.Health.Level > 0 && (r.Grade == piano.Kool || r.Grade == piano.Cool) && n.Kind != "mine" {
			p.health.gain = r.At
		}
		p.meter.add(r, n)
		if n.Kind == "mine" && r.Grade != piano.Miss {
			continue
		}
		if r.Grade != piano.Miss {
			p.hits[n.Lane] = r.At
			if r.Tail {
				p.tails[n.Lane] = tailFade{at: r.At}
			}
		}
		// Simultaneous chords show their worst judgement independent of lane order.
		if !p.hasJudgement || r.At > p.judgement.At || r.At == p.judgement.At && gradeRank(r.Grade) > gradeRank(p.judgement.Grade) {
			p.judgement = r
			p.hasJudgement = true
		}
	}
	if p.hasJudgement && judgementShown(p.judgement.Grade, p.judgementShow) && timingText(p.judgement, p.timingThreshold) != "" {
		p.timingShown, p.hasTiming = p.judgement, true
	}
	p.results = len(p.session.Results)
	p.hud.update(p)
}

func gradeRank(g piano.Grade) int {
	switch g {
	case piano.Kool:
		return 0
	case piano.Cool:
		return 1
	case piano.Good:
		return 2
	default:
		return 3
	}
}

func (p *view) Draw(dst *ebiten.Image) {
	p.background.Draw(dst, p.state.Position)
	op := &ebiten.DrawImageOptions{}
	op.ColorScale.ScaleAlpha(1 - float32(p.laneDim)/100)
	dst.DrawImage(p.base, op)
	p.drawField(dst)
	if !p.hideHUD || p.editing {
		p.drawHUD(dst)
	}
	if !p.editing {
		p.drawChat(dst)
		p.drawPhase(dst)
	}
	p.drawFrames(dst)
}
