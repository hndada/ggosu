package play

import (
	"image"
	"time"

	"github.com/hajimehoshi/ebiten/v2"

	"ggosu/internal/chart"
	"ggosu/internal/clock"
	"ggosu/internal/keyboard"
	"ggosu/internal/piano"
	"ggosu/internal/settings"
)

type scene struct {
	scoreServer                        string
	playerName                         string
	roomPassword                       string
	preview                            bool
	previewItem                        hudItem
	previewRevision                    int
	previewAt                          time.Duration
	hudReadAt                          time.Duration
	hudStamp                           time.Time
	health                             int
	healthFail                         int
	settingsPath                       string
	hudEdit                            bool
	editing                            bool
	editor                             hudEditor
	display                            settings.Display
	chart                              *chart.Chart
	session                            *piano.Session
	clock                              *clock.Counter
	timeline                           *clock.Timeline
	source                             *keyboard.Source
	box                                keyboard.Queue
	events                             []keyboard.Event
	bindings                           []binding
	physical                           []bool
	player                             *piano.Player
	due                                []piano.Event
	recording                          piano.Replay
	bank                               *bank
	images                             map[string]*ebiten.Image
	warnings                           []string
	position, last                     time.Duration
	preroll                            time.Duration
	runFor, startAt                    time.Duration
	sectionStart, sectionEnd           time.Duration // Practice section; zero end is the whole chart.
	sectionStop                        time.Duration // Song position where the section loops back.
	resumeAt, inputSince               time.Duration
	pauseChoice                        int
	retries                            int
	pauseCursor                        image.Point
	speed, scroll                      float64
	scrollUntil                        time.Duration
	retryAt                            time.Duration // When the held retry key went down.
	retryHeld, retryLatched            bool          // Latched waits for release after a retry.
	pitch                              bool
	paused, focused, started, autoplay bool
	ready, announced                   bool
	loopShown, loopFocused             bool // Pause-loop target last sent to the bank.
	lastCombo                          int  // Combo at the previous update, for the combo-break sound.
	comboBroken                        bool // Whether this attempt has had a combo break yet.
	finishing                          bool
	aborted                            bool // The ghost target went out of reach with Abort on.
	captured                           bool
	completed, practice                bool
	stopRequested                      bool
	debug                              bool
	replayIncomplete                   bool
	replayEnd                          time.Duration
	initialRate                        float64
	offsets                            settings.Offsets
	profile                            settings.Profile
	mods                               chart.Mod
	audio                              settings.Audio
	guides                             []time.Duration // Judgement times of the guide sound, sorted.
	guide                              int             // Next entry of guides.
	volume                             volume
	startCue, finishCue                bool
	screenshot                         bool        // F12 was pressed; the next Draw saves the frame.
	shots                              chan string // Paths of saved screenshots, for toasts.
	toasts                             []*toast
	toastImage                         *ebiten.Image
	english                            bool          // The menu language is English.
	ended                              bool          // The chart finished normally; the end hold is running.
	endedAt                            time.Duration // Clock time the chart finished.
	view                               *view
	chat                               chat
	chatPrefs                          settings.Chat
	mode, result                       string
	frames, limit                      int
	capture, record, report            string
	err                                error
	network                            *network
	frame                              *ebiten.Image   // The fixed-size frame before letterboxing.
	area                               image.Rectangle // Where the frame was last drawn, in screen pixels.
	deviceScale                        float64
	mouse                              image.Point // In screen pixels.
	mouseSeen, cursorShown             bool
	keysPressed                        []ebiten.Key
}
