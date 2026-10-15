package download

import (
	"context"
	"os"
	"testing"
	"time"
)

// Optional live verification installs a real mania set into a caller-selected
// QA directory. It never starts gameplay or audio and never alters user songs.
func TestLiveManiaDownload(t *testing.T) {
	root := os.Getenv("GGOSU_DOWNLOAD_LIVE")
	if root == "" {
		t.Skip("set GGOSU_DOWNLOAD_LIVE for live search and installation")
	}
	l, err := New(root, "")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	page, err := l.Search(context.Background(), Query{Catalog: "osu", Text: "Rain"})
	if err != nil {
		t.Fatal(err)
	}
	key := "osu:osu:2625900"
	found := false
	for _, p := range page.Items {
		found = found || p.Key == key
	}
	if !found {
		t.Fatal("verified live test set absent from current search response")
	}
	if _, err := l.Start(key); err != nil {
		t.Fatal(err)
	}
	l.mu.Lock()
	done := l.done
	l.mu.Unlock()
	select {
	case <-done:
	case <-time.After(2 * time.Minute):
		l.Cancel()
		t.Fatal("live install timed out")
	}
	s := l.State()
	if s.Phase != "done" || s.Result == nil || s.Result.Charts == 0 {
		t.Fatal(s)
	}
	t.Logf("live search and install: %s/%s, %d charts, %d bytes", s.Result.Source, s.Result.ID, s.Result.Charts, s.Received)
}
