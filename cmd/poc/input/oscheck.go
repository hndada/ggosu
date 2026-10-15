package main

import (
	"errors"
	"fmt"
	"time"

	"ggosu/internal/keyboard"
)

var checkKeys = [...]uint16{'D', 'D', 'F', 'F', 'J', 'J', 'K', 'K'}

func (g *game) checkOSInput() {
	if g.osFinished {
		return
	}
	if g.osSent == len(checkKeys) && g.osSeen == len(checkKeys) {
		g.osFinished, g.osPassed = true, true
		return
	}
	if g.clock.Now()-g.started > 12*time.Second {
		g.osFinished = true
		g.err = fmt.Errorf("OS input check incomplete: sent %d, raw matched %d/8", g.osSent, g.osSeen)
		return
	}
	if g.osSent == len(checkKeys) || !g.focused || g.clock.Now() < g.osNext {
		return
	}
	if err := sendPair(checkKeys[g.osSent]); err != nil {
		if errors.Is(err, errForeground) {
			return // IsFocused can be stale before the first presented frame.
		}
		g.osFinished = true
		g.err = err
		return
	}
	g.osSent += 2
	g.osNext = g.clock.Now() + 100*time.Millisecond
}

func (g *game) observeOSInput(e keyboard.Event) {
	if !g.opts.oscheck || e.Synthetic || g.osSeen == len(checkKeys) {
		return
	}
	if e.Key == checkKeys[g.osSeen] && e.Down == (g.osSeen%2 == 0) {
		g.osSeen++
	}
}
