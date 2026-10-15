package menu

import (
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"ggosu/internal/piano"
	"ggosu/internal/settings"
)

// Player is the profile shown in song select's command bar.
type Player struct {
	Name      string  `json:"name"`
	PP        float64 `json:"pp"`
	KeysToday int     `json:"keysToday"`
}

func (a *App) Player() (Player, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	path, _, err := a.settingsKey(Selection{})
	if err != nil {
		return Player{}, err
	}
	c, err := a.loadSettings(path)
	if err != nil {
		return Player{}, err
	}
	records, err := a.records()
	if err != nil {
		return Player{}, err
	}
	return Player{Name: c.PlayerName(), PP: piano.TotalPP(records), KeysToday: keysToday(records, time.Now())}, nil
}

// keysToday counts key presses in live plays saved on now's local date.
func keysToday(records []piano.Record, now time.Time) int {
	y, m, d := now.Date()
	n := 0
	for _, r := range records {
		if ry, rm, rd := r.Date.In(now.Location()).Date(); r.Mode != "live" || ry != y || rm != m || rd != d {
			continue
		}
		for _, e := range r.Replay.Events {
			if e.Down {
				n++
			}
		}
	}
	return n
}

func (a *App) SaveName(name string) (string, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	name = strings.TrimSpace(name)
	path, _, err := a.settingsKey(Selection{})
	if err != nil {
		return "", err
	}
	c, err := a.loadSettings(path)
	if err != nil {
		return "", err
	}
	if utf8.RuneCountInString(name) > settings.NameLimit {
		return c.PlayerName(), fmt.Errorf("닉네임은 %d자까지예요", settings.NameLimit)
	}
	c.Name = name
	return c.PlayerName(), settings.Save(path, c)
}
