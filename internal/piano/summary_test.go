package piano

import (
	"testing"
	"time"
)

func TestSummarySeparatesVisitsAndAchievements(t *testing.T) {
	_, r := recordFixture(t)
	r.Mode = "live"
	r.Date = time.Date(2026, 10, 9, 0, 0, 0, 0, time.UTC)
	for _, change := range []func(*Record){
		func(r *Record) { r.Completed = false },
		func(r *Record) { r.Practice = true },
		func(r *Record) { r.Rate = 2 },
		func(r *Record) { r.Version = 1 },
		func(r *Record) { r.Score.Possible = 0 },
	} {
		other := r
		change(&other)
		rows := Summarize([]Record{other})
		if len(rows) != 1 || !rows[0].LastPlayed.Equal(r.Date) || rows[0].Completed || rows[0].FullCombo || rows[0].Perfect || rows[0].BestPoints != nil {
			t.Fatalf("ineligible live play: %+v", rows)
		}
	}
	for _, change := range []func(*Record){
		func(r *Record) { r.Mode = "auto" },
		func(r *Record) { r.Mode = "replay" },
		func(r *Record) { r.Replay.Rules = "other-rules" },
	} {
		other := r
		change(&other)
		if rows := Summarize([]Record{other}); len(rows) != 0 {
			t.Fatalf("non-live or incompatible play leaked: %+v", rows)
		}
	}
}

func TestSummaryPointsIncludeLaneMods(t *testing.T) {
	_, r := recordFixture(t)
	r.Mode = "live"
	for _, kind := range []string{"mirror", "random"} {
		modified := r
		modified.Replay.Mods.Kind = kind
		rows := Summarize([]Record{modified})
		if len(rows) != 1 || rows[0].BestPoints == nil || *rows[0].BestPoints != modified.Score.Points || rows[0].Completed || rows[0].FullCombo || rows[0].Perfect {
			t.Fatalf("%s points or achievements: %+v", kind, rows)
		}
		r.Score.Points = modified.Score.Points - 1
		for _, records := range [][]Record{{r, modified}, {modified, r}} {
			s := Summarize(records)[0]
			if *s.BestPoints != modified.Score.Points || !s.Completed || !s.FullCombo || !s.Perfect {
				t.Fatalf("%s mixed records: %+v", kind, s)
			}
		}
	}
}

func TestSummaryKeepsAchievementsAcrossRetries(t *testing.T) {
	_, r := recordFixture(t)
	r.Mode = "live"
	r.Date = time.Date(2026, 10, 9, 0, 0, 0, 0, time.UTC)
	latest := r
	latest.Completed = false
	latest.Score.Points = 0
	latest.Date = r.Date.Add(time.Hour)
	rows := Summarize([]Record{latest, r})
	if len(rows) != 1 || !rows[0].Completed || !rows[0].FullCombo || !rows[0].Perfect || *rows[0].BestPoints != r.Score.Points || !rows[0].LastPlayed.Equal(latest.Date) {
		t.Fatalf("summary lost an earlier achievement: %+v", rows)
	}
	if r.Score.Points != 1_100_000 {
		t.Fatal("fixture is not all KOOL")
	}
	// Full combo may contain GOOD; perfect may contain COOL but not GOOD.
	for _, grade := range []string{"cool", "good", "miss"} {
		other := r
		other.Score.Kool--
		switch grade {
		case "cool":
			other.Score.Cool++
		case "good":
			other.Score.Good++
		case "miss":
			other.Score.Miss++
			other.Score.MaxCombo--
		}
		s := Summarize([]Record{other})[0]
		if !s.Completed || s.Perfect != (grade == "cool") || s.FullCombo != (grade != "miss") {
			t.Fatalf("%s achievement: %+v", grade, s)
		}
	}
}

func TestSummaryKeepsHighestStageStillStanding(t *testing.T) {
	_, r := recordFixture(t)
	r.Mode = "live"
	stage := func(level, value int, failed bool, gauge int) Record {
		c := r
		c.Health = &Health{Level: level, Value: value, Failed: failed, Start: 3, Gauge: gauge}
		return c
	}
	cases := []struct {
		records []Record
		want    int
	}{
		{[]Record{stage(2, 500, false, HealthGauge)}, 2},
		{[]Record{stage(2, 500, false, HealthGauge), stage(1, 9000, false, HealthGauge)}, 2},
		{[]Record{stage(1, 0, false, HealthGauge)}, 0}, // stage 1 emptied but play continued
		{[]Record{stage(3, 100, false, 0)}, 0},         // older gauge table
		{[]Record{r}, 0},                               // health off
	}
	for i, tc := range cases {
		if s := Summarize(tc.records)[0]; s.Gauge != tc.want || !s.Completed {
			t.Fatalf("case %d: %+v", i, s)
		}
	}
}
