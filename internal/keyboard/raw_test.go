package keyboard

import (
	"encoding/binary"
	"testing"
)

func packet(header int, flags uint16) []byte {
	b := make([]byte, header+16)
	binary.LittleEndian.PutUint32(b, 1)
	binary.LittleEndian.PutUint32(b[4:], uint32(len(b)))
	if header == 24 {
		binary.LittleEndian.PutUint64(b[8:], 0x123456789)
	} else {
		binary.LittleEndian.PutUint32(b[8:], 0x12345678)
	}
	binary.LittleEndian.PutUint16(b[header:], 0x20)
	binary.LittleEndian.PutUint16(b[header+2:], flags)
	binary.LittleEndian.PutUint16(b[header+6:], 'D')
	return b
}

func TestDecodeNativePackets(t *testing.T) {
	for _, header := range []int{16, 24} {
		for _, flags := range []uint16{0, 1, 2, 3, 4, 5} {
			e, ok, err := decode(packet(header, flags), header)
			if err != nil || !ok {
				t.Fatalf("header %d, flags %d: ok=%v error=%v", header, flags, ok, err)
			}
			if e.Key != 'D' || e.Scan != 0x20 || e.Flags != flags || e.Down != (flags&1 == 0) {
				t.Fatalf("decoded wrong keyboard fields: %+v", e)
			}
			want := uintptr(0x12345678)
			if header == 24 {
				want = uintptr(0x123456789)
			}
			if e.Device != want {
				t.Fatalf("device=%x, want %x", e.Device, want)
			}
		}
	}
}

func TestDecodeRejectsMalformedPackets(t *testing.T) {
	for _, n := range []int{0, 8, 23, 24, 39} {
		b := packet(24, 0)[:n]
		if _, _, err := decode(b, 24); err == nil {
			t.Fatalf("accepted truncated packet of %d bytes", n)
		}
	}
	b := packet(24, 0)
	binary.LittleEndian.PutUint32(b[4:], 1024)
	if _, _, err := decode(b, 24); err == nil {
		t.Fatal("accepted packet size larger than buffer")
	}
}

func TestDecodeIgnoresOtherDevicesAndInvalidKeys(t *testing.T) {
	for _, mutate := range []func([]byte){
		func(b []byte) { binary.LittleEndian.PutUint32(b, 0) },
		func(b []byte) { binary.LittleEndian.PutUint16(b[30:], 255) },
		func(b []byte) { binary.LittleEndian.PutUint16(b[24:], 255) },
	} {
		b := packet(24, 0)
		mutate(b)
		if _, ok, err := decode(b, 24); ok || err != nil {
			t.Fatalf("invalid/device packet: ok=%v error=%v", ok, err)
		}
	}
}
