package piano

import (
	"reflect"
	"testing"
	"time"

	"ggosu/internal/chart"
	"ggosu/internal/settings"
)

func TestRecalculateDetectsConsistentForgedScore(t *testing.T) {
	c, r := recordFixture(t)
	// Everything in the claimed all-KOOL score is internally consistent, but
	// the input log contains no hits. Structural validation alone accepts it.
	r.Replay.Events = nil
	if err := r.Validate(); err != nil {
		t.Fatal(err)
	}
	x, changed, err := r.Recalculate(c)
	if err != nil || !changed || x.Score.Points != 0 || x.Score.Miss != 3 || x.Verification != "repaired" {
		t.Fatalf("forged score: %+v %v %v", x, changed, err)
	}
	if r.Score.Points != MaxScore {
		t.Fatal("mutated original record")
	}
	x, changed, err = x.Recalculate(c)
	if err != nil || changed {
		t.Fatal("repair was not stable", err)
	}
}

func TestReplayScoreOffsetsModsAndKinds(t *testing.T) {
	c := chart.New(4)
	c.Timing = []chart.Timing{{BPM: 120, Scroll: 1, Meter: 4}}
	c.Notes = []chart.Note{
		{At: -time.Second, End: -time.Second, Kind: "tap"},
		{At: time.Second, End: 2 * time.Second, Lane: 1, Kind: "hold"},
		{At: time.Second, End: time.Second, Lane: 2, Kind: "mine"},
		{At: 3 * time.Second, End: 4 * time.Second, Lane: 3, Kind: "roll"},
		{At: 5 * time.Second, End: 5 * time.Second, Lane: 2, Kind: "lift"},
		{At: 6 * time.Second, End: 6 * time.Second, Lane: 1, Kind: "fake"},
	}
	for _, kind := range []string{"", "mirror", "random", "mirror+random"} {
		mods, err := chart.NewMod(c, kind, 7)
		if err != nil {
			t.Fatal(err)
		}
		o := settings.Offsets{GlobalMS: 40, ChartMS: -10}
		shifted, err := o.Apply(c)
		if err != nil {
			t.Fatal(err)
		}
		shifted, err = mods.Apply(shifted)
		if err != nil {
			t.Fatal(err)
		}
		f, err := Auto(shifted, Rules)
		if err != nil {
			t.Fatal(err)
		}
		f.ChartHash, err = c.Digest()
		if err != nil {
			t.Fatal(err)
		}
		f.Offsets, f.Mods = o, mods
		want, err := NewSession(shifted)
		if err != nil {
			t.Fatal(err)
		}
		for _, e := range f.Events {
			if err := want.Input(e.At, int(e.Key), e.Down); err != nil {
				t.Fatal(err)
			}
		}
		if err := want.Advance(f.Duration); err != nil {
			t.Fatal(err)
		}
		got, hit, err := ReplayScore(c, f)
		if err != nil || got != want.Score || !reflect.DeepEqual(hit, want.HitError()) || got.Points != MaxScore {
			t.Fatalf("%s: %+v %v", kind, got, err)
		}
	}
}

func TestRecalculateRejectsWrongChartAndDowngradesFalseCompletion(t *testing.T) {
	c, r := recordFixture(t)
	r.Replay.Duration = time.Second + time.Nanosecond
	r.Replay.Events = r.Replay.Events[:2]
	x, changed, err := r.Recalculate(c)
	if err != nil || !changed || x.Completed || x.Score.Total != 1 {
		t.Fatalf("partial play: %+v %v", x, err)
	}
	c.Notes[0].At++
	c.Notes[0].End++
	if _, _, err := r.Recalculate(c); err == nil {
		t.Fatal("accepted changed source chart")
	}
}
