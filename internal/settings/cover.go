package settings

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

// Cover is beatoraja's lane cover (PlayConfig): SUDDEN+ hides notes from the top, HIDDEN+ above
// the judgement line, and LIFT raises the line. Each is a fraction of the lane height that is
// kept while switched off.
type Cover struct {
	Sudden   float64 `json:"sudden"`
	Hidden   float64 `json:"hidden"`
	Lift     float64 `json:"lift"`
	SuddenOn bool    `json:"suddenOn,omitempty"`
	HiddenOn bool    `json:"hiddenOn,omitempty"`
	LiftOn   bool    `json:"liftOn,omitempty"`
}

// DefaultCover uses beatoraja's PlayConfig amounts with every cover off.
func DefaultCover() Cover { return Cover{Sudden: .2, Hidden: .1, Lift: .1} }

func (c Cover) Validate() error {
	for _, v := range []float64{c.Sudden, c.Hidden, c.Lift} {
		if math.IsNaN(v) || v < 0 || v > 1 {
			return fmt.Errorf("lane cover must be between 0 and 1")
		}
	}
	return nil
}

// LaneCover returns the profile's cover; older profiles and replays have none switched on.
func (p Profile) LaneCover() Cover {
	if p.Cover == nil {
		return DefaultCover()
	}
	return *p.Cover
}

// Amounts are the parts in use: zero for a cover that is off.
func (c Cover) Amounts() (sudden, hidden, lift float64) {
	if c.SuddenOn {
		sudden = c.Sudden
	}
	if c.HiddenOn {
		hidden = c.Hidden
	}
	if c.LiftOn {
		lift = c.Lift
	}
	return
}

// Adjust follows beatoraja's setCoverValue: SUDDEN+ moves while it is on or when LIFT and
// HIDDEN+ are both off; otherwise LIFT, then HIDDEN+. A positive step lowers the top cover and
// raises LIFT and HIDDEN+ by the same amount.
func (c *Cover) Adjust(step float64) {
	clamp := func(v float64) float64 { return min(1, max(0, math.Round(v*1000)/1000)) }
	switch {
	case c.SuddenOn || !c.LiftOn && !c.HiddenOn:
		c.Sudden = clamp(c.Sudden + step)
	case c.LiftOn:
		c.Lift = clamp(c.Lift - step)
	default:
		c.Hidden = clamp(c.Hidden - step)
	}
}

// Flag is the game's -cover value: SUDDEN+, HIDDEN+ and LIFT fractions, each followed by
// "+" when it is on, e.g. "0.2+,0.1,0.1".
func (c Cover) Flag() string {
	part := func(v float64, on bool) string {
		s := strconv.FormatFloat(v, 'f', -1, 64)
		if on {
			s += "+"
		}
		return s
	}
	return strings.Join([]string{part(c.Sudden, c.SuddenOn), part(c.Hidden, c.HiddenOn), part(c.Lift, c.LiftOn)}, ",")
}

func ParseCover(s string) (Cover, error) {
	parts := strings.Split(s, ",")
	if len(parts) != 3 {
		return Cover{}, fmt.Errorf("lane cover needs sudden,hidden,lift")
	}
	values := make([]float64, 3)
	on := make([]bool, 3)
	for i, p := range parts {
		p, on[i] = strings.CutSuffix(strings.TrimSpace(p), "+")
		v, err := strconv.ParseFloat(p, 64)
		if err != nil {
			return Cover{}, fmt.Errorf("lane cover: %w", err)
		}
		values[i] = v
	}
	c := Cover{Sudden: values[0], Hidden: values[1], Lift: values[2], SuddenOn: on[0], HiddenOn: on[1], LiftOn: on[2]}
	return c, c.Validate()
}
