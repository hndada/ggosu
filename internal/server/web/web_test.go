package web

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"ggosu/internal/chart"
)

func TestEditorCopyRoute(t *testing.T) {
	h, id, root := fixture(t)
	r := httptest.NewRecorder()
	h.ServeHTTP(r, httptest.NewRequest("GET", "/api/chart/"+id, nil))
	c, err := chart.Read(r.Body)
	if err != nil {
		t.Fatal(err)
	}
	c.Notes[0].Lane = 2
	var body bytes.Buffer
	if err := chart.Write(&body, c); err != nil {
		t.Fatal(err)
	}
	original, err := os.ReadFile(filepath.Join(root, "test.ggc"))
	if err != nil {
		t.Fatal(err)
	}
	for _, tt := range []struct {
		header, site string
		code         int
	}{{"", "", 403}, {"1", "cross-site", 403}, {"1", "same-origin", 200}} {
		r := httptest.NewRecorder()
		req := httptest.NewRequest("POST", "/api/edit/"+id, bytes.NewReader(body.Bytes()))
		req.Header.Set("X-Gosu-Editor", tt.header)
		req.Header.Set("Sec-Fetch-Site", tt.site)
		h.ServeHTTP(r, req)
		if r.Code != tt.code {
			t.Fatalf("%+v: %d %s", tt, r.Code, r.Body.String())
		}
		if r.Code == 200 {
			var path string
			if err := json.Unmarshal(r.Body.Bytes(), &path); err != nil {
				t.Fatal(err)
			}
			f, err := os.Open(filepath.Join(root, path))
			if err != nil {
				t.Fatal(err)
			}
			copy, readErr := chart.Read(f)
			closeErr := f.Close()
			if readErr != nil || closeErr != nil || copy.Notes[0].Lane != 2 {
				t.Fatal("copy did not reload", readErr, closeErr)
			}
		}
	}
	after, err := os.ReadFile(filepath.Join(root, "test.ggc"))
	if err != nil || !bytes.Equal(original, after) {
		t.Fatal("source overwritten", err)
	}
}

func TestBundledWebFontsHaveFontMIMEAndNoRemoteDependency(t *testing.T) {
	h, _, _ := fixture(t)
	for _, name := range []string{"gmarketmedium.woff2", "gmarketbold.woff2", "counter.woff2"} {
		r := httptest.NewRecorder()
		h.ServeHTTP(r, httptest.NewRequest("GET", "/ui/fonts/"+name, nil))
		if r.Code != http.StatusOK || r.Header().Get("Content-Type") != "font/woff2" || !bytes.HasPrefix(r.Body.Bytes(), []byte("wOF2")) {
			t.Fatalf("font %s is not served: %d %s", name, r.Code, r.Header().Get("Content-Type"))
		}
	}
}

func fixture(t *testing.T) (http.Handler, string, string) {
	t.Helper()
	root := t.TempDir()
	c := chart.New(4)
	c.Metadata.Title = "Test"
	c.Timing = []chart.Timing{{BPM: 120, Scroll: 1, Meter: 4}}
	c.Notes = []chart.Note{{At: 1, End: 1, Kind: "tap"}}
	c.Assets = map[string]chart.Asset{"image": {Path: "ok.png", Kind: "image"}, "escape": {Path: "../secret.png", Kind: "image"}}
	f, err := os.Create(filepath.Join(root, "test.ggc"))
	if err != nil {
		t.Fatal(err)
	}
	writeErr := chart.Write(f, c)
	closeErr := f.Close()
	if writeErr != nil || closeErr != nil {
		t.Fatal(writeErr, closeErr)
	}
	if err := os.WriteFile(filepath.Join(root, "ok.png"), []byte("image"), 0644); err != nil {
		t.Fatal(err)
	}
	h, err := NewHandler(Config{Songs: root, WASM: root, Scores: filepath.Join(root, "scores")})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(h.Close)
	r := httptest.NewRecorder()
	h.ServeHTTP(r, httptest.NewRequest("GET", "/api/catalog", nil))
	var catalog struct {
		Entries []entry `json:"entries"`
	}
	if err := json.Unmarshal(r.Body.Bytes(), &catalog); err != nil {
		t.Fatal(err)
	}
	if len(catalog.Entries) != 1 {
		t.Fatal("missing chart")
	}
	e := catalog.Entries[0]
	if strings.Contains(e.Path, root) || len(e.Hash) != 64 {
		t.Fatal("path leaked or missing digest")
	}
	return h, e.Path, root
}

func TestChartAndAssetRoutes(t *testing.T) {
	h, id, _ := fixture(t)
	for _, tt := range []struct {
		url  string
		code int
	}{{"/", 200}, {"/about", 200}, {"/play", 200}, {"/home.css", 200}, {"/home.js", 200}, {"/main.js", 200}, {"/web.js", 200}, {"/game.html", 200}, {"/boot.js", 200}, {"/api/chart/" + id, 200}, {"/api/asset/" + id + "?id=image", 200}, {"/api/asset/" + id + "?id=escape", 404}, {"/api/asset/" + id + "?id=unknown", 404}, {"/api/chart/unknown", 404}, {"/missing", 404}} {
		r := httptest.NewRecorder()
		h.ServeHTTP(r, httptest.NewRequest("GET", tt.url, nil))
		if r.Code != tt.code {
			t.Errorf("%s: got %d", tt.url, r.Code)
		}
		if tt.url == "/play" && (!strings.Contains(r.Body.String(), `src="/web.js"`) || strings.Contains(r.Body.String(), `src="/main.js"`)) {
			t.Fatal("served native bridge in web mode")
		}
		if strings.HasPrefix(tt.url, "/api/chart/") && r.Code == 200 {
			c, err := chart.Read(r.Body)
			if err != nil {
				t.Fatal(err)
			}
			if filepath.IsAbs(c.Source.Path) {
				t.Fatal("absolute source exposed")
			}
		}
	}
}
func TestRejectBadRecord(t *testing.T) {
	h, _, _ := fixture(t)
	for _, method := range []string{"GET", "POST"} {
		r := httptest.NewRecorder()
		h.ServeHTTP(r, httptest.NewRequest(method, "/api/validate", strings.NewReader(`{"version":1}`)))
		want := 405
		if method == "POST" {
			want = 400
		}
		if r.Code != want {
			t.Fatalf("%s: %d", method, r.Code)
		}
	}
}

func TestSearchAssets(t *testing.T) {
	h, _, _ := fixture(t)
	for _, path := range []string{"/main.js", "/settings/preferences.js", "/preferences/display.js", "/songlist/search.js", "/songlist/analysis.js", "/lobby/chat.css"} {
		r := httptest.NewRecorder()
		h.ServeHTTP(r, httptest.NewRequest("GET", path, nil))
		if r.Code != http.StatusOK || !strings.Contains(r.Header().Get("Content-Type"), map[bool]string{true: "css", false: "javascript"}[strings.HasSuffix(path, ".css")]) {
			t.Fatalf("UI module %s: %d %s", path, r.Code, r.Header().Get("Content-Type"))
		}
	}
}

func TestDistributionRoute(t *testing.T) {
	h, id, _ := fixture(t)
	r := httptest.NewRecorder()
	h.ServeHTTP(r, httptest.NewRequest("GET", "/api/distribution/"+id, nil))
	var body struct {
		Total int   `json:"total"`
		Lanes []int `json:"lanes"`
	}
	if err := json.Unmarshal(r.Body.Bytes(), &body); err != nil || r.Code != 200 || body.Total != 1 || len(body.Lanes) != 4 {
		t.Fatalf("distribution: %s, %v", r.Body.String(), err)
	}
	r = httptest.NewRecorder()
	h.ServeHTTP(r, httptest.NewRequest("GET", "/api/distribution/unknown", nil))
	if r.Code != 404 {
		t.Fatal("unknown chart accepted")
	}
}

func TestDurationRouteRejectsUnknownChartsAndMissingAudio(t *testing.T) {
	h, id, root := fixture(t)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/api/duration/"+id, nil))
	if w.Code != 200 || strings.TrimSpace(w.Body.String()) != "0" {
		t.Fatalf("silent chart duration: %d %s", w.Code, w.Body.String())
	}
	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/api/duration/unknown", nil))
	if w.Code != 404 {
		t.Fatal("unknown chart accepted")
	}
	f, err := os.Open(filepath.Join(root, "test.ggc"))
	if err != nil {
		t.Fatal(err)
	}
	c, err := chart.Read(f)
	if closeErr := f.Close(); err != nil || closeErr != nil {
		t.Fatal(err, closeErr)
	}
	c.Assets["missing"] = chart.Asset{Path: "missing.wav", Kind: "audio"}
	c.Audio = []chart.Audio{{Sound: chart.Sound{Asset: "missing", Volume: 1}}}
	f, err = os.Create(filepath.Join(root, "test.ggc"))
	if err != nil {
		t.Fatal(err)
	}
	err = chart.Write(f, c)
	if closeErr := f.Close(); err != nil || closeErr != nil {
		t.Fatal(err, closeErr)
	}
	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/api/catalog", nil))
	if w.Code != 200 {
		t.Fatal("refresh failed", w.Body.String())
	}
	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/api/duration/"+id, nil))
	if w.Code != 422 {
		t.Fatalf("missing audio: %d %s", w.Code, w.Body.String())
	}
}

func TestCatalogRefresh(t *testing.T) {
	h, _, root := fixture(t)
	b, err := os.ReadFile(filepath.Join(root, "test.ggc"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "new.ggc"), b, 0644); err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRecorder()
	h.ServeHTTP(r, httptest.NewRequest("GET", "/api/catalog", nil))
	var data struct {
		Entries []entry `json:"entries"`
	}
	if err := json.Unmarshal(r.Body.Bytes(), &data); err != nil {
		t.Fatal(err)
	}
	if len(data.Entries) != 2 {
		t.Fatal("refresh did not find new source chart")
	}
	for _, e := range data.Entries {
		r := httptest.NewRecorder()
		h.ServeHTTP(r, httptest.NewRequest("GET", "/api/chart/"+e.Path, nil))
		if r.Code != 200 {
			t.Fatal("new chart ID is not playable")
		}
	}
}

func TestEmbeddedImagePreviewReturnsOnlyImageRange(t *testing.T) {
	h, id, root := fixture(t)
	cs, err := chart.Load(filepath.Join(root, "test.ggc"))
	if err != nil {
		t.Fatal(err)
	}
	c := cs[0]
	c.Assets["image"] = chart.Asset{Path: "ok.png", Kind: "image", Offset: 6, Size: 5}
	data, err := json.Marshal(c)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "test.ggc"), data, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "ok.png"), []byte("prefixcovertrailer"), 0600); err != nil {
		t.Fatal(err)
	}
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/api/catalog", nil))
	r := httptest.NewRecorder()
	h.ServeHTTP(r, httptest.NewRequest("GET", "/api/asset/"+id+"?id=image&preview=1", nil))
	if r.Code != 200 || r.Body.String() != "cover" {
		t.Fatal(r.Code, r.Body.String())
	}
	r = httptest.NewRecorder()
	h.ServeHTTP(r, httptest.NewRequest("GET", "/api/asset/"+id+"?id=image", nil))
	if r.Body.String() != "prefixcovertrailer" {
		t.Fatal("raw file fetch changed; embedded game still needs source offsets")
	}
}
