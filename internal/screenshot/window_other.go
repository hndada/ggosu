//go:build !windows

package screenshot

import (
	"errors"
	"image"
)

// Window is only implemented on Windows.
func Window() (image.Image, error) {
	return nil, errors.New("screenshot: window capture is only supported on Windows")
}
