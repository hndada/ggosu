package settings

import (
	"encoding/json"
	"path/filepath"
	"testing"
)

func TestPreferencesPersistence(t *testing.T) {
	p, err := (Config{}).Profile(4)
	if err != nil {
		t.Fatal(err)
	}
	p.Keys = []string{"1", "2", "3", "4"}
	p.Scroll = 16
	p.ScrollMode = "exposure"
	p.ExposureMS = 900
	p.SpeedChange = 0
	a := Audio{Music: 0, Keys: 37, Effects: 0}
	d := Display{Dim: 0, Background: false, Storyboard: false, HUD: "focus", ComboPosition: "below", JudgementPosition: "left", TimeDisplay: "pie", WindowMode: "borderless", Letterbox: true, LetterboxWidth: 60, LetterboxX: -50, CursorSize: 150}
	d.Layout.Ghost = Point{X: 20, Y: -30, Scale: 125}
	c := Config{GlobalMS: 20, Profiles: map[int]Profile{4: p}, Audio: &a, Display: &d}
	path := filepath.Join(t.TempDir(), "settings.json")
	if err := Save(path, c); err != nil {
		t.Fatal(err)
	}
	got, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	q, err := got.Profile(4)
	if err != nil || q.SpeedChange != 0 || q.Keys[0] != "1" || q.ExposureMS != 900 || q.Scroll != 16 || got.AudioSettings() != a || got.DisplaySettings() != d {
		t.Fatal("lost zero values or preferences")
	}
	if _, err := got.Profile(7); err != nil {
		t.Fatal(err)
	}
	if (Audio{Music: 101}).Validate() == nil || (Display{Dim: -1}).Validate() == nil || (Display{HUD: "other"}).Validate() == nil || (Display{ComboPosition: "other"}).Validate() == nil || (Display{JudgementPosition: "other"}).Validate() == nil || (Display{TimeDisplay: "clock"}).Validate() == nil || (Display{WindowMode: "exclusive"}).Validate() == nil || (Display{LetterboxWidth: 10}).Validate() == nil || (Display{LetterboxY: 51}).Validate() == nil || (Display{CursorSize: 300}).Validate() == nil {
		t.Fatal("accepted invalid preference")
	}
	if w, h, x, y := d.Area(); w != 60 || h != 80 || x != -50 || y != 0 || d.Cursor() != 150 || (Display{}).Cursor() != 100 {
		t.Fatal("letterbox or cursor defaults", w, h, x, y)
	}
	if w, h, _, _ := (Display{LetterboxWidth: 30}).Area(); w != 100 || h != 100 {
		t.Fatal("letterbox applied while off")
	}
}

func TestTimingThresholdPersistence(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	for _, threshold := range []string{"", "kool", "cool", "good", "off"} {
		d := Display{TimingThreshold: threshold}
		if err := Save(path, Config{Display: &d}); err != nil {
			t.Fatal(err)
		}
		c, err := Load(path)
		if err != nil {
			t.Fatal(err)
		}
		if c.DisplaySettings().TimingThreshold != threshold {
			t.Fatal("lost timing threshold", threshold)
		}
	}
	for _, threshold := range []string{"miss", "COOL", "unknown"} {
		if (Display{TimingThreshold: threshold}).Validate() == nil {
			t.Fatal("accepted invalid timing threshold", threshold)
		}
	}
}

func TestAudioMasterDefaultsAndScales(t *testing.T) {
	var a Audio
	if err := json.Unmarshal([]byte(`{"music":80,"keys":40,"effects":50}`), &a); err != nil || a.Master != 100 {
		t.Fatalf("legacy audio = %+v, %v", a, err)
	}
	a.Master = 50
	if m := a.Mixed(); m.Music != 40 || m.Keys != 20 || m.Effects != 25 {
		t.Fatalf("mixed = %+v", m)
	}
}

func TestCadenceSettings(t *testing.T) {
	for _, rate := range []int{0, -1, 60, 144, 240, 360, 480, 1000} {
		d := Display{TPS: rate, FrameLimit: "unlimited"}
		if err := d.Validate(); err != nil {
			t.Fatal(err)
		}
		want := rate
		if rate == 0 {
			want = 240
		}
		if d.TickRate() != want {
			t.Fatal("wrong tick default")
		}
	}
	for _, d := range []Display{{TPS: -2}, {TPS: 29}, {TPS: 1001}, {FrameLimit: "invalid"}} {
		if d.Validate() == nil {
			t.Fatalf("accepted %+v", d)
		}
	}
}

func TestDefaultGhostTarget(t *testing.T) {
	if got := (Display{}).TargetScore(); got != 900000 {
		t.Fatal(got)
	}
	if got := (Display{GhostScore: 1000000}).TargetScore(); got != 1000000 {
		t.Fatal(got)
	}
}
