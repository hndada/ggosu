//go:build !js

package play

import (
	"os"
	"path/filepath"
	"runtime"
)

// Use the installed Windows font for Korean titles. Other platforms and a
// missing optional font fall back to scene's bundled Go font.
func sceneFont() []byte {
	if runtime.GOOS != "windows" {
		return nil
	}
	root := os.Getenv("WINDIR")
	if root == "" {
		return nil
	}
	data, err := os.ReadFile(filepath.Join(root, "Fonts", "malgun.ttf"))
	if err != nil {
		return nil
	}
	return data
}
