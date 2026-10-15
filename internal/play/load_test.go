package play

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"ggosu/internal/chart"
	"github.com/hajimehoshi/ebiten/v2/audio/vorbis"
)

// Optional real-file regression: every stored referenced Ogg must retain the
// exact PCM bytes produced by the original Ebitengine decode path.
func TestRealBankPCM(t *testing.T) {
	paths := os.Getenv("GGOSU_BENCH_CHART")
	if paths == "" {
		t.Skip("set GGOSU_BENCH_CHART to OJN paths separated by semicolons")
	}
	for _, path := range strings.Split(paths, ";") {
		t.Run(filepath.Base(path), func(t *testing.T) {
			cs, err := chart.Load(path)
			if err != nil {
				t.Fatal(err)
			}
			used := map[int]bool{}
			var bankPath string
			for _, c := range cs {
				for _, a := range c.Assets {
					if a.Kind == "bank" {
						used[a.Sample] = true
						bankPath = filepath.Join(filepath.Dir(path), a.Path)
					}
				}
			}
			data, err := readLimited(bankPath)
			if err != nil {
				t.Fatal(err)
			}
			samples, err := chart.ReadBank(bankPath, data)
			if err != nil {
				t.Fatal(err)
			}
			var ids []int
			for id := range used {
				ids = append(ids, id)
			}
			slices.Sort(ids)
			count, bytesChecked := 0, 0
			for _, id := range ids {
				s := samples[id]
				if s.Extension != ".ogg" || len(s.Data) == 0 {
					continue
				}
				r, err := vorbis.DecodeWithSampleRate(sampleRate, bytes.NewReader(s.Data))
				if err != nil {
					t.Fatal(err)
				}
				want, err := io.ReadAll(r)
				if err != nil {
					t.Fatal(err)
				}
				got, err := decode(s.Data, s.Extension, maxPCM)
				if err != nil {
					t.Fatal(err)
				}
				if !bytes.Equal(got, want) {
					t.Fatalf("sample %d: PCM differs (bytes %d/%d)", id, len(got), len(want))
				}
				count++
				bytesChecked += len(got)
			}
			t.Logf("%d samples, %d PCM bytes identical", count, bytesChecked)
		})
	}
}

// Set GGOSU_BENCH_CHART to a real OJN to measure its audio without playing it.
func BenchmarkBank(b *testing.B) {
	path := os.Getenv("GGOSU_BENCH_CHART")
	if path == "" {
		b.Skip("set GGOSU_BENCH_CHART to an OJN file")
	}
	cs, err := chart.Load(path)
	if err != nil {
		b.Fatal(err)
	}
	c := cs[0]
	var bankPath string
	for _, a := range c.Assets {
		if a.Kind == "bank" {
			bankPath = filepath.Join(filepath.Dir(path), a.Path)
			break
		}
	}
	data, err := readLimited(bankPath)
	if err != nil {
		b.Fatal(err)
	}
	samples, err := chart.ReadBank(bankPath, data)
	if err != nil {
		b.Fatal(err)
	}
	b.Run("Read", func(b *testing.B) {
		for range b.N {
			if _, err := readLimited(bankPath); err != nil {
				b.Fatal(err)
			}
		}
	})
	b.Run("Unpack", func(b *testing.B) {
		for range b.N {
			if _, err := chart.ReadBank(bankPath, data); err != nil {
				b.Fatal(err)
			}
		}
	})
	b.Run("Vorbis", func(b *testing.B) {
		for range b.N {
			for _, a := range c.Assets {
				s := samples[a.Sample]
				if len(s.Data) == 0 || s.Extension != ".ogg" {
					continue
				}
				r, err := vorbis.DecodeWithoutResampling(bytes.NewReader(s.Data))
				if err != nil {
					b.Fatal(err)
				}
				if _, err := io.Copy(io.Discard, r); err != nil {
					b.Fatal(err)
				}
			}
		}
	})
	b.Run("Load", func(b *testing.B) {
		b.ReportAllocs()
		for range b.N {
			bank, err := loadBank(c)
			if err != nil {
				b.Fatal(err)
			}
			if len(bank.warnings) != 0 {
				b.Fatal(bank.warnings)
			}
			bank.Close()
		}
	})
}
