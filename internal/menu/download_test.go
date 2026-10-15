package menu

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

func TestSongDownloadRegistersFolderAndAppearsAfterReload(t *testing.T) {
	root := t.TempDir()
	t.Chdir(root)
	if err := os.Mkdir("songs", 0755); err != nil {
		t.Fatal(err)
	}
	c := chart.New(4)
	c.Metadata.Title = "Installed silent song"
	c.Timing = []chart.Timing{{BPM: 120, Scroll: 1, Meter: 4}}
	c.Notes = []chart.Note{{At: time.Second, End: time.Second, Kind: "tap"}}
	body, err := json.Marshal(c)
	if err != nil {
		t.Fatal(err)
	}
	var packed bytes.Buffer
	z := zip.NewWriter(&packed)
	file, err := z.Create("song/chart.ggc")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.Write(body); err != nil {
		t.Fatal(err)
	}
	if err := z.Close(); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/packs" {
			_, _ = w.Write([]byte(`{"total":1,"items":[{"source":"smo","id":"1","title":"Silent","keys":[4],"download":"/d/smo/1","file_name":"song.zip"}]}`))
			return
		}
		_, _ = w.Write(packed.Bytes())
	}))
	defer server.Close()
	a := &App{dataDir: filepath.Join(root, "data", "records")}
	t.Cleanup(func() {
		if err := a.close(); err != nil {
			t.Error(err)
		}
	})
	if _, err := a.SaveArchiveServer(server.URL); err != nil {
		t.Fatal(err)
	}
	page, err := a.SearchSongs(download.Query{Catalog: "archive", Text: "Silent", Keys: 4})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := a.StartInstall(page.Items[0].Key); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(5 * time.Second)
	var state download.State
	for time.Now().Before(deadline) {
		state = a.InstallState()
		if state.Phase == "done" || state.Phase == "failed" {
			break
		}
		time.Sleep(time.Millisecond)
	}
	if state.Phase != "done" || state.Result == nil {
		t.Fatal(state)
	}
	folders, err := a.SongFolders()
	if err != nil || len(folders) != 1 || folders[0] != filepath.Join(root, "data", "downloads") {
		t.Fatal("installation folder was not registered", folders, err)
	}
	catalog, err := a.Catalog()
	if err != nil || len(catalog.Entries) != 1 || catalog.Entries[0].Hash != state.Result.Hashes[0] {
		t.Fatal("installed chart absent from catalogue", catalog, err)
	}
	if _, err := a.songs.Lookup(Selection{Folder: catalog.Entries[0].Folder, ChartID: catalog.Entries[0].Path, Index: catalog.Entries[0].Index}); err != nil {
		t.Fatal("installed chart cannot be selected", err)
	}
	after := &App{dataDir: a.dataDir}
	info, err := after.DownloadInfo()
	if err != nil || info.Archive != server.URL {
		t.Fatal("archive setting was not retained", info, err)
	}
	if err := after.close(); err != nil {
		t.Fatal(err)
	}
}
