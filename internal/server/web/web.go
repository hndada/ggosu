// Package web serves the browser client and normalized charts, not game ticks.
package web

import (
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"math"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"ggosu/internal/chart"
	_ "ggosu/internal/chart/bms"
	_ "ggosu/internal/chart/ojn"
	"ggosu/internal/chart/osu"
	_ "ggosu/internal/chart/sm"
	"ggosu/internal/difficulty"
	"ggosu/internal/download"
	"ggosu/internal/piano"
	"ggosu/internal/server/scores"
	"ggosu/internal/songselect"
	"ggosu/scenes"
)

//go:embed static/*
var assets embed.FS

type source struct {
	chart *chart.Chart
	path  string
}
type entry = chart.Entry

type Config struct{ Songs, WASM, Scores, Archive string }

type Handler struct {
	http.Handler
	library *download.Library
}

func (h *Handler) Close() { h.library.Close() }

func NewHandler(config Config) (*Handler, error) {
	root, wasmDir, scoreDir := config.Songs, config.WASM, config.Scores
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	current, err := scan(abs, wasmDir)
	if err != nil {
		return nil, err
	}
	var mu sync.RWMutex
	lookup := func(id string) (source, bool) {
		mu.RLock()
		defer mu.RUnlock()
		s, ok := current.sources[id]
		return s, ok
	}
	page, err := fs.ReadFile(scenes.Assets, "index.html")
	if err != nil {
		return nil, err
	}
	page = []byte(strings.Replace(string(page), `<script type="module" src="/main.js"></script>`, `<script type="module" src="/web.js"></script>`, 1))
	mux := http.NewServeMux()
	library, err := download.New(filepath.Join(abs, "downloads"), config.Archive)
	if err != nil {
		return nil, err
	}
	downloadRoutes(mux, library)
	store, err := scores.New(scoreDir, func(hash string) (*chart.Chart, bool) {
		mu.RLock()
		defer mu.RUnlock()
		for _, e := range current.entries {
			if e.Hash == hash {
				return current.sources[e.Path].chart, true
			}
		}
		return nil, false
	})
	if err != nil {
		return nil, err
	}
	mux.Handle("POST /api/scores", store)
	if err := homeRoutes(mux, wasmDir, func() []entry {
		mu.RLock()
		defer mu.RUnlock()
		return append([]entry(nil), current.entries...)
	}); err != nil {
		return nil, err
	}
	static := func(route, name, mime string) {
		mux.HandleFunc("GET "+route, func(w http.ResponseWriter, r *http.Request) {
			b, err := assets.ReadFile("static/" + name)
			if err != nil {
				http.Error(w, "missing web asset", 500)
				return
			}
			w.Header().Set("Content-Type", mime)
			w.Header().Set("Cache-Control", "no-cache")
			if _, err := w.Write(b); err != nil {
				return
			}
		})
	}
	mux.HandleFunc("GET /play", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-cache")
		if _, err := w.Write(page); err != nil {
			return
		}
	})
	static("/web.js", "web.js", "text/javascript; charset=utf-8")
	static("/about", "about.html", "text/html; charset=utf-8")
	names, err := scenes.Paths()
	if err != nil {
		return nil, err
	}
	for _, name := range names {
		mime := map[string]string{".js": "text/javascript; charset=utf-8", ".css": "text/css; charset=utf-8", ".png": "image/png", ".html": "text/html; charset=utf-8", ".woff2": "font/woff2", ".ttf": "font/ttf", ".ogg": "audio/ogg", ".txt": "text/plain; charset=utf-8"}[path.Ext(name)]
		if mime == "" {
			continue
		}
		mux.HandleFunc("GET /"+name, func(w http.ResponseWriter, r *http.Request) {
			b, err := fs.ReadFile(scenes.Assets, name)
			if err != nil {
				http.Error(w, "missing UI", 500)
				return
			}
			w.Header().Set("Content-Type", mime)
			w.Header().Set("Cache-Control", "no-cache")
			if _, err := w.Write(b); err != nil {
				return
			}
		})
	}
	static("/game.html", "game.html", "text/html; charset=utf-8")
	static("/game.css", "game.css", "text/css; charset=utf-8")
	static("/boot.js", "boot.js", "text/javascript; charset=utf-8")
	for _, name := range []string{"game.wasm", "wasm_exec.js"} {
		name := name
		mux.HandleFunc("GET /"+name, func(w http.ResponseWriter, r *http.Request) {
			if name == "game.wasm" {
				w.Header().Set("Content-Type", "application/wasm")
			}
			// Revalidate so a rebuilt game reaches browsers at once; unchanged files still answer 304.
			w.Header().Set("Cache-Control", "no-cache")
			http.ServeFile(w, r, filepath.Join(wasmDir, name))
		})
	}
	jsonResponse := func(w http.ResponseWriter, v any) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		if err := json.NewEncoder(w).Encode(v); err != nil {
			return
		}
	}
	mux.HandleFunc("GET /api/catalog", func(w http.ResponseWriter, r *http.Request) {
		fresh, err := scan(abs, wasmDir)
		if err != nil {
			http.Error(w, "cannot refresh chart catalogue", 500)
			return
		}
		mu.Lock()
		current = fresh
		mu.Unlock()
		diagnostics := make([]string, len(fresh.errors))
		for i, e := range fresh.errors {
			diagnostics[i] = strings.ReplaceAll(e, abs, "songs")
		}
		jsonResponse(w, struct {
			Entries []entry  `json:"entries"`
			Errors  []string `json:"errors"`
		}{fresh.entries, diagnostics})
	})
	mux.HandleFunc("GET /api/level/{chart}", func(w http.ResponseWriter, r *http.Request) {
		s, ok := lookup(r.PathValue("chart"))
		if !ok {
			http.NotFound(w, r)
			return
		}
		rate, err := strconv.ParseFloat(r.URL.Query().Get("rate"), 64)
		if err != nil || math.IsNaN(rate) || rate < .5 || rate > 2 {
			http.Error(w, "invalid playback rate", http.StatusBadRequest)
			return
		}
		value, err := difficulty.RateAt(s.chart, rate)
		if err != nil {
			http.Error(w, "cannot calculate chart level", http.StatusUnprocessableEntity)
			return
		}
		jsonResponse(w, value)
	})
	mux.HandleFunc("GET /api/chart/{chart}", func(w http.ResponseWriter, r *http.Request) {
		s, ok := lookup(r.PathValue("chart"))
		if !ok {
			http.NotFound(w, r)
			return
		}
		c := *s.chart
		c.Source.Path = filepath.Base(s.path)
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		if err := chart.Write(w, &c); err != nil {
			return
		}
	})
	mux.HandleFunc("GET /api/distribution/{chart}", func(w http.ResponseWriter, r *http.Request) {
		s, ok := lookup(r.PathValue("chart"))
		if !ok {
			http.NotFound(w, r)
			return
		}
		report, err := chart.AnalyzeDensity(s.chart)
		if err != nil {
			http.Error(w, "cannot analyze chart", 500)
			return
		}
		jsonResponse(w, report)
	})
	var lengthMu sync.Mutex
	mux.HandleFunc("GET /api/duration/{chart}", func(w http.ResponseWriter, r *http.Request) {
		s, ok := lookup(r.PathValue("chart"))
		if !ok {
			http.NotFound(w, r)
			return
		}
		lengthMu.Lock()
		defer lengthMu.Unlock()
		if r.Context().Err() != nil {
			return
		}
		cache := filepath.Join(wasmDir, "audio-lengths.json")
		lengths, err := chart.LoadAudioLengths(cache)
		if err != nil {
			http.Error(w, "cannot read audio length cache", 500)
			return
		}
		end, err := lengths.Duration(s.chart)
		if err != nil {
			http.Error(w, "cannot measure song duration: missing or invalid audio", http.StatusUnprocessableEntity)
			return
		}
		if err := lengths.Save(cache); err != nil {
			http.Error(w, "cannot save audio length cache", 500)
			return
		}
		jsonResponse(w, end)
	})
	mux.HandleFunc("POST /api/export/{chart}", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Gosu-Editor") != "1" || (r.Header.Get("Sec-Fetch-Site") != "" && r.Header.Get("Sec-Fetch-Site") != "same-origin") {
			http.Error(w, "editor request required", http.StatusForbidden)
			return
		}
		s, ok := lookup(r.PathValue("chart"))
		if !ok {
			http.NotFound(w, r)
			return
		}
		draft, err := chart.Read(http.MaxBytesReader(w, r.Body, 64<<20))
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		path, dropped, err := osu.SaveEdit(s.path, s.chart, draft)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		jsonResponse(w, map[string]any{"path": filepath.Base(path), "dropped": dropped})
	})
	mux.HandleFunc("POST /api/edit/{chart}", func(w http.ResponseWriter, r *http.Request) {
		// Only this site's editor can write local copies through a browser.
		if r.Header.Get("X-Gosu-Editor") != "1" || (r.Header.Get("Sec-Fetch-Site") != "" && r.Header.Get("Sec-Fetch-Site") != "same-origin") {
			http.Error(w, "editor request required", http.StatusForbidden)
			return
		}
		s, ok := lookup(r.PathValue("chart"))
		if !ok {
			http.NotFound(w, r)
			return
		}
		draft, err := chart.Read(http.MaxBytesReader(w, r.Body, 64<<20))
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		path, err := chart.SaveEdit(s.path, s.chart, draft)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		jsonResponse(w, filepath.Base(path))
	})
	mux.HandleFunc("GET /api/asset/{chart}", func(w http.ResponseWriter, r *http.Request) {
		s, ok := lookup(r.PathValue("chart"))
		if !ok {
			http.NotFound(w, r)
			return
		}
		if r.URL.Query().Get("preview") == "1" {
			id := r.URL.Query().Get("id")
			if a, ok := s.chart.Assets[id]; ok && a.Kind == "image" {
				name, mod, image, err := chart.OpenImage(s.chart, id)
				if err != nil {
					http.NotFound(w, r)
					return
				}
				http.ServeContent(w, r, name, mod, image)
				if err := image.Close(); err != nil {
					log.Printf("close image preview: %v", err)
				}
				return
			}
			serveAudio(w, r, s.chart, r.URL.Query().Get("id"))
			return
		}
		a, ok := s.chart.Assets[r.URL.Query().Get("id")]
		if !ok {
			http.NotFound(w, r)
			return
		}
		path, err := chart.AssetPath(s.path, a.Path)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		info, err := os.Stat(path)
		if err != nil || !info.Mode().IsRegular() || info.Size() > 256<<20 {
			http.Error(w, "asset unavailable or too large", 404)
			return
		}
		http.ServeFile(w, r, path)
	})
	mux.HandleFunc("POST /api/validate", func(w http.ResponseWriter, r *http.Request) {
		body := http.MaxBytesReader(w, r.Body, 128<<20)
		saved, err := piano.DecodeRecord(body)
		stateErr := err
		if err != nil && !errors.Is(err, piano.ErrRecordState) {
			http.Error(w, err.Error(), 400)
			return
		}
		mu.RLock()
		var canonical *chart.Chart
		for _, e := range current.entries {
			if e.Hash == saved.Replay.ChartHash {
				canonical = current.sources[e.Path].chart
				break
			}
		}
		mu.RUnlock()
		if canonical == nil {
			if stateErr != nil {
				http.Error(w, stateErr.Error(), http.StatusBadRequest)
				return
			}
			saved.Verification = "unavailable"
			saved.Warnings = append(saved.Warnings, "원본 채보가 없어 점수를 검증하지 못했습니다.")
		} else {
			saved, _, err = saved.Recalculate(canonical)
			if err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
		}
		jsonResponse(w, saved)
	})
	return &Handler{Handler: mux, library: library}, nil
}

type collection struct {
	sources map[string]source
	entries []entry
	errors  []string
}

func scan(abs, cache string) (*collection, error) {
	catalog, err := chart.Scan(abs, nil)
	if err != nil {
		return nil, err
	}
	if err := songselect.Rate(catalog.Entries, filepath.Join(cache, "ratings.json")); err != nil {
		catalog.Errors = append(catalog.Errors, err.Error())
	}
	sources := map[string]source{}
	files := map[string][]*chart.Chart{}
	entries := []entry{}
	for _, e := range catalog.Entries {
		cs, ok := files[e.Path]
		if !ok {
			cs, err = chart.Load(e.Path)
			if err != nil {
				return nil, err
			}
			files[e.Path] = cs
		}
		c := cs[e.Index]
		rel, err := filepath.Rel(abs, e.Path)
		if err != nil {
			return nil, err
		}
		sum := sha256.Sum256([]byte(filepath.ToSlash(rel) + fmt.Sprintf("#%d", e.Index)))
		id := hex.EncodeToString(sum[:16])
		sources[id] = source{c, e.Path}
		e.Path = id
		e.Folder = filepath.ToSlash(filepath.Dir(rel))
		entries = append(entries, e)
	}
	return &collection{sources: sources, entries: entries, errors: catalog.Errors}, nil
}

// serveAudio answers a preview request with an audio asset or bank sample of c.
func serveAudio(w http.ResponseWriter, r *http.Request, c *chart.Chart, id string) {
	name, mod, a, err := chart.OpenAudio(c, id)
	if errors.Is(err, chart.ErrSilentAudio) {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if errors.Is(err, fs.ErrNotExist) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnprocessableEntity)
		return
	}
	defer a.Close()
	http.ServeContent(w, r, name, mod, a)
}
