package chart_test

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"
	"time"

	"ggosu/internal/chart"
	_ "ggosu/internal/chart/ojn"
)

// writeWAV writes one second of stereo 16-bit silence declared at rate, with a
// chunk before data.
func writeWAV(t *testing.T, path string, rate uint32) {
	t.Helper()
	wav := []byte("RIFF\x00\x00\x00\x00WAVEfmt \x10\x00\x00\x00")
	wav = binary.LittleEndian.AppendUint16(wav, 1)
	wav = binary.LittleEndian.AppendUint16(wav, 2)
	wav = binary.LittleEndian.AppendUint32(wav, rate)
	wav = binary.LittleEndian.AppendUint32(wav, rate*4)
	wav = binary.LittleEndian.AppendUint16(wav, 4)
	wav = binary.LittleEndian.AppendUint16(wav, 16)
	wav = append(wav, "LIST\x03\x00\x00\x00abc\x00data"...)
	wav = binary.LittleEndian.AppendUint32(wav, 44100*4)
	wav = append(wav, make([]byte, 44100*4)...)
	if err := os.WriteFile(path, wav, 0o644); err != nil {
		t.Fatal(err)
	}
}

func lengthChart(dir string) *chart.Chart {
	return &chart.Chart{
		Source: chart.Source{Path: filepath.Join(dir, "song.osu")},
		Assets: map[string]chart.Asset{"bgm": {Path: "bgm.wav", Kind: "audio"}, "gone": {Path: "gone.wav", Kind: "audio"}},
		Audio: []chart.Audio{
			{At: 2 * time.Second, Sound: chart.Sound{Asset: "bgm", Volume: 1}},
			{At: 2500 * time.Millisecond, Sound: chart.Sound{Asset: "gone", Volume: 1}},
		},
	}
}

func TestDurationIncludesKeysoundHeadsNotTails(t *testing.T) {
	dir := t.TempDir()
	writeWAV(t, filepath.Join(dir, "bgm.wav"), 44100)
	c := lengthChart(dir)
	c.Audio = c.Audio[:1]
	c.Notes = []chart.Note{
		{At: 4 * time.Second, End: 8 * time.Second, Kind: "hold", Sounds: []chart.Sound{{Asset: "bgm", Volume: 1}}, TailSounds: []chart.Sound{{Asset: "bgm", Volume: 1}}},
		{At: 20 * time.Second, Kind: "fake", Sounds: []chart.Sound{{Asset: "bgm", Volume: 1}}},
		{At: 30 * time.Second, Kind: "mine", Sounds: []chart.Sound{{Asset: "bgm", Volume: 1}}},
		{At: 40 * time.Second, Kind: "tap", Sounds: []chart.Sound{{Asset: "gone", Volume: 0}}},
	}
	end, err := chart.NewAudioLengths().Duration(c)
	if err != nil || end != 5*time.Second {
		t.Fatalf("head end: %v, %v", end, err)
	}
}

func TestEditedSkinHitsoundsDoNotAffectAudioLength(t *testing.T) {
	dir := t.TempDir()
	writeWAV(t, filepath.Join(dir, "bgm.wav"), 44100)
	c := lengthChart(dir)
	c.Audio = c.Audio[:1]
	c.Notes = []chart.Note{{At: 10 * time.Second, End: 10 * time.Second, Kind: "tap", Sounds: []chart.Sound{{Asset: "gone", Volume: 1}}}}
	for _, format := range []string{"osu", "sm"} {
		c.Source.Format, c.Source.OriginalFormat = "ggc", format
		end, err := chart.NewAudioLengths().Duration(c)
		if err != nil || end != 3*time.Second {
			t.Fatalf("%s copy counted missing skin hitsounds: %v, %v", format, end, err)
		}
	}
}

func TestDurationReportsMissingAndInvalidAudio(t *testing.T) {
	dir := t.TempDir()
	writeWAV(t, filepath.Join(dir, "bgm.wav"), 44100)
	c := lengthChart(dir)
	if _, err := chart.NewAudioLengths().Duration(c); err == nil {
		t.Fatal("missing audio treated as a trusted duration")
	}
	c.Audio = c.Audio[:1]
	if err := os.WriteFile(filepath.Join(dir, "bgm.wav"), []byte("broken"), 0644); err != nil {
		t.Fatal(err)
	}
	cache := filepath.Join(dir, "lengths.json")
	l := chart.NewAudioLengths()
	if _, err := l.Duration(c); err == nil {
		t.Fatal("invalid audio accepted")
	}
	if err := l.Save(cache); err != nil {
		t.Fatal(err)
	}
	l, err := chart.LoadAudioLengths(cache)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := l.Duration(c); err == nil {
		t.Fatal("cached invalid audio accepted")
	}
}

func TestDurationMeasuresBankKeysoundsAndSparseSlots(t *testing.T) {
	dir := t.TempDir()
	// One second of PCM in an OJM bank, with no BGM event.
	wav, end := 20, 20+56+8
	b := make([]byte, end)
	copy(b, "OJM\x00")
	binary.LittleEndian.PutUint32(b[8:], uint32(wav))
	binary.LittleEndian.PutUint32(b[12:], uint32(end))
	binary.LittleEndian.PutUint32(b[16:], uint32(end))
	h := b[wav:]
	binary.LittleEndian.PutUint16(h[32:], 1)
	binary.LittleEndian.PutUint16(h[34:], 1)
	binary.LittleEndian.PutUint32(h[36:], 4)
	binary.LittleEndian.PutUint32(h[40:], 8)
	binary.LittleEndian.PutUint16(h[44:], 2)
	binary.LittleEndian.PutUint16(h[46:], 16)
	binary.LittleEndian.PutUint32(h[52:], 8)
	if err := os.WriteFile(filepath.Join(dir, "song.ojm"), b, 0644); err != nil {
		t.Fatal(err)
	}
	c := chart.New(7)
	c.Source.Path = filepath.Join(dir, "song.ojn")
	c.Source.Format = "ojn"
	c.Assets = map[string]chart.Asset{
		"key":    {Path: "song.ojm", Kind: "bank", Sample: 1},
		"silent": {Path: "song.ojm", Kind: "bank", Sample: 2},
	}
	c.Notes = []chart.Note{
		{At: 5 * time.Second, End: 8 * time.Second, Kind: "hold", Sounds: []chart.Sound{{Asset: "key", Volume: 1}}, TailSounds: []chart.Sound{{Asset: "key", Volume: 1}}},
		{At: 30 * time.Second, Kind: "tap", Sounds: []chart.Sound{{Asset: "silent", Volume: 1}}},
	}
	endTime, err := chart.NewAudioLengths().Duration(c)
	if err != nil || endTime != 6*time.Second {
		t.Fatalf("bank end: %v, %v", endTime, err)
	}
}

func TestAudioDurationReadsWAVHeader(t *testing.T) {
	dir := t.TempDir()
	writeWAV(t, filepath.Join(dir, "bgm.wav"), 44100)
	c := lengthChart(dir)
	if end := c.AudioDuration(nil); end != 3*time.Second {
		t.Fatalf("audio duration %v, want 3s", end)
	}
	c.Audio = c.Audio[1:]
	if end := c.AudioDuration(nil); end != 2500*time.Millisecond {
		t.Fatalf("missing asset end %v, want its start", end)
	}
}

func TestAudioLengthsCacheBySizeAndTime(t *testing.T) {
	dir := t.TempDir()
	bgm, cache := filepath.Join(dir, "bgm.wav"), filepath.Join(dir, "cache", "lengths.json")
	writeWAV(t, bgm, 44100)
	stamp := time.Date(2026, 10, 10, 0, 0, 0, 0, time.UTC)
	if err := os.Chtimes(bgm, stamp, stamp); err != nil {
		t.Fatal(err)
	}
	lengths, err := chart.LoadAudioLengths(cache)
	if err != nil {
		t.Fatal(err)
	}
	if end := lengthChart(dir).AudioDuration(lengths); end != 3*time.Second {
		t.Fatalf("audio duration %v, want 3s", end)
	}
	if err := lengths.Save(cache); err != nil {
		t.Fatal(err)
	}

	// Same size and time: the cached length wins over the new header.
	writeWAV(t, bgm, 22050)
	if err := os.Chtimes(bgm, stamp, stamp); err != nil {
		t.Fatal(err)
	}
	lengths, err = chart.LoadAudioLengths(cache)
	if err != nil {
		t.Fatal(err)
	}
	if end := lengthChart(dir).AudioDuration(lengths); end != 3*time.Second {
		t.Fatalf("cached audio duration %v, want 3s", end)
	}

	// A new time measures the file again.
	if err := os.Chtimes(bgm, stamp, stamp.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	lengths, err = chart.LoadAudioLengths(cache)
	if err != nil {
		t.Fatal(err)
	}
	if end := lengthChart(dir).AudioDuration(lengths); end != 4*time.Second {
		t.Fatalf("changed audio duration %v, want 4s", end)
	}

	if err := os.WriteFile(cache, []byte("{"), 0o644); err != nil {
		t.Fatal(err)
	}
	if lengths, err := chart.LoadAudioLengths(cache); err == nil || lengths == nil {
		t.Fatal("broken cache accepted or not replaced by an empty one")
	}
}
