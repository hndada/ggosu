package web

import (
	"archive/zip"
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestHomepageAndDownload(t *testing.T) {
	h, id, root := fixture(t)
	r := httptest.NewRecorder()
	h.ServeHTTP(r, httptest.NewRequest("GET", "/", nil))
	body := r.Body.String()
	for _, want := range []string{`src="/home.js"`, `href="/theme.css"`, `href="/home.css"`, `href="/about"`, `action="/play"`, "/play?chart=" + id, "Test", "1개 채보", "다운로드 준비 중"} {
		if !strings.Contains(body, want) {
			t.Errorf("homepage missing %q", want)
		}
	}
	if strings.Contains(body, "src=\"/web.js\"") {
		t.Fatal("homepage loaded gameplay bridge")
	}
	r = httptest.NewRecorder()
	h.ServeHTTP(r, httptest.NewRequest("GET", "/download/gosu-windows.zip", nil))
	if r.Code != 404 {
		t.Fatalf("unavailable download: %d", r.Code)
	}
	var archive bytes.Buffer
	z := zip.NewWriter(&archive)
	f, err := z.Create("gosu.exe")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.WriteString(f, "fixture executable"); err != nil {
		t.Fatal(err)
	}
	if err := z.Close(); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(root, "downloads")
	if err := os.Mkdir(dir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "gosu-windows.zip"), archive.Bytes(), 0644); err != nil {
		t.Fatal(err)
	}
	r = httptest.NewRecorder()
	h.ServeHTTP(r, httptest.NewRequest("GET", "/", nil))
	if !strings.Contains(r.Body.String(), `href="/download/gosu-windows.zip"`) {
		t.Fatal("available download hidden")
	}
	r = httptest.NewRecorder()
	h.ServeHTTP(r, httptest.NewRequest("GET", "/download/gosu-windows.zip", nil))
	if r.Code != 200 || !bytes.Equal(r.Body.Bytes(), archive.Bytes()) || !strings.Contains(r.Header().Get("Content-Disposition"), "attachment") {
		t.Fatal("download bytes or attachment header changed")
	}
	// Only the named bundle is served, never the surrounding directory.
	r = httptest.NewRecorder()
	h.ServeHTTP(r, httptest.NewRequest("GET", "/download/secret.zip", nil))
	if r.Code != 404 {
		t.Fatalf("unexpected download exposed: %d", r.Code)
	}
}

func TestHomepageEscapesChartMetadata(t *testing.T) {
	// Render through the registered route so URL and text escaping use the same template as production.
	h := http.NewServeMux()
	if err := homeRoutes(h, t.TempDir(), func() []entry {
		return []entry{{Path: "a&b", Title: `<script>alert(1)</script>`, Creator: `<img src=x onerror=alert(1)>`, Modified: "2026-10-09T00:00:00Z"}}
	}); err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRecorder()
	h.ServeHTTP(r, httptest.NewRequest("GET", "/", nil))
	body := r.Body.String()
	if strings.Contains(body, "<script>alert") || strings.Contains(body, "<img src=x") || !strings.Contains(body, "&lt;script&gt;") || !strings.Contains(body, "chart=a%26b") {
		t.Fatal("unsafe chart metadata in homepage", body)
	}
}
