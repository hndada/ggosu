package piano

import (
	"testing"
	"time"

	"ggosu/internal/chart"
)

func TestMusicAndKeysAreSeparate(t *testing.T) {
	c := chart.New(1)
	c.Timing = []chart.Timing{{BPM: 120, Scroll: 1, Meter: 4}}
	c.Assets = map[string]chart.Asset{"music": {Path: "music.wav", Kind: "audio"}, "key": {Path: "key.wav", Kind: "audio"}}
	c.Audio = []chart.Audio{{Sound: chart.Sound{Asset: "music", Volume: .8}}}
	c.Notes = []chart.Note{{At: time.Second, End: time.Second, Kind: "tap", Sounds: []chart.Sound{{Asset: "key", Volume: .5}}}}
	s, err := NewSession(c)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Input(time.Second, 0, true); err != nil {
		t.Fatal(err)
	}
	if len(s.Music) != 1 || s.Music[0].Asset != "music" || len(s.Sounds) != 1 || s.Sounds[0].Asset != "key" {
		t.Fatal("mixed background audio and key sounds")
	}
}
