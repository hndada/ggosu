// Copyright 2017 The Ebiten Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package play

import (
	"encoding/binary"
	"fmt"
	"math"
	"sync"
)

// This is Ebitengine's windowed-sinc filter for already decoded stereo PCM.
// Unlike its seeking stream, this reads samples directly and reuses weights
// for identical fractional positions. The filter and rounding are unchanged.
func resamplePCM(pcm []byte, from, to, limit int) ([]byte, error) {
	if from <= 0 || to <= 0 || len(pcm)%4 != 0 || limit < 0 {
		return nil, fmt.Errorf("sound: invalid resampling input")
	}
	frames := int64(len(pcm)/4) * int64(to) / int64(from)
	if frames > int64(limit/4) {
		return nil, errPCM
	}
	if from == to {
		return pcm, nil
	}
	out := make([]byte, int(frames)*4)
	weights := make(map[float64][17]float64)
	// The filter only needs 17 adjacent frames. Normalize each input frame
	// once in a small ring instead of reading and dividing it for every tap.
	var source [32][2]float64
	next := 0
	for i := range int(frames) {
		pos := float64(i) * float64(from) / float64(to)
		center := int(pos)
		frac := pos - float64(center)
		w, ok := weights[frac]
		if !ok {
			for j := range w {
				d := frac - float64(j-8)
				s := 1.0
				if math.Abs(d/2) >= 1e-8 {
					s = filterCos(d/2-.25) / (d * math.Pi)
				}
				w[j] = (.5 + .5*filterCos(d/17)) * s
			}
			if len(weights) < 4096 {
				weights[frac] = w
			}
		}
		next = max(next, center-8)
		for next < min(center+9, len(pcm)/4) {
			n := next * 4
			source[next&31] = [2]float64{
				float64(int16(binary.LittleEndian.Uint16(pcm[n:]))) / 32767,
				float64(int16(binary.LittleEndian.Uint16(pcm[n+2:]))) / 32767,
			}
			next++
		}
		var left, right float64
		for j := max(0, 8-center); j < 17; j++ {
			n := center + j - 8
			if n >= len(pcm)/4 {
				break
			}
			left += source[n&31][0] * w[j]
			right += source[n&31][1] * w[j]
		}
		left = max(-1, min(1, left))
		right = max(-1, min(1, right))
		binary.LittleEndian.PutUint16(out[i*4:], uint16(int16(left*32767)))
		binary.LittleEndian.PutUint16(out[i*4+2:], uint16(int16(right*32767)))
	}
	return out, nil
}

var filterTable = sync.OnceValue(func() []float64 {
	table := make([]float64, 65536)
	for i := range table {
		table[i] = math.Cos(float64(i) * math.Pi / 2 / float64(len(table)))
	}
	return table
})

// The same cosine lookup as Ebitengine keeps the resulting PCM bit-identical.
func filterCos(x float64) float64 {
	table := filterTable()
	x = math.Abs(x)
	i := int(4 * float64(len(table)) * x)
	if 4*len(table) < i {
		i %= 4 * len(table)
	}
	sign := 1
	switch {
	case i < len(table):
	case i < len(table)*2:
		i = len(table)*2 - i
		sign = -1
	case i < len(table)*3:
		i -= len(table) * 2
		sign = -1
	default:
		i = len(table)*4 - i
	}
	if i == len(table) {
		return 0
	}
	return float64(sign) * table[i]
}
