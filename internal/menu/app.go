// Package menu is the Wails menu app: song select bindings, the game process, records, settings, the editor and the lobby.
package menu

import (
	"context"
	"os/exec"
	"slices"
	"sync"

	"ggosu/internal/chart"
	"ggosu/internal/download"
	"ggosu/internal/settings"
	"ggosu/internal/songselect"
)

// App owns the song list, game process and persistent lobby. Its zero value is ready to use.
type App struct {
	library      *download.Library
	submitWake   chan struct{}
	submitDone   chan struct{}
	submitCancel context.CancelFunc
	mu           sync.Mutex
	game         *process
	warm         *exec.Cmd
	warmDone     chan struct{}
	warmPath     string
	closed       bool
	onChange     func()
	window       func(settings.Display) // Applies the menu window mode and size.
	// pickFolder opens the system folder dialog with a title; "" means cancelled.
	pickFolder func(title string) (string, error)
	songs      songselect.Songs // Own lock; taken after mu, never before.
	state      State
	dataDir    string
	dataMu     sync.Mutex // Record inspection, settings recovery and notices; after mu.
	lengthMu   sync.Mutex // Audio length cache reads and writes.
	notices    []string
	lobby      *lobby
}

// Selection keeps the menu signatures short; the type belongs to the song select.
type Selection = songselect.Selection

type State struct {
	Desktop   bool      `json:"desktop"`
	Activity  string    `json:"activity,omitempty"`
	Title     string    `json:"title,omitempty"`
	Running   bool      `json:"running"`
	Selection Selection `json:"selection"`
	Result    *Result   `json:"result"`
	Error     string    `json:"error"`
	Notices   []string  `json:"notices,omitempty"`
}

func (a *App) State() State {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.dataMu.Lock()
	defer a.dataMu.Unlock()
	s := a.state
	s.Desktop = true
	s.Notices = slices.Clone(a.notices)
	return s
}

func (a *App) Retry() (string, error) {
	a.mu.Lock()
	s := a.state.Selection
	a.mu.Unlock()
	return a.Play(s)
}

// Catalog rescans songs. Play only accepts an entry from this catalogue.
func (a *App) Catalog() (chart.Catalog, error) {
	cache, err := a.lengthsPath()
	if err != nil {
		return chart.Catalog{}, err
	}
	folders, err := a.SongFolders()
	if err != nil {
		return chart.Catalog{}, err
	}
	a.lengthMu.Lock()
	defer a.lengthMu.Unlock()
	return a.songs.Scan(cache, folders)
}

func (a *App) Distribution(s Selection) (chart.Density, error) { return a.songs.Distribution(s) }

// Level is the selected chart's calculated level at its Time Rate.
func (a *App) Level(s Selection) (float64, error) { return a.songs.Level(s) }
