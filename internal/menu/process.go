package menu

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	wire "ggosu/internal/launch"
)

type flight struct {
	launch
	id    uint64
	ready chan error
}

// process owns one game executable. The menu lock guards its current song and
// writes; the reader goroutine owns stdout and Wait for the process lifetime.
type process struct {
	cmd     *exec.Cmd
	input   io.WriteCloser
	output  *json.Encoder
	done    chan struct{}
	next    uint64
	current *flight
}

// A resident process can log across thousands of songs. Retain only the latest
// 64KiB for an abnormal exit; Wait joins the writer before the reader uses it.
type gameLog struct{ data []byte }

func (l *gameLog) Write(p []byte) (int, error) {
	const limit = 64 << 10
	n := len(p)
	if len(p) >= limit {
		l.data = append(l.data[:0], p[len(p)-limit:]...)
	} else {
		if len(l.data)+len(p) > limit {
			l.data = l.data[len(l.data)+len(p)-limit:]
		}
		l.data = append(l.data, p...)
	}
	return n, nil
}

// startGame is called with mu held. A hidden, idle process is reused by Play.
func (a *App) startGame() error {
	if a.game != nil {
		return nil
	}
	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("게임 경로 확인: %w", err)
	}
	cmd := exec.Command(filepath.Join(filepath.Dir(exe), "game.exe"), "-resident")
	input, err := cmd.StdinPipe()
	if err != nil {
		return err
	}
	output, err := cmd.StdoutPipe()
	if err != nil {
		input.Close()
		return err
	}
	var stderr gameLog
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		input.Close()
		output.Close()
		return fmt.Errorf("게임 실행: %w", err)
	}
	p := &process{cmd: cmd, input: input, output: json.NewEncoder(input), done: make(chan struct{})}
	a.game = p
	go a.readGame(p, output, &stderr)
	return nil
}

func (a *App) readGame(p *process, output io.ReadCloser, stderr *gameLog) {
	defer close(p.done)
	decoder := json.NewDecoder(output)
	for {
		var e wire.Event
		if err := decoder.Decode(&e); err != nil {
			if !errors.Is(err, io.EOF) {
				// A malformed pipe cannot leave the child waiting for another song.
				if killErr := p.cmd.Process.Kill(); killErr != nil && !errors.Is(killErr, os.ErrProcessDone) {
					fmt.Fprintln(os.Stderr, "게임 통신 실패 후 종료:", killErr)
				}
			}
			break
		}
		a.mu.Lock()
		f := p.current
		if a.game != p || f == nil || e.ID != f.id {
			a.mu.Unlock()
			continue
		}
		var err error
		if e.Error != "" {
			err = errors.New(e.Error)
		}
		switch e.Type {
		case "ready":
			signalReady(f, err)
		case "done":
			if err == nil {
				signalReady(f, fmt.Errorf("첫 화면이 준비되기 전에 플레이가 종료됐습니다"))
			} else {
				signalReady(f, err)
			}
			a.finishPlay(f.launch, err)
			p.current = nil
		}
		changed, closed := a.onChange, a.closed
		a.mu.Unlock()
		if e.Type == "done" && !closed && changed != nil {
			changed()
		}
	}
	err := p.cmd.Wait()
	if err != nil && len(stderr.data) > 0 {
		err = fmt.Errorf("%w: %s", err, strings.TrimSpace(string(stderr.data)))
	}
	a.mu.Lock()
	if a.game == p {
		a.game = nil
		if f := p.current; f != nil {
			if err == nil {
				err = fmt.Errorf("게임 프로세스가 플레이 중 종료됐습니다")
			}
			signalReady(f, err)
			a.finishPlay(f.launch, err)
			p.current = nil
		}
		if err != nil && !a.closed {
			a.state.Error = err.Error()
		}
	}
	changed, closed := a.onChange, a.closed
	a.mu.Unlock()
	if !closed && changed != nil {
		changed()
	}
}

func signalReady(f *flight, err error) {
	select {
	case f.ready <- err:
	default:
	}
}
