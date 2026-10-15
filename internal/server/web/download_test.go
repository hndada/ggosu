package web

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"ggosu/internal/chart"
	"ggosu/internal/download"
)

func TestWebDownloadInstallUpdatesCatalogueAndRejectsRemoteWrites(t *testing.T) {
	c := chart.New(4)
	c.Metadata.Title = "Web installed song"
	c.Timing = []chart.Timing{{BPM: 120, Scroll: 1, Meter: 4}}
	c.Notes = []chart.Note{{At: time.Second, End: time.Second, Kind: "tap"}}
	b, err := json.Marshal(c)
	if err != nil {
		t.Fatal(err)
	}
	var packed bytes.Buffer
	z := zip.NewWriter(&packed)
	f, err := z.Create("chart.ggc")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.Write(b); err != nil {
		t.Fatal(err)
	}
	if err := z.Close(); err != nil {
		t.Fatal(err)
	}
	archive := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/packs" {
			_, _ = w.Write([]byte(`{"total":1,"items":[{"source":"smo","id":"1","title":"Web fixture","download":"/d/smo/1","file_name":"fixture.zip"}]}`))
			return
		}
		_, _ = w.Write(packed.Bytes())
	}))
	defer archive.Close()
	root := t.TempDir()
	h, err := NewHandler(Config{Songs: root, WASM: root, Scores: filepath.Join(root, "scores"), Archive: archive.URL})
	if err != nil {
		t.Fatal(err)
	}
	defer h.Close()
	request := func(method, route, body, remote, host string, header bool) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest(method, route, bytes.NewBufferString(body))
		req.RemoteAddr = remote
		req.Host = host
		if header {
			req.Header.Set("X-Gosu-Install", "1")
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)
		return w
	}
	w := request("GET", "/api/download/search?catalog=archive", "", "127.0.0.1:1234", "localhost:8080", false)
	var page download.Page
	if err := json.Unmarshal(w.Body.Bytes(), &page); err != nil || len(page.Items) != 1 {
		t.Fatal(w.Code, w.Body.String(), err)
	}
	body := `{"key":"` + page.Items[0].Key + `"}`
	for _, tt := range []struct {
		remote, host string
		header       bool
	}{{"192.0.2.1:1234", "example.com", true}, {"127.0.0.1:1234", "example.com", true}, {"127.0.0.1:1234", "localhost:8080", false}} {
		if w := request("POST", "/api/download/start", body, tt.remote, tt.host, tt.header); w.Code != 403 {
			t.Fatal("unauthorized server write accepted", w.Code)
		}
	}
	w = request("POST", "/api/download/start", body, "127.0.0.1:1234", "localhost:8080", true)
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	deadline := time.Now().Add(5 * time.Second)
	var state download.State
	for time.Now().Before(deadline) {
		state = h.library.State()
		if state.Phase == "done" || state.Phase == "failed" {
			break
		}
		time.Sleep(time.Millisecond)
	}
	if state.Phase != "done" {
		t.Fatal(state)
	}
	w = request("GET", "/api/catalog", "", "127.0.0.1:1234", "localhost:8080", false)
	var catalog struct {
		Entries []entry `json:"entries"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &catalog); err != nil || len(catalog.Entries) != 1 || catalog.Entries[0].Hash != state.Result.Hashes[0] {
		t.Fatal("web install did not become playable", w.Code, w.Body.String(), err)
	}
	if _, err := os.Stat(filepath.Join(root, "downloads", "smo-1", "chart.ggc")); err != nil {
		t.Fatal(err)
	}
}
