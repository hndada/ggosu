package ojn

import (
	"encoding/binary"
	"fmt"
	"ggosu/internal/chart"
)

// ReadBank reads M30 (plain/nami/0412), OJM and legacy OMC containers.
func (Format) ReadBank(b []byte) (map[int]chart.Sample, error) {
	if len(b) < 20 {
		return nil, fmt.Errorf("truncated O2Jam bank")
	}
	out := map[int]chart.Sample{}
	u32 := binary.LittleEndian.Uint32
	u16 := binary.LittleEndian.Uint16
	switch string(b[:4]) {
	case "M30\x00":
		if len(b) < 28 {
			return nil, fmt.Errorf("short M30 header")
		}
		flag := u32(b[8:])
		count := int(u32(b[12:]))
		offset := 28
		reported := int(u32(b[16:]))
		declared := int(u32(b[20:])) // Payload end; banks store it with or without the 28-byte header.
		if reported != 0 && reported != 28 {
			return nil, fmt.Errorf("unsupported M30 payload offset")
		}
		mask := ""
		switch flag {
		case 0:
		case 16:
			mask = "nami"
		case 32:
			mask = "0412"
		default:
			return nil, fmt.Errorf("unknown M30 encryption %d", flag)
		}
		for i := 0; i < count; i++ {
			// Official banks can count an unwritten, empty final slot.
			// Only a clean record boundary is allowed; partial records still fail.
			if offset == len(b) {
				break
			}
			if len(b)-offset < 52 {
				// Some official banks pad two bytes after the declared payload end.
				if offset == declared || offset == 28+declared {
					break
				}
				return out, fmt.Errorf("truncated M30 sample header")
			}
			h := b[offset : offset+52]
			offset += 52
			size := int(u32(h[32:]))
			codec := u16(h[36:])
			ref := int(u16(h[44:])) + 1
			if codec == 0 {
				ref += 1000
			} else if codec != 5 {
				return out, fmt.Errorf("unsupported M30 codec %d", codec)
			}
			if size > len(b)-offset {
				return out, fmt.Errorf("invalid M30 sample size")
			}
			data := append([]byte(nil), b[offset:offset+size]...)
			offset += size
			if mask != "" {
				for j := 0; j+3 < len(data); j += 4 {
					for k := 0; k < 4; k++ {
						data[j+k] ^= mask[k]
					}
				}
			}
			// A later definition replaces the same slot, as in the original player.
			out[ref] = chart.Sample{Data: data, Extension: ".ogg"}
		}
	case "OJM\x00", "OMC\x00":
		encrypted := string(b[:4]) == "OMC\x00"
		cipher := scramble{key: 0xff}
		wavStart, oggStart, end := int(u32(b[8:])), int(u32(b[12:])), int(u32(b[16:]))
		if wavStart < 20 || oggStart < wavStart || oggStart > len(b) || end < oggStart {
			return nil, fmt.Errorf("invalid OJM offsets")
		}
		truncated := end > len(b)
		end = min(end, len(b))
		offset := wavStart
		ref := 1
		for offset < oggStart {
			if oggStart-offset < 56 {
				return out, fmt.Errorf("truncated OJM WAV header")
			}
			h := b[offset : offset+56]
			offset += 56
			size := int(u32(h[52:]))
			if size > oggStart-offset {
				return out, fmt.Errorf("invalid OJM WAV size")
			}
			if size > 0 {
				data := make([]byte, 44+size)
				copy(data, "RIFF")
				binary.LittleEndian.PutUint32(data[4:], uint32(36+size))
				copy(data[8:], "WAVEfmt ")
				binary.LittleEndian.PutUint32(data[16:], 16)
				copy(data[20:36], h[32:48])
				copy(data[36:], "data")
				binary.LittleEndian.PutUint32(data[40:], uint32(size))
				copy(data[44:], b[offset:offset+size])
				if encrypted {
					cipher.decode(data[44:])
				}
				out[ref] = chart.Sample{Data: data, Extension: ".wav"}
			}
			offset += size
			ref++
		}
		ref = 1001
		for offset < end {
			if end-offset < 36 {
				return out, fmt.Errorf("truncated OJM Ogg header")
			}
			size := int(u32(b[offset+32:]))
			offset += 36
			if size > end-offset {
				return out, fmt.Errorf("invalid OJM Ogg size")
			}
			if size > 0 {
				out[ref] = chart.Sample{Data: append([]byte(nil), b[offset:offset+size]...), Extension: ".ogg"}
			}
			offset += size
			ref++
		}
		if truncated {
			return out, fmt.Errorf("truncated OJM bank")
		}
	default:
		return nil, fmt.Errorf("unknown O2Jam bank signature")
	}
	return out, nil
}
