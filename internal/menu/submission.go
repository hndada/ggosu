package menu

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"ggosu/internal/piano"
	"ggosu/internal/score"
	"ggosu/internal/settings"
)

type submission struct {
	ID       string       `json:"id"`
	Server   string       `json:"server"`
	Player   string       `json:"player"`
	Revision uint64       `json:"revision"`
	Status   score.Status `json:"status"`
}

func (a *App) ScoreServer() (string, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	c, err := a.scoreConfig()
	return c.ScoreURL(), err
}

func (a *App) Submission(id string) (score.Status, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if _, err := a.replayPath(id); err != nil {
		return score.Status{}, err
	}
	dir, err := a.submissionDir()
	if err != nil {
		return score.Status{}, err
	}
	j, err := readSubmission(filepath.Join(dir, id+".json"))
	if os.IsNotExist(err) {
		return score.Status{State: "unsent"}, nil
	}
	return j.Status, err
}

func (a *App) SaveScoreServer(server string) (string, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.closed || a.state.Running {
		return server, fmt.Errorf("플레이 종료 후 서버를 변경해 주세요")
	}
	if err := settings.ValidateScoreServer(server); err != nil {
		return server, err
	}
	path, _, err := a.settingsKey(Selection{})
	if err != nil {
		return server, err
	}
	c, err := a.loadSettings(path)
	if err != nil {
		return server, err
	}
	c.ScoreServer = &server
	return server, settings.Save(path, c)
}

// RetrySubmission only queues an existing, locally checked live result.
func (a *App) RetrySubmission(id string) (score.Status, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	path, err := a.replayPath(id)
	if err != nil {
		return score.Status{}, err
	}
	r, err := a.readRecord(path)
	if err != nil {
		return score.Status{}, err
	}
	if !score.Eligible(r) {
		return score.Status{}, fmt.Errorf("일반 플레이 기록만 제출할 수 있습니다")
	}
	status := a.queueScore(r)
	if status == nil {
		return score.Status{}, fmt.Errorf("제출할 기록이 없습니다")
	}
	if a.state.Result != nil && a.state.Result.ID == id {
		x := *a.state.Result
		x.Submission = status
		a.state.Result = &x
	}
	return *status, nil
}

func (a *App) scoreConfig() (settings.Config, error) {
	path, _, err := a.settingsKey(Selection{})
	if err != nil {
		return settings.Config{}, err
	}
	return a.loadSettings(path)
}

func (a *App) submissionDir() (string, error) {
	dir, err := a.recordsDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(filepath.Dir(dir), "submissions"), nil
}

// queueScore is called with mu held. Network work starts only after it releases.
func (a *App) queueScore(r piano.Record) *score.Status {
	if !score.Eligible(r) {
		return nil
	}
	failed := func(err error) *score.Status { return &score.Status{State: "failed", Error: err.Error()} }
	c, err := a.scoreConfig()
	if err != nil {
		return failed(err)
	}
	if c.ScoreURL() == "" {
		return &score.Status{State: "disabled"}
	}
	dir, err := a.submissionDir()
	if err != nil {
		return failed(err)
	}
	if _, err := a.replayPath(r.ID); err != nil {
		return failed(err)
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		return failed(err)
	}
	path := filepath.Join(dir, r.ID+".json")
	j := submission{ID: r.ID, Server: c.ScoreURL(), Player: c.PlayerName(), Revision: 1, Status: score.Status{State: "pending"}}
	old, err := readSubmission(path)
	if err == nil {
		if old.Status.State == "sent" && old.Server == j.Server {
			return &old.Status
		}
		j.Revision = old.Revision + 1
	} else if !os.IsNotExist(err) {
		return failed(err)
	}
	if err := saveSubmission(path, j); err != nil {
		return failed(err)
	}
	// A result arriving during shutdown still belongs in the next launch's outbox.
	if a.closed {
		return &j.Status
	}
	newWorker := a.submitWake == nil
	if err := a.startSubmitLocked(); err != nil {
		return failed(err)
	}
	if newWorker {
		return &j.Status
	}
	select {
	case a.submitWake <- struct{}{}:
	default:
	}
	return &j.Status
}

func (a *App) startSubmissions() error {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.startSubmitLocked()
}

func (a *App) startSubmitLocked() error {
	if a.closed {
		return fmt.Errorf("곡 선택창이 종료 중입니다")
	}
	if a.submitWake != nil {
		return nil
	}
	dir, err := a.submissionDir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	ctx, cancel := context.WithCancel(context.Background())
	wake, done := make(chan struct{}, 1), make(chan struct{})
	a.submitWake, a.submitDone, a.submitCancel = wake, done, cancel
	wake <- struct{}{}
	go a.submitLoop(ctx, dir, wake, done)
	return nil
}

// One worker belongs to App. close cancels its HTTP request and joins it.
// Failed jobs retry on the next launch, next saved play, or explicit retry.
func (a *App) submitLoop(ctx context.Context, dir string, wake <-chan struct{}, done chan<- struct{}) {
	defer close(done)
	for {
		select {
		case <-ctx.Done():
			return
		case <-wake:
		}
		files, err := os.ReadDir(dir)
		if err != nil {
			a.submissionNotice("점수 제출 대기열 읽기 실패: " + err.Error())
			continue
		}
		for _, file := range files {
			if ctx.Err() != nil {
				return
			}
			if file.IsDir() || !strings.HasSuffix(file.Name(), ".json") {
				continue
			}
			path := filepath.Join(dir, file.Name())
			a.mu.Lock()
			config, configErr := a.scoreConfig()
			if configErr != nil || config.ScoreURL() == "" {
				a.mu.Unlock()
				if configErr != nil {
					a.submissionNotice("점수 제출 설정 읽기 실패: " + configErr.Error())
				}
				break
			}
			j, err := readSubmission(path)
			if err != nil || j.Status.State == "sent" || j.Status.State == "disabled" {
				a.mu.Unlock()
				if err != nil {
					a.submissionNotice("점수 제출 대기열 읽기 실패: " + err.Error())
				}
				continue
			}
			var record piano.Record
			recordPath, err := a.replayPath(j.ID)
			if err == nil {
				record, err = a.readRecord(recordPath)
			}
			a.mu.Unlock()
			var receipt score.Receipt
			if err == nil {
				receipt, err = score.Submit(ctx, j.Server, score.Request{Player: j.Player, Record: record})
			}
			if ctx.Err() != nil {
				return
			}
			j.Status = score.Status{State: "sent", Receipt: &receipt}
			if err != nil {
				j.Status = score.Status{State: "failed", Error: err.Error()}
			}
			a.mu.Lock()
			current, readErr := readSubmission(path)
			if readErr != nil || current.Revision != j.Revision {
				a.mu.Unlock()
				if readErr != nil {
					a.submissionNotice("점수 제출 상태 읽기 실패: " + readErr.Error())
				}
				continue
			}
			if err := saveSubmission(path, j); err != nil {
				j.Status = score.Status{State: "failed", Error: "제출 상태 저장 실패: " + err.Error()}
			}
			if a.state.Result != nil && a.state.Result.ID == j.ID {
				x := *a.state.Result
				x.Submission = &j.Status
				a.state.Result = &x
			}
			changed, closed := a.onChange, a.closed
			a.mu.Unlock()
			if !closed && changed != nil {
				changed()
			}
		}
	}
}

func readSubmission(path string) (submission, error) {
	var j submission
	b, err := os.ReadFile(path)
	if err != nil {
		return j, err
	}
	err = json.Unmarshal(b, &j)
	return j, err
}

func (a *App) submissionNotice(message string) {
	a.dataMu.Lock()
	defer a.dataMu.Unlock()
	a.notice(message)
}

func saveSubmission(path string, j submission) (err error) {
	f, err := os.CreateTemp(filepath.Dir(path), ".submission-*")
	if err != nil {
		return err
	}
	defer func() {
		if e := os.Remove(f.Name()); e != nil && !os.IsNotExist(e) {
			err = errors.Join(err, e)
		}
	}()
	err = json.NewEncoder(f).Encode(j)
	if err == nil {
		err = f.Sync()
	}
	if err = errors.Join(err, f.Close()); err != nil {
		return err
	}
	return os.Rename(f.Name(), path)
}
