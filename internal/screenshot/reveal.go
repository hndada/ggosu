package screenshot

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
)

// Reveal shows a saved screenshot selected in the system file manager, as osu!lazer's
// notification does with Storage.PresentFileExternally. Only files in Dir are opened.
func Reveal(path string) error {
	dir, err := Dir()
	if err != nil {
		return err
	}
	path = filepath.Clean(path)
	if !strings.EqualFold(filepath.Dir(path), filepath.Clean(dir)) || !strings.EqualFold(filepath.Ext(path), ".png") {
		return errors.New("screenshot: not a saved screenshot")
	}
	if _, err := os.Stat(path); err != nil {
		return err
	}
	return present(path)
}
