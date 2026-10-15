package menu

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/windows"
	"github.com/wailsapp/wails/v2/pkg/runtime"

	"ggosu/internal/settings"
	"ggosu/scenes"
)

// Run starts Wails for app. The binding receiver determines the JavaScript namespace.
// A command may embed App in its own binding type without duplicating UI methods.
func Run(app *App, binding any) error {
	return wails.Run(&options.App{
		Title:  "gosu",
		Width:  1600,
		Height: 900,
		// The settings dialog is laid out to fit without scrolling down to this size.
		MinWidth:    1024,
		MinHeight:   640,
		AssetServer: &assetserver.Options{Assets: scenes.Assets, Handler: http.HandlerFunc(app.serveMissing)},
		Bind:        []interface{}{binding},
		// WebView2 zooms on Ctrl+wheel and pinch by default; the window never rescales its page.
		Windows: &windows.Options{IsZoomControlEnabled: false, DisablePinchZoom: true, ZoomFactor: 1},
		OnStartup: func(ctx context.Context) {
			var windowMu sync.Mutex
			var applied *settings.Display
			app.mu.Lock()
			app.onChange = func() {
				state := app.State()
				title := "gosu"
				if state.Running && state.Title != "" {
					title = state.Title
				}
				runtime.WindowSetTitle(ctx, title)
				if !state.Running {
					if d, err := app.DisplaySettings(); err == nil {
						app.window(d)
					}
				}
				runtime.EventsEmit(ctx, "play:changed")
			}
			// Wails has no runtime frameless switch, and its fullscreen is already a
			// borderless window covering the monitor, so both modes use it here.
			app.window = func(d settings.Display) {
				windowMu.Lock()
				defer windowMu.Unlock()
				if applied != nil && applied.WindowMode == d.WindowMode && applied.Resolution == d.Resolution {
					return
				}
				if d.WindowMode == "fullscreen" || d.WindowMode == "borderless" {
					runtime.WindowFullscreen(ctx)
				} else {
					runtime.WindowUnfullscreen(ctx)
					runtime.WindowUnmaximise(ctx)
					w, h := d.WindowSize()
					runtime.WindowSetSize(ctx, w, h)
				}
				applied = &d
			}
			app.pickFolder = func(title string) (string, error) {
				return runtime.OpenDirectoryDialog(ctx, runtime.OpenDialogOptions{Title: title})
			}
			app.mu.Unlock()
			if err := app.startSubmissions(); err != nil {
				app.submissionNotice("점수 제출 대기열을 읽지 못했습니다: " + err.Error())
			}
			if d, err := app.DisplaySettings(); err == nil {
				app.window(d)
			}
		},
		OnShutdown: func(context.Context) {
			if err := app.close(); err != nil {
				fmt.Fprintln(os.Stderr, err)
			}
		},
	})
}

// serveMissing answers what the asset server did not find: chart audio and images,
// and under wails dev, which serves only scenes/ from disk, the embedded module folders.
func (a *App) serveMissing(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "/game.wasm" || r.URL.Path == "/wasm_exec.js" {
		exe, err := os.Executable()
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		name := strings.TrimPrefix(r.URL.Path, "/")
		for _, dir := range []string{filepath.Join(filepath.Dir(exe), "hud"), filepath.Join("build", "web"), filepath.Join("..", "..", "build", "web")} {
			file := filepath.Join(dir, name)
			if _, err := os.Stat(file); err == nil {
				if name == "game.wasm" {
					w.Header().Set("Content-Type", "application/wasm")
				} else {
					w.Header().Set("Content-Type", "text/javascript")
				}
				http.ServeFile(w, r, file)
				return
			}
		}
		http.Error(w, "HUD preview runtime missing; rebuild desktop", http.StatusNotFound)
		return
	}
	if r.URL.Path == "/preview/warm" {
		a.serveWarm(w, r)
		return
	}
	if strings.HasPrefix(r.URL.Path, "/editor/") || strings.HasPrefix(r.URL.Path, "/preview/") {
		a.serveAudio(w, r)
		return
	}
	switch path.Ext(r.URL.Path) {
	case ".js":
		w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
	case ".css":
		w.Header().Set("Content-Type", "text/css; charset=utf-8")
	default:
		http.NotFound(w, r)
		return
	}
	http.ServeFileFS(w, r, scenes.Assets, strings.TrimPrefix(r.URL.Path, "/"))
}
