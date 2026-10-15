package web

import (
	"bytes"
	"fmt"
	"html/template"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"time"
)

type song struct {
	Title, Artist string
	Charts        []entry
}

func homeRoutes(mux *http.ServeMux, dir string, entries func() []entry) error {
	page, err := template.ParseFS(assets, "static/home.html")
	if err != nil {
		return fmt.Errorf("parse homepage: %w", err)
	}
	archive := filepath.Join(dir, "downloads", "gosu-windows.zip")
	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
		charts := entries()
		dates := make(map[string]time.Time, len(charts))
		for _, c := range charts {
			date, err := time.Parse(time.RFC3339Nano, c.Modified)
			if err != nil {
				http.Error(w, "invalid chart modification time", http.StatusInternalServerError)
				return
			}
			dates[c.Path] = date
		}
		sort.SliceStable(charts, func(i, j int) bool {
			return dates[charts[i].Path].After(dates[charts[j].Path])
		})
		var songs []song
		groups := make(map[[3]string]int)
		for _, c := range charts {
			key := [3]string{c.Folder, c.Title, c.Artist}
			i, ok := groups[key]
			if !ok {
				i = len(songs)
				groups[key] = i
				songs = append(songs, song{Title: c.Title, Artist: c.Artist})
			}
			songs[i].Charts = append(songs[i].Charts, c)
		}
		info, err := os.Stat(archive)
		data := struct {
			Songs    []song
			Total    int
			Download bool
		}{songs[:min(6, len(songs))], len(charts), err == nil && info.Mode().IsRegular()}
		var body bytes.Buffer
		if err := page.Execute(&body, data); err != nil {
			http.Error(w, "cannot render homepage", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		if _, err := body.WriteTo(w); err != nil {
			return
		}
	})
	for _, name := range []string{"home.css", "home.js"} {
		mux.HandleFunc("GET /"+name, func(w http.ResponseWriter, r *http.Request) {
			b, err := assets.ReadFile("static/" + name)
			if err != nil {
				http.Error(w, "missing homepage asset", http.StatusInternalServerError)
				return
			}
			mime := "text/javascript; charset=utf-8"
			if name == "home.css" {
				mime = "text/css; charset=utf-8"
			}
			w.Header().Set("Content-Type", mime)
			w.Header().Set("Cache-Control", "no-cache")
			if _, err := w.Write(b); err != nil {
				return
			}
		})
	}
	mux.HandleFunc("GET /download/gosu-windows.zip", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Disposition", `attachment; filename="gosu-windows.zip"`)
		http.ServeFile(w, r, archive)
	})
	return nil
}
