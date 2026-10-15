package menu

import (
	"path/filepath"
	"testing"

	"ggosu/internal/settings"
)

func TestLanguagePersistsAndPreservesSettings(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.json")
	want := settings.Config{Name: "Player One", GlobalMS: 15, AutoOffset: true}
	if err := settings.Save(path, want); err != nil {
		t.Fatal(err)
	}
	a := &App{dataDir: filepath.Join(dir, "records")}
	if lang, err := a.Language(); err != nil || lang != "ko" {
		t.Fatal(lang, err)
	}
	a.state.Running = true
	if lang, err := a.SaveLanguage("en"); err != nil || lang != "en" {
		t.Fatal(lang, err)
	}
	b := &App{dataDir: a.dataDir}
	if lang, err := b.Language(); err != nil || lang != "en" {
		t.Fatal("language did not survive restart", lang, err)
	}
	if _, err := b.SaveLanguage("invalid"); err == nil {
		t.Fatal("accepted unsupported language")
	}
	got, err := settings.Load(path)
	if err != nil || got.Language != "en" || got.Name != want.Name || got.GlobalMS != want.GlobalMS || got.AutoOffset != want.AutoOffset {
		t.Fatal("language save changed other settings", got, err)
	}
	if _, err := b.SaveLanguage("ko"); err != nil {
		t.Fatal(err)
	}
}
