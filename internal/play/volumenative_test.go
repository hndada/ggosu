//go:build !js

package play

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/hajimehoshi/ebiten/v2"

	"ggosu/internal/settings"
)

func TestVolumeSaveSurvivesReloadAndPreservesPreferences(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	prefs := settings.Config{GlobalMS: 17, Name: "Player", AutoOffset: true}
	d := prefs.DisplaySettings()
	d.Dim = 40
	prefs.Display = &d
	if err := settings.Save(path, prefs); err != nil {
		t.Fatal(err)
	}
	g := scene{settingsPath: path, audio: prefs.AudioSettings()}
	g.volumeKey(ebiten.KeyArrowDown, time.Second)
	if err := g.saveVolume(); err != nil {
		t.Fatal(err)
	}
	got, err := settings.Load(path)
	if err != nil || got.AudioSettings() != g.audio || got.GlobalMS != 17 || got.Name != prefs.Name || !got.AutoOffset || got.Display.Dim != 40 {
		t.Fatal("volume save lost another preference or did not survive reload", got, err)
	}
	backup, err := settings.Load(path + ".bak")
	if err != nil || backup.AudioSettings() != g.audio {
		t.Fatal("volume backup did not update", err)
	}
}
