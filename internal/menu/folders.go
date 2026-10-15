package menu

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"ggosu/internal/settings"
)

// Song folders are extra local folders scanned beside the songs folder, such as an O2Mania
// Music\o2jam folder. They are kept in settings.json as absolute paths.

func (a *App) SongFolders() ([]string, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	path, _, err := a.settingsKey(Selection{})
	if err != nil {
		return nil, err
	}
	c, err := a.loadSettings(path)
	return slices.Clone(c.SongFolders), err
}

// AddSongFolder accepts a path as typed or pasted, quotes and trailing slashes included.
func (a *App) AddSongFolder(folder string) ([]string, error) {
	folder = strings.Trim(strings.TrimSpace(folder), `"'`)
	if folder == "" {
		return nil, fmt.Errorf("폴더 경로를 입력해 주세요")
	}
	abs, err := filepath.Abs(folder)
	if err != nil {
		return nil, err
	}
	if info, err := os.Stat(abs); err != nil || !info.IsDir() {
		return nil, fmt.Errorf("폴더를 찾을 수 없습니다: %s", abs)
	}
	return a.editSongFolders(func(list []string) ([]string, error) {
		for _, f := range list {
			if strings.EqualFold(f, abs) {
				return nil, fmt.Errorf("이미 추가한 폴더입니다")
			}
		}
		return append(list, abs), nil
	})
}

func (a *App) RemoveSongFolder(folder string) ([]string, error) {
	return a.editSongFolders(func(list []string) ([]string, error) {
		return slices.DeleteFunc(list, func(f string) bool { return f == folder }), nil
	})
}

// PickSongFolder opens the system folder dialog; an empty result means it was cancelled.
func (a *App) PickSongFolder() (string, error) {
	a.mu.Lock()
	pick := a.pickFolder
	a.mu.Unlock()
	if pick == nil {
		return "", fmt.Errorf("폴더 선택 창을 열 수 없습니다")
	}
	return pick("곡 폴더 선택")
}

func (a *App) editSongFolders(edit func([]string) ([]string, error)) ([]string, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.closed || a.state.Running {
		return nil, fmt.Errorf("플레이 종료 후 설정을 변경해 주세요")
	}
	path, _, err := a.settingsKey(Selection{})
	if err != nil {
		return nil, err
	}
	c, err := a.loadSettings(path)
	if err != nil {
		return nil, err
	}
	list, err := edit(slices.Clone(c.SongFolders))
	if err != nil {
		return nil, err
	}
	c.SongFolders = list
	if err := settings.Save(path, c); err != nil {
		return nil, err
	}
	return slices.Clone(list), nil
}
