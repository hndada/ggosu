package menu

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	wire "ggosu/internal/launch"
	"ggosu/internal/piano"
)

func (a *App) Play(s Selection) (string, error) { return a.play(s, nil) }

// EditHUD opens Auto play in the HUD editor, using the resident game window.
func (a *App) EditHUD(s Selection) (string, error) {
	s.Mode = "hud"
	return a.play(s, nil)
}

func (a *App) play(s Selection, networkArgs []string) (string, error) {
	if strings.TrimSpace(s.Folder) == "" || strings.TrimSpace(s.ChartID) == "" {
		return "", fmt.Errorf("폴더와 채보를 선택해 주세요")
	}
	a.mu.Lock()
	if a.closed {
		a.mu.Unlock()
		return "", fmt.Errorf("곡 선택창이 종료 중입니다")
	}
	if a.state.Running {
		a.mu.Unlock()
		return "", fmt.Errorf("현재 플레이를 마친 뒤 다시 실행해 주세요")
	}
	if err := a.stopWarm(); err != nil {
		a.mu.Unlock()
		return "", err
	}
	p, err := a.prepare(s, networkArgs)
	if err != nil {
		a.mu.Unlock()
		return "", err
	}
	if err := a.startGame(); err != nil {
		a.mu.Unlock()
		return "", err
	}
	proc := a.game
	proc.next++
	f := &flight{launch: p, id: proc.next, ready: make(chan error, 1)}
	wire.AllowFocus(proc.cmd.Process.Pid)
	if err := proc.output.Encode(wire.Request{ID: f.id, Args: p.args}); err != nil {
		a.mu.Unlock()
		return "", fmt.Errorf("게임 요청 전달: %w", err)
	}
	proc.current = f
	selection := p.selection
	if selection.Mode == "hud" {
		selection = a.state.Selection
	}
	a.state = State{Running: true, Selection: selection, Activity: s.Mode, Title: p.title}
	changed := a.onChange
	a.mu.Unlock()
	if changed != nil {
		changed()
	}

	timer := time.NewTimer(2 * time.Minute)
	defer timer.Stop()
	select {
	case err := <-f.ready:
		if err != nil {
			return "", err
		}
	case <-timer.C:
		if err := proc.cmd.Process.Kill(); err != nil && !errors.Is(err, os.ErrProcessDone) {
			return "", fmt.Errorf("게임 준비 시간 초과 및 종료 실패: %w", err)
		}
		return "", fmt.Errorf("게임 준비 시간이 초과됐습니다")
	}
	return p.message, nil
}

// finishPlay is called with mu held after the child has saved its result and
// released the song. The idle process remains available for the next request.
func (a *App) finishPlay(p launch, err error) {
	var saved *Result
	var record piano.Record
	if err == nil && p.resultPath != "" {
		r, readErr := a.readRecord(p.resultPath)
		if os.IsNotExist(readErr) {
			// An aborted run intentionally writes no score record.
		} else if readErr != nil {
			err = fmt.Errorf("결과 저장 확인: %w", readErr)
		} else if !r.Retained() {
			record = r
			// A 0-point play is not kept: no record and no result screen.
			if rmErr := os.Remove(p.resultPath); rmErr != nil && !os.IsNotExist(rmErr) {
				err = fmt.Errorf("0점 기록 삭제: %w", rmErr)
			}
		} else {
			saved, record = result(r), r
		}
	}
	if saved != nil {
		cal, calErr := a.calibrate(record)
		if calErr != nil {
			err = fmt.Errorf("오프셋 자동 보정: %w", calErr)
		}
		saved.Calibration = cal
		saved.Submission = a.queueScore(record)
	}

	a.state.Running, a.state.Result = false, saved
	if err != nil {
		a.state.Error = err.Error()
	}
}

func (a *App) close() error {
	a.mu.Lock()
	a.closed = true
	if a.submitCancel != nil {
		a.submitCancel()
	}
	submitDone := a.submitDone
	warmErr := a.stopWarm()
	proc, l := a.game, a.lobby
	library := a.library
	a.lobby = nil
	a.mu.Unlock()
	if library != nil {
		library.Close()
	}
	if submitDone != nil {
		<-submitDone
	}
	if l != nil {
		l.close()
	}
	if proc == nil {
		return warmErr
	}
	// EOF asks the resident game to release the current song and exit.
	closeErr := proc.input.Close()
	timer := time.NewTimer(3 * time.Second)
	defer timer.Stop()
	select {
	case <-proc.done:
	case <-timer.C:
		if err := proc.cmd.Process.Kill(); err != nil && !errors.Is(err, os.ErrProcessDone) {
			return errors.Join(warmErr, closeErr, fmt.Errorf("게임 종료: %w", err))
		}
		<-proc.done
	}
	return errors.Join(warmErr, closeErr)
}
