package main

import (
	"bytes"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"ggosu/internal/clock"
	"ggosu/internal/keyboard"
	"ggosu/internal/piano"
)

func playbackGame(t *testing.T, rate float64) *game {
	t.Helper()
	g := &game{opts: options{playback: rate, record: filepath.Join(t.TempDir(), "input.json")}, box: testInbox(t)}
	g.clock = g.box.clock
	if err := g.initPlayback(); err != nil {
		t.Fatal(err)
	}
	return g
}

func TestRecordQueuedPauseAndSpeedChanges(t *testing.T) {
	g := playbackGame(t, 1)
	// All events can arrive in one Update after a rendering stall. Times must
	// still reflect each receiver timestamp and the intervening controls.
	events := []keyboard.Event{
		{At: 100 * time.Millisecond, Key: 'D', Down: true},
		{At: 200 * time.Millisecond, Key: 'P', Down: true},
		{At: 300 * time.Millisecond, Key: 0x76, Down: true},
		{At: 301 * time.Millisecond, Key: 0x76},
		{At: 400 * time.Millisecond, Key: 'D'},
		{At: 500 * time.Millisecond, Key: 'F', Down: true},
		{At: 600 * time.Millisecond, Key: 'F'},
		{At: 650 * time.Millisecond, Key: 'P'},
		{At: 700 * time.Millisecond, Key: 'P', Down: true},
		{At: 710 * time.Millisecond, Key: 'P', Down: true}, // Repeat cannot pause again.
		{At: 800 * time.Millisecond, Key: 'D', Down: true},
		{At: 900 * time.Millisecond, Key: 'D'},
		{At: 950 * time.Millisecond, Key: 'J', Down: true, Synthetic: true},
	}
	for i, e := range events {
		e.Seq = uint64(i + 1)
		if err := g.playInput(e); err != nil {
			t.Fatal(err)
		}
		g.apply(e)
	}
	want := []piano.Event{
		{At: 100 * time.Millisecond, Key: 'D', Down: true},
		{At: 200 * time.Millisecond, Key: 'D'},
		{At: 350 * time.Millisecond, Key: 'D', Down: true},
		{At: 500 * time.Millisecond, Key: 'D'},
	}
	if !reflect.DeepEqual(g.recording.Events, want) || g.paused || g.speed != 1.5 {
		t.Fatalf("recording=%+v paused=%v speed=%v", g.recording.Events, g.paused, g.speed)
	}
	g.recording.Duration = g.timeline.Position(time.Second)
	p, err := piano.NewPlayer(g.recording)
	if err != nil {
		t.Fatal(err)
	}
	got, err := p.AppendDue(nil, g.recording.Duration)
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatal("recorded input did not replay")
	}
}

func TestHeldKeyNeedsFreshPressAfterPause(t *testing.T) {
	g := playbackGame(t, 1)
	for i, e := range []keyboard.Event{
		{At: time.Millisecond, Key: 'D', Down: true},
		{At: 2 * time.Millisecond, Key: 'P', Down: true},
		{At: 3 * time.Millisecond, Key: 'P'},
		{At: 4 * time.Millisecond, Key: 'P', Down: true},
		{At: 5 * time.Millisecond, Key: 'D', Down: true},
		{At: 6 * time.Millisecond, Key: 'D'},
		{At: 7 * time.Millisecond, Key: 'D', Down: true},
	} {
		e.Seq = uint64(i + 1)
		if err := g.playInput(e); err != nil {
			t.Fatal(err)
		}
		g.apply(e)
	}
	if len(g.recording.Events) != 3 || !g.gameHeld['D'] {
		t.Fatalf("held key was resumed by repeat: %+v", g.recording.Events)
	}
}

func TestReplayPauseAtBoundaryAndDelayedControl(t *testing.T) {
	g := playbackGame(t, 1)
	var err error
	f := piano.NewReplay()
	f.Duration = time.Second
	f.Events = []piano.Event{
		{At: 100 * time.Millisecond, Key: 'D', Down: true},
		{At: 200 * time.Millisecond, Key: 'D'},
	}
	g.player, err = piano.NewPlayer(f)
	if err != nil {
		t.Fatal(err)
	}
	if err := g.advanceReplay(150 * time.Millisecond); err != nil {
		t.Fatal(err)
	}
	// This control was timestamped before the previous Update sampled time.
	if err := g.playInput(keyboard.Event{At: 120 * time.Millisecond, Key: 'P', Down: true}); err != nil {
		t.Fatal(err)
	}
	if err := g.advanceReplay(time.Second); err != nil || g.replayed != 1 || !g.gameHeld['D'] {
		t.Fatalf("paused replay advanced or rewound: count=%d, err=%v", g.replayed, err)
	}
	// Resume with a fresh control edge; release occurs at game time 200ms.
	g.held['P'] = false
	if err := g.playInput(keyboard.Event{At: 2 * time.Second, Key: 'P', Down: true}); err != nil {
		t.Fatal(err)
	}
	if err := g.advanceReplay(2050 * time.Millisecond); err != nil || g.replayed != 2 || g.gameHeld['D'] {
		t.Fatalf("resume missed release: count=%d, err=%v", g.replayed, err)
	}
	// Pause before the current batch is consumed: events at the frozen boundary
	// must still be delivered, without requiring a resume first.
	g.player.Reset()
	g.timeline = clock.NewTimeline(0)
	g.sampled, g.replayed = 0, 0
	g.paused = false
	if err := g.playInput(keyboard.Event{At: 100 * time.Millisecond, Key: 'P', Down: true}); err != nil {
		t.Fatal(err)
	}
	if err := g.advanceReplay(time.Second); err != nil || g.replayed != 1 || !g.gameHeld['D'] {
		t.Fatal("did not consume events at pause boundary")
	}
}

func TestSaveRecordingAndRejectLoss(t *testing.T) {
	g := playbackGame(t, 1)
	g.gameHeld['D'] = true
	g.recordEvent(piano.Event{Key: 'D', Down: true})
	if err := g.saveRecording(); err != nil {
		t.Fatal(err)
	}
	f, err := os.Open(g.opts.record)
	if err != nil {
		t.Fatal(err)
	}
	r, err := piano.ReadReplay(f)
	closeErr := f.Close()
	if err != nil || closeErr != nil || len(r.Events) != 2 || r.Events[1].Down || r.Events[1].At != r.Duration {
		t.Fatalf("saved replay: %+v, %v, %v", r, err, closeErr)
	}
	if err := g.saveRecording(); err != nil {
		t.Fatalf("replace existing recording: %v", err)
	}
	before, err := os.ReadFile(g.opts.record)
	if err != nil {
		t.Fatal(err)
	}
	g.box.dropped = 1
	if err := g.saveRecording(); err == nil {
		t.Fatal("saved an incomplete recording")
	}
	after, err := os.ReadFile(g.opts.record)
	if err != nil || !reflect.DeepEqual(before, after) {
		t.Fatal("loss overwrote a previous recording")
	}
}

func TestFixedTimeBaseWithMidReplayRateChanges(t *testing.T) {
	record := func(rate float64) piano.Replay {
		t.Helper()
		g := playbackGame(t, rate)
		for i, tc := range []struct {
			position time.Duration
			down     bool
		}{{10 * time.Second, true}, {12 * time.Second, false}} {
			e := keyboard.Event{Seq: uint64(i + 1),
				At: time.Duration(float64(tc.position) / rate), Key: 'D', Down: tc.down}
			if err := g.playInput(e); err != nil {
				t.Fatal(err)
			}
			g.apply(e)
		}
		g.recording.Duration = g.timeline.Position(time.Duration(float64(14*time.Second) / rate))
		return g.recording
	}
	f := record(2) // Real times 5s/6s must become fixed positions 10s/12s.
	if !reflect.DeepEqual(f, record(1)) || f.Duration != 14*time.Second ||
		f.Events[0].At != 10*time.Second || f.Events[1].At != 12*time.Second {
		t.Fatalf("recording rate changed the time base: %+v", f)
	}
	var encoded bytes.Buffer
	if err := piano.WriteReplay(&encoded, f); err != nil {
		t.Fatal(err)
	}
	g := playbackGame(t, 1)
	g.opts.replay, g.opts.record = g.opts.record, ""
	if err := os.WriteFile(g.opts.replay, encoded.Bytes(), 0600); err != nil {
		t.Fatal(err)
	}
	if err := g.initPlayback(); err != nil {
		t.Fatal(err)
	}
	control := func(at time.Duration, key uint16) {
		t.Helper()
		for _, down := range []bool{true, false} {
			e := keyboard.Event{Seq: g.lastSeq + 1, At: at, Key: key, Down: down}
			if err := g.playInput(e); err != nil {
				t.Fatal(err)
			}
			g.apply(e)
		}
	}
	consume := func(at, position time.Duration, count uint64, down bool) {
		t.Helper()
		if err := g.advanceReplay(at); err != nil {
			t.Fatal(err)
		}
		if g.timeline.Position(at) != position || g.replayed != count || g.gameHeld['D'] != down {
			t.Fatalf("at %s: position=%s count=%d held=%v", at, g.timeline.Position(at), g.replayed, g.gameHeld['D'])
		}
	}
	consume(6*time.Second, 6*time.Second, 0, false)
	control(6*time.Second, 0x76) // 1 -> 1.5 -> 2 at the same current position.
	control(6*time.Second, 0x76)
	consume(7*time.Second, 8*time.Second, 0, false)
	control(7*time.Second, 'P')
	consume(20*time.Second, 8*time.Second, 0, false)
	control(20*time.Second, 'P')
	consume(21*time.Second, 10*time.Second, 1, true)
	control(21*time.Second, 0x76) // 2 -> 0.5 while D is held.
	consume(24*time.Second, 11500*time.Millisecond, 1, true)
	consume(25*time.Second, 12*time.Second, 2, false)
	after, err := os.ReadFile(g.opts.replay)
	if err != nil || !bytes.Equal(encoded.Bytes(), after) {
		t.Fatal("changing replay speed modified the recording file")
	}
	if g.due[0].At != 12*time.Second {
		t.Fatal("consumed event was rescaled with playback speed")
	}
}
