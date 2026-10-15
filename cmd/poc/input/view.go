package main

import (
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/ebitenutil"
	"github.com/hajimehoshi/ebiten/v2/vector"
)

func (g *game) Layout(int, int) (int, int) { return 1100, 740 }

func (g *game) Draw(screen *ebiten.Image) {
	if g.opts.stall > 0 && !g.testFinished && time.Since(g.lastStall) >= 2*time.Second {
		g.lastStall = time.Now()
		time.Sleep(g.opts.stall) // Deliberate, visible POC fault injection.
	}
	screen.Fill(color.RGBA{13, 18, 28, 255})
	line := func(y int, format string, args ...any) {
		ebitenutil.DebugPrintAt(screen, fmt.Sprintf(format, args...), 30, y)
	}
	line(25, "GGOSU / NATIVE KEYBOARD -> EBITENGINE")
	line(48, "%s on a dedicated OS thread. Callback is immediate; visuals wait for Draw.", g.source.Backend())
	line(80, "VSync: %v    Target TPS: %d    Actual FPS / TPS: %.1f / %.1f    Draw stall: %s",
		g.opts.vsync, g.opts.tps, ebiten.ActualFPS(), ebiten.ActualTPS(), g.opts.stall)
	line(103, "F1: VSync   F2: TPS   F3: 100ms stall   F5: synthetic load   F6: save frame/report   Esc: exit")
	mode := "LIVE"
	if g.opts.record != "" {
		mode = fmt.Sprintf("RECORD (%d events)", len(g.recording.Events))
	} else if g.player != nil {
		mode = fmt.Sprintf("REPLAY (%d events, done=%v)", g.replayed, g.player.Done())
	}
	line(126, "P: pause   F7: playback speed   Game (1x): %.3fs  %.2fx  paused=%v   %s",
		g.timeline.Position(g.clock.Now()).Seconds(), g.speed, g.paused, mode)
	for i, lane := range lanes {
		x := float32(30 + i*145)
		c := color.RGBA{30, 45, 65, 255}
		if g.gameHeld[lane.code] {
			c = color.RGBA{0, 150, 135, 255}
		}
		if g.syntheticHeld[lane.code] {
			c = color.RGBA{133, 78, 170, 255}
		}
		vector.FillRect(screen, x, 158, 125, 70, c, false)
		ebitenutil.DebugPrintAt(screen, lane.name, int(x)+57, 183)
	}
	r := g.view
	line(250, "Callback received: %d  (RAW %d / SYNTHETIC %d)    Update handled: %d",
		r.Inbox.Received, r.Inbox.Raw, r.Inbox.Synthetic, r.Handled)
	line(273, "Queue dropped: %d    Sequence gaps: %d    Order errors: %d    Queue high-water: %d",
		r.Inbox.Dropped, r.SequenceGaps, r.OrderErrors, r.Inbox.HighWater)
	line(296, "D/F/J/K: native down/up %d/%d    Ebitengine built-in down/up %d/%d",
		r.RawLaneDown, r.RawLaneUp, r.StandardDown, r.StandardUp)
	line(333, "LATENCY (microseconds)                  mean         p50         p95         p99         max")
	printLatency := func(y int, name string, d distribution) {
		line(y, "%-35s %9.1f   %9.1f   %9.1f   %9.1f   %9.1f", name, d.MeanUS, d.P50US, d.P95US, d.P99US, d.MaxUS)
	}
	printLatency(356, "Native receive -> game callback", r.Inbox.Delivery)
	printLatency(379, "Native receive -> Update applied", r.QueueDelay)
	printLatency(402, "Synthetic post -> native receive", r.Inbox.Post)
	line(430, "Percentiles: latest 4096 samples. Mean/max: whole run. These are NOT hardware-to-photon timings.")
	status := "Real keyboard input ready. F5 starts a separate synthetic throughput test."
	if notice := g.source.Notice(); notice != "" {
		status = notice
	}
	if g.testStarted {
		status = fmt.Sprintf("SYNTHETIC LOAD: target %d events/s for %s (1ms batches; NOT hardware 8000Hz)", g.opts.rate, g.opts.duration)
	}
	if g.testFinished {
		status = fmt.Sprintf("SYNTHETIC COMPLETE: passed=%v  posted=%d  actual=%.1f events/s", g.testPassed, g.loadResult.Sent, g.loadResult.Rate)
	}
	if g.opts.oscheck {
		status = fmt.Sprintf("OS ROUTE CHECK: SendInput D/F/J/K to this foreground app only. Sent %d / RAW matched %d / pass %v", g.osSent, g.osSeen, g.osPassed)
	}
	line(464, "%s", status)
	line(493, "Recent events (individual downs/ups are retained even between two Update calls):")
	for i, e := range g.events {
		if e.Seq == 0 {
			continue
		}
		kind, state := "RAW", "UP"
		if e.Synthetic {
			kind = "SYNTHETIC"
		}
		if e.Down {
			state = "DOWN"
		}
		line(515+i*15, "#%-7d %-9s key=0x%02X scan=0x%02X %-4s received at %9.3f ms",
			e.Seq, kind, e.Key, e.Scan, state, float64(e.At-g.started)/float64(time.Millisecond))
	}
	if ((g.opts.selftest && g.testFinished) || (g.opts.oscheck && g.osFinished)) && !g.finalDrawn {
		if g.opts.capture != "" {
			if err := capture(screen, g.opts.capture); err != nil {
				g.err = err
			}
		}
		g.finalDrawn = true
	}
	if g.captureRequested {
		g.captureRequested = false
		path := g.opts.capture
		if path == "" {
			path = "artifacts/01-poc-input/manual.png"
		}
		if err := capture(screen, path); err != nil {
			g.err = err
		}
		if err := saveReport(g.opts.report, g.report()); err != nil {
			g.err = err
		}
	}
}

func capture(screen *ebiten.Image, path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return fmt.Errorf("create capture directory: %w", err)
	}
	img := image.NewRGBA(screen.Bounds())
	screen.ReadPixels(img.Pix)
	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("create capture: %w", err)
	}
	if err := png.Encode(f, img); err != nil {
		_ = f.Close()
		return fmt.Errorf("encode capture: %w", err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("close capture: %w", err)
	}
	return nil
}
