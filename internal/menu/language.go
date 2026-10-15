package menu

import (
	"fmt"

	"ggosu/internal/settings"
)

func (a *App) Language() (string, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	path, _, err := a.settingsKey(Selection{})
	if err != nil {
		return "", err
	}
	c, err := a.loadSettings(path)
	if err != nil {
		return "", err
	}
	if c.Language == "" {
		return "ko", nil
	}
	return c.Language, nil
}

func (a *App) SaveLanguage(lang string) (string, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.closed {
		return "", fmt.Errorf("app is closed")
	}
	if lang != "ko" && lang != "en" {
		return "", fmt.Errorf("unsupported language %q", lang)
	}
	path, _, err := a.settingsKey(Selection{})
	if err != nil {
		return "", err
	}
	c, err := a.loadSettings(path)
	if err != nil {
		return "", err
	}
	c.Language = lang
	if err := settings.Save(path, c); err != nil {
		return "", fmt.Errorf("save language: %w", err)
	}
	return lang, nil
}
