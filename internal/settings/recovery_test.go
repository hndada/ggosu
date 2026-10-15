package settings_test

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"ggosu/internal/settings"
)

func TestRecoveryPreservesInvalidContentAndRestoresSavedSettings(t *testing.T) {
	for _, text := range []string{`{"globalMS":`, `{"globalMS":1001}`, `{"unknown":true}`, `null`} {
		t.Run(text, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "settings.json")
			want := settings.Config{GlobalMS: 73, Name: "Player"}
			if err := settings.Save(path, want); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, []byte(text), 0600); err != nil {
				t.Fatal(err)
			}
			got, recovery, err := settings.Recover(path)
			if err != nil || recovery == nil || !recovery.Backup || got.GlobalMS != want.GlobalMS || got.Name != want.Name {
				t.Fatalf("recovery: %+v %+v %v", got, recovery, err)
			}
			original, err := os.ReadFile(recovery.Path)
			if err != nil || !bytes.Equal(original, []byte(text)) {
				t.Fatal("lost original content", err)
			}
			got, recovery, err = settings.Recover(path)
			if err != nil || recovery != nil || got.GlobalMS != want.GlobalMS {
				t.Fatal("restoration did not survive reload")
			}
		})
	}
}

func TestRecoveryDefaultsAndMissingPrimary(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	if err := os.WriteFile(path, []byte("broken"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path+".bak", []byte("also broken"), 0600); err != nil {
		t.Fatal(err)
	}
	c, r, err := settings.Recover(path)
	if err != nil || r == nil || r.Backup || c.GlobalMS != 0 {
		t.Fatalf("defaults: %+v %+v %v", c, r, err)
	}
	if err := settings.Save(path, settings.Config{GlobalMS: -50}); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	c, r, err = settings.Recover(path)
	if err != nil || r == nil || !r.Backup || r.Path != "" || c.GlobalMS != -50 {
		t.Fatalf("missing primary: %+v %+v %v", c, r, err)
	}
}

func TestValidManualChangesAreNotTampering(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	if err := settings.Save(path, settings.Config{GlobalMS: 40}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(`{"globalMS":80}`), 0600); err != nil {
		t.Fatal(err)
	}
	c, r, err := settings.Recover(path)
	if err != nil || r != nil || c.GlobalMS != 80 {
		t.Fatalf("valid edit rejected: %+v %+v %v", c, r, err)
	}
	if _, _, err := settings.Recover(t.TempDir()); err == nil {
		t.Fatal("treated a filesystem error as damaged settings")
	}
}
