package play

import (
	"reflect"
	"testing"
	"time"

	"ggosu/internal/clock"
	"ggosu/internal/piano"
)

func TestRestartClearsAttemptAndRewindsReplay(t *testing.T) {
	for _, mode := range []string{"live", "auto", "replay"} {
		t.Run(mode, func(t *testing.T) {
			g := pauseGame(t, 1.5)
			g.playerName = "고수"
			g.mode, g.autoplay = mode, mode == "auto"
			g.preroll, g.result = 2*time.Second, "test"
			if err := g.prepare(); err != nil {
				t.Fatal(err)
			}
			if mode == "replay" {
				f, err := piano.Auto(g.session.Chart, piano.Rules)
				if err != nil {
					t.Fatal(err)
				}
				g.player, err = piano.NewPlayer(f)
				if err != nil {
					t.Fatal(err)
				}
			}
			hash, err := g.chart.Digest()
			if err != nil {
				t.Fatal(err)
			}
			var first []piano.Event
			if g.player != nil {
				first, err = g.player.AppendDue(nil, 11*time.Second)
				if err != nil {
					t.Fatal(err)
				}
			}
			if err := g.input(10*time.Second, 0, true); err != nil {
				t.Fatal(err)
			}
			g.startCue, g.finishCue, g.paused, g.completed, g.finishing = true, true, true, true, true
			g.resumeAt = 30 * time.Second
			if err := g.restart(20 * time.Second); err != nil {
				t.Fatal(err)
			}
			if g.session.Score.Total != 0 || g.session.Held[0] || len(g.recording.Events) != 0 || g.position != -g.preroll || g.last != g.position || g.paused || g.completed || g.finishing || g.resumeAt != 0 || g.startCue || g.finishCue || g.inputSince != 20*time.Second {
				t.Fatal("restart retained the previous attempt")
			}
			if g.recording.Player != "고수" {
				t.Fatal("restart lost the player name")
			}
			if g.timeline.Position(21*time.Second) != 1500*time.Millisecond {
				t.Fatal("restart changed playback rate or skipped preroll")
			}
			if after, err := g.chart.Digest(); err != nil || hash != after {
				t.Fatal("restart changed source chart")
			}
			if g.player != nil {
				again, err := g.player.AppendDue(nil, 11*time.Second)
				if err != nil || !reflect.DeepEqual(first, again) {
					t.Fatal("restart lost or rescaled replay events")
				}
			}
		})
	}
}

func TestRestartKeepsMultiplayerTimeline(t *testing.T) {
	g := pauseGame(t, 1)
	g.network = &network{}
	g.timeline = clock.NewTimeline(0)
	before := g.session
	if err := g.restart(time.Second); err != nil || g.session != before {
		t.Fatal("personal restart replaced a synchronized session")
	}
}
