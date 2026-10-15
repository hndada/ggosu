package main

import (
	"fmt"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"

	"ggosu/internal/clock"
	"ggosu/internal/keyboard"
	"ggosu/internal/piano"
)

type game struct {
	opts             options
	box              *inbox
	source           *keyboard.Source
	stop             chan struct{}
	clock            *clock.Counter
	started          time.Duration
	lastMetrics      time.Time
	lastStall        time.Time
	batch            []keyboard.Event
	held             [256]bool
	syntheticHeld    [256]bool
	events           [12]keyboard.Event
	lastSeq          uint64
	handled          uint64
	rawHandled       uint64
	synthHandled     uint64
	orderErrors      uint64
	sequenceGaps     uint64
	repeats          uint64
	standardDown     uint64
	standardUp       uint64
	rawLaneDown      uint64
	rawLaneUp        uint64
	queueDelay       latency
	view             report
	loadDone         chan loadResult
	loadResult       *loadResult
	baseReceived     uint64
	baseHandled      uint64
	testStarted      bool
	testFinished     bool
	testPassed       bool
	finalDrawn       bool
	osSent           int
	osSeen           int
	osNext           time.Duration
	osFinished       bool
	osPassed         bool
	captureRequested bool
	focused          bool
	err              error
	timeline         *clock.Timeline
	speed            float64
	paused           bool
	gameHeld         [256]bool
	recording        piano.Replay
	player           *piano.Player
	due              []piano.Event
	replayed         uint64
	sampled          time.Duration
}

// receive is invoked by the native thread, NOT by Update or Draw. It hands the
// event directly to this game object's inbox; it cannot invoke Ebitengine input
// callbacks because the public engine API has no event-injection entry point.
func (g *game) receive(e keyboard.Event) { g.box.push(e) }

var lanes = [...]struct {
	key  ebiten.Key
	code uint16
	name string
}{
	{ebiten.KeyD, 'D', "D"}, {ebiten.KeyF, 'F', "F"},
	{ebiten.KeyJ, 'J', "J"}, {ebiten.KeyK, 'K', "K"},
}

func (g *game) Update() error {
	if g.finalDrawn {
		return ebiten.Termination
	}
	select {
	case err := <-g.source.Errors():
		return err
	default:
	}
	// Consume preserved edges before any other game work. All state mutations
	// below belong to Ebitengine's goroutine.
	g.batch = g.box.drain(g.batch[:0])
	for _, e := range g.batch {
		g.queueDelay.add(g.clock.Now() - e.At)
		if err := g.playInput(e); err != nil {
			return err
		}
		g.apply(e)
	}
	if err := g.advanceReplay(g.clock.Now()); err != nil {
		return err
	}
	focused := ebiten.IsFocused()
	g.source.SetActive(focused)
	if !focused && g.focused {
		g.held = [256]bool{}
		if g.player == nil {
			g.releaseKeys(g.timeline.Position(g.clock.Now()))
		}
	}
	g.focused = focused
	if g.opts.oscheck {
		g.checkOSInput()
	}
	for _, lane := range lanes {
		if inpututil.IsKeyJustPressed(lane.key) {
			g.standardDown++
		}
		if inpututil.IsKeyJustReleased(lane.key) {
			g.standardUp++
		}
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyEscape) {
		return ebiten.Termination
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyF6) {
		g.captureRequested = true
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyF1) {
		g.opts.vsync = !g.opts.vsync
		ebiten.SetVsyncEnabled(g.opts.vsync)
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyF2) {
		switch g.opts.tps {
		case 60:
			g.opts.tps = 240
		case 240:
			g.opts.tps = 1000
		case 1000:
			g.opts.tps = ebiten.SyncWithFPS
		default:
			g.opts.tps = 60
		}
		ebiten.SetTPS(g.opts.tps)
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyF3) {
		if g.opts.stall == 0 {
			g.opts.stall = 100 * time.Millisecond
		} else {
			g.opts.stall = 0
		}
	}
	if (g.opts.selftest && !g.testStarted) || (inpututil.IsKeyJustPressed(ebiten.KeyF5) && (g.loadDone == nil || g.testFinished)) {
		g.startLoad()
	}
	if g.loadDone != nil && g.loadResult == nil {
		select {
		case r := <-g.loadDone:
			g.loadResult = &r
		default:
		}
	}
	if g.loadResult != nil && !g.testFinished {
		r := g.box.report()
		expected := g.loadResult.Sent
		if r.Synthetic-g.baseReceived >= expected && g.synthHandled-g.baseHandled >= expected {
			g.testFinished = true
			g.testPassed = g.loadResult.Error == "" && r.Dropped == 0 && g.orderErrors == 0 && g.sequenceGaps == 0
		} else if time.Since(g.loadResult.finished) > 3*time.Second || g.loadResult.Error != "" {
			g.testFinished = true
			g.testPassed = false
			g.err = fmt.Errorf("synthetic drain incomplete: posted %d, received %d, handled %d", expected,
				r.Synthetic-g.baseReceived, g.synthHandled-g.baseHandled)
		}
	}
	if time.Since(g.lastMetrics) >= 500*time.Millisecond || (g.opts.selftest && g.testFinished) || g.osFinished {
		g.view = g.report()
		g.lastMetrics = time.Now()
	}
	return nil
}

func (g *game) apply(e keyboard.Event) {
	g.observeOSInput(e)
	if e.Seq <= g.lastSeq {
		g.orderErrors++
	} else if e.Seq != g.lastSeq+1 {
		g.sequenceGaps += e.Seq - g.lastSeq - 1
	}
	g.lastSeq = e.Seq
	g.handled++
	held := &g.held
	if e.Synthetic {
		g.synthHandled++
		held = &g.syntheticHeld
	} else {
		g.rawHandled++
	}
	if int(e.Key) < len(held) {
		if !e.Synthetic && held[e.Key] != e.Down {
			for _, lane := range lanes {
				if e.Key == lane.code {
					if e.Down {
						g.rawLaneDown++
					} else {
						g.rawLaneUp++
					}
					break
				}
			}
		}
		if e.Down && held[e.Key] {
			g.repeats++
		}
		held[e.Key] = e.Down
	}
	copy(g.events[1:], g.events[:len(g.events)-1])
	g.events[0] = e
}
