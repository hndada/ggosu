package menu

import "ggosu/internal/screenshot"

// Screenshot saves the menu window as a PNG under data/screenshots (F12).
func (a *App) Screenshot() (string, error) {
	img, err := screenshot.Window()
	if err != nil {
		return "", err
	}
	return screenshot.Save(img)
}

// RevealScreenshot shows a saved screenshot in the file manager (clicking its notification).
func (a *App) RevealScreenshot(path string) error {
	return screenshot.Reveal(path)
}
