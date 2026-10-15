//go:build !js

package play

import (
	"path/filepath"
	"testing"
	"time"

	"ggosu/internal/settings"
)

func TestLiveHUDSettingsKeepJudgementAndPlayback(t *testing.T) {
	p := editView(t, 7)
	d := (settings.Config{}).DisplaySettings()
	g := &scene{view: p, display: d, settingsPath: filepath.Join(t.TempDir(), "settings.json"), speed: 1, position: time.Second}
	d.Layout.Score = settings.Point{X: -300, Y: 100, Scale: 150}
	d.Skin, d.NoteThickness = "o2jam", 150
	if err := settings.Save(g.settingsPath, settings.Config{Display: &d}); err != nil {
		t.Fatal(err)
	}
	score, position := p.session.Score, g.position
	if err := g.beforeUpdate(time.Second); err != nil {
		t.Fatal(err)
	}
	if p.layout.Score != d.Layout.Score || p.hudScale(scoreHUD) != 1.5 {
		t.Fatal(p.layout)
	}
	if p.noteHeight(52) != 48 || g.display.NoteThickness != 150 {
		t.Fatal("live note thickness was not applied", p.noteHeight(52))
	}
	if p.session.Score != score || g.position != position || g.speed != 1 {
		t.Fatal("HUD changed play state")
	}
}

func TestLiveChatSettingsKeepPlayback(t *testing.T) {
	p := editView(t, 4)
	g := &scene{view: p, display: (settings.Config{}).DisplaySettings(), settingsPath: filepath.Join(t.TempDir(), "settings.json"), speed: 1, position: time.Second}
	want := settings.Chat{Ticker: true, AutoHide: false, MentionPopup: true, MentionSound: true, Instant: true}
	if err := settings.Save(g.settingsPath, settings.Config{Chat: &want}); err != nil {
		t.Fatal(err)
	}
	score := p.session.Score
	if err := g.beforeUpdate(time.Second); err != nil {
		t.Fatal(err)
	}
	if g.chatPrefs != want || g.position != time.Second || g.speed != 1 || p.session.Score != score {
		t.Fatal("chat settings were not applied independently of playback")
	}
}
