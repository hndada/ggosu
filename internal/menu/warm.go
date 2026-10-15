package menu

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// stopWarm is called with mu held. Wait closes warmDone before taking mu,
// so cancellation completes before another worker or game process starts.
func (a *App) stopWarm() error {
	if a.warm == nil {
		return nil
	}
	if err := a.warm.Process.Kill(); err != nil && !errors.Is(err, os.ErrProcessDone) {
		return fmt.Errorf("키음 사전 로딩 종료: %w", err)
	}
	<-a.warmDone
	a.warm, a.warmDone, a.warmPath = nil, nil, ""
	return nil
}

func (a *App) serveWarm(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var s *Selection
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&s); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.closed || a.state.Running {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if s == nil {
		if err := a.stopWarm(); err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		w.WriteHeader(http.StatusNoContent)
		return
	}
	e, err := a.songs.Lookup(*s)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	// Prepare the hidden graphics window while the user is choosing a song.
	if err := a.startGame(); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	if a.warmPath == e.Path {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if err := a.stopWarm(); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	prefs, _, err := a.settingsKey(Selection{})
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	exe, err := os.Executable()
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	dir := filepath.Join(filepath.Dir(prefs), "cache", "pcm")
	cmd := exec.Command(filepath.Join(filepath.Dir(exe), "game.exe"), "-warm", "-chart", e.Path, "-cache", dir)
	if err := cmd.Start(); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	done := make(chan struct{})
	a.warm, a.warmDone, a.warmPath = cmd, done, e.Path
	go func() {
		err := cmd.Wait()
		if cleanupErr := clearWarmFiles(dir, cmd.Process.Pid); cleanupErr != nil {
			fmt.Fprintln(os.Stderr, "키음 임시 캐시 정리:", cleanupErr)
		}
		close(done)
		a.mu.Lock()
		defer a.mu.Unlock()
		if a.warm != cmd {
			return
		} // A cancelled worker was already replaced.
		a.warm, a.warmDone = nil, nil
		if err != nil {
			a.warmPath = ""
			fmt.Fprintln(os.Stderr, "키음 사전 로딩:", err)
		}
	}()
	w.WriteHeader(http.StatusNoContent)
}

// A killed process cannot run its file cleanup. After Wait, remove only the
// temporary files belonging to that process; other workers and caches stay intact.
func clearWarmFiles(dir string, pid int) error {
	files, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	prefix := fmt.Sprintf(".pcm-%d-", pid)
	for _, f := range files {
		if f.IsDir() || !strings.HasPrefix(f.Name(), prefix) {
			continue
		}
		if err := os.Remove(filepath.Join(dir, f.Name())); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	return nil
}
