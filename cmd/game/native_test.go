//go:build !js

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"ggosu/internal/chart"
	"ggosu/internal/launch"
	"ggosu/internal/piano"
)

func TestOptionsDoNotCarryAcrossSongs(t *testing.T) {
	first, _, err := options([]string{"-auto", "-speed", "2", "-keys", "D,F", "-hud-edit", "-room-password", "secret"})
	if err != nil || !first.Auto || first.Rate != 2 || len(first.Profile.Keys) != 2 || !first.HUDEdit {
		t.Fatal(first, err)
	}
	second, _, err := options([]string{"-chart", "other.ggc"})
	if first.RoomPassword != "secret" {
		t.Fatal("room password flag was dropped")
	}
	if err != nil || second.Auto || second.Rate != 1 || len(second.Profile.Keys) != 0 || second.HUDEdit || second.RoomPassword != "" {
		t.Fatal("previous song's options leaked", second, err)
	}
	if _, _, err := options([]string{"-unsupported"}); err == nil {
		t.Fatal("invalid request accepted")
	}
}

func TestTimingThresholdOptions(t *testing.T) {
	for _, threshold := range []string{"kool", "cool", "good", "off"} {
		c, _, err := options([]string{"-timing-threshold", threshold})
		if err != nil || c.Display.TimingThreshold != threshold {
			t.Fatal("timing threshold flag", c.Display.TimingThreshold, err)
		}
	}
	c, _, err := options(nil)
	if err != nil || c.Display.TimingThreshold != "cool" {
		t.Fatal("timing threshold default", c.Display.TimingThreshold, err)
	}
}

// Optional native integration: use a built game executable with an actual UI
// driver. All audio volumes are zero; the fixture is a short test chart.
func TestResidentSongs(t *testing.T) {
	testResidentSongs(t, nil)
}

func testResidentSongs(t *testing.T, window func(pid int, mode, resolution string, ready bool)) {
	t.Helper()
	exe := os.Getenv("GGOSU_GAME_EXE")
	if exe == "" {
		t.Skip("set GGOSU_GAME_EXE to test the native resident executable")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "song.ggc")
	c := chart.New(4)
	c.Metadata.Title = "Resident lifecycle test"
	c.Timing = []chart.Timing{{BPM: 120, Scroll: 1, Meter: 4}}
	for i := range 4 {
		at := time.Duration(i+1) * 100 * time.Millisecond
		c.Notes = append(c.Notes, chart.Note{At: at, End: at, Lane: i, Kind: "tap"})
	}
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := chart.Write(f, c); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, exe, "-resident")
	input, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	output, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		input.Close()
		cancel()
		cmd.Wait()
		if t.Failed() {
			t.Logf("game stderr: %s", stderr.String())
		}
	})
	encoder, decoder := json.NewEncoder(input), json.NewDecoder(output)
	next := func(id uint64, kind string, wantErr bool) {
		t.Helper()
		var e launch.Event
		if err := decoder.Decode(&e); err != nil {
			t.Fatal("resident event", err)
		}
		if e.ID != id || e.Type != kind || (e.Error != "") != wantErr {
			t.Fatalf("unexpected event %+v, want %d/%s error=%v", e, id, kind, wantErr)
		}
	}
	next(0, "idle", false)
	// A failed request must leave the process available for another song.
	if err := encoder.Encode(launch.Request{ID: 1, Args: []string{"-unsupported"}}); err != nil {
		t.Fatal(err)
	}
	next(1, "done", true)
	for i, mode := range []string{"windowed", "fullscreen", "borderless", "fullscreen", "windowed"} {
		rate := []string{"1", "1.25", "0.75"}[i%3]
		resolution := []string{"1280x720", "1280x720", "1024x640", "1280x720", "1024x640"}[i]
		id := uint64(i + 2)
		result := filepath.Join(dir, fmt.Sprintf("result-%d.json", id))
		// A bounded QA run keeps judging when external window-state checks
		// briefly move OS focus; manual play retains its normal focus pause.
		args := []string{"-chart", path, "-auto", "-input", "ebiten", "-duration", "2s", "-speed", rate, "-result", result, "-window-mode", mode,
			"-resolution", resolution,
			"-master-volume", "0", "-music-volume", "0", "-key-volume", "0", "-effect-volume", "0", "-cache", filepath.Join(dir, "pcm")}
		if window != nil {
			window(cmd.Process.Pid, mode, resolution, false)
		}
		started := time.Now()
		if err := encoder.Encode(launch.Request{ID: id, Args: args}); err != nil {
			t.Fatal(err)
		}
		next(id, "ready", false)
		if window != nil {
			window(cmd.Process.Pid, mode, resolution, true)
		}
		t.Logf("PID %d request %d first Draw: %s", cmd.Process.Pid, id, time.Since(started))
		next(id, "done", false)
		r, err := piano.ReadRecord(result)
		wantRate, _ := strconv.ParseFloat(rate, 64)
		if err != nil || !r.Completed || r.Rate != wantRate || r.Score.Points != piano.MaxScore || r.Score.Miss != 0 || len(r.Warnings) != 0 {
			t.Fatalf("song state or saved result leaked: %+v %v", r, err)
		}
	}
	// Raw input belongs to a song; repeated live runs must release the receiver.
	// These short aborted runs intentionally leave no score record.
	for id := uint64(10); id < 12; id++ {
		result := filepath.Join(dir, fmt.Sprintf("abort-%d.json", id))
		args := []string{"-chart", path, "-input", "native", "-frames", "4", "-result", result,
			"-master-volume", "0", "-music-volume", "0", "-key-volume", "0", "-effect-volume", "0", "-cache", filepath.Join(dir, "pcm")}
		if err := encoder.Encode(launch.Request{ID: id, Args: args}); err != nil {
			t.Fatal(err)
		}
		next(id, "ready", false)
		next(id, "done", false)
		if _, err := os.Stat(result); !os.IsNotExist(err) {
			t.Fatal("aborted live run retained a score", err)
		}
	}
	// Send the next request immediately after done to exercise the switch to
	// demand-driven idle rendering, including wake events in the final frame.
	for id := uint64(20); id < 35; id++ {
		args := []string{"-chart", path, "-auto", "-mute", "-input", "ebiten", "-frames", "1"}
		if err := encoder.Encode(launch.Request{ID: id, Args: args}); err != nil {
			t.Fatal(err)
		}
		next(id, "ready", false)
		next(id, "done", false)
	}
	if err := input.Close(); err != nil {
		t.Fatal(err)
	}
	if err := cmd.Wait(); err != nil {
		t.Fatalf("resident exit: %v\n%s", err, stderr.String())
	}
}
