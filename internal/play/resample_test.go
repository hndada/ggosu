package play

import (
	"bytes"
	"encoding/binary"
	"io"
	"math/rand/v2"
	"testing"

	"github.com/hajimehoshi/ebiten/v2/audio"
)

func TestResampleMatchesEbitengine(t *testing.T) {
	for _, rate := range []int{8000, 11025, 22050, 32000, 44100, 48000, 96000} {
		for _, frames := range []int{0, 1, 7, 16, 4097, 48001} {
			pcm := make([]byte, frames*4)
			r := rand.New(rand.NewPCG(1, 2))
			for i := 0; i < len(pcm); i += 2 {
				binary.LittleEndian.PutUint16(pcm[i:], uint16(r.Uint32()))
			}
			want, err := io.ReadAll(audio.ResampleReader(bytes.NewReader(pcm), int64(len(pcm)), rate, sampleRate))
			if err != nil {
				t.Fatal(err)
			}
			got, err := resamplePCM(pcm, rate, sampleRate, maxPCM)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(got, want) {
				t.Fatalf("rate %d, frames %d: PCM differs (bytes %d/%d)", rate, frames, len(got), len(want))
			}
		}
	}
}

func TestResampleLimits(t *testing.T) {
	for _, tc := range []struct{ from, to, bytes, limit int }{
		{0, 48000, 4, 100}, {44100, 0, 4, 100}, {44100, 48000, 3, 100},
		{44100, 48000, 100, 100}, {48000, 48000, 100, 99}, {48000, 48000, 4, -1},
	} {
		if _, err := resamplePCM(make([]byte, tc.bytes), tc.from, tc.to, tc.limit); err == nil {
			t.Fatalf("accepted %+v", tc)
		}
	}
}
