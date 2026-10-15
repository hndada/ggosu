package play

import (
	"fmt"
	"strings"
	"time"

	"github.com/hajimehoshi/ebiten/v2"

	"ggosu/internal/piano"
	"ggosu/internal/settings"
)

type binding struct {
	name string
	code uint16
	key  ebiten.Key
}

func bindings(lanes int, custom string) ([]binding, error) {
	names, err := settings.DefaultKeys(lanes)
	if err != nil {
		return nil, err
	}
	if custom != "" {
		names = strings.Split(strings.ToUpper(custom), ",")
	}
	for i := range names {
		names[i] = strings.TrimSpace(names[i])
	}
	if err := (settings.Profile{Keys: names}).Validate(lanes); err != nil {
		return nil, err
	}
	var out []binding
	for _, name := range names {
		name = strings.ToUpper(strings.TrimSpace(name))
		b := binding{name: name}
		code, err := settings.KeyCode(name)
		if err != nil {
			return nil, err
		}
		b.code = code
		switch {
		case name == "SPACE":
			b.code = 32
			b.key = ebiten.KeySpace
		case name == "SEMICOLON":
			b.code = 186
			b.key = ebiten.KeySemicolon
		case len(name) == 1 && name[0] >= 'A' && name[0] <= 'Z':
			b.key = ebiten.KeyA + ebiten.Key(name[0]-'A')
		case len(name) == 1 && name[0] >= '0' && name[0] <= '9':
			b.key = ebiten.Key0 + ebiten.Key(name[0]-'0')
		default:
			return nil, fmt.Errorf("unsupported lane key %q", name)
		}
		out = append(out, b)
	}
	return out, nil
}

func (g *scene) updateInput(now time.Duration, focused bool) error {
	// Drain the final countdown frame while judgement is still paused. A
	// release followed by a re-hold in this batch must not break a long note.
	if err := g.readInput(now, focused); err != nil {
		return err
	}
	if err := g.updateResume(now, focused); err != nil {
		return err
	}
	// Pause freezes logical holds. Apply releases made while paused only when
	// play resumes; keys re-held during the countdown retain their long notes.
	if !g.paused && focused && g.player == nil {
		for k, held := range g.session.Held {
			if held && !g.physical[k] {
				if err := g.input(g.position, k, false); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func (g *scene) readInput(now time.Duration, focused bool) error {
	if g.source != nil {
		g.source.SetActive(focused && !g.chatTyping())
		select {
		case err := <-g.source.Errors():
			if err != nil {
				return err
			}
		default:
		}
	}
	if g.eventInput() {
		var lost bool
		g.events, lost = g.box.Drain(g.events)
		if lost {
			return fmt.Errorf("native input queue overflow; score and replay invalid")
		}
		for _, e := range g.events {
			for lane, b := range g.bindings {
				if b.code != e.Key {
					continue
				}
				if g.physical[lane] == e.Down {
					continue
				}
				g.physical[lane] = e.Down
				if !focused || g.paused || g.chatTyping() || e.At < g.inputSince || e.Down && b.key == ebiten.KeyR && g.held(ebiten.KeyControl) {
					continue
				}
				at := g.networkPosition(max(g.last, g.timeline.Position(e.At)-g.preroll))
				if err := g.input(at, lane, e.Down); err != nil {
					return err
				}
				g.last = at
			}
		}
	}
	g.position = g.networkPosition(max(g.last, g.timeline.Position(now)-g.preroll))
	if g.replayIncomplete {
		g.position = min(g.position, g.replayEnd)
	}
	if g.player == nil && !g.eventInput() {
		for lane, b := range g.bindings {
			down := ebiten.IsKeyPressed(b.key)
			if down != g.physical[lane] {
				g.physical[lane] = down
				if g.paused || !focused || g.chatTyping() || down && b.key == ebiten.KeyR && g.held(ebiten.KeyControl) {
					continue
				}
				if err := g.input(g.position, lane, down); err != nil {
					return err
				}
			}
		}
	}
	if !focused && g.focused && !g.paused && g.player == nil {
		if err := g.release(g.position); err != nil {
			return err
		}
	}
	g.focused = focused
	return nil
}

func (g *scene) input(at time.Duration, lane int, down bool) error {
	if g.session.Health.Failed {
		return nil
	}
	if g.session.Held[lane] == down {
		return nil
	}
	if err := g.session.Input(at, lane, down); err != nil {
		return err
	}
	if g.session.Health.Failed && at > g.session.Health.At {
		return nil
	}
	if down && g.view != nil {
		g.view.Press(lane, at)
	}
	if down && g.bank != nil {
		if err := g.bank.Cue(g.audio.HitKind(), g.audio.Hit); err != nil {
			return err
		}
	}
	if g.record != "" || g.result != "" || g.networkEnabled() {
		g.recording.Events = append(g.recording.Events, piano.Event{At: at, Key: uint16(lane), Down: down})
	}
	return nil
}
func (g *scene) release(at time.Duration) error {
	for lane, held := range g.session.Held {
		if held {
			if err := g.input(at, lane, false); err != nil {
				return err
			}
		}
	}
	return nil
}
