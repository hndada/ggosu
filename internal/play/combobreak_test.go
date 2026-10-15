package play

import (
	"testing"

	"ggosu/internal/piano"
)

func TestComboBreakFollowsLazer(t *testing.T) {
	for _, c := range []struct {
		from, to    int
		brokenEarly bool
		want        bool
	}{
		{3, 0, false, true}, // the first break always plays
		{3, 0, true, false}, // later short combos stay quiet
		{21, 0, true, true}, // above 20 plays
		{20, 0, true, false},
		{0, 0, false, false}, // no combo to break
		{5, 6, false, false},
	} {
		g := scene{session: &piano.Session{}, lastCombo: c.from, comboBroken: c.brokenEarly}
		g.session.Score.Combo = c.to
		if got := g.comboBreakDue(); got != c.want || g.lastCombo != c.to {
			t.Errorf("%d -> %d (broken before %v): play %v, want %v", c.from, c.to, c.brokenEarly, got, c.want)
		}
	}
}

func TestComboBreakSampleDecodes(t *testing.T) {
	data, err := sounds.ReadFile("sounds/combobreak.ogg")
	if err != nil {
		t.Fatal(err)
	}
	pcm, err := decode(data, ".ogg", maxPCM)
	if err != nil || len(pcm) < sampleRate/10*4 {
		t.Fatalf("combobreak decoded to %d bytes: %v", len(pcm), err)
	}
}
