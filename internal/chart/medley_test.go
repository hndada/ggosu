package chart

import (
	"slices"
	"testing"
	"time"
)

// medleySong is 60 bars at 120 BPM (2 s bars) with a backing track at 0, one
// note a beat, and extra notes in every beat of bars [chorus, chorus+16).
func medleySong(title string, chorus, extra int) *Chart {
	c := &Chart{Version: Version, Lanes: 4, Metadata: Metadata{Title: title},
		Source: Source{Path: "/songs/" + title + "/x.osu"},
		Timing: []Timing{{At: 0, BPM: 120, Scroll: 1, Meter: 4}},
		Assets: map[string]Asset{"bgm": {Path: "audio.ogg", Kind: "audio"}},
		Audio:  []Audio{{At: 0, Sound: Sound{Asset: "bgm", Volume: 1}}}}
	beat := 500 * time.Millisecond
	for i := range 240 {
		at := time.Duration(i) * beat
		c.Notes = append(c.Notes, Note{At: at, End: at, Lane: 0, Kind: "tap"})
		if bar := i / 4; bar >= chorus && bar < chorus+16 {
			for l := 1; l <= extra; l++ {
				c.Notes = append(c.Notes, Note{At: at, End: at, Lane: l, Kind: "tap"})
			}
		}
	}
	c.Normalize()
	return c
}

func TestHighlightPrefersKiaiThenDensity(t *testing.T) {
	c := medleySong("a", 30, 2)
	bars := c.Bars(c.lastNote())
	if len(bars) != 61 || bars[1] != 2*time.Second {
		t.Fatal("bars", len(bars), bars[1])
	}
	if from, to := c.Highlight(bars); from != 30 || to != 46 {
		t.Fatal("density chorus", from, to)
	}
	c.Timing = append(c.Timing, Timing{At: 20 * time.Second, BPM: 120, Scroll: 1, Meter: 4, Highlight: true}, Timing{At: 40 * time.Second, BPM: 120, Scroll: 1, Meter: 4})
	if from, to := c.Highlight(c.Bars(c.lastNote())); from != 10 || to != 20 {
		t.Fatal("kiai chorus", from, to)
	}
}

func TestMedleyRisesAndReturnsToTheOpener(t *testing.T) {
	light, mid, heavy := medleySong("light", 20, 1), medleySong("mid", 30, 2), medleySong("heavy", 40, 3)
	parts, err := PlanMedley([]*Chart{heavy, light, mid})
	if err != nil {
		t.Fatal(err)
	}
	want := []struct {
		c        *Chart
		from, to time.Duration
	}{{light, 24 * time.Second, 56 * time.Second}, {mid, 44 * time.Second, 92 * time.Second}, {heavy, 64 * time.Second, 112 * time.Second}, {light, 96 * time.Second, 119500 * time.Millisecond}}
	for i, w := range want {
		if parts[i].Chart != w.c || parts[i].From != w.from || parts[i].To != w.to {
			t.Fatal("part", i, parts[i].Chart.Metadata.Title, parts[i].From, parts[i].To)
		}
	}
	m, err := Medley(parts)
	if err != nil {
		t.Fatal(err)
	}
	// Part two starts at 32 s; its first beat is left for listening.
	for _, n := range m.Notes {
		if n.At >= 32*time.Second && n.At < 32500*time.Millisecond {
			t.Fatal("note in the listening beat", n.At)
		}
	}
	if len(m.Audio) != 4 {
		t.Fatal("audio", m.Audio)
	}
	a := m.Audio[1]
	if a.At != 32*time.Second || a.Sound.Asset != "1/bgm" || a.Sound.From != 44*time.Second || a.Sound.FadeIn != 500*time.Millisecond {
		t.Fatal("incoming backing track", a)
	}
	// It plays its 48 s and one more beat, fading over the last bar and that beat.
	if a.Sound.Length != 48500*time.Millisecond || a.Sound.FadeOut != 2500*time.Millisecond {
		t.Fatal("outgoing clip", a.Sound)
	}
	if last := m.Audio[3].Sound; last.Length != 0 || last.FadeOut != 0 {
		t.Fatal("the ending must play out", last)
	}
	if m.Assets["3/bgm"].Source != light.Source.Path {
		t.Fatal("asset source", m.Assets)
	}
}

func TestMedleyWithFewerSongs(t *testing.T) {
	light, heavy := medleySong("light", 20, 1), medleySong("heavy", 40, 3)
	for _, cs := range [][]*Chart{{light}, {heavy, light}} {
		parts, err := PlanMedley(cs)
		if err != nil {
			t.Fatal(err)
		}
		if len(parts) != len(cs)+1 || parts[0].Chart != light || parts[len(parts)-1].Chart != light || parts[len(parts)-2].Chart != cs[0] {
			t.Fatal("parts", len(cs), parts)
		}
		if _, err := Medley(parts); err != nil {
			t.Fatal(err)
		}
	}
}

func TestScrollOnlyPointKeepsBars(t *testing.T) {
	c := &Chart{Timing: []Timing{{At: 0, BPM: 120, Scroll: 1, Meter: 4}, {At: time.Second, BPM: 120, Scroll: 2, Meter: 4}}}
	bars := c.Measures(8 * time.Second)
	if len(bars) != 4 || bars[1].At != 2*time.Second || !bars[0].Major || bars[1].Major {
		t.Fatal(bars)
	}
}

func TestVisualTimingKeepsMedleyCuts(t *testing.T) {
	c := medleySong("a", 20, 1)
	want := c.Bars(c.lastNote())
	parts, err := PlanMedley([]*Chart{c})
	if err != nil {
		t.Fatal(err)
	}
	c.Timing = append(c.Timing,
		Timing{At: time.Second, BPM: 120, Scroll: 2, Meter: 4},
		Timing{At: 3 * time.Second, BPM: 120, Scroll: 1, Meter: 4, Highlight: true},
		Timing{At: 4 * time.Second, BPM: 120, Scroll: 1, Meter: 4},
	)
	if got := c.Bars(c.lastNote()); !slices.Equal(got, want) {
		t.Fatal("visual timing shifted bars", got[:5])
	}
	got, err := PlanMedley([]*Chart{c})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(got, parts) {
		t.Fatalf("visual timing shifted cuts: got %+v, want %+v", got, parts)
	}
}
