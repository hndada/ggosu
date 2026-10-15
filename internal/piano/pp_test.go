package piano

import (
	"math"
	"testing"
	"time"
)

func TestTotalPPWeightsBestPlayPerChart(t *testing.T) {
	play := func(hash string, kool, good int) Record {
		n := kool + good
		return Record{Version: RecordVersion, Mode: "live", Completed: true, Rate: 1,
			Score:  Score{Kool: kool, Good: good, Total: n, Possible: n},
			Replay: Replay{Rules: Rules, ChartHash: hash, Duration: 10 * time.Second}}
	}
	// 100 judgements in 10 s: 10/s, so a perfect play is 100 pp.
	if pp, ok := PP(play("a", 100, 0)); !ok || math.Abs(pp-100) > 1e-9 {
		t.Fatalf("perfect play: %v %v", pp, ok)
	}
	// 90% accuracy (80 KOOL, 20 GOOD) is halfway between 80% and 100%.
	if pp, _ := PP(play("a", 80, 20)); math.Abs(pp-50) > 1e-9 {
		t.Fatalf("90%% play: %v", pp)
	}
	auto := play("a", 100, 0)
	auto.Mode = "auto"
	if _, ok := PP(auto); ok {
		t.Fatal("auto play earned pp")
	}
	got := TotalPP([]Record{play("a", 80, 20), play("a", 100, 0), play("b", 80, 20), auto})
	if want := 100 + 50*.95; math.Abs(got-want) > 1e-9 {
		t.Fatalf("total %v, want %v", got, want)
	}
}
