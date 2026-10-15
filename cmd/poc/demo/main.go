// Run from the repository root: go run ./cmd/poc/demo
// These original procedural assets contain no external music or artwork.
package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"math"
	"os"
	"path/filepath"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
func run() error {
	root := "songs/demo"
	if err := os.MkdirAll(root, 0755); err != nil {
		return err
	}
	for _, s := range []struct {
		name                string
		seconds, freq, gain float64
	}{{"tone.wav", 8, 220, .08}, {"tap.wav", .12, 880, .3}} {
		frames := int(s.seconds * 48000)
		data := make([]byte, 44+frames*4)
		copy(data, "RIFF")
		binary.LittleEndian.PutUint32(data[4:], uint32(len(data)-8))
		copy(data[8:], "WAVEfmt ")
		binary.LittleEndian.PutUint32(data[16:], 16)
		binary.LittleEndian.PutUint16(data[20:], 1)
		binary.LittleEndian.PutUint16(data[22:], 2)
		binary.LittleEndian.PutUint32(data[24:], 48000)
		binary.LittleEndian.PutUint32(data[28:], 192000)
		binary.LittleEndian.PutUint16(data[32:], 4)
		binary.LittleEndian.PutUint16(data[34:], 16)
		copy(data[36:], "data")
		binary.LittleEndian.PutUint32(data[40:], uint32(frames*4))
		for i := 0; i < frames; i++ {
			t := float64(i) / 48000
			gain := s.gain
			if s.name == "tap.wav" {
				gain *= math.Exp(-35 * t)
			}
			sample := int16(math.Sin(2*math.Pi*s.freq*t) * gain * 32767)
			binary.LittleEndian.PutUint16(data[44+i*4:], uint16(sample))
			binary.LittleEndian.PutUint16(data[46+i*4:], uint16(sample))
		}
		if err := os.WriteFile(filepath.Join(root, s.name), data, 0644); err != nil {
			return err
		}
	}
	bg := image.NewRGBA(image.Rect(0, 0, 640, 480))
	for y := 0; y < 480; y++ {
		for x := 0; x < 640; x++ {
			bg.SetRGBA(x, y, color.RGBA{uint8(20 + x/8), uint8(30 + y/8), 90, 255})
		}
	}
	sprite := image.NewRGBA(image.Rect(0, 0, 64, 64))
	for y := 0; y < 64; y++ {
		for x := 0; x < 64; x++ {
			if math.Hypot(float64(x-32), float64(y-32)) < 29 {
				sprite.SetRGBA(x, y, color.RGBA{100, 220, 240, 255})
			}
		}
	}
	for name, img := range map[string]image.Image{"background.png": bg, "sprite.png": sprite} {
		f, err := os.Create(filepath.Join(root, name))
		if err != nil {
			return err
		}
		encodeErr := png.Encode(f, img)
		closeErr := f.Close()
		if encodeErr != nil {
			return encodeErr
		}
		if closeErr != nil {
			return closeErr
		}
	}
	return o2jam(root)
}

type ojnHeader struct {
	ID                                uint32
	Signature                         [4]byte
	Version                           float32
	Genre                             uint32
	BPM                               float32
	Level                             [4]uint16
	Events, Notes, Measures, Packages [3]uint32
	OldVersion, OldID                 uint16
	OldGenre                          [20]byte
	BMPSize, FileVersion              uint32
	Title                             [64]byte
	Artist, Creator, Bank             [32]byte
	CoverSize                         uint32
	Duration, Offset                  [3]uint32
	CoverOffset                       uint32
}

func o2jam(root string) error {
	bank := make([]byte, 20)
	copy(bank, "OJM\x00")
	binary.LittleEndian.PutUint16(bank[4:], 2)
	binary.LittleEndian.PutUint32(bank[8:], 20)
	for _, name := range []string{"tap.wav", "tone.wav"} {
		wav, err := os.ReadFile(filepath.Join(root, name))
		if err != nil {
			return err
		}
		h := make([]byte, 56)
		copy(h, name)
		copy(h[32:48], wav[20:36])
		copy(h[48:52], "data")
		binary.LittleEndian.PutUint32(h[52:], uint32(len(wav)-44))
		bank = append(bank, h...)
		bank = append(bank, wav[44:]...)
	}
	binary.LittleEndian.PutUint32(bank[12:], uint32(len(bank)))
	binary.LittleEndian.PutUint32(bank[16:], uint32(len(bank)))
	if err := os.WriteFile(filepath.Join(root, "demo.ojm"), bank, 0644); err != nil {
		return err
	}
	var section bytes.Buffer
	write := func(v any) error { return binary.Write(&section, binary.LittleEndian, v) }
	if err := write(struct {
		Measure        uint32
		Channel, Count uint16
	}{0, 1, 4}); err != nil {
		return err
	}
	if err := write([4]float32{0, 0, 180, 0}); err != nil {
		return err
	}
	for _, p := range []struct {
		measure uint32
		channel uint16
		events  [4][4]byte
	}{{0, 2, [4][4]byte{{}, {1, 0, 0, 2}, {}, {1, 0, 0, 3}}}, {0, 3, [4][4]byte{{}, {}, {1, 0, 0, 0}, {}}}, {1, 4, [4][4]byte{{}, {1, 0, 0, 0}, {}, {}}}, {1, 8, [4][4]byte{{}, {}, {1, 0, 0, 0}, {}}}} {
		if err := write(struct {
			Measure        uint32
			Channel, Count uint16
		}{p.measure, p.channel, 4}); err != nil {
			return err
		}
		if err := write(p.events); err != nil {
			return err
		}
	}
	if err := write(struct {
		Measure        uint32
		Channel, Count uint16
	}{0, 9, 1}); err != nil {
		return err
	}
	if err := write([4]byte{2, 0, 0, 0}); err != nil {
		return err
	}
	h := ojnHeader{ID: 1, Signature: [4]byte{'o', 'j', 'n', 0}, Version: 2.9, BPM: 120, Level: [4]uint16{2, 3, 4}, Packages: [3]uint32{6, 6, 6}, Notes: [3]uint32{4, 4, 4}, Events: [3]uint32{21, 21, 21}, Measures: [3]uint32{2, 2, 2}, Duration: [3]uint32{3, 3, 3}}
	copy(h.Title[:], "Gosu O2Jam Demo")
	copy(h.Artist[:], "ggosu")
	copy(h.Creator[:], "ggosu")
	copy(h.Bank[:], "demo.ojm")
	for i := range h.Offset {
		h.Offset[i] = uint32(binary.Size(h) + i*section.Len())
	}
	var file bytes.Buffer
	if err := binary.Write(&file, binary.LittleEndian, h); err != nil {
		return err
	}
	for i := 0; i < 3; i++ {
		file.Write(section.Bytes())
	}
	return os.WriteFile(filepath.Join(root, "demo.ojn"), file.Bytes(), 0644)
}
