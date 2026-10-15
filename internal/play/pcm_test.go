//go:build !js

package play

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
	"time"

	"ggosu/internal/chart"
)

func TestPCMCacheSharesDifficultiesAndRejectsChangedOrDamagedData(t *testing.T) {
	dir := t.TempDir()
	wav := filepath.Join(dir, "sound.wav")
	if err := os.WriteFile(wav, testWAV(44100, 4410), 0600); err != nil {
		t.Fatal(err)
	}
	c := chart.New(4)
	c.Source.Path = filepath.Join(dir, "chart.ggc")
	c.Assets = map[string]chart.Asset{"one": {Kind: "audio", Path: "sound.wav"}, "alias": {Kind: "audio", Path: "sound.wav"}}
	cache := filepath.Join(dir, "cache")
	b, err := loadCachedBank(c, []*chart.Chart{c}, cache)
	if err != nil || len(b.warnings) != 0 || len(b.pcm["one"]) == 0 {
		t.Fatal(b, err)
	}
	other := *c
	other.Assets = map[string]chart.Asset{"different-id": c.Assets["one"]}
	all := soundChart([]*chart.Chart{c, &other})
	path, err := pcmPath(all, cache)
	if err != nil {
		t.Fatal(err)
	}
	got, err := readPCM(path, &other)
	if err != nil || !bytes.Equal(got["different-id"], b.pcm["one"]) {
		t.Fatal("sibling PCM changed", err)
	}
	if &b.pcm["one"][0] != &b.pcm["alias"][0] {
		t.Fatal("aliases copied PCM")
	}
	f, err := os.OpenFile(path, os.O_RDWR, 0600)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteAt([]byte{0xff}, 12+68); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := readPCM(path, &other); err == nil {
		t.Fatal("damaged PCM accepted")
	}
	rebuilt, err := loadCachedBank(c, []*chart.Chart{c}, cache)
	if err != nil || !bytes.Equal(rebuilt.pcm["one"], b.pcm["one"]) {
		t.Fatal("cache fallback changed sound", err)
	}
	if _, err := readPCM(path, c); err != nil {
		t.Fatal("cache was not repaired", err)
	}
	now := time.Now().Add(time.Second)
	if err := os.Chtimes(wav, now, now); err != nil {
		t.Fatal(err)
	}
	changed, err := pcmPath(all, cache)
	if err != nil || changed == path {
		t.Fatal("changed source retained cache key", err)
	}
}

func TestPCMCacheQuotaAndIncompleteFallback(t *testing.T) {
	dir := t.TempDir()
	c := chart.New(4)
	c.Assets = map[string]chart.Asset{"one": {Kind: "audio", Path: "one.wav"}}
	all := soundChart([]*chart.Chart{c})
	path := filepath.Join(dir, "keep.pcm")
	if err := writePCM(path, all, map[string][]byte{soundKey(c.Assets["one"]): {1, 2, 3, 4}}); err != nil {
		t.Fatal(err)
	}
	c.Assets["two"] = chart.Asset{Kind: "audio", Path: "two.wav"}
	if _, err := readPCM(path, c); err == nil {
		t.Fatal("incomplete bank accepted")
	}
	old := filepath.Join(dir, "old.pcm")
	if err := os.WriteFile(old, make([]byte, 100), 0600); err != nil {
		t.Fatal(err)
	}
	if err := trimPCM(dir, path, 100); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(old); !os.IsNotExist(err) {
		t.Fatal("old bank was not evicted", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatal("current bank evicted", err)
	}
}

func BenchmarkCachedBank(b *testing.B) {
	path := os.Getenv("GGOSU_BENCH_CHART")
	if path == "" {
		b.Skip("set GGOSU_BENCH_CHART")
	}
	charts, err := chart.Load(path)
	if err != nil {
		b.Fatal(err)
	}
	dir := b.TempDir()
	if err := Warm(path, dir); err != nil {
		b.Fatal(err)
	}
	b.ResetTimer()
	b.ReportAllocs()
	for range b.N {
		bank, err := loadCachedBank(charts[0], charts, dir)
		if err != nil || len(bank.warnings) != 0 {
			b.Fatal(err)
		}
		bank.Close()
	}
}
