package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"ggosu/internal/clock"
	"ggosu/internal/keyboard"
	"ggosu/internal/piano"
)

func (g *game) initPlayback() error {
	g.timeline = clock.NewTimeline(g.started)
	g.speed = g.opts.playback
	if g.speed <= 0 {
		return fmt.Errorf("playback speed must be positive")
	}
	if err := g.timeline.SetRate(g.started, g.speed); err != nil {
		return err
	}
	g.recording = piano.NewReplay()
	if g.opts.replay == "" {
		return nil
	}
	f, err := os.Open(g.opts.replay)
	if err != nil {
		return fmt.Errorf("open replay: %w", err)
	}
	r, err := piano.ReadReplay(f)
	closeErr := f.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return fmt.Errorf("close replay: %w", closeErr)
	}
	for _, e := range r.Events {
		if !laneKey(e.Key) {
			return fmt.Errorf("input POC replay contains unsupported lane key 0x%X", e.Key)
		}
	}
	g.player, err = piano.NewPlayer(r)
	return err
}

func laneKey(key uint16) bool {
	for _, lane := range lanes {
		if key == lane.code {
			return true
		}
	}
	return false
}

// Process native events in receiver order. P and F7 changes use their received
// timestamps too, so a stalled frame cannot apply the new speed retroactively
// to earlier queued lane events. Controls and synthetic load are not recorded.
func (g *game) playInput(e keyboard.Event) error {
	if e.Synthetic {
		return nil
	}
	if e.Down && int(e.Key) < len(g.held) && !g.held[e.Key] {
		at := e.At
		if g.player != nil {
			// A delayed local control cannot rewind replay already consumed by
			// a previous Update. Apply it no earlier than that Update's sample.
			at = max(at, g.sampled)
		}
		switch e.Key {
		case 'P':
			g.paused = !g.paused
			rate := g.speed
			if g.paused {
				rate = 0
				if g.player == nil {
					g.releaseKeys(g.timeline.Position(e.At))
				}
			}
			return g.timeline.SetRate(at, rate)
		case 0x76: // Windows VK_F7.
			switch g.speed {
			case 0.5:
				g.speed = 1
			case 1:
				g.speed = 1.5
			case 1.5:
				g.speed = 2
			default:
				g.speed = 0.5
			}
			if !g.paused {
				return g.timeline.SetRate(at, g.speed)
			}
			return nil
		}
	}
	if g.player != nil || g.paused || !laneKey(e.Key) {
		return nil
	}
	// Ignore auto-repeat downs; a key held across a pause needs a fresh press.
	if e.Down && g.held[e.Key] || g.gameHeld[e.Key] == e.Down {
		return nil
	}
	g.gameHeld[e.Key] = e.Down
	// Position is already on the fixed 1x axis. Store it directly, with no
	// additional multiplication or division by the current playback rate.
	g.recordEvent(piano.Event{At: g.timeline.Position(e.At), Key: e.Key, Down: e.Down})
	return nil
}

func (g *game) advanceReplay(at time.Duration) error {
	if g.player == nil {
		return nil
	}
	var err error
	g.due, err = g.player.AppendDue(g.due[:0], g.timeline.Position(at))
	if err != nil {
		return err
	}
	g.sampled = at
	for _, e := range g.due {
		g.gameHeld[e.Key] = e.Down
		g.replayed++
	}
	return nil
}

func (g *game) recordEvent(e piano.Event) {
	if g.opts.record != "" {
		g.recording.Events = append(g.recording.Events, e)
	}
}

func (g *game) releaseKeys(at time.Duration) {
	for _, lane := range lanes {
		if g.gameHeld[lane.code] {
			g.gameHeld[lane.code] = false
			g.recordEvent(piano.Event{At: at, Key: lane.code})
		}
	}
}

func (g *game) saveRecording() (err error) {
	if g.opts.record == "" {
		return nil
	}
	if g.box.report().Dropped != 0 || g.orderErrors != 0 || g.sequenceGaps != 0 {
		return fmt.Errorf("input loss or reordering detected; recording was not saved")
	}
	g.recording.Duration = g.timeline.Position(g.clock.Now())
	g.releaseKeys(g.recording.Duration)
	if err := g.recording.Validate(); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(g.opts.record), 0755); err != nil {
		return fmt.Errorf("create recording directory: %w", err)
	}
	// Encode and close a temporary file first, so a write failure does not leave
	// a truncated replay at the requested path.
	f, err := os.CreateTemp(filepath.Dir(g.opts.record), ".replay-*.json")
	if err != nil {
		return fmt.Errorf("create recording: %w", err)
	}
	path := f.Name()
	defer func() {
		if removeErr := os.Remove(path); removeErr != nil && !os.IsNotExist(removeErr) {
			err = errors.Join(err, fmt.Errorf("remove recording temporary file: %w", removeErr))
		}
	}()
	writeErr := piano.WriteReplay(f, g.recording)
	closeErr := f.Close()
	if writeErr != nil {
		return writeErr
	}
	if closeErr != nil {
		return fmt.Errorf("close recording: %w", closeErr)
	}
	if err := os.Rename(path, g.opts.record); err != nil {
		return fmt.Errorf("commit recording: %w", err)
	}
	return nil
}
