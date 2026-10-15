package chart_test

import (
	"bytes"
	"encoding/binary"
	"errors"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"ggosu/internal/chart"
)

// serve mirrors how desktop and web answer preview requests.
func serve(c *chart.Chart, id string, r *http.Request) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	name, mod, a, err := chart.OpenAudio(c, id)
	if errors.Is(err, chart.ErrSilentAudio) {
		w.WriteHeader(http.StatusNoContent)
		return w
	}
	if errors.Is(err, fs.ErrNotExist) {
		http.NotFound(w, r)
		return w
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnprocessableEntity)
		return w
	}
	defer a.Close()
	http.ServeContent(w, r, name, mod, a)
	return w
}

func TestOpenAudioAndBank(t *testing.T) {
	for _, name := range []string{"demo.osu", "demo.sm", "demo.bms", "demo.ojn"} {
		t.Run(name, func(t *testing.T) {
			cs, err := chart.Load(filepath.Join("..", "..", "songs", "demo", name))
			if err != nil {
				t.Fatal(err)
			}
			c := cs[0]
			for id, a := range c.Assets {
				if a.Kind != "audio" && a.Kind != "bank" {
					continue
				}
				if name == "demo.osu" && id != "music" {
					continue
				}
				w := serve(c, id, httptest.NewRequest(http.MethodGet, "/preview/audio", nil))
				if w.Code != http.StatusOK || !bytes.HasPrefix(w.Body.Bytes(), []byte("RIFF")) {
					t.Fatalf("%s: status %d, content %q", id, w.Code, w.Body.Bytes()[:min(w.Body.Len(), 64)])
				}
				request := httptest.NewRequest(http.MethodGet, "/preview/audio", nil)
				request.Header.Set("Range", "bytes=0-3")
				w = serve(c, id, request)
				if w.Code != http.StatusPartialContent || w.Body.String() != "RIFF" {
					t.Fatalf("%s: range response %d %q", id, w.Code, w.Body.String())
				}
			}
		})
	}
}

func TestRejectUnavailableAudio(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "broken.ojm"), []byte("broken"), 0600); err != nil {
		t.Fatal(err)
	}
	c := chart.New(4)
	c.Source.Path = filepath.Join(dir, "chart.ggc")
	c.Assets = map[string]chart.Asset{
		"outside": {Path: "../private.wav", Kind: "audio"},
		"image":   {Path: "cover.png", Kind: "image"},
		"missing": {Path: "missing.wav", Kind: "audio"},
		"bank":    {Path: "broken.ojm", Kind: "bank", Sample: 1},
	}
	for _, tt := range []struct {
		id   string
		code int
	}{{"unknown", 404}, {"outside", 404}, {"image", 404}, {"missing", 404}, {"bank", 422}} {
		if w := serve(c, tt.id, httptest.NewRequest(http.MethodGet, "/preview/audio", nil)); w.Code != tt.code {
			t.Errorf("%s: status %d, want %d", tt.id, w.Code, tt.code)
		}
	}
}

func TestSparseAudioBank(t *testing.T) {
	dir := t.TempDir()
	b := make([]byte, 28)
	copy(b, "M30\x00")
	if err := os.WriteFile(filepath.Join(dir, "empty.ojm"), b, 0600); err != nil {
		t.Fatal(err)
	}
	c := chart.New(7)
	c.Source.Path = filepath.Join(dir, "chart.ojn")
	c.Assets["silent"] = chart.Asset{Path: "empty.ojm", Kind: "bank", Sample: 1000}
	w := serve(c, "silent", httptest.NewRequest(http.MethodGet, "/preview/audio", nil))
	if w.Code != http.StatusNoContent || w.Body.Len() != 0 {
		t.Fatalf("empty bank slot: %d %q", w.Code, w.Body.String())
	}
	c.Assets["silent"] = chart.Asset{Path: "missing.ojm", Kind: "bank", Sample: 1000}
	if w := serve(c, "silent", httptest.NewRequest(http.MethodGet, "/preview/audio", nil)); w.Code != http.StatusNotFound {
		t.Fatalf("missing bank was treated as silence: %d", w.Code)
	}
}

func TestPartialBankKeepsCompleteAudio(t *testing.T) {
	dir := t.TempDir()
	b := make([]byte, 28+52+8+1)
	copy(b, "M30\x00")
	binary.LittleEndian.PutUint32(b[12:], 2)
	binary.LittleEndian.PutUint32(b[60:], 8)
	binary.LittleEndian.PutUint16(b[64:], 5)
	copy(b[80:], "OggSdata")
	if err := os.WriteFile(filepath.Join(dir, "partial.ojm"), b, 0600); err != nil {
		t.Fatal(err)
	}
	c := chart.New(7)
	c.Source.Path = filepath.Join(dir, "chart.ojn")
	c.Assets["complete"] = chart.Asset{Path: "partial.ojm", Kind: "bank", Sample: 1}
	c.Assets["damaged"] = chart.Asset{Path: "partial.ojm", Kind: "bank", Sample: 2}
	w := serve(c, "complete", httptest.NewRequest(http.MethodGet, "/preview/audio", nil))
	if w.Code != http.StatusOK || w.Body.String() != "OggSdata" {
		t.Fatalf("complete sample in a partial bank: %d %q", w.Code, w.Body.String())
	}
	w = serve(c, "damaged", httptest.NewRequest(http.MethodGet, "/preview/audio", nil))
	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("damaged record was silently skipped: %d %q", w.Code, w.Body.String())
	}
}
