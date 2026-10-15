package play

import (
	"bytes"
	"encoding/binary"
	"math"
	"testing"
)

func TestStretchPitchAndDuration(t *testing.T) {
	pcm := make([]byte, sampleRate*4)
	for i := 0; i < sampleRate; i++ {
		v := int16(math.Sin(2*math.Pi*220*float64(i)/sampleRate) * 12000)
		binary.LittleEndian.PutUint16(pcm[i*4:], uint16(v))
		binary.LittleEndian.PutUint16(pcm[i*4+2:], uint16(-v))
	}
	for _, rate := range []float64{.5, .75, 1, 1.5, 2} {
		out, err := stretchPCM(pcm, rate)
		if err != nil {
			t.Fatal(err)
		}
		frames := len(out) / 4
		if frames != int(math.Round(sampleRate/rate)) {
			t.Fatalf("rate %g: %d frames", rate, frames)
		}
		from, to, crossings := frames/10, frames*9/10, 0
		for i := from + 1; i < to; i++ {
			if sample(out, i-1, 0) <= 0 && sample(out, i, 0) > 0 {
				crossings++
			}
			if math.Abs(sample(out, i, 0)+sample(out, i, 1)) > 1 {
				t.Fatalf("rate %g: stereo phase changed", rate)
			}
		}
		freq := float64(crossings) * sampleRate / float64(to-from)
		if math.Abs(freq-220) > 6 {
			t.Fatalf("rate %g: pitch %gHz, want 220Hz", rate, freq)
		}
	}
	unchanged, err := stretchPCM(pcm, 1)
	if err != nil || !bytes.Equal(pcm, unchanged) {
		t.Fatal("1x PCM changed")
	}
}

func TestStretchSilenceShortAndInvalid(t *testing.T) {
	for _, frames := range []int{0, 1, 8, 240, 960} {
		for _, rate := range []float64{.5, 1, 2} {
			out, err := stretchPCM(make([]byte, frames*4), rate)
			if err != nil {
				t.Fatal(err)
			}
			for _, b := range out {
				if b != 0 {
					t.Fatal("silence changed")
				}
			}
		}
	}
	for _, rate := range []float64{0, .25, .49, 2.01, 4, math.NaN(), math.Inf(1)} {
		if _, err := stretchPCM(make([]byte, 4), rate); err == nil {
			t.Fatalf("accepted rate %g", rate)
		}
	}
	if _, err := stretchPCM([]byte{1}, 2); err == nil {
		t.Fatal("accepted incomplete stereo frame")
	}
}

func TestPitchModePreparesAndInvalidatesAudio(t *testing.T) {
	pcm := make([]byte, sampleRate*4)
	b := &bank{rate: 1, pcm: map[string][]byte{"music": pcm, "alias": pcm}}
	if err := b.SetPitch(false); err != nil {
		t.Fatal(err)
	}
	if err := b.SetRate(2); err != nil {
		t.Fatal(err)
	}
	key := stretchKey{&pcm[0], len(pcm)}
	if len(b.stretched[key]) != len(pcm)/2 || len(b.stretched) != 1 {
		t.Fatal("shared audio not prepared once")
	}
	if err := b.SetRate(.5); err != nil {
		t.Fatal(err)
	}
	if len(b.stretched[key]) != len(pcm)*2 {
		t.Fatal("old rate cache retained")
	}
	if err := b.SetPitch(true); err != nil {
		t.Fatal(err)
	}
	if b.preserve || len(b.stretched) != 0 {
		t.Fatal("pitch on retained stretched audio")
	}
}

func TestRateBounds(t *testing.T) {
	b := &bank{rate: 1}
	for _, rate := range []float64{.5, 1, 2} {
		if err := b.SetRate(rate); err != nil {
			t.Fatalf("rate %g: %v", rate, err)
		}
	}
	for _, rate := range []float64{.25, .49, 2.01, 4, math.NaN(), math.Inf(1)} {
		if err := b.SetRate(rate); err == nil || b.rate != 2 {
			t.Fatalf("invalid rate %g changed playback: rate=%g, err=%v", rate, b.rate, err)
		}
	}
}
