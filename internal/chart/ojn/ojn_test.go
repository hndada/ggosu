package ojn

import (
	"bytes"
	"encoding/binary"
	"testing"
	"time"

	"ggosu/internal/chart"
)

func TestOJN(t *testing.T) {
	h := ojnHeader{Signature: [4]byte{'o', 'j', 'n', 0}, BPM: 120}
	copy(h.Title[:], "Test")
	copy(h.Bank[:], "test.ojm")
	var sections [3][]byte
	for diff := range sections {
		var b bytes.Buffer
		write := func(v any) {
			t.Helper()
			if err := binary.Write(&b, binary.LittleEndian, v); err != nil {
				t.Fatal(err)
			}
		}
		write(struct {
			M    uint32
			C, N uint16
		}{0, 0, 1})
		write(float32(.5))
		write(struct {
			M    uint32
			C, N uint16
		}{0, 1, 2})
		write(float32(0))
		write(float32(240))
		write(struct {
			M    uint32
			C, N uint16
		}{0, 2, 4})
		write([4]byte{1, 0, 0x84, 2})
		write([4]byte{})
		write([4]byte{})
		write([4]byte{2, 0, 0, 3})
		write(struct {
			M    uint32
			C, N uint16
		}{1, 3, 1})
		write([4]byte{3, 0, 0, 4})
		h.Packages[diff] = 4
		sections[diff] = b.Bytes()
	}
	size := binary.Size(h)
	for i := range sections {
		h.Offset[i] = uint32(size)
		size += len(sections[i])
	}
	var b bytes.Buffer
	if err := binary.Write(&b, binary.LittleEndian, h); err != nil {
		t.Fatal(err)
	}
	for _, s := range sections {
		b.Write(s)
	}
	cs, err := Format{}.Parse(b.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range cs {
		c.Normalize()
		if err := c.Validate(); err != nil {
			t.Fatal(err)
		}
	}
	if len(cs) != 3 {
		t.Fatal("missing difficulties")
	}
	c := cs[0]
	if c.Notes[0].End != 625*time.Millisecond || c.Notes[1].At != 750*time.Millisecond || c.Notes[0].Sounds[0].Volume != .5 || c.Notes[0].Sounds[0].Pan != -.5 || c.Assets[c.Notes[1].Sounds[0].Asset].Sample != 1003 {
		t.Fatalf("bad OJN conversion: %+v", c)
	}
}

// O2Mania charts carry background channels past 22 and stray long-note marks; one broken difficulty
// must not hide the other two.
func TestOJNTolerance(t *testing.T) {
	h := ojnHeader{Signature: [4]byte{'o', 'j', 'n', 0}, BPM: 120}
	copy(h.Bank[:], "test.ojm")
	var sections [3][]byte
	for diff := 0; diff < 2; diff++ {
		var b bytes.Buffer
		write := func(v any) {
			if err := binary.Write(&b, binary.LittleEndian, v); err != nil {
				t.Fatal(err)
			}
		}
		pkg := func(m uint32, c, n uint16) {
			write(struct {
				M    uint32
				C, N uint16
			}{m, c, n})
		}
		pkg(0, 23, 1) // background sound on channel 23
		write([4]byte{5, 0, 0, 0})
		pkg(0, 2, 4) // lane 0: tail without head, then a tap, a hold and a tap inside it
		write([4]byte{1, 0, 0, 3})
		write([4]byte{2, 0, 0, 0})
		write([4]byte{3, 0, 0, 2})
		write([4]byte{4, 0, 0, 0})
		h.Packages[diff] = 2
		sections[diff] = b.Bytes()
	}
	size := binary.Size(h)
	for i := range sections {
		h.Offset[i] = uint32(size)
		size += len(sections[i])
	}
	h.Packages[2] = 1 << 20 // Hard claims more packages than its section holds.
	var b bytes.Buffer
	if err := binary.Write(&b, binary.LittleEndian, h); err != nil {
		t.Fatal(err)
	}
	for _, s := range sections {
		b.Write(s)
	}
	cs, err := Format{}.Parse(b.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	if len(cs) != 2 {
		t.Fatalf("difficulties: %d", len(cs))
	}
	c := cs[0]
	c.Normalize()
	if err := c.Validate(); err != nil {
		t.Fatal(err)
	}
	if len(c.Audio) != 1 || len(c.Notes) != 2 || c.Notes[0].Kind != "tap" || c.Notes[1].Kind != "tap" || len(c.Fixes) == 0 || len(c.Warnings) != 0 {
		t.Fatalf("tolerant parse: audio %d notes %+v fixes %v warnings %v", len(c.Audio), c.Notes, c.Fixes, c.Warnings)
	}
}

func TestInvalidDifficultyKeepsValidCharts(t *testing.T) {
	for _, bad := range []int{0, 1, 2, -1} {
		h := ojnHeader{Signature: [4]byte{'o', 'j', 'n', 0}, BPM: 120}
		copy(h.Bank[:], "test.ojm")
		size := binary.Size(h)
		var sections bytes.Buffer
		for diff := range 3 {
			h.Offset[diff], h.Packages[diff] = uint32(size+sections.Len()), 1
			measure := uint32(0)
			if diff == bad || bad < 0 {
				measure = 50000
			}
			for _, v := range []any{measure, uint16(2), uint16(1), [4]byte{1, 0, 0, 0}} {
				if err := binary.Write(&sections, binary.LittleEndian, v); err != nil {
					t.Fatal(err)
				}
			}
		}
		var b bytes.Buffer
		if err := binary.Write(&b, binary.LittleEndian, h); err != nil {
			t.Fatal(err)
		}
		b.Write(sections.Bytes())
		cs, err := chart.Decode("partial.ojn", bytes.NewReader(b.Bytes()))
		if bad < 0 {
			if err == nil || len(cs) != 0 {
				t.Fatal("accepted all-invalid difficulties", err)
			}
			continue
		}
		if err != nil || len(cs) != 2 {
			t.Fatalf("bad difficulty %d: charts=%d, err=%v", bad, len(cs), err)
		}
		for i, c := range cs {
			index := i
			if i >= bad {
				index++
			}
			if c.Source.Index != i || c.Metadata.Difficulty != []string{"Easy", "Normal", "Hard"}[index] || len(c.Fixes) != 1 {
				t.Fatalf("lost identity/diagnostic after skipping %d: %+v", bad, c)
			}
			if err := c.Validate(); err != nil {
				t.Fatal(err)
			}
		}
	}
}
