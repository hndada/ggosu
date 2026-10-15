//go:build !js

package play

import (
	"time"

	"ggosu/internal/settings"
)

// Config supplies the native game's chart, playback settings and optional outputs.
// Command-line defaults and flag parsing belong to cmd/game.
type Config struct {
	ScoreServer   string // Optional score submission for standalone CLI play; the menu owns its outbox.
	Visibility    string
	O2Jam         string // Event mod life bar: easy, normal or hard.
	Pills         bool
	Cache         string // Optional PCM cache directory; empty uses the user cache.
	Health        int
	HealthFail    int
	Settings      string        // Optional preferences file, used to persist HUD edits and volume.
	HUDEdit       bool          // Loops Auto play under the HUD editor; closing the editor exits.
	SectionStart  time.Duration // Practice section; a non-zero SectionEnd loops [start, end) without a record.
	SectionEnd    time.Duration
	Chart         string
	Index         int
	Input         string
	Auto          bool
	Replay        string
	ReplayRecord  string
	Result        string
	Record        string
	Report        string
	Rate          float64
	Pitch         bool
	Mod           string
	Seed          uint
	Profile       settings.Profile
	Audio         settings.Audio
	Display       settings.Display
	Offsets       settings.Offsets
	Mute          bool
	Ready         bool
	Frames        int
	Duration      time.Duration
	Capture       string
	Server        string
	Name          string
	Room          string
	RoomName      string
	RoomPassword  string
	MultiStart    bool
	MultiToken    string
	MultiRevision uint64
	MultiReport   string
	ChatOpen      bool
	Debug         bool // Shows FLOW/PREC and per-note strain as small text; play itself hides them.
}
