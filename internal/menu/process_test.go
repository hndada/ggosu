package menu

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"ggosu/internal/chart"
	wire "ggosu/internal/launch"
)

func TestGamePipeChild(t *testing.T) {
	if os.Getenv("GGOSU_PIPE_CHILD") != "1" {
		return
	}
	d, e := json.NewDecoder(os.Stdin), json.NewEncoder(os.Stdout)
	for {
		var r wire.Request
		if err := d.Decode(&r); err != nil {
			os.Exit(0)
		}
		if err := e.Encode(wire.Event{ID: r.ID, Type: "ready"}); err != nil {
			os.Exit(1)
		}
		time.Sleep(20 * time.Millisecond)
		if err := e.Encode(wire.Event{ID: r.ID, Type: "done"}); err != nil {
			os.Exit(1)
		}
	}
}

func TestMenuReusesIdleGameAndJoinsShutdown(t *testing.T) {
	a := &App{dataDir: filepath.Join(t.TempDir(), "records")}
	c := chart.New(4)
	c.Timing = []chart.Timing{{BPM: 120, Scroll: 1, Meter: 4}}
	c.Notes = []chart.Note{{At: time.Second, End: time.Second, Kind: "tap"}}
	registerChart(t, a, c)
	hash, err := c.Digest()
	if err != nil {
		t.Fatal(err)
	}
	entry, ok := a.songs.Find(hash)
	if !ok {
		t.Fatal("test chart missing")
	}
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(exe, "-test.run=^TestGamePipeChild$")
	cmd.Dir = filepath.Join("internal", "menu") // TestMain returns to the checkout root.
	cmd.Env = append(os.Environ(), "GGOSU_PIPE_CHILD=1")
	input, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	output, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	var stderr gameLog
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	p := &process{cmd: cmd, input: input, output: json.NewEncoder(input), done: make(chan struct{})}
	a.game = p
	go a.readGame(p, output, &stderr)
	t.Cleanup(func() {
		if err := a.close(); err != nil {
			t.Error(err)
		}
	})
	for i := range 3 {
		if i == 0 {
			_, err = a.Play(Selection{Folder: entry.Folder, ChartID: entry.Path, Index: entry.Index})
		} else {
			_, err = a.Retry()
		}
		if err != nil {
			t.Fatal(err)
		}
		deadline := time.Now().Add(2 * time.Second)
		for a.State().Running && time.Now().Before(deadline) {
			time.Sleep(time.Millisecond)
		}
		a.mu.Lock()
		retained := a.game == p && p.current == nil && !a.state.Running && a.state.Error == ""
		a.mu.Unlock()
		if !retained {
			t.Fatal("song completion lost the idle process or menu state", a.State())
		}
	}
	if err := a.close(); err != nil {
		t.Fatal(err)
	}
	select {
	case <-p.done:
	default:
		t.Fatal("shutdown did not join the idle game")
	}
}
