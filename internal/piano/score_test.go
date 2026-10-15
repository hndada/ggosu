package piano

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"ggosu/internal/chart"
)

func TestScoreJudgementNames(t *testing.T) {
	for _, input := range []string{`{"Perfect":3,"Great":2,"Good":1,"Miss":1}`, `{"Kool":3,"Cool":2,"Good":1,"Miss":1}`} {
		var s Score
		if err := json.Unmarshal([]byte(input), &s); err != nil {
			t.Fatal(err)
		}
		if s.Kool != 3 || s.Cool != 2 || s.Good != 1 || s.Miss != 1 {
			t.Fatalf("changed counts: %+v", s)
		}
		b, err := json.Marshal(s)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(b), "Perfect") || strings.Contains(string(b), "Great") || !strings.Contains(string(b), `"Kool":3`) || !strings.Contains(string(b), `"Cool":2`) {
			t.Fatalf("noncanonical output: %s", b)
		}
	}
	for _, input := range []string{`{"Kool":1,"Perfect":2}`, `{"Cool":1,"Great":2}`, `{"Unknown":1}`} {
		var s Score
		if err := json.Unmarshal([]byte(input), &s); err == nil {
			t.Fatalf("accepted conflicting or unknown fields: %s", input)
		}
	}
}

func TestMaximumScoreIsExact(t *testing.T) {
	for _, count := range []int{1, 3, 7, 14, 9999} {
		s := Score{Possible: count, Flow: 50, Precision: 20}
		for range count {
			s.add(Kool)
			if s.Points > MaxScore {
				t.Fatal("exceeded maximum")
			}
		}
		if s.Points != MaxScore || s.Accuracy() != 1 || s.MaxCombo != count {
			t.Fatalf("%d judgements: %+v", count, s)
		}
		if err := s.Validate(); err != nil {
			t.Fatal(err)
		}
	}
}

func TestFlowPrecisionAndBonus(t *testing.T) {
	for _, tc := range []struct {
		grades          []Grade
		points          int
		accuracy        float64
		flow, precision int
	}{
		{[]Grade{Cool, Cool, Cool, Cool}, 1_000_000, 1, 50, 20},
		{[]Grade{Good, Good, Good, Good}, 350_000, .5, 50, 0},
		{[]Grade{Miss, Miss, Miss, Miss}, 0, 0, 0, 0},
		{[]Grade{Kool, Cool, Good, Miss}, 612_500, .625, 0, 0},
		{[]Grade{Miss, Kool, Cool, Good}, 52_000, .625, 3, 0},
	} {
		s := Score{Possible: len(tc.grades), Flow: 50, Precision: 20}
		for _, g := range tc.grades {
			s.add(g)
		}
		if s.Points != tc.points || s.Accuracy() != tc.accuracy || s.Flow != tc.flow || s.Precision != tc.precision {
			t.Fatalf("%v: %+v, accuracy %.3f", tc.grades, s, s.Accuracy())
		}
		if err := s.Validate(); err != nil {
			t.Fatal(err)
		}
	}
}

func TestPartialScoreAndFactorRecovery(t *testing.T) {
	s := Score{Possible: 51, Flow: 50, Precision: 20}
	s.add(Miss)
	for i := 1; i <= 50; i++ {
		s.add(Kool)
		if s.Flow != i || s.Precision != min(i, 20) {
			t.Fatalf("recovery %d: %+v", i, s)
		}
	}
	if s.Points >= MaxScore {
		t.Fatal("recovered factors restored lost points")
	}
	p := Score{Possible: 14, Flow: 50, Precision: 20}
	p.add(Kool)
	if p.Points != 78_571 || p.Accuracy() != 1 {
		t.Fatalf("partial score divided by processed count: %+v", p)
	}
}

func TestMissScoreTrace(t *testing.T) {
	// 1000 possible judgements makes these reference increments whole points.
	s := Score{Possible: 1000, Flow: 50, Precision: 20}
	for i, tc := range []struct {
		grade                          Grade
		points, flow, precision, combo int
	}{
		{Kool, 1100, 50, 20, 1},
		{Miss, 1100, 0, 0, 0},
		{Kool, 1229, 1, 1, 1},
		{Cool, 1287, 2, 2, 2},
		{Good, 1308, 3, 0, 3},
		{Miss, 1308, 0, 0, 0},
		{Miss, 1308, 0, 0, 0},
		{Good, 1315, 1, 0, 1},
	} {
		before := s
		s.add(tc.grade)
		if s.Points != tc.points || s.Flow != tc.flow || s.Precision != tc.precision || s.Combo != tc.combo {
			t.Fatalf("step %d (%s): %+v", i+1, tc.grade, s)
		}
		if tc.grade == Miss && (s.Points != before.Points || s.Earned != before.Earned || s.MaxCombo != before.MaxCombo) {
			t.Fatalf("MISS changed earned points or best combo: before %+v, after %+v", before, s)
		}
		if err := s.Validate(); err != nil {
			t.Fatal(err)
		}
		t.Logf("%s: %d points, Flow %d, Precision %d, Combo %d", tc.grade, s.Points, s.Flow, s.Precision, s.Combo)
	}
}

func TestScoreCountsHoldTailAndIgnoresFake(t *testing.T) {
	c := chart.New(3)
	c.Timing = []chart.Timing{{BPM: 120, Scroll: 1, Meter: 4}}
	c.Notes = []chart.Note{{Kind: "tap"}, {At: time.Second, End: 3 * time.Second, Lane: 1, Kind: "hold"}, {At: 2 * time.Second, End: 2 * time.Second, Lane: 2, Kind: "fake"}}
	s, err := NewSession(c)
	if err != nil || s.Score.Possible != 3 || s.Score.Points != 0 {
		t.Fatalf("denominator: %+v, %v", s, err)
	}
	if err := s.Input(0, 0, true); err != nil {
		t.Fatal(err)
	}
	if err := s.Input(time.Second, 1, true); err != nil {
		t.Fatal(err)
	}
	if err := s.Input(3*time.Second, 1, false); err != nil {
		t.Fatal(err)
	}
	if err := s.Advance(4 * time.Second); err != nil {
		t.Fatal(err)
	}
	if s.Score.Points != MaxScore || s.Score.Total != 3 {
		t.Fatalf("hold/fake score: %+v", s.Score)
	}
}

func TestMaxPointsMatchesRemainingKools(t *testing.T) {
	for _, prefix := range [][]Grade{nil, {Kool}, {Miss, Good, Cool}, {Kool, Kool, Miss, Good, Good, Kool}} {
		for _, total := range []int{6, 40, 300} {
			s := Score{Possible: total}
			for _, g := range prefix {
				s.add(g)
			}
			want := s
			for want.Total < want.Possible {
				want.add(Kool)
			}
			if got := s.MaxPoints(); got != want.Points {
				t.Fatalf("prefix %v of %d: max %d, want %d", prefix, total, got, want.Points)
			}
		}
	}
}
