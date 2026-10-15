package menu

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"ggosu/internal/download"
	"ggosu/internal/settings"
)

func (a *App) libraryLocked() (*download.Library, error) {
	if a.closed {
		return nil, fmt.Errorf("곡 선택창이 종료 중입니다")
	}
	if a.library != nil {
		return a.library, nil
	}
	dir, err := a.recordsDir()
	if err != nil {
		return nil, err
	}
	path, _, err := a.settingsKey(Selection{})
	if err != nil {
		return nil, err
	}
	c, err := a.loadSettings(path)
	if err != nil {
		return nil, err
	}
	a.library, err = download.New(filepath.Join(filepath.Dir(dir), "downloads"), c.ArchiveServer)
	return a.library, err
}

func (a *App) ArchiveServer() (string, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	l, err := a.libraryLocked()
	if err != nil {
		return "", err
	}
	return l.Archive(), nil
}

func (a *App) DownloadInfo() (download.Info, error) {
	server, err := a.ArchiveServer()
	return download.Info{Archive: server, CanInstall: true, CanConfigure: true}, err
}

func (a *App) SaveArchiveServer(server string) (string, error) {
	if server == "" {
		server = download.DefaultArchive
	}
	if err := download.ValidateServer(server); err != nil {
		return server, err
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	l, err := a.libraryLocked()
	if err != nil {
		return server, err
	}
	s := l.State()
	if s.Phase == "download" || s.Phase == "extract" || s.Phase == "verify" {
		return server, fmt.Errorf("설치를 마친 후 주소를 변경해 주세요")
	}
	path, _, err := a.settingsKey(Selection{})
	if err != nil {
		return server, err
	}
	c, err := a.loadSettings(path)
	if err != nil {
		return server, err
	}
	c.ArchiveServer = strings.TrimRight(server, "/")
	if err := settings.Save(path, c); err != nil {
		return server, err
	}
	if err := l.SetArchive(c.ArchiveServer); err != nil {
		return server, err
	}
	return l.Archive(), nil
}

func (a *App) SearchSongs(q download.Query) (download.Page, error) {
	a.mu.Lock()
	l, err := a.libraryLocked()
	a.mu.Unlock()
	if err != nil {
		return download.Page{}, err
	}
	return l.Search(context.Background(), q)
}

func (a *App) StartInstall(key string) (download.State, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.state.Running {
		return download.State{}, fmt.Errorf("플레이 종료 후 곡을 설치해 주세요")
	}
	l, err := a.libraryLocked()
	if err != nil {
		return download.State{}, err
	}
	dir, err := a.recordsDir()
	if err != nil {
		return download.State{}, err
	}
	root := filepath.Join(filepath.Dir(dir), "downloads")
	if err := os.MkdirAll(root, 0755); err != nil {
		return download.State{}, err
	}
	path, _, err := a.settingsKey(Selection{})
	if err != nil {
		return download.State{}, err
	}
	c, err := a.loadSettings(path)
	if err != nil {
		return download.State{}, err
	}
	registered := false
	for _, f := range c.SongFolders {
		registered = registered || strings.EqualFold(f, root)
	}
	if !registered {
		c.SongFolders = append(c.SongFolders, root)
		if err := settings.Save(path, c); err != nil {
			return download.State{}, err
		}
	}
	return l.Start(key)
}

func (a *App) InstallState() download.State {
	a.mu.Lock()
	l := a.library
	a.mu.Unlock()
	if l == nil {
		return download.State{Phase: "idle"}
	}
	return l.State()
}

func (a *App) CancelInstall() {
	a.mu.Lock()
	l := a.library
	a.mu.Unlock()
	if l != nil {
		l.Cancel()
	}
}
