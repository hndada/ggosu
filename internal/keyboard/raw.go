package keyboard

import (
	"encoding/binary"
	"fmt"
)

// decode uses the native header size (24 bytes on Windows amd64). The payload
// itself uses fixed-width fields; decoding it avoids unsafe, unaligned reads.
func decode(data []byte, headerSize int) (Event, bool, error) {
	if len(data) < headerSize || headerSize < 16 {
		return Event{}, false, fmt.Errorf("short raw input header: %d bytes", len(data))
	}
	if binary.LittleEndian.Uint32(data) != 1 { // RIM_TYPEKEYBOARD
		return Event{}, false, nil
	}
	size := int(binary.LittleEndian.Uint32(data[4:]))
	if size < headerSize+16 || size > len(data) {
		return Event{}, false, fmt.Errorf("invalid raw keyboard size: %d (buffer %d)", size, len(data))
	}
	k := data[headerSize:]
	key := binary.LittleEndian.Uint16(k[6:])
	scan := binary.LittleEndian.Uint16(k)
	if key >= 255 || scan == 255 { // Invalid key or KEYBOARD_OVERRUN_MAKE_CODE.
		return Event{}, false, nil
	}
	flags := binary.LittleEndian.Uint16(k[2:])
	e := Event{Key: scanKey(scan, flags, key), Scan: scan, Flags: flags, Down: flags&1 == 0}
	if headerSize == 24 {
		e.Device = uintptr(binary.LittleEndian.Uint64(data[8:]))
	} else {
		e.Device = uintptr(binary.LittleEndian.Uint32(data[8:]))
	}
	return e, true, nil
}
