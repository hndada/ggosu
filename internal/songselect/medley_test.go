package songselect

import (
	"testing"

	"ggosu/internal/chart"
)

func TestMedleyPicksDistinctSongsInRange(t *testing.T) {
	var l Songs
	e := func(path string, lanes int, rating, length float64) chart.Entry {
		return chart.Entry{Path: path, Lanes: lanes, Rating: rating, Length: length}
	}
	l.entries = []chart.Entry{
		e("a/1.osu", 7, 10, 120), e("a/2.osu", 7, 11, 120), // One song folder, two charts.
		e("b/1.osu", 7, 12, 120), e("c/1.osu", 7, 13, 120),
		e("d/1.osu", 4, 12, 120), e("e/1.osu", 7, 30, 120), e("f/1.osu", 7, 12, 30),
	}
	for range 20 {
		got, err := l.Medley(7, 10, 15, 3)
		if err != nil {
			t.Fatal(err)
		}
		dirs := map[byte]bool{}
		for _, g := range got {
			if g.Lanes != 7 || g.Rating > 15 || g.Length < 60 || dirs[g.Path[0]] {
				t.Fatal("bad pick", got)
			}
			dirs[g.Path[0]] = true
		}
	}
	if got, err := l.Medley(7, 12, 12, 3); err != nil || len(got) != 1 {
		t.Fatal("a lone song still makes a medley", got, err)
	}
	if _, err := l.Medley(7, 20, 25, 3); err == nil {
		t.Fatal("no songs must fail")
	}
}
