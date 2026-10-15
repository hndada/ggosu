package piano

import (
	"bytes"
	"encoding/json"
	"fmt"
)

const MaxScore = 1_100_000

// Points follows gosu's 70% flow, 30% precision and 10% Kool bonus.
// Earned uses 1/2000 of a per-judgement million-point unit. Integer arithmetic
// makes a full Kool score exact, independent of chart length.
type Score struct {
	Kool, Cool, Good, Miss  int
	Combo, MaxCombo         int
	Points, Total, Possible int
	Flow, Precision         int
	Earned                  int64
}

// UnmarshalJSON accepts the former Perfect/Great field names without changing
// stored scores. New output uses Kool/Cool; the numerical rules are unchanged.
func (s *Score) UnmarshalJSON(data []byte) error {
	type score Score
	var decoded score
	wire := struct {
		*score
		Kool    *int
		Cool    *int
		Perfect *int
		Great   *int
	}{score: &decoded}
	d := json.NewDecoder(bytes.NewReader(data))
	d.DisallowUnknownFields()
	if err := d.Decode(&wire); err != nil {
		return fmt.Errorf("read score: %w", err)
	}
	if wire.Kool != nil && wire.Perfect != nil || wire.Cool != nil && wire.Great != nil {
		return fmt.Errorf("read score: duplicate judgement names")
	}
	if wire.Kool != nil {
		decoded.Kool = *wire.Kool
	} else if wire.Perfect != nil {
		decoded.Kool = *wire.Perfect
	}
	if wire.Cool != nil {
		decoded.Cool = *wire.Cool
	} else if wire.Great != nil {
		decoded.Cool = *wire.Great
	}
	*s = Score(decoded)
	return nil
}

func (s Score) Accuracy() float64 {
	if s.Total == 0 {
		return 1
	}
	return (float64(s.Kool+s.Cool) + float64(s.Good)/2) / float64(s.Total)
}

func (s *Score) add(g Grade) {
	s.Total++
	var earned int64
	switch g {
	case Kool, Cool:
		s.Flow = min(50, s.Flow+1)
		s.Precision = min(20, s.Precision+1)
		earned = int64(28*s.Flow + 30*s.Precision)
		if g == Kool {
			s.Kool++
			earned += 200
		} else {
			s.Cool++
		}
	case Good:
		s.Good++
		s.Flow = min(50, s.Flow+1)
		s.Precision = 0
		earned = int64(14 * s.Flow)
	case Miss:
		s.Miss++
		s.Flow, s.Precision = 0, 0
	}
	s.Earned += earned
	if s.Possible > 0 {
		s.Points = int(s.Earned * 500 / int64(s.Possible))
	}
	if g == Miss {
		s.Combo = 0
	} else {
		s.Combo++
		s.MaxCombo = max(s.MaxCombo, s.Combo)
	}
}

// MaxPoints is the final score if every remaining judgement is KOOL.
func (s Score) MaxPoints() int {
	if s.Possible == 0 {
		return 0
	}
	k := int64(s.Possible - s.Total)
	earned := s.Earned + ramp(s.Flow, 50, k)*28 + ramp(s.Precision, 20, k)*30 + k*200
	return int(earned * 500 / int64(s.Possible))
}

// ramp sums min(limit, start+i) for i = 1..k.
func ramp(start, limit int, k int64) int64 {
	n := min(k, int64(limit-start))
	return n*int64(start) + n*(n+1)/2 + (k-n)*int64(limit)
}

func (s Score) Validate() error {
	if s.Kool < 0 || s.Cool < 0 || s.Good < 0 || s.Miss < 0 || s.Total < 0 || s.Possible < s.Total || s.Possible > 1<<31-1 ||
		s.Kool > s.Total || s.Cool > s.Total || s.Good > s.Total || s.Miss > s.Total ||
		int64(s.Total) != int64(s.Kool)+int64(s.Cool)+int64(s.Good)+int64(s.Miss) ||
		s.Combo < 0 || s.MaxCombo < s.Combo || s.MaxCombo > s.Total || s.Flow < 0 || s.Flow > 50 || s.Precision < 0 || s.Precision > 20 || s.Earned < 0 {
		return fmt.Errorf("play: invalid score state")
	}
	if s.Earned > int64(s.Kool)*2200+int64(s.Cool)*2000+int64(s.Good)*700 {
		return fmt.Errorf("play: earned score exceeds judgement weights")
	}
	points := 0
	if s.Possible > 0 {
		points = int(s.Earned * 500 / int64(s.Possible))
	}
	if s.Points != points || s.Points < 0 || s.Points > MaxScore {
		return fmt.Errorf("play: score does not match earned units")
	}
	return nil
}
