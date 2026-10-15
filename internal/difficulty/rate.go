package difficulty

import (
	"math"
	"strconv"

	"ggosu/internal/chart"
)

// Model names the calculation behind Rate; cached ratings from another model are recomputed.
const Model = "finger-motion-v4-poc"

// Hands splits the lanes between two hands, the extra middle lane going to the right hand
// (7K: 0,0,0,1,1,1,1).
func Hands(lanes int) []int {
	h := make([]int, lanes)
	for i := lanes / 2; i < lanes; i++ {
		h[i] = 1
	}
	return h
}

// Presets are the Time Rates the song scan rates ahead of time; other rates are rated on request.
var Presets = []float64{1, 1.1, 1.25, 1.5, 1.75, 2}

// RateKey is how a rate is written in caches and in the UI ("1.25", "2"), matching JavaScript's String(rate).
func RateKey(rate float64) string { return strconv.FormatFloat(rate, 'f', -1, 64) }

// Rate is the song select's level: Analyze at 1x with the default profile and options,
// divided by ten and kept to one decimal for display. Source chart levels are
// not inputs to this calculation and do not define its scale.
// The coefficients are uncalibrated, so the number is provisional.
func Rate(c *chart.Chart) (float64, error) { return RateAt(c, 1) }

// RateAt is Rate with the chart played at the given Time Rate.
func RateAt(c *chart.Chart, rate float64) (float64, error) {
	o := DefaultOptions()
	o.Rate = rate
	r, err := Analyze(c, NewProfile(Hands(c.Lanes)), o)
	if err != nil {
		return 0, err
	}
	return math.Round(r.Difficulty) / 10, nil
}
