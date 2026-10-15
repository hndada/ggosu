package piano

import (
	"path/filepath"
	"testing"
	"time"

	"ggosu/internal/chart"
)

func TestHealthFailureStopsJudgementsAndReplays(t *testing.T) {
	c := chart.New(1)
	c.Timing = []chart.Timing{{BPM: 120, Scroll: 1, Meter: 4}}
	for i := 1; i <= 40; i++ {
		at := time.Duration(i) * time.Second
		c.Notes = append(c.Notes, chart.Note{At: at, End: at, Kind: "tap"})
	}
	for level, count := range []int{40, 25, 13, 9} {
		s, err := NewSession(c)
		if err != nil {
			t.Fatal(err)
		}
		s.Health = replayHealth(level, 0, false, 0)
		if err := s.Advance(50 * time.Second); err != nil {
			t.Fatal(err)
		}
		if len(s.Results) != count || s.Health.Failed != (level > 0) {
			t.Fatal(level, s.Health, s.Score)
		}
		replay, err := NewChartReplay(c, Rules)
		if err != nil {
			t.Fatal(err)
		}
		replay.Health, replay.Duration = level, 50*time.Second
		record := Record{Version: RecordVersion, ID: "health", Date: time.Now(), Rate: 1, Mode: "live", Replay: replay}
		got, _, err := record.Recalculate(c)
		if err != nil || got.Score != s.Score {
			t.Fatal("health replay diverged", level, err)
		}
		if got.Retained() != (level > 0) {
			t.Fatal("aborted/off or failed/zero record retention", got)
		}
		if level > 0 && (got.Health == nil || *got.Health != s.Health || got.Completed) {
			t.Fatal("failure metadata changed")
		}
	}
}

func TestHealthStagesInheritShadowValuesAndNeverPromote(t *testing.T) {
	h := NewHealth(3, 0)
	for i := 1; i <= 6; i++ {
		h.add(Miss, time.Duration(i)*time.Second)
	}
	// Stage 2 loses full damage down to 50%, then the HARD guts table softens it.
	if h.Level != 3 || h.Value != 400 || h.Values != [3]int{8200, 4000, 400} {
		t.Fatal(h)
	}
	h.add(Miss, 7*time.Second)
	if h.Level != 2 || h.Value != 3200 || h.Failed {
		t.Fatal("fallback refilled stage 2", h)
	}
	for i := 8; i <= 14; i++ {
		h.add(Miss, time.Duration(i)*time.Second)
	}
	if h.Level != 1 || h.Value != 5800 || h.Failed {
		t.Fatal("fallback refilled stage 1", h)
	}
	for i := 15; i <= 34; i++ {
		h.add(Miss, time.Duration(i)*time.Second)
	}
	if h.Level != 1 || h.Value != 0 || h.Failed {
		t.Fatal("stage 1 depletion stopped play", h)
	}
	h.add(Kool, 35*time.Second)
	if h.Value != 30 || h.Level != 1 {
		t.Fatal("empty gauge did not recover", h)
	}
	for i := 36; i <= 600; i++ {
		h.add(Kool, time.Duration(i)*time.Second)
	}
	if h.Value != 10000 || h.Level != 1 {
		t.Fatal("recovery promoted a fallen stage", h)
	}
}

func TestHealthStagesReplayAndGameOver(t *testing.T) {
	c := chart.New(1)
	c.Timing = []chart.Timing{{BPM: 120, Scroll: 1, Meter: 4}}
	for i := 1; i <= 41; i++ {
		at := time.Duration(i) * time.Second
		c.Notes = append(c.Notes, chart.Note{At: at, End: at, Kind: "tap"})
	}
	for start := 0; start <= 3; start++ {
		for fail := 0; fail <= start; fail++ {
			s, err := NewSession(c)
			if err != nil {
				t.Fatal(err)
			}
			s.Health = NewHealth(start, fail)
			if err := s.Input(41*time.Second, 0, true); err != nil {
				t.Fatal(err)
			}
			if err := s.Advance(50 * time.Second); err != nil {
				t.Fatal(err)
			}
			want := []int{41, 34, 14, 7}[fail]
			if len(s.Results) != want || s.Health.Failed != (fail > 0) {
				t.Fatal(start, fail, s.Health, len(s.Results))
			}
			replay, err := NewChartReplay(c, Rules)
			if err != nil {
				t.Fatal(err)
			}
			replay.Health, replay.HealthStages, replay.HealthFail, replay.HealthGauge = start, true, fail, s.Health.Gauge
			replay.Duration, replay.Events = 50*time.Second, []Event{{At: 41 * time.Second, Down: true}}
			record := Record{Version: RecordVersion, ID: "stages", Date: time.Now(), Rate: 1, Mode: "live", Completed: fail == 0, Replay: replay}
			got, _, err := record.Recalculate(c)
			if err != nil || got.Score != s.Score || !got.Retained() || got.Completed != (fail == 0) {
				t.Fatal(start, fail, got, err)
			}
			if start > 0 && (got.Health == nil || *got.Health != s.Health) {
				t.Fatal("stage replay diverged", got.Health, s.Health)
			}
			if err := SaveRecord(filepath.Join(t.TempDir(), "record.json"), got); err != nil {
				t.Fatal(err)
			}
			p, err := NewPlayer(replay)
			if err != nil {
				t.Fatal(err)
			}
			if p.HealthState() != NewHealth(start, fail) {
				t.Fatal("playback lost its gauge policy")
			}
		}
	}
}

func TestHealthStagesCalculateEachGradeAtItsOwnDifficulty(t *testing.T) {
	h := NewHealth(3, 0)
	h.add(Miss, time.Second)
	h.add(Good, 2*time.Second)
	h.add(Cool, 3*time.Second)
	h.add(Kool, 4*time.Second)
	if h.Values != [3]int{9775, 9030, 8321} || h.Value != 8321 {
		t.Fatal(h)
	}
}

func TestHealthStageThreeGoodsNeverEmpty(t *testing.T) {
	h := NewHealth(3, 0)
	for i := range 200 {
		h.add(Good, time.Duration(i)*time.Millisecond)
	}
	if h.Level != 3 || h.Value != 1 || h.Values != [3]int{10000, 10000, 1} {
		t.Fatal(h)
	}
	h.add(Miss, time.Second)
	if h.Level == 3 {
		t.Fatal("a miss must still empty stage 3", h)
	}
}

func TestHealthStagesWithoutGaugeKeepTheFirstTable(t *testing.T) {
	h := replayHealth(3, 0, true, 0)
	for i := 1; i <= 8; i++ {
		h.add(Miss, time.Duration(i)*time.Second)
	}
	if h.Gauge != 0 || h.Values != [3]int{6800, 3600, 400} {
		t.Fatal("older stage replays changed", h)
	}
	c := chart.New(1)
	c.Timing = []chart.Timing{{BPM: 120, Scroll: 1, Meter: 4}}
	c.Notes = []chart.Note{{At: time.Second, End: time.Second, Kind: "tap"}}
	r, err := NewChartReplay(c, Rules)
	if err != nil {
		t.Fatal(err)
	}
	r.Health, r.HealthStages, r.Duration = 3, true, 2*time.Second
	for gauge, ok := range []bool{true, true, false} {
		r.HealthGauge = gauge
		if (r.Validate() == nil) != ok {
			t.Fatal("gauge table validation", gauge, r.Validate())
		}
	}
}
