//go:build !windows

package screenshot

import (
	"os/exec"
	"path/filepath"
	"runtime"
)

// present selects the file in Finder, or opens its folder elsewhere.
func present(path string) error {
	if runtime.GOOS == "darwin" {
		return exec.Command("open", "-R", path).Start()
	}
	return exec.Command("xdg-open", filepath.Dir(path)).Start()
}
