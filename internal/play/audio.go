package play

import (
	"slices"
	"time"

	"ggosu/internal/chart"
)

func (g *scene) playAudio() error {
	if g.bank != nil {
		if err := g.bank.PrimeMusic(g.position); err != nil {
			return err
		}
		for _, s := range g.session.Music {
			if err := g.bank.PlayMusic(s, g.position); err != nil {
				return err
			}
		}
		for _, s := range g.session.Sounds {
			if err := g.bank.Play(s); err != nil {
				return err
			}
		}
	}
	if err := g.guideSound(); err != nil {
		return err
	}
	if err := g.comboBreak(); err != nil {
		return err
	}
	g.session.Sounds = g.session.Sounds[:0]
	g.session.Music = g.session.Music[:0]
	if !g.startCue && g.position >= 0 && !g.paused {
		g.startCue = true
		if g.bank != nil {
			if err := g.bank.Effect("start", 1); err != nil {
				return err
			}
		}
	}
	return nil
}

// osu!lazer ComboEffects: a combo break plays when a combo above 20 drops to zero, and the
// first break of an attempt always plays (AlwaysPlayFirstComboBreak, on by default).
func (g *scene) comboBreak() error {
	if g.comboBreakDue() && g.bank != nil {
		return g.bank.Effect("combobreak", 1)
	}
	return nil
}

func (g *scene) comboBreakDue() bool {
	combo := g.session.Score.Combo
	broke := combo == 0 && g.lastCombo > 0
	due := broke && (g.lastCombo > 20 || !g.comboBroken)
	g.lastCombo = combo
	g.comboBroken = g.comboBroken || broke
	return due
}

// A guide later than this is skipped, as when a practice section starts mid-song.
const guideLate = 50 * time.Millisecond

// guideSound plays ONGEKI's guide sound once at each judgement time: note heads
// and long-note ends, with chords sounding once.
func (g *scene) guideSound() error {
	for ; g.guide < len(g.guides) && g.guides[g.guide] <= g.position; g.guide++ {
		at := g.guides[g.guide]
		if g.bank == nil || g.guide > 0 && g.guides[g.guide-1] == at || g.position-at > guideLate {
			continue
		}
		if err := g.bank.Cue("guide", g.audio.Guide); err != nil {
			return err
		}
	}
	return nil
}

func guideTimes(c *chart.Chart) []time.Duration {
	var out []time.Duration
	for _, n := range c.Notes {
		if n.Kind == "mine" || n.Kind == "fake" {
			continue
		}
		out = append(out, n.At)
		if n.End > n.At {
			out = append(out, n.End)
		}
	}
	slices.Sort(out)
	return out
}
