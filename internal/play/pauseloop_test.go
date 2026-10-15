package play

import (
	"encoding/binary"
	"math"
	"testing"
	"time"

	"ggosu/internal/settings"
)

func TestPauseLoopPCM(t *testing.T) {
	pcm, err := pauseLoopPCM()
	if err != nil {
		t.Fatal(err)
	}
	sample := func(i int) float64 { return float64(int16(binary.LittleEndian.Uint16(pcm[i*4:]))) / 32768 }
	frames := len(pcm) / 4
	if frames < 30*sampleRate {
		t.Fatalf("pause loop is %d frames, want the whole 33.65 s Heavenly Loop", frames)
	}
	sum := 0.0
	for i := range frames {
		sum += sample(i) * sample(i)
	}
	if db := 10 * math.Log10(sum/float64(frames)); db < -35 || db > -29 {
		t.Fatalf("pause loop level %.1f dB", db)
	}
	// The wrap must not click: the last frame continues into the first.
	if d := math.Abs(sample(frames-1) - sample(0)); d > .01 {
		t.Fatalf("pause loop wrap jump %.4f", d)
	}
}

func TestPauseLoopFollowsMenu(t *testing.T) {
	var b bank
	if err := b.PauseLoop(0, true, pauseLoopFade, easeInQuint); err != nil {
		t.Fatal(err)
	}
	if b.loop.player == nil {
		t.Fatal("pause loop did not start")
	}
	b.UpdatePauseLoop(pauseLoopFade / 2)
	if v := b.loop.volume; v <= 0 || v >= .1 {
		t.Fatalf("InQuint midpoint volume %.3f", v)
	}
	b.UpdatePauseLoop(pauseLoopFade)
	if err := b.PauseLoop(pauseLoopFade, false, pauseLoopFade, easeOutQuad); err != nil {
		t.Fatal(err)
	}
	b.UpdatePauseLoop(2 * pauseLoopFade)
	if b.loop.player != nil {
		t.Fatal("pause loop kept playing after fading out")
	}
	b.audio = &settings.Audio{Master: 100, Effects: 0}
	if err := b.PauseLoop(time.Second, true, pauseLoopFade, easeInQuint); err != nil || b.loop.player != nil {
		t.Fatal("muted effects started the pause loop")
	}
}
