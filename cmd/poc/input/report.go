package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
)

type report struct {
	Error            string        `json:"error,omitempty"`
	CounterHz        int64         `json:"clock_frequency_hz"`
	InputNotice      string        `json:"input_notice,omitempty"`
	Platform         string        `json:"platform"`
	Measurement      string        `json:"measurement"`
	Percentiles      string        `json:"percentiles"`
	Inbox            inboxReport   `json:"inbox"`
	Handled          uint64        `json:"handled"`
	RawHandled       uint64        `json:"raw_handled"`
	SyntheticHandled uint64        `json:"synthetic_handled"`
	OrderErrors      uint64        `json:"order_errors"`
	SequenceGaps     uint64        `json:"sequence_gaps"`
	StandardDown     uint64        `json:"ebitengine_lane_presses"`
	StandardUp       uint64        `json:"ebitengine_lane_releases"`
	RawLaneDown      uint64        `json:"native_lane_presses"`
	RawLaneUp        uint64        `json:"native_lane_releases"`
	QueueDelay       distribution  `json:"receiver_to_update"`
	Load             *loadResult   `json:"synthetic_load,omitempty"`
	SelftestPassed   bool          `json:"selftest_passed"`
	OSCheckPassed    bool          `json:"os_input_check_passed"`
	OSCheckSent      int           `json:"os_input_check_sent"`
	OSCheckSeen      int           `json:"os_input_check_matched"`
	VSync            bool          `json:"vsync"`
	TPS              int           `json:"target_tps"`
	FPS              float64       `json:"actual_fps"`
	ActualTPS        float64       `json:"actual_tps"`
	StallMS          float64       `json:"draw_stall_ms"`
	GamePositionNS   time.Duration `json:"game_position_ns"`
	Playback         float64       `json:"playback"`
	Paused           bool          `json:"paused"`
	Recorded         int           `json:"recorded_events"`
	Replayed         uint64        `json:"replayed_events"`
}

func (g *game) report() report {
	message := ""
	if g.err != nil {
		message = g.err.Error()
	}
	return report{
		Error:       message,
		CounterHz:   g.clock.Frequency(),
		Platform:    fmt.Sprintf("%s/%s %s -> game callback -> Ebitengine Update", runtime.GOOS, runtime.GOARCH, g.source.Backend()),
		InputNotice: g.source.Notice(),
		Measurement: "receiver observation time; not hardware actuation or screen presentation latency",
		Percentiles: "latest 4096 samples; mean and max cover the whole run",
		Inbox:       g.box.report(), Handled: g.handled, RawHandled: g.rawHandled,
		SyntheticHandled: g.synthHandled, OrderErrors: g.orderErrors, SequenceGaps: g.sequenceGaps,
		StandardDown: g.standardDown, StandardUp: g.standardUp,
		RawLaneDown: g.rawLaneDown, RawLaneUp: g.rawLaneUp,
		QueueDelay: g.queueDelay.summary(), Load: g.loadResult, SelftestPassed: g.testPassed,
		OSCheckPassed: g.osPassed, OSCheckSent: g.osSent, OSCheckSeen: g.osSeen,
		VSync: g.opts.vsync, TPS: g.opts.tps, FPS: ebiten.ActualFPS(), ActualTPS: ebiten.ActualTPS(),
		StallMS:        float64(g.opts.stall) / float64(time.Millisecond),
		GamePositionNS: g.timeline.Position(g.clock.Now()), Playback: g.speed, Paused: g.paused,
		Recorded: len(g.recording.Events), Replayed: g.replayed,
	}
}

func saveReport(path string, r report) error {
	if path == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return fmt.Errorf("create report directory: %w", err)
	}
	data, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return fmt.Errorf("encode report: %w", err)
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("write report: %w", err)
	}
	return nil
}
