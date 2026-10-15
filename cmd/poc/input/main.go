package main

import (
	"flag"
	"fmt"
	"os"
	"runtime"
	"time"

	"github.com/hajimehoshi/ebiten/v2"

	"ggosu/internal/clock"
	"ggosu/internal/keyboard"
)

type options struct {
	vsync    bool
	tps      int
	selftest bool
	oscheck  bool
	rate     int
	duration time.Duration
	stall    time.Duration
	report   string
	capture  string
	record   string
	replay   string
	playback float64
}

func main() {
	var o options
	flag.BoolVar(&o.vsync, "vsync", true, "enable VSync (native input remains independent)")
	flag.IntVar(&o.tps, "tps", 60, "Ebitengine ticks per second; -1 means SyncWithFPS")
	flag.BoolVar(&o.selftest, "selftest", false, "post synthetic Windows messages and exit after verification")
	flag.BoolVar(&o.oscheck, "oscheck", false, "verify WM_INPUT via eight SendInput transitions to this POC's foreground window")
	flag.IntVar(&o.rate, "rate", 8000, "synthetic messages per second")
	flag.DurationVar(&o.duration, "duration", 5*time.Second, "synthetic test duration")
	flag.DurationVar(&o.stall, "stall", 0, "block Draw for this duration once every 2 seconds")
	flag.StringVar(&o.report, "report", "artifacts/report.json", "JSON report path")
	flag.StringVar(&o.capture, "capture", "", "save the final self-test frame as PNG")
	flag.StringVar(&o.record, "record", "", "save D/F/J/K transitions in game time as JSON on exit")
	flag.StringVar(&o.replay, "replay", "", "replay a recorded JSON file instead of live lane input")
	flag.Float64Var(&o.playback, "playback", 1, "initial game playback speed; must be positive")
	flag.Parse()
	if err := run(o); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(o options) error {
	if (o.tps <= 0 && o.tps != ebiten.SyncWithFPS) || o.rate <= 0 || o.duration <= 0 || o.stall < 0 {
		return fmt.Errorf("invalid TPS, rate, duration, or stall")
	}
	if o.oscheck && o.selftest {
		return fmt.Errorf("choose either -selftest or -oscheck")
	}
	if o.oscheck && runtime.GOOS != "windows" {
		return fmt.Errorf("-oscheck uses Windows SendInput; use -selftest on %s", runtime.GOOS)
	}
	if o.record != "" && o.replay != "" {
		return fmt.Errorf("choose either -record or -replay")
	}
	counter, err := clock.New()
	if err != nil {
		return err
	}
	g := &game{opts: o, stop: make(chan struct{}), clock: counter,
		box: &inbox{clock: counter}, started: counter.Now(),
		batch: make([]keyboard.Event, 0, queueSize)}
	if err := g.initPlayback(); err != nil {
		return err
	}
	source, err := keyboard.Start(counter, g.receive)
	if err != nil {
		close(g.stop)
		return err
	}
	g.source = source
	ebiten.SetWindowSize(1100, 740)
	ebiten.SetWindowResizingMode(ebiten.WindowResizingModeEnabled)
	ebiten.SetWindowTitle("ggosu - native keyboard / Ebitengine POC")
	ebiten.SetVsyncEnabled(o.vsync)
	ebiten.SetTPS(o.tps)
	// An automated synthetic test must also complete if another app has focus.
	// Real keyboard input is still rejected by Source outside our foreground app.
	if o.selftest || o.oscheck {
		ebiten.SetRunnableOnUnfocused(true)
	}
	runErr := ebiten.RunGame(g)
	close(g.stop)
	if g.loadDone != nil && g.loadResult == nil {
		r := <-g.loadDone
		g.loadResult = &r
	}
	closeErr := source.Close()
	// The receiver is joined; preserve any final transitions still in the queue.
	g.batch = g.box.drain(g.batch[:0])
	for _, e := range g.batch {
		if err := g.playInput(e); err != nil && runErr == nil {
			runErr = err
		}
		g.apply(e)
	}
	if runErr != nil {
		g.err = runErr
	} else if closeErr != nil {
		g.err = closeErr
	}
	if err := saveReport(o.report, g.report()); err != nil {
		return err
	}
	if runErr != nil {
		return runErr
	}
	if closeErr != nil {
		return closeErr
	}
	if g.err != nil {
		return g.err
	}
	if o.selftest && !g.testPassed {
		return fmt.Errorf("synthetic self-test failed; inspect %s", o.report)
	}
	if o.oscheck && !g.osPassed {
		return fmt.Errorf("WM_INPUT route check failed; inspect %s", o.report)
	}
	return g.saveRecording()
}
