package ojn

import (
	"bytes"
	"encoding/binary"
	"testing"
)

func TestM30(t *testing.T) {
	for _, flag := range []uint32{0, 16, 32} {
		b := make([]byte, 28+52+8)
		copy(b, "M30\x00")
		binary.LittleEndian.PutUint32(b[8:], flag)
		binary.LittleEndian.PutUint32(b[12:], 1)
		binary.LittleEndian.PutUint32(b[16:], 28)
		binary.LittleEndian.PutUint32(b[60:], 8)
		binary.LittleEndian.PutUint16(b[64:], 5)
		binary.LittleEndian.PutUint16(b[72:], 2)
		plain := []byte("OggSdata")
		copy(b[80:], plain)
		mask := ""
		if flag == 16 {
			mask = "nami"
		}
		if flag == 32 {
			mask = "0412"
		}
		if mask != "" {
			for i := 0; i < 8; i++ {
				b[80+i] ^= mask[i%4]
			}
		}
		samples, err := (Format{}).ReadBank(b)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(samples[3].Data, plain) {
			t.Fatal("bank reference or decode mismatch")
		}
		binary.LittleEndian.PutUint32(b[60:], 100)
		if _, err := (Format{}).ReadBank(b); err == nil {
			t.Fatal("accepted truncated bank")
		}
	}
}
func FuzzBank(f *testing.F) {
	f.Add([]byte("M30\x00"))
	f.Add([]byte("OJM\x00"))
	f.Fuzz(func(t *testing.T, b []byte) {
		if len(b) > 1<<20 {
			return
		}
		(Format{}).ReadBank(b)
	})
}

func TestM30CountAndTruncation(t *testing.T) {
	b := make([]byte, 28+52+8)
	copy(b, "M30\x00")
	binary.LittleEndian.PutUint32(b[12:], 2) // One complete sample and an unwritten final slot.
	binary.LittleEndian.PutUint32(b[16:], 28)
	binary.LittleEndian.PutUint32(b[60:], 8)
	binary.LittleEndian.PutUint16(b[64:], 5)
	copy(b[80:], "OggSdata")
	samples, err := (Format{}).ReadBank(b)
	if err != nil || !bytes.Equal(samples[1].Data, []byte("OggSdata")) {
		t.Fatalf("complete bank with inaccurate count: %v, %v", samples, err)
	}
	if _, err := (Format{}).ReadBank(b[:len(b)-1]); err == nil {
		t.Fatal("accepted a truncated sample payload")
	}
	if _, err := (Format{}).ReadBank(append(b, 1)); err == nil {
		t.Fatal("accepted a partial sample header")
	}
	samples, err = (Format{}).ReadBank(append(b, 1))
	if err == nil || !bytes.Equal(samples[1].Data, []byte("OggSdata")) {
		t.Fatal("a partial final header discarded an earlier complete sample")
	}
}

func TestM30PaddingAfterDeclaredPayload(t *testing.T) {
	for _, declared := range []uint32{28 + 52 + 8, 52 + 8} {
		b := make([]byte, 28+52+8)
		copy(b, "M30\x00")
		binary.LittleEndian.PutUint32(b[12:], 3)
		binary.LittleEndian.PutUint32(b[16:], 28)
		binary.LittleEndian.PutUint32(b[20:], declared)
		binary.LittleEndian.PutUint32(b[60:], 8)
		binary.LittleEndian.PutUint16(b[64:], 5)
		copy(b[80:], "OggSdata")
		samples, err := (Format{}).ReadBank(append(b, 0xb1, 0x16))
		if err != nil || !bytes.Equal(samples[1].Data, []byte("OggSdata")) {
			t.Fatalf("declared end %d: %v, %v", declared, samples, err)
		}
	}
}
