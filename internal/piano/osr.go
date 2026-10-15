package piano

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/ulikunitz/xz/lzma"

	"ggosu/internal/chart"
)

// ReadOSR imports osu!mania state masks into portable lane transitions.
// Modded files are rejected until their transformation is implemented.
func ReadOSR(r io.Reader, c *chart.Chart, rules string) (Replay, error) {
	if err := c.Validate(); err != nil {
		return Replay{}, err
	}
	b, err := io.ReadAll(io.LimitReader(r, 64<<20+1))
	if err != nil {
		return Replay{}, err
	}
	if len(b) > 64<<20 {
		return Replay{}, fmt.Errorf("OSR too large")
	}
	rd := bytes.NewReader(b)
	var mode byte
	var version int32
	if err := binary.Read(rd, binary.LittleEndian, &mode); err != nil {
		return Replay{}, err
	}
	if err := binary.Read(rd, binary.LittleEndian, &version); err != nil {
		return Replay{}, err
	}
	if mode != 3 {
		return Replay{}, fmt.Errorf("OSR is not osu!mania")
	}
	md5, err := osrString(rd)
	if err != nil {
		return Replay{}, err
	}
	player, err := osrString(rd)
	if err != nil {
		return Replay{}, err
	}
	if _, err := osrString(rd); err != nil {
		return Replay{}, err
	}
	if c.Source.Format != "osu" || c.Source.MD5 == "" || !strings.EqualFold(md5, c.Source.MD5) {
		return Replay{}, fmt.Errorf("OSR beatmap MD5 mismatch")
	}
	var counts [6]uint16
	var score int32
	var combo uint16
	var perfect byte
	var mods uint32
	for _, dst := range []any{&counts, &score, &combo, &perfect, &mods} {
		if err := binary.Read(rd, binary.LittleEndian, dst); err != nil {
			return Replay{}, err
		}
	}
	if mods & ^uint32(1<<12) != 0 {
		return Replay{}, fmt.Errorf("OSR mods %x are unsupported (only autoplay is allowed)", mods)
	}
	if _, err := osrString(rd); err != nil {
		return Replay{}, err
	}
	var ticks int64
	var size int32
	for _, dst := range []any{&ticks, &size} {
		if err := binary.Read(rd, binary.LittleEndian, dst); err != nil {
			return Replay{}, err
		}
	}
	if size < 0 || int64(size) > int64(rd.Len()) {
		return Replay{}, fmt.Errorf("invalid OSR payload length")
	}
	payload := make([]byte, size)
	if _, err := io.ReadFull(rd, payload); err != nil {
		return Replay{}, err
	}
	if rd.Len() < 8 {
		return Replay{}, fmt.Errorf("missing OSR online ID")
	}
	lr, err := (lzma.ReaderConfig{DictCap: 16 << 20}).NewReader(bytes.NewReader(payload))
	if err != nil {
		return Replay{}, err
	}
	data, err := io.ReadAll(io.LimitReader(lr, 64<<20+1))
	if err != nil {
		return Replay{}, err
	}
	if len(data) > 64<<20 {
		return Replay{}, fmt.Errorf("expanded OSR exceeds limit")
	}
	f, err := NewChartReplay(c, rules)
	if err != nil {
		return Replay{}, err
	}
	f.Player = player
	var at time.Duration
	var held uint32
	haveFrame := false
	for _, raw := range strings.Split(string(data), ",") {
		if raw == "" {
			continue
		}
		fields := strings.Split(raw, "|")
		if len(fields) != 4 {
			return Replay{}, fmt.Errorf("invalid OSR action")
		}
		delta, err := strconv.ParseInt(fields[0], 10, 64)
		if err != nil {
			return Replay{}, err
		}
		if delta == -12345 {
			continue
		}
		if delta > math.MaxInt64/int64(time.Millisecond) || delta < math.MinInt64/int64(time.Millisecond) {
			return Replay{}, fmt.Errorf("OSR time overflow")
		}
		step := time.Duration(delta) * time.Millisecond
		if step > 0 && at > time.Duration(math.MaxInt64)-step || step < 0 && at < time.Duration(math.MinInt64)-step {
			return Replay{}, fmt.Errorf("OSR time overflow")
		}
		at += step
		value, err := strconv.ParseFloat(fields[1], 64)
		if err != nil || math.IsNaN(value) || math.IsInf(value, 0) || value != math.Trunc(value) || value < 0 || value >= float64(uint32(1)<<c.Lanes) {
			return Replay{}, fmt.Errorf("invalid OSR lane mask")
		}
		mask := uint32(value)
		if step < 0 && haveFrame {
			return Replay{}, fmt.Errorf("OSR time moved backwards")
		}
		haveFrame = true
		for lane := 0; lane < c.Lanes; lane++ {
			bit := uint32(1) << lane
			if held&bit != mask&bit {
				f.Events = append(f.Events, Event{At: at, Key: uint16(lane), Down: mask&bit != 0})
			}
		}
		held = mask
	}
	f.Duration = max(c.Duration()+time.Second, at)
	for lane := 0; lane < c.Lanes; lane++ {
		if held&(1<<lane) != 0 {
			f.Events = append(f.Events, Event{At: f.Duration, Key: uint16(lane)})
		}
	}
	return f, f.Validate()
}

func osrString(r *bytes.Reader) (string, error) {
	kind, err := r.ReadByte()
	if err != nil {
		return "", err
	}
	if kind == 0 {
		return "", nil
	}
	if kind != 0x0b {
		return "", fmt.Errorf("invalid OSR string marker")
	}
	size, err := binary.ReadUvarint(r)
	if err != nil || size > uint64(r.Len()) || size > 1<<20 {
		return "", fmt.Errorf("invalid OSR string size")
	}
	b := make([]byte, size)
	_, err = io.ReadFull(r, b)
	return string(b), err
}
