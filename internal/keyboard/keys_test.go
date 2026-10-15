package keyboard

import (
	"encoding/binary"
	"testing"
)

func TestPortableLaneKeys(t *testing.T) {
	for _, item := range []struct {
		hid   uint32
		evdev uint16
		key   uint16
	}{
		{7, 32, 'D'}, {9, 33, 'F'}, {13, 36, 'J'}, {14, 37, 'K'},
		{41, 1, 27}, {58, 59, 112}, {62, 63, 116}, {224, 29, 162}, {228, 97, 163},
	} {
		if got := hidKey(item.hid); got != item.key {
			t.Errorf("HID %d: %d, want %d", item.hid, got, item.key)
		}
		for _, value := range []uint32{0, 1, 2} {
			var packet [24]byte
			binary.NativeEndian.PutUint16(packet[16:], 1)
			binary.NativeEndian.PutUint16(packet[18:], item.evdev)
			binary.NativeEndian.PutUint32(packet[20:], value)
			e, ok, err := decodeEvdev(packet[:])
			if err != nil || !ok || e.Key != item.key || e.Down != (value != 0) {
				t.Errorf("evdev %d / %d: %+v, ok=%v, error=%v", item.evdev, value, e, ok, err)
			}
		}
	}
}

func TestEvdevReportsKernelQueueLoss(t *testing.T) {
	var packet [24]byte
	binary.NativeEndian.PutUint16(packet[18:], 3) // EV_SYN / SYN_DROPPED
	if _, _, err := decodeEvdev(packet[:]); err == nil {
		t.Fatal("kernel input loss was silently ignored")
	}
	if _, _, err := decodeEvdev(packet[:23]); err == nil {
		t.Fatal("partial event accepted")
	}
}

func TestWindowsPhysicalKeysAndModifiers(t *testing.T) {
	for _, item := range []struct{ scan, flags, fallback, want uint16 }{
		{0x20, 0, 'X', 'D'}, // Layout-dependent VK cannot replace a physical binding.
		{0x2a, 0, 16, 160}, {0x36, 0, 16, 161},
		{0x1d, 0, 17, 162}, {0x1d, 2, 17, 163},
		{0x38, 0, 18, 164}, {0x38, 2, 18, 165},
		{0x48, 2, 38, 38}, {0x45, 4, 19, 19}, {0, 0, 200, 200},
	} {
		if got := scanKey(item.scan, item.flags, item.fallback); got != item.want {
			t.Errorf("scan=%x flags=%x: %d, want %d", item.scan, item.flags, got, item.want)
		}
	}
}
