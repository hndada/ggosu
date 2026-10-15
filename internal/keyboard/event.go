package keyboard

import "time"

// Event is one received key transition. At is the receiver's observation time,
// not a timestamp supplied by the keyboard. Repeated downs are retained.
type Event struct {
	Seq       uint64
	At        time.Duration // Shared monotonic clock time domain.
	PostedAt  time.Duration // Only set for synthetic messages.
	Device    uintptr
	Key       uint16 // Portable identifier: ASCII letters/digits, VK-compatible controls.
	Scan      uint16
	Flags     uint16
	Down      bool
	Synthetic bool
}
