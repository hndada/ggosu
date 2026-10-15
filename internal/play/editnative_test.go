//go:build !js

package play

import (
	"os"
	"path/filepath"
	"testing"

	"ggosu/internal/settings"
)

func TestHUDSavePreservesPreferencesAndFailedSaveStaysEditable(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	prefs := settings.Config{GlobalMS: 17, Name: "Player", AutoOffset: true}
	d := prefs.DisplaySettings()
	d.Dim, d.TimeDisplay = 40, "pie"
	prefs.Display = &d
	if err := settings.Save(path, prefs); err != nil {
		t.Fatal(err)
	}
	p := editView(t, 4)
	p.layout = settings.Layout{HitPosition: 500, Score: settings.Point{X: 260, Y: -70, Scale: 125}, Counts: settings.Point{X: -180, Y: 220, Scale: 150}}
	p.timeDisplay = "pie"
	p.noteThickness = 175
	g := &scene{settingsPath: path, view: p, display: d, editing: true, paused: true}
	p.editing = true
	g.commitHUD()
	got, err := settings.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if got.DisplaySettings().Layout != p.layout || got.Display.NoteThickness != 175 || got.GlobalMS != 17 || got.Name != prefs.Name || !got.AutoOffset || got.Display.Dim != 40 || got.Display.TimeDisplay != "pie" {
		t.Fatal(got)
	}
	if g.editing || p.editing || !g.paused || g.display.Layout != p.layout {
		t.Fatal("save did not return to pause with new layout")
	}
	// Damaged settings recover from the backup before applying the HUD draft.
	if err := os.WriteFile(path, []byte("broken"), 0600); err != nil {
		t.Fatal(err)
	}
	g.editing, p.editing = true, true
	g.commitHUD()
	if g.editing || p.editing {
		t.Fatal("recoverable settings blocked saving")
	}
	kept, err := filepath.Glob(path + ".invalid-*")
	if err != nil || len(kept) != 1 {
		t.Fatal("lost damaged original", err)
	}
	data, err := os.ReadFile(kept[0])
	if err != nil || string(data) != "broken" {
		t.Fatal("damaged original changed", err)
	}
	got, err = settings.Load(path)
	if err != nil || got.GlobalMS != 17 || got.DisplaySettings().Layout != p.layout {
		t.Fatal("lost backup preferences or HUD draft", err)
	}
	// Filesystem errors still leave the editor open and the draft available.
	g.settingsPath = t.TempDir()
	g.editing, p.editing = true, true
	g.commitHUD()
	if !g.editing || !p.editing || g.editor.status == "" {
		t.Fatal("failed save closed the editor")
	}
}

// The settings window's editor saves every HUD choice and then ends its game.
func TestHUDEditSessionSavesPresetsAndExits(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	prefs := settings.Config{Name: "Player"}
	d := prefs.DisplaySettings()
	d.Dim = 30
	prefs.Display = &d
	if err := settings.Save(path, prefs); err != nil {
		t.Fatal(err)
	}
	p := editView(t, 4)
	g := &scene{settingsPath: path, view: p, display: d, hudEdit: true}
	g.openEditor()
	p.layout.Combo = settings.Point{X: 40, Y: 10}
	argonWidth := p.field.layout.lanes[0].width
	g.cyclePreset(0)
	g.cyclePreset(1)
	g.cyclePreset(3)
	if !p.focus || p.comboPosition != "below" || p.layout.Combo != (settings.Point{}) {
		t.Fatal("presets did not show at once", p.focus, p.comboPosition, p.layout.Combo)
	}
	if p.skin != "triangles" || p.field.layout.lanes[0].width == argonWidth || p.field.layout.line != skinLine("triangles") {
		t.Fatal("skin did not rebuild the field", p.skin, p.field.layout.lanes[0].width)
	}
	g.commitHUD()
	got, err := settings.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if s := got.DisplaySettings(); s.HUD != "focus" || s.ComboPosition != "below" || s.Skin != "triangles" || s.Dim != 30 {
		t.Fatal(s)
	}
	if g.editing || !g.stopRequested {
		t.Fatal("closing the editor did not end the HUD edit game")
	}
}
