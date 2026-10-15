package play

import (
	"math"
	"testing"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/audio"

	"ggosu/internal/chart"
	"ggosu/internal/settings"
)

func TestVolumeFirstPressAndBounds(t *testing.T) {
	g := scene{audio: settings.Audio{Master: 50, Music: 70, Keys: 80, Effects: 30}, paused: true, editing: true}
	now := time.Second
	g.volumeKey(ebiten.KeyArrowDown, now)
	if g.audio.Master != 45 || !g.volume.dirty || !g.paused || !g.editing {
		t.Fatal("first press did not change volume independently of pause/editor", g.audio)
	}
	g.volumeKey(ebiten.KeyArrowRight, now)
	g.volumeKey(ebiten.KeyArrowDown, now)
	if g.audio.Music != 65 || g.audio.Master != 45 {
		t.Fatal("wrong volume selected", g.audio)
	}
	g.audio.Music = 0
	g.volumeKey(ebiten.KeyArrowDown, now)
	if g.audio.Music != 0 {
		t.Fatal("volume below zero")
	}
	g.audio.Music = 100
	g.volumeKey(ebiten.KeyArrowUp, now)
	if g.audio.Music != 100 {
		t.Fatal("volume above 100")
	}
	g.volumeKey(ebiten.KeyArrowDown, now+2*time.Second)
	if g.audio.Master != 40 || g.audio.Music != 100 {
		t.Fatal("hidden overlay did not return to master", g.audio)
	}
}

func TestVolumeUpdatesExistingAndPrimedPlayers(t *testing.T) {
	ctx := audio.CurrentContext()
	if ctx == nil {
		ctx = audio.NewContext(sampleRate)
	}
	// Silent PCM permits real player checks without playing the demo song.
	pcm := make([]byte, sampleRate*4)
	b := bank{ctx: ctx, pcm: map[string][]byte{"music": pcm, "key": pcm}, rate: 1, paused: true,
		effects: map[string][]byte{"start": pcm}}
	defer b.Close()
	a := settings.Audio{Master: 0, Music: 60, Keys: 40, Effects: 20}
	b.SetAudio(a)
	s := chart.Sound{Asset: "music", Volume: .8}
	if err := b.PrepareMusic([]chart.Audio{{Sound: s}, {At: 100 * time.Millisecond, Sound: s}}); err != nil {
		t.Fatal(err)
	}
	primed := b.music[1].player
	if err := b.PlayMusic(s, 0); err != nil {
		t.Fatal(err)
	}
	if err := b.Play(chart.Sound{Asset: "key", Volume: .5}); err != nil {
		t.Fatal(err)
	}
	players := []*audio.Player{b.voices[0].player, b.voices[1].player}
	a.Master = 50
	b.SetAudio(a)
	for i, want := range []float64{.24, .1} {
		if b.voices[i].player != players[i] || math.Abs(players[i].Volume()-want) > 1e-9 {
			t.Fatal("volume did not restore from mute without replacing player", i, players[i].Volume())
		}
	}
	if b.music[1].player != primed || math.Abs(primed.Volume()-.24) > 1e-9 {
		t.Fatal("primed music retained old volume")
	}
	if err := b.Effect("start", .5); err != nil {
		t.Fatal(err)
	}
	if math.Abs(b.voices[2].player.Volume()-.05) > 1e-9 {
		t.Fatal("effect did not use master and effects")
	}
	a.Master = 100
	b.SetAudio(a)
	if math.Abs(b.voices[2].player.Volume()-.1) > 1e-9 {
		t.Fatal("playing effect did not update")
	}
	if err := b.SetRate(1.5); err != nil {
		t.Fatal(err)
	}
	if math.Abs(b.voices[0].player.Volume()-.48) > 1e-9 || math.Abs(b.voices[1].player.Volume()-.2) > 1e-9 {
		t.Fatal("rate change lost the volume")
	}
	if err := b.PlayMusic(s, 100*time.Millisecond); err != nil {
		t.Fatal("volume adjustment changed prepared music order", err)
	}
}
