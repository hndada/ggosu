package menu

import (
	"path/filepath"
	"testing"

	"ggosu/internal/piano"
	"ggosu/internal/settings"
)

func TestSettingsAcrossAppInstances(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "records")
	a := &App{dataDir: dir}
	catalog, err := a.Catalog()
	if err != nil || len(catalog.Entries) == 0 {
		t.Fatal(err)
	}
	e := catalog.Entries[0]
	s := Selection{ChartID: e.Path, Folder: e.Folder, Index: e.Index}
	want := settings.Offsets{GlobalMS: 35, ChartMS: -10}
	if _, err := a.SaveOffsets(s, want); err != nil {
		t.Fatal(err)
	}
	b := &App{dataDir: dir}
	if _, err := b.Catalog(); err != nil {
		t.Fatal(err)
	}
	got, err := b.Offsets(s)
	if err != nil || got != want {
		t.Fatalf("settings did not survive restart: %+v %v", got, err)
	}
	if _, err := b.SaveOffsets(s, settings.Offsets{GlobalMS: 35}); err != nil {
		t.Fatal(err)
	}
	got, err = b.Offsets(s)
	if err != nil || got.ChartMS != 0 {
		t.Fatal("chart reset failed")
	}
	b.state.Running = true
	if _, err := b.SaveOffsets(s, want); err == nil {
		t.Fatal("changed settings while playing")
	}
	b.state.Running = false
	if _, err := b.Offsets(Selection{ChartID: "outside.osu"}); err == nil {
		t.Fatal("allowed a chart outside the catalogue")
	}
}

func TestHUDLayoutCanSaveDuringPlayWithoutChangingWindow(t *testing.T) {
	a := &App{dataDir: filepath.Join(t.TempDir(), "records")}
	d := (settings.Config{}).DisplaySettings()
	if _, err := a.SaveDisplay(d); err != nil {
		t.Fatal(err)
	}
	a.state.Running = true
	a.window = func(settings.Display) { t.Error("live HUD save changed menu window") }
	v := d
	v.WindowMode = "fullscreen"
	v.Resolution = "1280x720"
	v.NoteThickness = 175
	v.TimingThreshold = "good"
	v.Layout.Score = settings.Point{X: -300, Y: 100, Scale: 150}
	got, err := a.SaveDisplay(v)
	if err != nil || got.Layout.Score != v.Layout.Score || got.NoteThickness != v.NoteThickness || got.TimingThreshold != v.TimingThreshold || got.WindowMode != d.WindowMode || got.Resolution != d.Resolution {
		t.Fatal(got, err)
	}
	b := &App{dataDir: a.dataDir}
	if saved, err := b.DisplaySettings(); err != nil || saved.NoteThickness != 175 || saved.TimingThreshold != "good" {
		t.Fatal("HUD preferences did not survive restart", saved, err)
	}
}

func TestResolutionPersistsAndAppliesToMenu(t *testing.T) {
	a := &App{dataDir: filepath.Join(t.TempDir(), "records")}
	var applied []settings.Display
	a.window = func(d settings.Display) { applied = append(applied, d) }
	d := (settings.Config{}).DisplaySettings()
	d.Resolution = "1280x720"
	if _, err := a.SaveDisplay(d); err != nil {
		t.Fatal(err)
	}
	b := &App{dataDir: a.dataDir}
	got, err := b.DisplaySettings()
	if err != nil || got.Resolution != d.Resolution || len(applied) != 1 || applied[0] != d {
		t.Fatal("resolution did not persist or apply", got, applied, err)
	}
	d.Dim = 40
	if _, err := a.SaveDisplay(d); err != nil {
		t.Fatal(err)
	}
	if len(applied) != 1 {
		t.Fatal("background change resized the window")
	}
	d.Resolution = "800x600"
	if _, err := a.SaveDisplay(d); err == nil {
		t.Fatal("unsupported resolution accepted")
	}
	got, err = b.DisplaySettings()
	if err != nil || got.Resolution != "1280x720" || len(applied) != 1 {
		t.Fatal("invalid resolution changed saved settings or window", got, err)
	}
}

func TestAllPreferencesPreserveOtherSettings(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "records")
	a := &App{dataDir: dir}
	p, err := a.Profile(4)
	if err != nil {
		t.Fatal(err)
	}
	p.Keys = []string{"1", "2", "3", "4"}
	p.Scroll = 18
	p.ScrollMode = "exposure"
	p.ExposureMS = 820
	p.SpeedChange = 0
	if _, err := a.SaveProfile(4, p); err != nil {
		t.Fatal(err)
	}
	audio := settings.Audio{Music: 0, Keys: 40, Effects: 0}
	display := settings.Display{Dim: 0, Background: false, Storyboard: false, HUD: "focus", ComboPosition: "below", JudgementPosition: "left", Skin: "circles"}
	playback := settings.Playback{Health: 2, HealthFail: 1, Mode: "auto", Mod: "mirror+random", Rate: 1.5, Pitch: false}
	if _, err := a.SavePlayback(playback); err != nil {
		t.Fatal(err)
	}
	if _, err := a.SaveAudio(audio); err != nil {
		t.Fatal(err)
	}
	if _, err := a.SaveDisplay(display); err != nil {
		t.Fatal(err)
	}
	if _, err := a.SaveOffsets(Selection{}, settings.Offsets{GlobalMS: 25}); err != nil {
		t.Fatal(err)
	}
	b := &App{dataDir: dir}
	gp, err := b.PlaybackSettings()
	if err != nil || gp != playback {
		t.Fatal("playback lost after other saves", gp, err)
	}
	got, err := b.Profile(4)
	if err != nil || got.Keys[0] != "1" || got.ExposureMS != 820 || got.SpeedChange != 0 {
		t.Fatal("profile lost after other saves")
	}
	// Scroll is shared: another key count reads it and adds only its own offset.
	other, err := b.Profile(7)
	if err != nil || other.ScrollMode != "exposure" || other.ExposureMS != 820 || other.Scroll != 18 || other.SpeedChange != 0 {
		t.Fatal("scroll is not shared", other)
	}
	other.ExposureOffset, other.ScrollOffset = -120, 1.5
	if _, err := b.SaveProfile(7, other); err != nil {
		t.Fatal(err)
	}
	if e := mustProfile(t, b, 7).Effective(); e.ExposureMS != 700 || e.Scroll != 19.5 {
		t.Fatal("offset not applied", e)
	}
	if e := mustProfile(t, b, 4).Effective(); e.ExposureMS != 820 || e.Scroll != 18 {
		t.Fatal("one key count's offset changed another", e)
	}
	if reset, err := b.ResetProfile(7); err != nil || reset.ExposureMS != 820 || reset.ExposureOffset != 0 || reset.SpeedChange != 0 {
		t.Fatal("reset changed the shared scroll", reset)
	}
	ga, err := b.AudioSettings()
	if err != nil || ga != audio {
		t.Fatal("audio lost")
	}
	gd, err := b.DisplaySettings()
	if err != nil || gd != display {
		t.Fatal("display lost")
	}
	if _, err := b.ResetProfile(4); err != nil {
		t.Fatal(err)
	}
	ga, err = b.AudioSettings()
	if err != nil || ga != audio {
		t.Fatal("reset damaged audio")
	}
	o, err := b.Offsets(Selection{})
	if err != nil || o.GlobalMS != 25 {
		t.Fatal("reset damaged offsets")
	}
	bad := p
	bad.Keys = []string{"D", "D", "J", "K"}
	if _, err := b.SaveProfile(4, bad); err == nil {
		t.Fatal("API saved duplicate keys")
	}
}

func TestCalibrateMovesGlobalOffsetByMeanError(t *testing.T) {
	a := &App{dataDir: filepath.Join(t.TempDir(), "records")}
	if _, err := a.SaveOffsets(Selection{}, settings.Offsets{GlobalMS: 10}); err != nil {
		t.Fatal(err)
	}
	live := piano.Record{Mode: "live", HitError: &piano.HitError{MeanMS: 31.6, Count: calibrationHits}}
	if cal, err := a.calibrate(live); cal != nil || err != nil {
		t.Fatalf("calibrated while auto offset is off: %+v %v", cal, err)
	}
	if _, err := a.SaveAutoOffset(true); err != nil {
		t.Fatal(err)
	}
	cal, err := a.calibrate(live)
	if err != nil || cal == nil || cal.FromMS != 10 || cal.ToMS != 42 {
		t.Fatalf("calibration %+v %v", cal, err)
	}
	if got, _ := a.Offsets(Selection{}); got.GlobalMS != 42 {
		t.Fatalf("global offset %d", got.GlobalMS)
	}
	for _, r := range []piano.Record{
		{Mode: "auto", HitError: live.HitError},
		{Mode: "live", Practice: true, HitError: live.HitError},
		{Mode: "live", HitError: &piano.HitError{MeanMS: 40, Count: calibrationHits - 1}},
		{Mode: "live"},
	} {
		if cal, err := a.calibrate(r); cal != nil || err != nil {
			t.Fatalf("calibrated %+v: %+v %v", r, cal, err)
		}
	}
}

func mustProfile(t *testing.T, a *App, lanes int) settings.Profile {
	t.Helper()
	p, err := a.Profile(lanes)
	if err != nil {
		t.Fatal(err)
	}
	return p
}
