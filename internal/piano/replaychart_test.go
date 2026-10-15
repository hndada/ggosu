package piano

import (
	"bytes"
	"encoding/binary"
	"strings"
	"testing"
	"time"

	"github.com/ulikunitz/xz/lzma"

	"ggosu/internal/chart"
)

func testChart() *chart.Chart {
	c := chart.New(4)
	c.Source = chart.Source{Format: "osu", MD5: strings.Repeat("a", 32)}
	c.Timing = []chart.Timing{{BPM: 120, Scroll: 1, Meter: 4}}
	c.Notes = []chart.Note{{At: time.Second, End: time.Second, Kind: "tap", Lane: 0}}
	return c
}
func TestChartBinding(t *testing.T) {
	c := testChart()
	f, err := Auto(c, "rules")
	if err != nil {
		t.Fatal(err)
	}
	if err := f.Match(c, "rules"); err != nil {
		t.Fatal(err)
	}
	c.Lanes = 5
	if f.Match(c, "rules") == nil {
		t.Fatal("accepted wrong layout")
	}
	c.Lanes = 4
	c.Notes[0].At++
	c.Notes[0].End++
	if f.Match(c, "rules") == nil {
		t.Fatal("accepted changed chart")
	}
}

func TestNegativeLaneReplay(t *testing.T) {
	c := testChart()
	c.Notes = []chart.Note{{At: -time.Second, End: -time.Second, Kind: "tap", Lane: 0}, {At: 0, End: 0, Kind: "lift", Lane: 1}}
	f, err := Auto(c, "rules")
	if err != nil {
		t.Fatal(err)
	}
	p, err := NewPlayer(f)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := p.AppendDue(nil, -2*time.Second); err != nil {
		t.Fatal(err)
	}
	due, err := p.AppendDue(nil, 0)
	if err != nil || len(due) != 4 {
		t.Fatalf("negative transitions lost: %+v %v", due, err)
	}
	p.Reset()
	again, err := p.AppendDue(nil, 0)
	if err != nil || len(again) != 4 {
		t.Fatal("reset lost negative timeline")
	}
	legacy := NewReplay()
	legacy.Events = []Event{{At: -1}}
	if legacy.Validate() == nil {
		t.Fatal("changed legacy nonnegative replay contract")
	}
}

func osrFixture(t *testing.T, mode byte, mods uint32, md5, actions string) []byte {
	t.Helper()
	var data bytes.Buffer
	w, err := (lzma.WriterConfig{DictCap: 1 << 20}).NewWriter(&data)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.Write([]byte(actions)); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	var b bytes.Buffer
	write := func(v any) {
		t.Helper()
		if err := binary.Write(&b, binary.LittleEndian, v); err != nil {
			t.Fatal(err)
		}
	}
	str := func(s string) {
		t.Helper()
		if s == "" {
			b.WriteByte(0)
			return
		}
		b.WriteByte(0x0b)
		var size [10]byte
		n := binary.PutUvarint(size[:], uint64(len(s)))
		b.Write(size[:n])
		b.WriteString(s)
	}
	write(mode)
	write(int32(20200101))
	str(md5)
	str("tester")
	str("")
	write([6]uint16{})
	write(int32(0))
	write(uint16(0))
	write(byte(0))
	write(mods)
	str("")
	write(int64(0))
	write(int32(data.Len()))
	b.Write(data.Bytes())
	write(int64(0))
	return b.Bytes()
}

func TestOSRImport(t *testing.T) {
	c := testChart()
	data := osrFixture(t, 3, 0, c.Source.MD5, "-100|0|0|0,1100|1|0|0,10|0|0|0,-12345|0|0|42,")
	f, err := ReadOSR(bytes.NewReader(data), c, "rules")
	if err != nil {
		t.Fatal(err)
	}
	if len(f.Events) != 2 || f.Events[0].At != time.Second || !f.Events[0].Down || f.Events[1].At != 1010*time.Millisecond || f.Player != "tester" {
		t.Fatalf("bad OSR: %+v", f)
	}
	for _, bad := range [][]byte{osrFixture(t, 0, 0, c.Source.MD5, ""), osrFixture(t, 3, 64, c.Source.MD5, ""), osrFixture(t, 3, 0, strings.Repeat("b", 32), ""), osrFixture(t, 3, 0, c.Source.MD5, "1000|16|0|0,")} {
		if _, err := ReadOSR(bytes.NewReader(bad), c, "rules"); err == nil {
			t.Fatal("accepted invalid OSR")
		}
	}
}
func FuzzOSR(f *testing.F) {
	f.Add([]byte{3, 0, 0, 0, 0})
	f.Fuzz(func(t *testing.T, b []byte) {
		if len(b) > 1<<20 {
			return
		}
		ReadOSR(bytes.NewReader(b), testChart(), "rules")
	})
}
