package play

import (
	"encoding/binary"
	"fmt"
	"os"
	"testing"
	"time"

	"ggosu/internal/chart"
)

func TestOJMDecode(t *testing.T) {
	data, err := os.ReadFile("../../songs/demo/demo.ojm")
	if err != nil {
		t.Fatal(err)
	}
	samples, err := chart.ReadBank("demo.ojm", data)
	if err != nil {
		t.Fatal(err)
	}
	if len(samples) != 2 {
		t.Fatal("missing bank samples")
	}
	for id, s := range samples {
		pcm, err := decode(s.Data, s.Extension, 16<<20)
		if err != nil {
			t.Fatalf("sample %d: %v", id, err)
		}
		if len(pcm) == 0 || len(pcm)%4 != 0 {
			t.Fatal("invalid stereo PCM")
		}
	}
}

func TestSparseAndPartialBank(t *testing.T) {
	// A complete, silent PCM sample followed by one damaged Ogg header.
	b := make([]byte, 20+56+8+1)
	copy(b, "OJM\x00")
	binary.LittleEndian.PutUint32(b[8:], 20)
	binary.LittleEndian.PutUint32(b[12:], 84)
	binary.LittleEndian.PutUint32(b[16:], uint32(len(b)))
	h := b[20:76]
	binary.LittleEndian.PutUint16(h[32:], 1)
	binary.LittleEndian.PutUint16(h[34:], 1)
	binary.LittleEndian.PutUint32(h[36:], sampleRate)
	binary.LittleEndian.PutUint32(h[40:], sampleRate*2)
	binary.LittleEndian.PutUint16(h[44:], 2)
	binary.LittleEndian.PutUint16(h[46:], 16)
	binary.LittleEndian.PutUint32(h[52:], 8)
	c := chart.New(7)
	c.Assets["complete"] = chart.Asset{Kind: "bank", Path: "partial.ojm", Sample: 1}
	c.Assets["empty"] = chart.Asset{Kind: "bank", Path: "partial.ojm", Sample: 1000}
	reads := 0
	bank, err := loadBankAssets(c, func(chart.Asset) ([]byte, error) { reads++; return b, nil })
	if err != nil || len(bank.pcm["complete"]) == 0 || len(bank.warnings) != 1 || reads != 1 {
		t.Fatalf("partial bank: reads %d, error %v, bank %+v", reads, err, bank)
	}
	b = b[:84]
	binary.LittleEndian.PutUint32(b[16:], 84)
	bank, err = loadBankAssets(c, func(chart.Asset) ([]byte, error) { return b, nil })
	if err != nil || len(bank.warnings) != 0 || len(bank.pcm["complete"]) == 0 || len(bank.pcm["empty"]) != 0 {
		t.Fatalf("sparse bank: error %v, bank %+v", err, bank)
	}
}

func TestWebBufferSuppliesWorklet(t *testing.T) {
	const refillFrames = 2048
	bufferedFrames := int(webBuffer * time.Duration(sampleRate) / time.Second)
	if bufferedFrames < 2*refillFrames {
		t.Fatalf("web buffer has %d frames; need room for two %d-frame worklet refills", bufferedFrames, refillFrames)
	}
	if nativeBuffer != 20*time.Millisecond {
		t.Fatal("native low-latency profile changed")
	}
}
func TestDefaultSound(t *testing.T) {
	pcm := fallback("soft-hitnormal3.wav")
	if len(pcm) == 0 {
		t.Fatal("missing default hit sound")
	}
	var peak int16
	for i := 0; i < len(pcm); i += 2 {
		v := int16(binary.LittleEndian.Uint16(pcm[i:]))
		if v > peak {
			peak = v
		}
	}
	if peak == 0 {
		t.Fatal("silent fallback")
	}
	if fallback("custom.wav") != nil {
		t.Fatal("invented custom sample")
	}
	// The browser reports a missing file wrapped with its path.
	c := chart.New(4)
	c.Assets["whistle"] = chart.Asset{Kind: "audio", Path: "drum-hitwhistle.wav"}
	bank, err := loadBankAssets(c, func(a chart.Asset) ([]byte, error) { return nil, fmt.Errorf("%s: %w", a.Path, os.ErrNotExist) })
	if err != nil || len(bank.pcm["whistle"]) == 0 || len(bank.warnings) != 0 {
		t.Fatalf("wrapped missing default sample: error %v, warnings %v", err, bank.warnings)
	}
}

func TestPlaybackEffects(t *testing.T) {
	for _, kind := range []string{"start", "resume", "finish", "mention"} {
		pcm, err := cuePCM(kind)
		if err != nil || len(pcm) != sampleRate/12*4 {
			t.Fatalf("invalid effect %s", kind)
		}
	}
	if _, err := cuePCM("unknown"); err == nil {
		t.Fatal("accepted unknown effect")
	}
	var b bank
	if err := b.Effect("start", 0); err != nil || len(b.voices) != 0 {
		t.Fatal("mute created a player")
	}
}
