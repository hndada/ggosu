package menu

import (
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestWarmProcess(t *testing.T) {
	if os.Getenv("GGOSU_WARM_TEST") == "1" {
		time.Sleep(time.Minute)
	}
}

func TestWarmCleanupKeepsOtherWorkersAndCompletedCaches(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{".pcm-123-partial", ".pcm-1234-other", "complete.pcm"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("fixture"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	if err := clearWarmFiles(dir, 123); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".pcm-123-partial")); !os.IsNotExist(err) {
		t.Fatal("partial cache retained", err)
	}
	for _, name := range []string{".pcm-1234-other", "complete.pcm"} {
		if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
			t.Fatal("unrelated cache removed", err)
		}
	}
}

func TestWarmCancellationJoinsProcess(t *testing.T) {
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(exe, "-test.run=^TestWarmProcess$")
	cmd.Env = append(os.Environ(), "GGOSU_WARM_TEST=1")
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	a := &App{warm: cmd, warmDone: done, warmPath: "old"}
	go func() {
		if err := cmd.Wait(); err == nil {
			t.Error("cancelled process exited successfully")
		}
		close(done)
	}()
	if err := a.close(); err != nil {
		t.Fatal(err)
	}
	if a.warm != nil || a.warmDone != nil || a.warmPath != "" {
		t.Fatal("worker retained after shutdown")
	}
	select {
	case <-done:
	default:
		t.Fatal("shutdown did not join worker")
	}
}

func TestWarmRejectsUnscannedPathsAndAllowsCancellation(t *testing.T) {
	a := &App{}
	for _, item := range []struct {
		method, body string
		want         int
	}{
		{"GET", "", http.StatusMethodNotAllowed},
		{"POST", "{", http.StatusBadRequest},
		{"POST", `{"folder":"unknown","chartID":"outside.ojn"}`, http.StatusBadRequest},
		{"POST", "null", http.StatusNoContent},
	} {
		r := httptest.NewRequest(item.method, "/preview/warm", strings.NewReader(item.body))
		w := httptest.NewRecorder()
		a.serveMissing(w, r)
		if w.Code != item.want {
			t.Fatalf("%s: %d %s", item.body, w.Code, w.Body.String())
		}
	}
}
