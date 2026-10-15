//go:build !js

package play

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"runtime"
	"time"

	"github.com/hajimehoshi/ebiten/v2"

	"ggosu/internal/launch"
)

type host struct {
	scene    *scene
	id       uint64
	ready    bool
	booted   bool
	err      error
	requests <-chan request
	inputErr <-chan error
	output   *json.Encoder
	parse    func([]string) (Config, error)
}

type request struct {
	launch.Request
	accepted chan struct{}
}

// GLFW can consume a wake event in the preceding frame while switching to
// idle. Retry only while a request is outstanding, stopping as soon as Update
// accepts it. With no requests the hidden game still performs no polling.
func wakeGame(accepted, stop <-chan struct{}) bool {
	tick := time.NewTicker(10 * time.Millisecond)
	defer tick.Stop()
	ebiten.ScheduleFrame()
	for {
		select {
		case <-accepted:
			return true
		case <-stop:
			return false
		case <-tick.C:
			ebiten.ScheduleFrame()
		}
	}
}

// Serve runs one Ebitengine loop for the lifetime of the menu's input pipe.
// Scene termination returns to an invisible idle window, not to the OS.
// parse belongs to cmd/game, so standalone and resident options stay identical.
func Serve(input io.ReadCloser, output io.Writer, parse func([]string) (Config, error)) (err error) {
	requests := make(chan request, 1)
	inputErr := make(chan error, 1)
	stop := make(chan struct{})
	done := make(chan struct{})
	go func() {
		var err error
		defer close(done)
		defer func() {
			inputErr <- err
			close(requests)
			wakeGame(nil, stop) // EOF must also wake a game that just became idle.
		}()
		s := bufio.NewScanner(input)
		s.Buffer(make([]byte, 4096), 1<<20)
		for s.Scan() {
			var r launch.Request
			if e := json.Unmarshal(s.Bytes(), &r); e != nil {
				err = fmt.Errorf("game request: %w", e)
				return // A damaged private pipe must not leave an orphan game.
			}
			q := request{Request: r, accepted: make(chan struct{})}
			select {
			case requests <- q:
				if !wakeGame(q.accepted, stop) {
					return
				}
			case <-stop:
				return
			}
		}
		if e := s.Err(); e != nil {
			err = fmt.Errorf("game pipe: %w", e)
		}
	}()
	h := &host{requests: requests, inputErr: inputErr, output: json.NewEncoder(output), parse: parse}
	defer func() {
		close(stop)
		err = errors.Join(err, input.Close())
		<-done
		if h.scene != nil {
			err = errors.Join(err, h.scene.close())
		}
	}()
	if err := windowIcon(); err != nil {
		return err
	}
	ebiten.SetWindowSize(1600, 900)
	ebiten.SetWindowTitle("gosu")
	ebiten.SetWindowResizingMode(ebiten.WindowResizingModeEnabled)
	ebiten.SetRunnableOnUnfocused(true)
	ebiten.SetWindowClosingHandled(true)
	h.hide()
	// Bootstrap one frame before entering demand-driven idle rendering.
	ebiten.SetFPSMode(ebiten.FPSModeVsyncOn)
	// On Windows GLFW treats a successful SetFocus with no previous focused
	// window as an error. Showing without that call allows repeated hide/show.
	err = ebiten.RunGameWithOptions(h, &ebiten.RunGameOptions{InitUnfocused: runtime.GOOS == "windows"})
	zzDump()
	return err
}

func (h *host) emit(kind string, err error) error {
	e := launch.Event{ID: h.id, Type: kind}
	if err != nil {
		e.Error = err.Error()
	}
	return h.output.Encode(e)
}

func (h *host) hide() {
	ebiten.SetWindowVisible(false)
	ebiten.SetFullscreen(false)
	ebiten.SetTPS(30)
	ebiten.SetFPSMode(ebiten.FPSModeVsyncOffMinimum)
}

func (h *host) finish(err error) error {
	g := h.scene
	h.scene = nil
	if err == nil {
		err = errors.Join(g.save(), g.saveNetwork())
	}
	err = errors.Join(err, g.close())
	h.hide()
	return h.emit("done", err)
}

func (h *host) Update() error {
	if h.err != nil {
		return h.err
	}
	select {
	case r, ok := <-h.requests:
		if !ok {
			if err := <-h.inputErr; err != nil {
				return err
			}
			return ebiten.Termination
		}
		close(r.accepted)
		if h.scene != nil {
			return fmt.Errorf("play request while a song is active")
		}
		h.id, h.ready = r.ID, false
		c, err := h.parse(r.Args)
		if err != nil {
			return h.emit("done", err)
		}
		// stdout carries protocol events; scene readiness is reported after Draw.
		c.Ready = false
		g, err := newScene(c)
		if err != nil {
			return h.emit("done", err)
		}
		h.scene = g
		restoreWindow()
		g.window()
		ebiten.SetWindowVisible(true)
		return nil // Show the prepared frame before starting its timeline.
	default:
	}
	if h.scene == nil {
		return nil
	}
	if ebiten.IsWindowBeingClosed() {
		return h.finish(nil)
	}
	if err := h.scene.Update(); err != nil {
		if errors.Is(err, ebiten.Termination) {
			err = nil
		}
		return h.finish(err)
	}
	return nil
}

// Restore before changing the mode or size: a maximized window ignores sizing,
// and fullscreen transitions can change its minimized state asynchronously.
func restoreWindow() {
	if !ebiten.IsWindowMaximized() && !ebiten.IsWindowMinimized() {
		return
	}
	defer func() {
		// Ebitengine checks the state again inside RestoreWindow. The OS can
		// restore it between those checks; in that case no work remains.
		if r := recover(); r != nil && r != "ebiten: RestoreWindow must be called on a maximized or a minimized window" {
			panic(r)
		}
	}()
	ebiten.RestoreWindow()
}

func (h *host) Draw(dst *ebiten.Image) {
	if h.scene == nil {
		dst.Clear()
		if !h.booted {
			h.booted = true
			h.hide()
			ebiten.ScheduleFrame()
			if err := h.emit("idle", nil); err != nil {
				// Wake Update so a broken output pipe terminates the process.
				h.err = err
				ebiten.ScheduleFrame()
			}
		}
		return
	}
	h.scene.Draw(dst)
	if !h.ready {
		h.ready = true
		launch.Focus()
		if err := h.emit("ready", nil); err != nil {
			h.scene.err = err
		}
	}
}

func (h *host) Layout(w, hgt int) (int, int) {
	if h.scene != nil {
		return h.scene.Layout(w, hgt)
	}
	return 1, 1
}
