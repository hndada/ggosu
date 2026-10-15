package web

import (
	"encoding/json"
	"io/fs"
	"net/http/httptest"
	"path"
	"regexp"
	"strings"
	"testing"

	"ggosu/internal/difficulty"
	"ggosu/scenes"
)

// Check the actual HTTP import graph, including relative imports and workers,
// so moving a shared UI module cannot silently leave the web client behind.
func TestSharedUIRoutes(t *testing.T) {
	h, _, _ := fixture(t)
	names, err := scenes.Paths()
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"web.js", "boot.js", "home.js", "game.css", "game.html", "play"} {
		names = append(names, name)
	}
	imports := regexp.MustCompile(`(?:\b(?:from|import)\s*\(?\s*|new URL\s*\(\s*)['"]([^'"]+\.js)['"]`)
	for _, name := range names {
		r := httptest.NewRecorder()
		h.ServeHTTP(r, httptest.NewRequest("GET", "/"+name, nil))
		if r.Code != 200 || r.Body.Len() == 0 || r.Header().Get("Cache-Control") != "no-cache" {
			t.Errorf("UI %s: %d, cache %q", name, r.Code, r.Header().Get("Cache-Control"))
		}
		for _, m := range imports.FindAllStringSubmatch(r.Body.String(), -1) {
			p := m[1]
			if !strings.HasPrefix(p, "/") {
				p = path.Join("/", path.Dir(name), p)
			}
			dep := httptest.NewRecorder()
			h.ServeHTTP(dep, httptest.NewRequest("GET", p, nil))
			if dep.Code != 200 || !strings.Contains(dep.Header().Get("Content-Type"), "javascript") {
				t.Errorf("%s imports unavailable %s: %d", name, p, dep.Code)
			}
		}
	}
	page := httptest.NewRecorder()
	h.ServeHTTP(page, httptest.NewRequest("GET", "/play", nil))
	want, err := fs.ReadFile(scenes.Assets, "index.html")
	if err != nil {
		t.Fatal(err)
	}
	wantPage := strings.Replace(string(want), `src="/main.js"`, `src="/web.js"`, 1)
	if page.Body.String() != wantPage {
		t.Fatal("web page diverged from the desktop menu")
	}
}

func TestCalculatedLevelRoute(t *testing.T) {
	h, id, _ := fixture(t)
	for _, rate := range []string{"0.5", "1", "1.37", "2"} {
		r := httptest.NewRecorder()
		h.ServeHTTP(r, httptest.NewRequest("GET", "/api/level/"+id+"?rate="+rate, nil))
		var value float64
		if r.Code != 200 || json.Unmarshal(r.Body.Bytes(), &value) != nil || value < 0 {
			t.Errorf("rate %s: %d %s", rate, r.Code, r.Body.String())
		}
	}
	for _, rate := range []string{"NaN", "Inf", "0.49", "2.01", "bad"} {
		r := httptest.NewRecorder()
		h.ServeHTTP(r, httptest.NewRequest("GET", "/api/level/"+id+"?rate="+rate, nil))
		if r.Code != 400 {
			t.Errorf("invalid rate %s: %d", rate, r.Code)
		}
	}
	// The catalog uses the same preset cache as desktop song selection.
	r := httptest.NewRecorder()
	h.ServeHTTP(r, httptest.NewRequest("GET", "/api/catalog", nil))
	var catalog struct{ Entries []entry }
	if err := json.Unmarshal(r.Body.Bytes(), &catalog); err != nil {
		t.Fatal(err)
	}
	for _, rate := range difficulty.Presets[1:] {
		if _, ok := catalog.Entries[0].Rates[difficulty.RateKey(rate)]; !ok {
			t.Errorf("missing calculated level at %v", rate)
		}
	}
}
