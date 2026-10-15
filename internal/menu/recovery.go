package menu

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"ggosu/internal/chart"
	"ggosu/internal/piano"
	"ggosu/internal/settings"
)

// notice runs under dataMu; notices persist for the lifetime of this app.
func (a *App) notice(s string) {
	if !slices.Contains(a.notices, s) {
		a.notices = append(a.notices, s)
		if len(a.notices) > 64 {
			a.notices = a.notices[len(a.notices)-64:]
		}
	}
}

func (a *App) loadSettings(path string) (settings.Config, error) {
	a.dataMu.Lock()
	defer a.dataMu.Unlock()
	c, recovered, err := settings.Recover(path)
	if recovered != nil && err == nil {
		from := "기본값"
		if recovered.Backup {
			from = "설정 백업"
		}
		a.notice(fmt.Sprintf("설정을 %s으로 복구했습니다. 원본 보존: %s", from, recovered.Path))
	}
	return c, err
}

func (a *App) readRecord(path string) (piano.Record, error) {
	a.dataMu.Lock()
	defer a.dataMu.Unlock()
	return a.inspectRecord(path, map[string]*chart.Chart{})
}

// inspectRecord runs under dataMu. Invalid files are preserved, not deleted.
func (a *App) inspectRecord(path string, charts map[string]*chart.Chart) (piano.Record, error) {
	r, err := piano.ReadRecord(path)
	stateErr := err
	if err != nil && !errors.Is(err, piano.ErrRecordState) {
		if errors.Is(err, piano.ErrRecord) {
			kept, keepErr := piano.PreserveRecord(path)
			if keepErr != nil {
				return piano.Record{}, keepErr
			}
			a.notice(fmt.Sprintf("잘못된 기록을 격리했습니다: %s (%v)", kept, err))
		}
		return piano.Record{}, err
	}
	hash := r.Replay.ChartHash
	c, loaded := charts[hash]
	if !loaded {
		if e, ok := a.songs.Find(hash); ok {
			cs, loadErr := chart.Load(e.Path)
			if loadErr == nil && e.Index >= 0 && e.Index < len(cs) {
				if actual, digestErr := cs[e.Index].Digest(); digestErr == nil && actual == hash {
					c = cs[e.Index]
				}
			}
		}
		charts[hash] = c
	}
	if c == nil {
		if stateErr != nil {
			kept, keepErr := piano.PreserveRecord(path)
			if keepErr != nil {
				return piano.Record{}, keepErr
			}
			a.notice(fmt.Sprintf("복구할 채보가 없는 잘못된 기록을 격리했습니다: %s", kept))
			return piano.Record{}, stateErr
		}
		r.Verification = "unavailable"
		r.Warnings = append(slices.Clone(r.Warnings), "원본 채보가 없어 점수를 검증하지 못했습니다.")
		return r, nil
	}
	name := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	idChanged := r.ID != name
	r.ID = name
	x, changed, err := r.Recalculate(c)
	if err != nil {
		kept, keepErr := piano.PreserveRecord(path)
		if keepErr != nil {
			return piano.Record{}, keepErr
		}
		a.notice(fmt.Sprintf("채보와 맞지 않는 기록을 격리했습니다: %s (%v)", kept, err))
		return piano.Record{}, fmt.Errorf("%w: %v", piano.ErrRecord, err)
	}
	if changed || idChanged {
		if idChanged {
			x.Warnings = append(x.Warnings, "기록 ID를 저장 파일에 맞게 복구했습니다.")
		}
		kept, err := piano.PreserveRecord(path)
		if err != nil {
			return piano.Record{}, err
		}
		if err := piano.SaveRecord(path, x); err != nil {
			return piano.Record{}, errors.Join(err, os.Rename(kept, path))
		}
		a.notice(fmt.Sprintf("리플레이에서 기록을 복구했습니다. 원본 보존: %s", kept))
	}
	return x, nil
}
