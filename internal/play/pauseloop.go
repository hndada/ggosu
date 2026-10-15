package play

import (
	"bytes"
	"time"

	"github.com/hajimehoshi/ebiten/v2/audio"
)

// osu!lazer GameplayMenuOverlay loops a pause sample while the pause menu is
// shown: 200 ms InQuint in, 200 ms OutQuad out, and a 1000 ms Out fade to
// silence while the window is inactive. The sample is isaiah658's Heavenly Loop
// (CC0), softened above 1.5 kHz and lowered to -32 dBFS (sounds/SOURCES.txt).
const (
	pauseLoopFade  = 200 * time.Millisecond
	pauseLoopFocus = 1000 * time.Millisecond
)

type pauseLoop struct {
	player       *audio.Player
	pcm          []byte
	volume, from float64
	to           float64
	start, dur   time.Duration
	ease         func(float64) float64
}

func easeInQuint(t float64) float64 { return t * t * t * t * t }
func easeOutQuad(t float64) float64 { return 1 - (1-t)*(1-t) }

func pauseLoopPCM() ([]byte, error) {
	data, err := sounds.ReadFile("sounds/pause-loop.ogg")
	if err != nil {
		return nil, err
	}
	return decode(data, ".ogg", maxPCM)
}

// PauseLoop retargets the loop volume; repeated calls with the same target are no-ops.
func (b *bank) PauseLoop(now time.Duration, on bool, dur time.Duration, ease func(float64) float64) error {
	l := &b.loop
	to := 0.0
	if on {
		to = 1
	}
	if to == l.to && (l.player != nil || !on) {
		return nil
	}
	if on && l.player == nil {
		if b.gain(false, true) == 0 {
			return nil
		}
		if l.pcm == nil {
			pcm, err := pauseLoopPCM()
			if err != nil {
				return err
			}
			l.pcm = pcm
		}
		if b.ctx == nil {
			b.ctx = audio.CurrentContext()
			if b.ctx == nil {
				b.ctx = audio.NewContext(sampleRate)
			}
		}
		p, err := b.ctx.NewPlayer(audio.NewInfiniteLoop(bytes.NewReader(l.pcm), int64(len(l.pcm))))
		if err != nil {
			return err
		}
		p.SetBufferSize(playerBuffer())
		p.SetVolume(0)
		p.Play()
		l.player, l.volume = p, 0
	}
	l.from, l.to, l.start, l.dur, l.ease = l.volume, to, now, dur, ease
	return nil
}

// UpdatePauseLoop advances the fade on the real clock and stops the player at silence.
func (b *bank) UpdatePauseLoop(now time.Duration) {
	l := &b.loop
	if l.player == nil {
		return
	}
	t := 1.0
	if l.dur > 0 {
		t = min(1, max(0, float64(now-l.start)/float64(l.dur)))
	}
	l.volume = l.from + (l.to-l.from)*l.ease(t)
	l.player.SetVolume(l.volume * b.gain(false, true))
	if t == 1 && l.to == 0 {
		l.player.PauseAndStopReading()
		l.player = nil
	}
}

func (g *scene) updatePauseLoop(now time.Duration, focused bool) error {
	if g.bank == nil {
		return nil
	}
	shown := g.paused && g.resumeAt == 0 && !g.outro()
	var err error
	switch {
	case shown != g.loopShown:
		ease := easeOutQuad
		if shown {
			ease = easeInQuint
		}
		err = g.bank.PauseLoop(now, shown && focused, pauseLoopFade, ease)
	case focused != g.loopFocused:
		err = g.bank.PauseLoop(now, shown && focused, pauseLoopFocus, easeOutQuad)
	}
	g.loopShown, g.loopFocused = shown, focused
	g.bank.UpdatePauseLoop(now)
	return err
}
