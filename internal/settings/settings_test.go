package settings_test

import (
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"ggosu/internal/chart"
	"ggosu/internal/piano"
	"ggosu/internal/settings"
)

func TestWindowResolution(t *testing.T) {
	for _, tt := range []struct {
		resolution    string
		width, height int
	}{
		{"", 1600, 900}, {"1024x640", 1024, 640}, {"1280x720", 1280, 720},
		{"1366x768", 1366, 768}, {"1600x900", 1600, 900}, {"1920x1080", 1920, 1080},
		{"2560x1440", 2560, 1440}, {"3840x2160", 3840, 2160},
	} {
		d := settings.Display{Resolution: tt.resolution}
		w, h := d.WindowSize()
		if err := d.Validate(); err != nil || w != tt.width || h != tt.height {
			t.Fatalf("%q: %dx%d %v", tt.resolution, w, h, err)
		}
	}
	for _, resolution := range []string{"800x600", "0x0", "1280X720", "1280x720 ", "bad"} {
		if err := (settings.Display{Resolution: resolution}).Validate(); err == nil {
			t.Fatalf("accepted %q", resolution)
		}
	}
}

func TestOffsetTimeline(t *testing.T) {
	c := chart.New(4)
	c.Timing = []chart.Timing{{BPM: 120, Scroll: 1, Meter: 4}, {At: time.Second, BPM: 180, Scroll: 2, Meter: 4}}
	c.Notes = []chart.Note{{At: time.Second, End: time.Second, Kind: "tap"}, {At: 2 * time.Second, End: 3 * time.Second, Kind: "hold", Lane: 1}}
	c.Assets["bgm"] = chart.Asset{Path: "music.wav", Kind: "audio"}
	c.Audio = []chart.Audio{{Sound: chart.Sound{Asset: "bgm", Volume: 1}}}
	c.Storyboard = []chart.StoryEvent{{At: time.Second, End: 2 * time.Second, Kind: "break"}}
	hash, err := c.Digest()
	if err != nil {
		t.Fatal(err)
	}
	for _, offset := range []settings.Offsets{{GlobalMS: 80, ChartMS: -20}, {GlobalMS: -80, ChartMS: 20}} {
		x, err := offset.Apply(c)
		if err != nil {
			t.Fatal(err)
		}
		if x.Notes[1].End != c.Notes[1].End+offset.Total() || x.Timing[1].At != c.Timing[1].At+offset.Total() {
			t.Fatal("note/scroll offset disagrees")
		}
		if !reflect.DeepEqual(x.Audio, c.Audio) || !reflect.DeepEqual(x.Storyboard, c.Storyboard) {
			t.Fatal("shifted music or storyboard")
		}
		if x.Position(x.Notes[0].At)-x.Position(time.Second+offset.Total()) != 0 {
			t.Fatal("wrong scroll position")
		}
		s, err := piano.NewSession(x)
		if err != nil {
			t.Fatal(err)
		}
		if err := s.Input(time.Second+offset.Total()-30*time.Millisecond, 0, true); err != nil {
			t.Fatal(err)
		}
		if s.Results[0].Delta != -30*time.Millisecond || s.Results[0].Grade != piano.Cool {
			t.Fatalf("incorrect corrected judgement: %+v", s.Results)
		}
		after, err := c.Digest()
		if err != nil || after != hash {
			t.Fatal("mutated source identity")
		}
	}
}

func TestPersistenceAndReset(t *testing.T) {
	path := filepath.Join(t.TempDir(), "data", "settings.json")
	c, err := settings.Load(path)
	if err != nil || c.Offsets("") != (settings.Offsets{}) {
		t.Fatal("missing file should use defaults")
	}
	a, b := strings.Repeat("a", 64), strings.Repeat("b", 64)
	c = settings.Config{GlobalMS: 40, Charts: map[string]int{a: -15, b: 30}}
	if err := settings.Save(path, c); err != nil {
		t.Fatal(err)
	}
	got, err := settings.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if got.Offsets(a) != (settings.Offsets{GlobalMS: 40, ChartMS: -15}) || got.Offsets(b).ChartMS != 30 {
		t.Fatal("lost chart separation")
	}
	delete(got.Charts, a)
	if err := settings.Save(path, got); err != nil {
		t.Fatal(err)
	}
	got, err = settings.Load(path)
	if err != nil || got.Offsets(a).ChartMS != 0 || got.Offsets(b).ChartMS != 30 {
		t.Fatal("reset removed unrelated chart")
	}
	got.GlobalMS = 1001
	if settings.Save(path, got) == nil {
		t.Fatal("saved invalid value")
	}
	got, err = settings.Load(path)
	if err != nil || got.GlobalMS != 40 {
		t.Fatal("invalid settings damaged previous file")
	}
}
