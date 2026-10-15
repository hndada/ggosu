package play

import (
	"testing"
	"time"

	"ggosu/internal/chart"
)

func TestOutroEscapePreservesCompleteScoreAndFade(t *testing.T) {
	for _, rate := range []float64{.5, 1, 2} {
		g := pauseGame(t, rate)
		if err := g.session.Input(10*time.Second, 0, true); err != nil {
			t.Fatal(err)
		}
		if err := g.session.Advance(10 * time.Second); err != nil {
			t.Fatal(err)
		}
		g.position = 10 * time.Second
		score := g.session.Score
		now := 30 * time.Second
		if !g.skipOutro(now) || !g.completed || !g.finishing || !g.ended || g.stopRequested || g.paused {
			t.Fatalf("rate %v: Escape did not start a completed transition", rate)
		}
		if g.endedAt+endHold != now || g.endedAt+endHold+endFade != now+250*time.Millisecond {
			t.Fatalf("rate %v: Escape retained a hold or skipped the fade", rate)
		}
		started := g.endedAt
		if !g.skipOutro(now+100*time.Millisecond) || g.endedAt != started || g.session.Score != score {
			t.Fatalf("rate %v: repeated Escape changed the fade or score", rate)
		}
	}
}

func TestOutroCannotBePaused(t *testing.T) {
	for _, reason := range []string{"escape", "key-p", "focus", "visibility", "browser-control"} {
		g := pauseGame(t, 1)
		if err := g.session.Input(10*time.Second, 0, true); err != nil {
			t.Fatal(err)
		}
		if err := g.session.Advance(10 * time.Second); err != nil {
			t.Fatal(err)
		}
		g.position = 10 * time.Second
		score := g.session.Score
		if err := g.pauseFor(11*time.Second, reason); err != nil {
			t.Fatal(err)
		}
		if g.paused || g.resumeAt != 0 || len(g.recording.Pauses) != 0 || g.timeline.Position(12*time.Second) != 12*time.Second {
			t.Fatalf("%s froze OUTRO or recorded a pause", reason)
		}
		if !g.skipOutro(12*time.Second) || g.paused || g.session.Score != score || !g.completed {
			t.Fatalf("%s did not preserve the result transition", reason)
		}
	}
}

func TestOutroEscapeDoesNotSkipUnfinishedPlay(t *testing.T) {
	g := pauseGame(t, 1)
	g.position = 9 * time.Second
	if g.skipOutro(20*time.Second) || g.completed || g.finishing {
		t.Fatal("Escape completed a play with unjudged notes")
	}
	// A judged final head is insufficient while its long note is still active.
	g.chart.Notes[0].Kind, g.chart.Notes[0].End = "hold", 12*time.Second
	if err := g.session.Input(10*time.Second, 0, true); err != nil {
		t.Fatal(err)
	}
	g.position = 11 * time.Second
	if g.skipOutro(20 * time.Second) {
		t.Fatal("Escape skipped an active long note")
	}
}

func TestOutroEscapeWaitsForScheduledAudioAndKeepsEditorControls(t *testing.T) {
	g := pauseGame(t, 1)
	g.chart.Audio = []chart.Audio{{At: 12 * time.Second}}
	if err := g.session.Input(10*time.Second, 0, true); err != nil {
		t.Fatal(err)
	}
	g.position = 10 * time.Second
	if g.skipOutro(20 * time.Second) {
		t.Fatal("Escape skipped audio scheduled before OUTRO")
	}
	g.position = 12 * time.Second
	if err := g.session.Advance(g.position); err != nil {
		t.Fatal(err)
	}
	if !g.afterNotes() {
		t.Fatal("judged final note did not end the note section")
	}
	for _, mode := range []string{"editor", "loop", "incomplete replay"} {
		g.editing, g.hudEdit, g.replayIncomplete = mode == "editor", mode == "loop", mode == "incomplete replay"
		if g.skipOutro(20 * time.Second) {
			t.Fatal("Escape replaced controls for", mode)
		}
	}
}
