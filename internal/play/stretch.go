package play

import (
	"encoding/binary"
	"fmt"
	"math"
)

// stretchPCM changes the duration of 48kHz stereo signed 16-bit PCM without
// resampling its pitch. WSOLA matches overlapping waveforms in both channels.
func stretchPCM(pcm []byte, rate float64) ([]byte, error) {
	if math.IsNaN(rate) || math.IsInf(rate, 0) || rate < .5 || rate > 2 || len(pcm)%4 != 0 {
		return nil, fmt.Errorf("sound: invalid stretch rate or PCM")
	}
	if rate == 1 || len(pcm) == 0 {
		return pcm, nil
	}
	n := len(pcm) / 4
	frames := max(1, int(math.Round(float64(n)/rate)))
	if frames > maxPCM/4 {
		return nil, fmt.Errorf("sound: stretched audio exceeds memory limit")
	}
	out := make([]byte, frames*4)
	window := min(960, n)
	hop := max(1, window/2)
	copy(out, pcm[:min(len(out), window*4)])
	previous := 0
	for at := hop; at < frames; at += hop {
		center := min(max(0, int(math.Round(float64(at)*rate))), n-window)
		lo, hi := max(0, center-240), min(n-window, center+240)
		best, score := center, math.Inf(-1)
		// Coarse search followed by a fine search keeps long songs affordable.
		for candidate := lo; candidate <= hi; candidate += 8 {
			v := similarity(pcm, previous+hop, candidate, min(hop, n-previous-hop), 8)
			if v > score {
				best, score = candidate, v
			}
		}
		coarse := best
		for candidate := max(lo, coarse-8); candidate <= min(hi, coarse+8); candidate++ {
			v := similarity(pcm, previous+hop, candidate, min(hop, n-previous-hop), 4)
			if v > score {
				best, score = candidate, v
			}
		}
		for i := 0; i < window && at+i < frames; i++ {
			for ch := 0; ch < 2; ch++ {
				value := sample(pcm, best+i, ch)
				if i < hop {
					weight := float64(i) / float64(hop)
					value = sample(out, at+i, ch)*(1-weight) + value*weight
				}
				binary.LittleEndian.PutUint16(out[(at+i)*4+ch*2:], uint16(int16(math.Round(value))))
			}
		}
		previous = best
	}
	return out, nil
}

func sample(pcm []byte, frame, ch int) float64 {
	return float64(int16(binary.LittleEndian.Uint16(pcm[frame*4+ch*2:])))
}

func similarity(pcm []byte, a, b, frames, step int) float64 {
	var cross, x, y float64
	for i := 0; i < frames; i += step {
		for ch := 0; ch < 2; ch++ {
			u, v := sample(pcm, a+i, ch), sample(pcm, b+i, ch)
			cross += u * v
			x += u * u
			y += v * v
		}
	}
	if x*y == 0 {
		return 0
	}
	return cross / math.Sqrt(x*y)
}
