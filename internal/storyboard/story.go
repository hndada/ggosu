// Sprite commands are evaluated at any song position.
package storyboard

import (
	"math"
	"time"

	"ggosu/internal/chart"
)

type Sprite struct {
	X, Y, ScaleX, ScaleY, Rotation, Alpha, R, G, B float64
	Visible                                        bool
}

// SpriteAt is pure: a seek produces the same state as continuous playback.
func SpriteAt(s chart.StoryEvent, at time.Duration) Sprite {
	x := Sprite{X: s.X, Y: s.Y, ScaleX: 1, ScaleY: 1, Alpha: 1, R: 1, G: 1, B: 1, Visible: at >= s.At && at <= s.End}
	for _, c := range s.Commands {
		if at < c.At {
			continue
		}
		dim := 0
		switch c.Kind {
		case "F", "MX", "MY", "S", "R":
			dim = 1
		case "M", "V":
			dim = 2
		case "C":
			dim = 3
		}
		if dim == 0 || len(c.Values) < dim {
			continue
		}
		var v [3]float64
		copy(v[:dim], c.Values[:dim])
		if len(c.Values) == 2*dim {
			p := 1.0
			if c.End > c.At {
				p = min(1, max(0, float64(at-c.At)/float64(c.End-c.At)))
			}
			switch c.Easing {
			case 1:
				p = 1 - (1-p)*(1-p)
			case 2:
				p *= p
			}
			for i := 0; i < dim; i++ {
				v[i] += (c.Values[i+dim] - v[i]) * p
			}
		}
		switch c.Kind {
		case "F":
			x.Alpha = v[0]
		case "M":
			x.X, x.Y = v[0], v[1]
		case "MX":
			x.X = v[0]
		case "MY":
			x.Y = v[0]
		case "S":
			x.ScaleX, x.ScaleY = v[0], v[0]
		case "V":
			x.ScaleX, x.ScaleY = v[0], v[1]
		case "R":
			x.Rotation = v[0]
		case "C":
			x.R, x.G, x.B = v[0]/255, v[1]/255, v[2]/255
		}
	}
	x.Alpha = min(1, max(0, x.Alpha))
	if math.IsNaN(x.X) {
		x.Visible = false
	}
	return x
}
