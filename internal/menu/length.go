package menu

import (
	"fmt"
	"time"

	"ggosu/internal/chart"
)

// AudioDuration measures only the selected chart, retaining sample lengths on disk.
// Each request checks file sizes and modification times before reusing the cache.
func (a *App) AudioDuration(s Selection) (time.Duration, error) {
	a.lengthMu.Lock()
	defer a.lengthMu.Unlock()
	c, err := a.EditChart(s)
	if err != nil {
		return 0, err
	}
	path, err := a.lengthsPath()
	if err != nil {
		return 0, err
	}
	lengths, err := chart.LoadAudioLengths(path)
	if err != nil {
		return 0, err
	}
	end, err := lengths.Duration(c)
	if err != nil {
		return 0, fmt.Errorf("곡 길이 계산: %w", err)
	}
	if err := lengths.Save(path); err != nil {
		return 0, fmt.Errorf("곡 길이 캐시 저장: %w", err)
	}
	return end, nil
}
