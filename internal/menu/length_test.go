package menu

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"
	"time"

	"ggosu/internal/chart"
)

func TestSelectedAudioDurationCachesAndInvalidates(t *testing.T) {
	dir := t.TempDir()
	wav := make([]byte, 44+8)
	copy(wav, "RIFF")
	binary.LittleEndian.PutUint32(wav[4:], uint32(len(wav)-8))
	copy(wav[8:], "WAVEfmt ")
	binary.LittleEndian.PutUint32(wav[16:], 16)
	binary.LittleEndian.PutUint16(wav[20:], 1)
	binary.LittleEndian.PutUint16(wav[22:], 1)
	binary.LittleEndian.PutUint32(wav[24:], 4)
	binary.LittleEndian.PutUint32(wav[28:], 8)
	binary.LittleEndian.PutUint16(wav[32:], 2)
	binary.LittleEndian.PutUint16(wav[34:], 16)
	copy(wav[36:], "data")
	binary.LittleEndian.PutUint32(wav[40:], 8)
	path := filepath.Join(dir, "key.wav")
	if err := os.WriteFile(path, wav, 0644); err != nil {
		t.Fatal(err)
	}
	c := chart.New(7)
	c.Metadata.Title = "Measured keysound"
	c.Timing = []chart.Timing{{BPM: 120, Scroll: 1, Meter: 4}}
	c.Assets["key"] = chart.Asset{Path: "key.wav", Kind: "audio"}
	c.Notes = []chart.Note{{At: 5 * time.Second, End: 5 * time.Second, Kind: "tap", Sounds: []chart.Sound{{Asset: "key", Volume: 1}}}}
	f, err := os.Create(filepath.Join(dir, "song.ggc"))
	if err != nil {
		t.Fatal(err)
	}
	if err := chart.Write(f, c); err != nil {
		f.Close()
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	a := &App{dataDir: filepath.Join(t.TempDir(), "records")}
	cache, err := a.lengthsPath()
	if err != nil {
		t.Fatal(err)
	}
	catalog, err := a.songs.Scan(cache, []string{dir})
	if err != nil {
		t.Fatalf("catalog: %+v, %v", catalog, err)
	}
	var e chart.Entry
	for _, entry := range catalog.Entries {
		if entry.Folder == dir {
			e = entry
			break
		}
	}
	if e.Path == "" {
		t.Fatal("test chart missing from catalog")
	}
	s := Selection{Folder: e.Folder, ChartID: e.Path, Index: e.Index}
	for i := 0; i < 2; i++ {
		// A fresh app must reuse the disk cache too.
		if i == 1 {
			a = &App{dataDir: a.dataDir}
			if _, err := a.songs.Scan(cache, []string{dir}); err != nil {
				t.Fatal(err)
			}
		}
		end, err := a.AudioDuration(s)
		if err != nil || end != 6*time.Second {
			t.Fatalf("measurement %d: %v, %v", i, end, err)
		}
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	// Same file size, new sample rate and modification time: duration doubles.
	binary.LittleEndian.PutUint32(wav[24:], 2)
	binary.LittleEndian.PutUint32(wav[28:], 4)
	if err := os.WriteFile(path, wav, 0644); err != nil {
		t.Fatal(err)
	}
	stamp := info.ModTime().Add(time.Second)
	if err := os.Chtimes(path, stamp, stamp); err != nil {
		t.Fatal(err)
	}
	if end, err := a.AudioDuration(s); err != nil || end != 7*time.Second {
		t.Fatalf("changed audio: %v, %v", end, err)
	}
	if _, err := a.AudioDuration(Selection{ChartID: "outside.ojn"}); err == nil {
		t.Fatal("unscanned chart accepted")
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if _, err := a.AudioDuration(s); err == nil {
		t.Fatal("missing audio accepted from cache")
	}
}
