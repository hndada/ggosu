package chart

import (
	"testing"
	"time"
)

func TestSectionKeepsNotesAndPlaysTheRestAsBackground(t *testing.T) {
	s := func(name string) []Sound { return []Sound{{Asset: name, Volume: 1}} }
	ms := func(n int) time.Duration { return time.Duration(n) * time.Millisecond }
	c := New(2)
	c.Metadata.Title, c.Metadata.Difficulty = "t", "d"
	c.Timing = []Timing{{BPM: 120, Scroll: 1, Meter: 4}}
	for _, name := range []string{"a", "b", "m", "c", "d", "bgm", "late"} {
		c.Assets[name] = Asset{Path: name + ".wav", Kind: "audio"}
	}
	c.Notes = []Note{
		{At: ms(500), End: ms(500), Lane: 0, Kind: "tap", Sounds: s("a")},
		{At: ms(1000), End: ms(2500), Lane: 1, Kind: "hold", Sounds: s("b")},
		{At: ms(1500), End: ms(1500), Lane: 0, Kind: "mine", Sounds: s("m")},
		{At: ms(2000), End: ms(2000), Lane: 0, Kind: "tap", Sounds: s("c")},
		{At: ms(3000), End: ms(3000), Lane: 0, Kind: "tap", Sounds: s("d")},
	}
	c.Audio = []Audio{{At: 0, Sound: s("bgm")[0]}, {At: ms(2500), Sound: s("late")[0]}}

	x, err := c.Section(ms(1000), ms(2500))
	if err != nil {
		t.Fatal(err)
	}
	// The hold whose tail passes the end stays whole; the mine inside is kept as a note.
	if len(x.Notes) != 3 || x.Notes[0].At != ms(1000) || x.Notes[0].End != ms(2500) || x.Notes[2].At != ms(2000) {
		t.Fatalf("notes = %+v", x.Notes)
	}
	var got []string
	for _, a := range x.Audio {
		got = append(got, a.Sound.Asset)
	}
	// "a" before the section becomes background; "d" and "late" at or after the end are dropped.
	if len(got) != 2 || got[0] != "bgm" || got[1] != "a" {
		t.Fatalf("audio = %v", got)
	}
	if len(c.Notes) != 5 || len(c.Audio) != 2 {
		t.Fatal("source chart changed")
	}
	if _, err := c.Section(ms(3100), ms(4000)); err == nil {
		t.Fatal("empty section accepted")
	}
	if _, err := c.Section(ms(2000), ms(2000)); err == nil {
		t.Fatal("zero-length section accepted")
	}
}
