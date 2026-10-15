package play

import (
	"encoding/binary"
	"testing"
	"time"

	"ggosu/internal/chart"
)

func TestClipPCMCutsAndFades(t *testing.T) {
	pcm := make([]byte, sampleRate*4) // One second of full-scale stereo.
	for i := 0; i < len(pcm); i += 2 {
		binary.LittleEndian.PutUint16(pcm[i:], uint16(int16(10000)))
	}
	out := clipPCM(pcm, chart.Sound{From: 500 * time.Millisecond, Length: 400 * time.Millisecond, FadeIn: 100 * time.Millisecond, FadeOut: 100 * time.Millisecond})
	if len(out) != sampleRate*4*4/10 {
		t.Fatal("length", len(out))
	}
	sample := func(frame int) int16 { return int16(binary.LittleEndian.Uint16(out[frame*4:])) }
	if sample(0) != 0 || sample(sampleRate/5) != 10000 || sample(len(out)/4-1) > 100 {
		t.Fatal("fades", sample(0), sample(sampleRate/5), sample(len(out)/4-1))
	}
	if clipPCM(pcm, chart.Sound{From: 2 * time.Second}) != nil {
		t.Fatal("a clip past the end must be silent")
	}
}
