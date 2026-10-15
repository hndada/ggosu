package screenshot

import (
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestSaveSameSecond(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "screenshots")
	now := time.Date(2026, 10, 15, 0, 10, 17, 858000000, time.Local)
	colours := []color.NRGBA{{R: 255, A: 255}, {G: 255, A: 255}, {B: 255, A: 255}}
	names := []string{"20261015-001017.png", "20261015-001017-2.png", "20261015-001017-3.png"}
	for i, c := range colours {
		img := image.NewNRGBA(image.Rect(0, 0, 1, 1))
		img.SetNRGBA(0, 0, c)
		path, err := save(dir, img, now)
		if err != nil {
			t.Fatal(err)
		}
		if filepath.Base(path) != names[i] {
			t.Fatalf("filename = %q, want %q", filepath.Base(path), names[i])
		}
	}
	// Every capture must still contain its own pixels after the later saves.
	for i, name := range names {
		f, err := os.Open(filepath.Join(dir, name))
		if err != nil {
			t.Fatal(err)
		}
		img, err := png.Decode(f)
		closeErr := f.Close()
		if err != nil {
			t.Fatal(err)
		}
		if closeErr != nil {
			t.Fatal(closeErr)
		}
		if got := color.NRGBAModel.Convert(img.At(0, 0)); got != colours[i] {
			t.Errorf("%s pixel = %v, want %v", name, got, colours[i])
		}
	}
}
