package play

import (
	"testing"
	"time"

	"ggosu/internal/chart"
	"ggosu/internal/piano"
)

func TestMeterUsesPlayerTiming(t *testing.T) {
	var m meter
	n := chart.Note{Kind: "tap"}
	m.add(piano.Result{Grade: piano.Cool, Delta: -30 * time.Millisecond}, n)
	m.add(piano.Result{Grade: piano.Cool, Delta: 40 * time.Millisecond}, n)
	m.add(piano.Result{Grade: piano.Kool, Tail: true}, chart.Note{Kind: "hold"})
	m.add(piano.Result{Grade: piano.Kool}, chart.Note{Kind: "mine"})
	m.add(piano.Result{Grade: piano.Miss, Delta: piano.Window}, n)
	if m.count != 2 || m.mean() != 5*time.Millisecond {
		t.Fatalf("false timing samples: %+v", m)
	}
	for i := 0; i < 100; i++ {
		m.add(piano.Result{Grade: piano.Cool, Delta: -25 * time.Millisecond}, n)
	}
	if m.count != 64 || m.mean() != -25*time.Millisecond {
		t.Fatal("meter did not keep a bounded recent window")
	}
}

func TestTimingErrorsLingerByMagnitude(t *testing.T) {
	var previous time.Duration
	for _, delta := range []time.Duration{0, 22 * time.Millisecond, 45 * time.Millisecond, 90 * time.Millisecond, piano.Window} {
		h := hit{at: time.Second, delta: delta}
		life := h.lifetime()
		if life <= previous || life != (hit{delta: -delta}).lifetime() {
			t.Fatalf("%s: asymmetric or non-increasing lifetime %s", delta, life)
		}
		if h.opacity(h.at-time.Nanosecond) != 0 || h.opacity(h.at) != 1 || h.opacity(h.at+life/2) != .5 || h.opacity(h.at+life) != 0 {
			t.Fatalf("%s: incorrect fade boundaries", delta)
		}
		previous = life
	}
	if (hit{}).lifetime() != time.Second || previous != 4*time.Second {
		t.Fatal("unexpected meter lifetime range")
	}
	if (hit{delta: 10 * piano.Window}).lifetime() != previous {
		t.Fatal("meter lifetime must remain bounded")
	}
}

func TestDenseHitsDoNotEraseLingeringErrors(t *testing.T) {
	var m meter
	n := chart.Note{Kind: "tap"}
	m.add(piano.Result{Grade: piano.Good, Delta: 90 * time.Millisecond}, n)
	for i := 1; i <= 100; i++ {
		m.add(piano.Result{Grade: piano.Kool, At: time.Duration(i) * 10 * time.Millisecond}, n)
	}
	if m.count != 64 || len(m.trails) != 101 || m.trails[0].delta != 90*time.Millisecond {
		t.Fatal("a dense sequence erased the larger error before its fade finished")
	}
	m.add(piano.Result{Grade: piano.Kool, At: 5 * time.Second}, n)
	if len(m.trails) != 1 {
		t.Fatal("expired markers were retained")
	}
}
