package keyboard

// hidKey normalizes physical HID keyboard usages without a layout-dependent
// character lookup. Letters use ASCII identifiers, matching the Windows POC.
func hidKey(usage uint32) uint16 {
	switch {
	case usage >= 4 && usage <= 29:
		return 'A' + uint16(usage-4)
	case usage >= 30 && usage <= 38:
		return '1' + uint16(usage-30)
	case usage >= 58 && usage <= 69:
		return 0x70 + uint16(usage-58) // F1-F12
	case usage >= 104 && usage <= 115:
		return 0x7c + uint16(usage-104) // F13-F24
	case usage >= 89 && usage <= 97:
		return 0x61 + uint16(usage-89) // keypad 1-9
	}
	return hidControls[usage]
}

var hidControls = map[uint32]uint16{
	39: '0', 40: 13, 41: 27, 42: 8, 43: 9, 44: 32,
	45: 189, 46: 187, 47: 219, 48: 221, 49: 220, 50: 220,
	51: 186, 52: 222, 53: 192, 54: 188, 55: 190, 56: 191,
	57: 20, 70: 44, 71: 145, 72: 19, 73: 45, 74: 36, 75: 33,
	76: 46, 77: 35, 78: 34, 79: 39, 80: 37, 81: 40, 82: 38,
	83: 144, 84: 111, 85: 106, 86: 109, 87: 107, 88: 13, 98: 96, 99: 110, 100: 226, 101: 93,
	224: 162, 225: 160, 226: 164, 227: 91,
	228: 163, 229: 161, 230: 165, 231: 92,
}

// Linux evdev key codes are physical positions. Normalize them once at ingress.
var evdevKeys = [...]uint16{
	1: 27, 2: '1', 3: '2', 4: '3', 5: '4', 6: '5', 7: '6', 8: '7', 9: '8', 10: '9', 11: '0',
	12: 189, 13: 187, 14: 8, 15: 9,
	16: 'Q', 17: 'W', 18: 'E', 19: 'R', 20: 'T', 21: 'Y', 22: 'U', 23: 'I', 24: 'O', 25: 'P',
	26: 219, 27: 221, 28: 13, 29: 162,
	30: 'A', 31: 'S', 32: 'D', 33: 'F', 34: 'G', 35: 'H', 36: 'J', 37: 'K', 38: 'L', 39: 186, 40: 222,
	41: 192, 42: 160, 43: 220,
	44: 'Z', 45: 'X', 46: 'C', 47: 'V', 48: 'B', 49: 'N', 50: 'M', 51: 188, 52: 190, 53: 191,
	54: 161, 55: 106, 56: 164, 57: 32, 58: 20,
	59: 112, 60: 113, 61: 114, 62: 115, 63: 116, 64: 117, 65: 118, 66: 119, 67: 120, 68: 121,
	69: 144, 70: 145, 71: 103, 72: 104, 73: 105, 74: 109, 75: 100, 76: 101, 77: 102, 78: 107,
	79: 97, 80: 98, 81: 99, 82: 96, 83: 110, 86: 226, 87: 122, 88: 123, 96: 13, 97: 163, 98: 111,
	99: 44, 100: 165, 102: 36, 103: 38, 104: 33, 105: 37, 106: 39, 107: 35, 108: 40,
	109: 34, 110: 45, 111: 46, 119: 19, 125: 91, 126: 92, 127: 93,
	183: 124, 184: 125, 185: 126, 186: 127, 187: 128, 188: 129, 189: 130, 190: 131,
	191: 132, 192: 133, 193: 134, 194: 135,
}

// PC set-1 scancodes match the early evdev positions. Extended keys need an
// explicit mapping. This keeps physical letter bindings stable across layouts.
func scanKey(scan, flags, fallback uint16) uint16 {
	if flags&4 != 0 {
		return fallback
	} // E1, including Pause.
	if flags&2 != 0 {
		if int(scan) < len(extendedScanKeys) && extendedScanKeys[scan] != 0 {
			return extendedScanKeys[scan]
		}
		return fallback
	}
	if int(scan) < len(evdevKeys) && evdevKeys[scan] != 0 {
		return evdevKeys[scan]
	}
	return fallback
}

var extendedScanKeys = [...]uint16{
	0x1c: 13, 0x1d: 163, 0x35: 111, 0x37: 44, 0x38: 165,
	0x47: 36, 0x48: 38, 0x49: 33, 0x4b: 37, 0x4d: 39, 0x4f: 35,
	0x50: 40, 0x51: 34, 0x52: 45, 0x53: 46, 0x5b: 91, 0x5c: 92, 0x5d: 93,
}
