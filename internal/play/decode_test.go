package play

import (
	"bytes"
	"encoding/binary"
	"errors"
	"ggosu/internal/chart"
	"io"
	"testing"

	"github.com/hajimehoshi/ebiten/v2/audio/wav"
)

func testWAV(rate, frames int) []byte {
	b := make([]byte, 44+frames*4)
	copy(b, "RIFF")
	binary.LittleEndian.PutUint32(b[4:], uint32(len(b)-8))
	copy(b[8:], "WAVEfmt ")
	binary.LittleEndian.PutUint32(b[16:], 16)
	binary.LittleEndian.PutUint16(b[20:], 1)
	binary.LittleEndian.PutUint16(b[22:], 2)
	binary.LittleEndian.PutUint32(b[24:], uint32(rate))
	binary.LittleEndian.PutUint32(b[28:], uint32(rate*4))
	binary.LittleEndian.PutUint16(b[32:], 4)
	binary.LittleEndian.PutUint16(b[34:], 16)
	copy(b[36:], "data")
	binary.LittleEndian.PutUint32(b[40:], uint32(frames*4))
	for i := 44; i < len(b); i += 2 {
		binary.LittleEndian.PutUint16(b[i:], uint16(i*73))
	}
	return b
}

func TestWAVDecodePreservesPCM(t *testing.T) {
	for _, rate := range []int{8000, 22050, 44100, 48000, 96000} {
		data := testWAV(rate, rate/10)
		s, err := wav.DecodeWithSampleRate(sampleRate, bytes.NewReader(data))
		if err != nil {
			t.Fatal(err)
		}
		want, err := io.ReadAll(s)
		if err != nil {
			t.Fatal(err)
		}
		got, err := decode(data, ".wav", len(want))
		if err != nil || !bytes.Equal(got, want) {
			t.Fatalf("rate %d: PCM changed: %v", rate, err)
		}
		if _, err := decode(data, ".wav", len(want)-1); err == nil {
			t.Fatalf("rate %d: accepted undersized memory limit", rate)
		}
	}
}

// Measure decoding only, without opening or playing a song.
func BenchmarkKeySoundDecode(b *testing.B) {
	data := testWAV(44100, 44100/5)
	b.Run("Stream", func(b *testing.B) {
		b.ReportAllocs()
		for range b.N {
			s, err := wav.DecodeWithSampleRate(sampleRate, bytes.NewReader(data))
			if err != nil {
				b.Fatal(err)
			}
			if _, err := io.ReadAll(s); err != nil {
				b.Fatal(err)
			}
		}
	})
	b.Run("CachedFilter", func(b *testing.B) {
		b.ReportAllocs()
		for range b.N {
			if _, err := decode(data, ".wav", maxPCM); err != nil {
				b.Fatal(err)
			}
		}
	})
}

func TestDecodeBatchBudget(t *testing.T) {
	jobs := []decodeJob{{data: testWAV(sampleRate, 400), ext: ".wav"}, {data: testWAV(sampleRate, 10), ext: ".wav"}}
	decodeBatch(jobs, 1640)
	if !errors.Is(jobs[0].err, errPCM) || jobs[1].err != nil || len(jobs[1].pcm) != 40 {
		t.Fatal("batch exceeded its per-worker budget or lost the small sample")
	}
	pcm, err := decode(jobs[0].data, jobs[0].ext, 1640-len(jobs[1].pcm))
	if err != nil || len(pcm) != 1600 {
		t.Fatal("large sample could not use remaining budget", err)
	}
}

func TestParallelBankSharesAliasedAssets(t *testing.T) {
	c := chart.New(4)
	for _, id := range []string{"a", "b", "c", "d", "e", "f"} {
		c.Assets[id] = chart.Asset{Kind: "audio", Path: "same.wav"}
	}
	reads := 0
	b, err := loadBankAssets(c, func(chart.Asset) ([]byte, error) { reads++; return testWAV(44100, 4410), nil })
	if err != nil || reads != 1 || len(b.warnings) != 0 {
		t.Fatal("aliased asset decoded more than once", reads, err)
	}
	for _, pcm := range b.pcm {
		if len(pcm) != 4800*4 || &pcm[0] != &b.pcm["a"][0] {
			t.Fatal("aliases did not share PCM")
		}
	}
}
