package menu

import (
	"fmt"

	"ggosu/internal/settings"
)

func (a *App) ChatSettings() (settings.Chat, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	path, _, err := a.settingsKey(Selection{})
	if err != nil {
		return settings.Chat{}, err
	}
	c, err := a.loadSettings(path)
	return c.ChatSettings(), err
}

// Chat settings can change during play; the game rereads the preferences file.
func (a *App) SaveChat(v settings.Chat) (settings.Chat, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.closed {
		return v, fmt.Errorf("앱이 종료됐습니다")
	}
	path, _, err := a.settingsKey(Selection{})
	if err != nil {
		return v, err
	}
	c, err := a.loadSettings(path)
	if err != nil {
		return v, err
	}
	c.Chat = &v
	if err := settings.Save(path, c); err != nil {
		return v, fmt.Errorf("채팅 설정 저장: %w", err)
	}
	return v, nil
}
