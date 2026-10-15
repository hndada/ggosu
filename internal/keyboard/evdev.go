package keyboard

import (
	"encoding/binary"
	"fmt"
)

// decodeEvdev accepts a native 64-bit input_event. Kernel timestamps are not
// mixed with the game's clock: the receiving loop supplies Event.At.
func decodeEvdev(b []byte) (Event, bool, error) {
	if len(b) != 24 {
		return Event{}, false, fmt.Errorf("evdev event has %d bytes, want 24", len(b))
	}
	kind, code := binary.NativeEndian.Uint16(b[16:]), binary.NativeEndian.Uint16(b[18:])
	if kind == 0 && code == 3 {
		return Event{}, false, fmt.Errorf("evdev reported SYN_DROPPED; input state must be resynchronised")
	}
	if kind != 1 {
		return Event{}, false, nil
	} // EV_KEY
	value := int32(binary.NativeEndian.Uint32(b[20:]))
	if value < 0 || value > 2 {
		return Event{}, false, nil
	}
	if int(code) >= len(evdevKeys) || evdevKeys[code] == 0 {
		return Event{}, false, nil
	}
	e := Event{Key: evdevKeys[code], Scan: code, Down: value != 0}
	if !e.Down {
		e.Flags = 1
	}
	return e, true, nil
}
