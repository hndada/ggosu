package play

import (
	"testing"
	"time"

	"ggosu/internal/chart"
	"ggosu/internal/clock"
)

func TestPracticeSectionStartsEarlyAndLoops(t *testing.T) {
	c := chart.New(1)
	c.Timing = []chart.Timing{{BPM: 120, Scroll: 1, Meter: 4}}
	for i := range 10 {
		at := time.Duration(i+1) * time.Second
		c.Notes = append(c.Notes, chart.Note{At: at, End: at, Kind: "tap"})
	}
	g := &scene{chart: c, speed: 2, health: 3, autoplay: true, sectionStart: 4 * time.Second, sectionEnd: 6 * time.Second}
	if err := g.prepare(); err != nil {
		t.Fatal(err)
	}
	// One real second before the section at 2x is two seconds of song time.
	if -g.preroll != 2*time.Second || g.sectionStop != 8*time.Second {
		t.Fatalf("start %v, stop %v", -g.preroll, g.sectionStop)
	}
	if n := len(g.session.Chart.Notes); n != 2 {
		t.Fatalf("section keeps %d notes", n)
	}
	g.timeline = clock.NewTimeline(0)
	g.position = g.sectionStop
	if err := g.finish(10 * time.Second); err != nil {
		t.Fatal(err)
	}
	if g.position != 2*time.Second || g.finishing || g.completed || g.retries != 1 {
		t.Fatalf("after the section: position %v, finishing %v, retries %d", g.position, g.finishing, g.retries)
	}
}
