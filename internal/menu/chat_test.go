package menu

import (
	"path/filepath"
	"testing"

	"ggosu/internal/settings"
)

func TestChatSettingsSurviveRestartAndSaveDuringPlay(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "records")
	a := &App{dataDir: dir}
	a.state.Running = true
	want := settings.Chat{Ticker: true, AutoHide: false, MentionPopup: true, MessagePopup: false, MentionSound: true, Instant: true}
	if _, err := a.SaveChat(want); err != nil {
		t.Fatal(err)
	}
	b := &App{dataDir: dir}
	got, err := b.ChatSettings()
	if err != nil || got != want {
		t.Fatalf("chat settings after restart: %+v %v", got, err)
	}
	a.closed = true
	if _, err := a.SaveChat(settings.Chat{}); err == nil {
		t.Fatal("closed app wrote settings")
	}
}
