package download

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"ggosu/internal/chart"
)

func silentPackage(t *testing.T) []byte {
	t.Helper()
	c := chart.New(4)
	c.Metadata = chart.Metadata{Title: "Silent installation fixture", Artist: "Test", Difficulty: "4K"}
	c.Timing = []chart.Timing{{BPM: 120, Scroll: 1, Meter: 4}}
	c.Notes = []chart.Note{{At: time.Second, End: time.Second, Lane: 0, Kind: "tap"}}
	c.Assets = map[string]chart.Asset{"music": {Path: "music.wav", Kind: "audio"}}
	c.Audio = []chart.Audio{{Sound: chart.Sound{Asset: "music"}}}
	b, err := json.Marshal(c)
	if err != nil {
		t.Fatal(err)
	}
	wave := make([]byte, 44+960)
	copy(wave, "RIFF")
	binary.LittleEndian.PutUint32(wave[4:], uint32(len(wave)-8))
	copy(wave[8:], "WAVEfmt ")
	binary.LittleEndian.PutUint32(wave[16:], 16)
	binary.LittleEndian.PutUint16(wave[20:], 1)
	binary.LittleEndian.PutUint16(wave[22:], 1)
	binary.LittleEndian.PutUint32(wave[24:], 48000)
	binary.LittleEndian.PutUint32(wave[28:], 96000)
	binary.LittleEndian.PutUint16(wave[32:], 2)
	binary.LittleEndian.PutUint16(wave[34:], 16)
	copy(wave[36:], "data")
	binary.LittleEndian.PutUint32(wave[40:], 960)
	return packageFiles(t, map[string][]byte{"song/chart.ggc": b, "song/music.wav": wave, "tool.exe": []byte("not executable")})
}

func packageFiles(t *testing.T, files map[string][]byte) []byte {
	t.Helper()
	var b bytes.Buffer
	z := zip.NewWriter(&b)
	for name, body := range files {
		f, err := z.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := f.Write(body); err != nil {
			t.Fatal(err)
		}
	}
	if err := z.Close(); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func waitInstall(t *testing.T, l *Library) State {
	t.Helper()
	l.mu.Lock()
	done := l.done
	l.mu.Unlock()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("installation did not finish")
	}
	return l.State()
}

func archiveFixture(t *testing.T, data []byte) (*httptest.Server, *atomic.Int32) {
	t.Helper()
	count := &atomic.Int32{}
	mux := http.NewServeMux()
	mux.HandleFunc("/api/packs", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("q") != "Silent" || r.URL.Query().Get("keys") != "4" {
			t.Error("search conditions were lost", r.URL)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"total": 1, "items": []any{map[string]any{"source": "smo", "id": "1", "title": "Silent", "keys": []int{4}, "download": "/d/smo/1", "file_name": "song.zip", "file_size": len(data)}}})
	})
	mux.HandleFunc("/d/smo/1", func(w http.ResponseWriter, r *http.Request) { count.Add(1); _, _ = w.Write(data) })
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	return server, count
}

func TestSearchDownloadInstallAndDuplicateKeepPlayableAssets(t *testing.T) {
	server, requests := archiveFixture(t, silentPackage(t))
	root := filepath.Join(t.TempDir(), "songs")
	l, err := New(root, server.URL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(l.Close)
	page, err := l.Search(context.Background(), Query{Catalog: "archive", Text: "Silent", Keys: 4})
	if err != nil || len(page.Items) != 1 || !page.Items[0].Available {
		t.Fatal(page, err)
	}
	if _, err := l.Start("arbitrary-url"); err == nil {
		t.Fatal("accepted an unsearched download")
	}
	if _, err := l.Start(page.Items[0].Key); err != nil {
		t.Fatal(err)
	}
	s := waitInstall(t, l)
	if s.Phase != "done" || s.Result.Charts != 1 {
		t.Fatal(s)
	}
	path := filepath.Join(root, "smo-1", "song", "chart.ggc")
	charts, err := chart.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	hash, err := charts[0].Digest()
	if err != nil || hash != s.Result.Hashes[0] {
		t.Fatal("moving changed chart identity", hash, err)
	}
	if _, _, audio, err := chart.OpenAudio(charts[0], "music"); err != nil {
		t.Fatal("installed music was not readable", err)
	} else if err := audio.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "smo-1", "tool.exe")); !os.IsNotExist(err) {
		t.Fatal("installed an executable")
	}
	if _, err := l.Start(page.Items[0].Key); err != nil {
		t.Fatal(err)
	}
	s = waitInstall(t, l)
	if s.Phase != "done" || !s.Result.Already || requests.Load() != 1 {
		t.Fatal("duplicate redownloaded or overwrote", s, requests.Load())
	}
	files, err := os.ReadDir(root)
	if err != nil || len(files) != 1 {
		t.Fatal("staging files leaked", files, err)
	}
}

func TestInvalidPackagesNeverPublishOrEscape(t *testing.T) {
	for _, name := range []string{"../escape.ggc", `..\escape.ggc`, "/absolute.ggc", "C:/escape.ggc", "song:stream.ggc"} {
		t.Run(name, func(t *testing.T) {
			server, _ := archiveFixture(t, packageFiles(t, map[string][]byte{name: []byte("invalid")}))
			root := filepath.Join(t.TempDir(), "songs")
			l, err := New(root, server.URL)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(l.Close)
			page, err := l.Search(context.Background(), Query{Catalog: "archive", Text: "Silent", Keys: 4})
			if err != nil {
				t.Fatal(err)
			}
			if _, err := l.Start(page.Items[0].Key); err != nil {
				t.Fatal(err)
			}
			if s := waitInstall(t, l); s.Phase != "failed" {
				t.Fatal("unsafe package installed", s)
			}
			files, err := os.ReadDir(root)
			if err != nil || len(files) != 0 {
				t.Fatal("failed install left files", files, err)
			}
			if _, err := os.Stat(filepath.Join(filepath.Dir(root), "escape.ggc")); !os.IsNotExist(err) {
				t.Fatal("escaped installation root")
			}
		})
	}
}

func TestDirectSearchFiltersManiaAndBuildsTrustedDownloadURL(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v2/search" || r.URL.Query().Get("mode") != "3" || r.URL.Query().Get("query") != "Rain" {
			t.Error(r.URL)
		}
		_, _ = w.Write([]byte(`[{"id":12,"title":"Rain","beatmaps":[{"mode":"mania","cs":4,"version":"4K"}]},{"id":13,"title":"Other","beatmaps":[{"mode":"mania","cs":7}]},{"id":14,"title":"Standard","beatmaps":[{"mode":"osu","cs":4}]}]`))
	}))
	defer server.Close()
	l, err := New(t.TempDir(), "")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	l.direct = server.URL
	page, err := l.Search(context.Background(), Query{Catalog: "osu", Text: "Rain", Keys: 4})
	if err != nil || len(page.Items) != 1 || page.Items[0].ID != "12" || page.Items[0].Key != "osu:osu:12" {
		t.Fatal(page, err)
	}
	if got := l.offers[page.Items[0].Key].url; got != server.URL+"/api/d/12" {
		t.Fatal(got)
	}
}

func TestCancelCleansStagingAndCloseJoinsRequest(t *testing.T) {
	entered := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/packs" {
			_, _ = w.Write([]byte(`{"total":1,"items":[{"source":"osu","id":"1","title":"Slow","download":"/d/osu/1","file_name":"slow.osz"}]}`))
			return
		}
		w.WriteHeader(200)
		w.(http.Flusher).Flush()
		close(entered)
		<-r.Context().Done()
	}))
	defer server.Close()
	l, err := New(filepath.Join(t.TempDir(), "songs"), server.URL)
	if err != nil {
		t.Fatal(err)
	}
	page, err := l.Search(context.Background(), Query{Catalog: "archive"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := l.Start(page.Items[0].Key); err != nil {
		t.Fatal(err)
	}
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("download did not start")
	}
	l.Cancel()
	l.Close()
	if s := l.State(); s.Phase != "cancelled" {
		t.Fatal(s)
	}
	files, err := os.ReadDir(l.root)
	if err != nil || len(files) != 0 {
		t.Fatal("cancel left staging files", files, err)
	}
	if _, err := l.Start(page.Items[0].Key); err == nil {
		t.Fatal("closed library started another worker")
	}
}

func TestArchiveMissingAndUnsupportedPackagesStayUnavailable(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"total":3,"items":[{"source":"bms","id":"1","title":"Missing"},{"source":"bms","id":"2","download":"/d/bms/2","file_name":"song.rar"},{"source":"bms","id":"3","kind":"diff","download":"/d/bms/3","file_name":"diff.zip"}]}`))
	}))
	defer server.Close()
	l, err := New(t.TempDir(), server.URL)
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	page, err := l.Search(context.Background(), Query{Catalog: "archive"})
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range page.Items {
		if p.Available || strings.TrimSpace(p.Reason) == "" {
			t.Fatal(p)
		}
	}
}
