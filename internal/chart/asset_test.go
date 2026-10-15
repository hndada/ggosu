package chart

import (
	"os"
	"path/filepath"
	"testing"
)

func TestAssetPaths(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "ok.wav"), []byte("sample"), 0644); err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(root, "a.osu")
	if _, err := AssetPath(source, "ok.wav"); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"../secret", "..\\secret", "C:\\secret", "/secret", "ok.wav:stream", ""} {
		if _, err := AssetPath(source, name); err == nil {
			t.Errorf("accepted %q", name)
		}
	}
}
