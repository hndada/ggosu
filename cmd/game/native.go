//go:build !js

package main

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"ggosu/internal/play"
	"ggosu/internal/settings"
)

func run() error {
	if len(os.Args) == 2 && os.Args[1] == "-resident" {
		return play.Serve(os.Stdin, os.Stdout, func(args []string) (play.Config, error) {
			c, warm, err := options(args)
			if err == nil && warm {
				return c, fmt.Errorf("warm is not a play request")
			}
			return c, err
		})
	}
	c, warm, err := options(os.Args[1:])
	if err == flag.ErrHelp {
		return nil
	}
	if err != nil {
		return err
	}
	if warm {
		return play.Warm(c.Chart, c.Cache)
	}
	return play.Run(c)
}

func options(args []string) (play.Config, bool, error) {
	f := flag.NewFlagSet("game", flag.ContinueOnError)
	f.SetOutput(os.Stderr)
	var c play.Config
	var names, cover string
	var warm bool
	f.BoolVar(&warm, "warm", false, "prepare keysound PCM cache without playing")
	f.StringVar(&c.Cache, "cache", "", "PCM cache directory")
	f.BoolVar(&c.Display.LaneLines, "lane-lines", false, "show lane separator lines")
	f.StringVar(&c.Settings, "settings", "", "preferences file for HUD layout and volume")
	f.BoolVar(&c.HUDEdit, "hud-edit", false, "loop Auto play in the HUD editor (requires -auto)")
	f.StringVar(&c.Chart, "chart", "songs/demo/demo.osu", "source chart (.osu/.ojn/.bms/.sm/.ggc)")
	f.IntVar(&c.Index, "index", 0, "difficulty index")
	f.StringVar(&c.Input, "input", "native", "native or ebiten input")
	f.StringVar(&names, "keys", "", "comma-separated physical lane keys")
	f.IntVar(&c.Health, "health", 3, "starting health stage: 0 off, 1 easy, 2 normal, 3 hard")
	f.IntVar(&c.HealthFail, "health-fail", 0, "game over when this stage empties: 0 continue, 1..3")
	f.BoolVar(&c.Auto, "auto", false, "autoplay through lane replay")
	f.StringVar(&c.Replay, "replay", "", "lane JSON or osu!mania OSR replay")
	f.StringVar(&c.ReplayRecord, "replay-record", "", "replay from a saved local result")
	f.StringVar(&c.Result, "result", "", "save result and replay on normal exit")
	f.StringVar(&c.Record, "record", "", "write lane replay on normal exit")
	f.StringVar(&c.Report, "report", "", "write score report on normal exit")
	f.Float64Var(&c.Rate, "speed", 1, "playback rate (0.5..2.0)")
	f.BoolVar(&c.Pitch, "pitch", true, "change pitch with playback rate; false preserves original pitch")
	f.StringVar(&c.Mod, "mod", "", "lane mod: mirror, random or mirror+random")
	f.StringVar(&c.Visibility, "visibility", "", "visibility mod: hidden, fadein, cover or flashlight")
	f.StringVar(&c.O2Jam, "o2jam", "", "O2Jam event mod life bar: easy, normal or hard (beat judgement, replaces -health)")
	f.BoolVar(&c.Pills, "pills", false, "O2Jam pills: every 15 KOOLs turns a later GOOD into a COOL")
	f.UintVar(&c.Seed, "seed", 0, "random seed (uint32); retry with the same seed keeps the layout")
	f.Float64Var(&c.Profile.Scroll, "scroll", settings.DefaultScroll, "osu!lazer mania scroll speed (1..40)")
	f.StringVar(&c.Profile.ScrollMode, "scroll-mode", "speed", "speed or exposure")
	f.IntVar(&c.Profile.ExposureMS, "exposure", 750, "nominal real-time note exposure ms (100..5000)")
	f.Float64Var(&c.Profile.SpeedChange, "speed-change", 1, "visual scroll multiplier variation scale (0..2)")
	f.StringVar(&cover, "cover", "", "lane cover sudden,hidden,lift as 0..1, \"+\" marks one on, e.g. 0.2+,0.1,0.1")
	f.IntVar(&c.Audio.Master, "master-volume", 100, "master volume percent")
	f.IntVar(&c.Audio.Music, "music-volume", 100, "music volume percent")
	f.IntVar(&c.Audio.Keys, "key-volume", 100, "key sound volume percent")
	f.IntVar(&c.Audio.Effects, "effect-volume", 50, "playback control effect volume percent")
	f.IntVar(&c.Audio.Hit, "hit-volume", 0, "hit sound volume percent on every key press (0 off)")
	f.StringVar(&c.Audio.HitSound, "hit-sound", "", "hit sound: click, wood, glass or pluck")
	f.IntVar(&c.Audio.Guide, "guide-volume", 0, "guide sound volume percent at every note's judgement time (0 off)")
	f.IntVar(&c.Display.Dim, "dim", 75, "background dim percent")
	f.BoolVar(&c.Display.Background, "background", true, "show background images")
	f.BoolVar(&c.Display.Storyboard, "storyboard", true, "show supported storyboard sprites")
	f.StringVar(&c.Display.HUD, "hud", "standard", "standard or focus HUD")
	f.IntVar(&c.Display.NoteThickness, "note-thickness", 100, "note thickness percent (50..200)")
	f.StringVar(&c.Display.Skin, "skin", "argon", "play skin: argon, triangles, classic, circles or o2jam")
	f.StringVar(&c.Display.TimeDisplay, "time-display", "pie", "time display: pie or bar")
	f.StringVar(&c.Display.Ghost, "ghost", "target", "target, max or off")
	f.IntVar(&c.Display.GhostScore, "ghost-score", 900000, "ghost target points")
	f.IntVar(&c.Display.TPS, "tps", 240, "ticks per second (30..1000); -1 follows FPS")
	f.StringVar(&c.Display.FrameLimit, "frame-limit", "vsync", "vsync or unlimited FPS")
	f.StringVar(&c.Display.ComboPosition, "combo-position", "center", "combo position: center, below or left")
	f.StringVar(&c.Display.JudgementPosition, "judgement-position", "center", "judgement position: center, below or left")
	f.StringVar(&c.Display.TimingThreshold, "timing-threshold", "cool", "Early/Late threshold: kool, cool, good or off")
	f.IntVar(&c.Display.LaneDim, "lane-dim", 0, "lane background dim percent (100 hides the lanes)")
	f.StringVar(&c.Display.NoteColour, "note-colour", "", "note colour: empty for the skin, snap or assist")
	f.StringVar(&c.Display.JudgementShow, "judgement-show", "", "show judgements of this grade or worse: cool, good, miss; off hides them")
	f.BoolVar(&c.Display.HideCombo, "hide-combo", false, "hide the combo counter")
	f.BoolVar(&c.Display.Abort, "abort", false, "end a live play once the ghost target score is out of reach")
	f.StringVar(&c.Display.WindowMode, "window-mode", "windowed", "windowed, borderless or fullscreen")
	f.StringVar(&c.Display.Resolution, "resolution", "", "window resolution, e.g. 1280x720 or 1920x1080 (default 1600x900)")
	f.BoolVar(&c.Display.Letterbox, "letterbox", false, "shrink the game area (osu!lazer screen scaling)")
	f.IntVar(&c.Display.LetterboxWidth, "letterbox-width", 80, "letterbox width percent (20..100)")
	f.IntVar(&c.Display.LetterboxHeight, "letterbox-height", 80, "letterbox height percent (20..100)")
	f.IntVar(&c.Display.LetterboxX, "letterbox-x", 0, "letterbox horizontal position from centre (-50..50)")
	f.IntVar(&c.Display.LetterboxY, "letterbox-y", 0, "letterbox vertical position from centre (-50..50)")
	f.IntVar(&c.Display.CursorSize, "cursor-size", 100, "in-window cursor size percent (50..200)")
	f.IntVar(&c.Offsets.GlobalMS, "offset", 0, "global note/judge offset ms (-1000..1000, positive delays notes)")
	f.IntVar(&c.Offsets.ChartMS, "chart-offset", 0, "additional chart note/judge offset ms (-1000..1000)")
	f.IntVar(&c.Offsets.JudgeMS, "judge-offset", 0, "judgement-only offset ms; notes keep their place (-1000..1000, positive judges later)")
	f.BoolVar(&c.Mute, "mute", false, "disable audio")
	f.BoolVar(&c.Ready, "ready", false, "notify parent after first draw")
	f.IntVar(&c.Frames, "frames", 0, "exit after this many drawn frames")
	f.DurationVar(&c.Duration, "duration", 0, "exit after this real-time duration")
	f.DurationVar(&c.SectionStart, "section-start", 0, "practice section start on the song timeline")
	f.DurationVar(&c.SectionEnd, "section-end", 0, "practice section end; loops the section without a record")
	f.StringVar(&c.Capture, "capture", "", "save final frame PNG (requires -frames)")
	f.StringVar(&c.Server, "multi", "", "multiplayer WebSocket URL, e.g. ws://127.0.0.1:8090/ws")
	f.StringVar(&c.Name, "name", "Player", "player nickname for multiplayer and score submission")
	f.StringVar(&c.ScoreServer, "score-server", "", "standalone live-score submission server URL")
	f.StringVar(&c.Room, "room", "", "join this room ID; empty creates a room")
	f.StringVar(&c.RoomName, "room-name", "gosu room", "name of a new multiplayer room")
	f.StringVar(&c.RoomPassword, "room-password", "", "password for creating or joining a multiplayer room")
	f.BoolVar(&c.MultiStart, "multi-start", false, "host starts once everyone is ready (otherwise F5)")
	f.StringVar(&c.MultiToken, "multi-token", "", "single-use lobby game attachment")
	f.Uint64Var(&c.MultiRevision, "multi-revision", 0, "lobby selection revision")
	f.StringVar(&c.MultiReport, "multi-report", "", "write synchronization and score-comparison report")
	f.BoolVar(&c.ChatOpen, "chat-open", false, "start with the multiplayer chat drawer open")
	f.BoolVar(&c.Debug, "debug", false, "show FLOW/PREC values and per-note strain as small text")
	if err := f.Parse(args); err != nil {
		return c, warm, err
	}
	if names != "" {
		c.Profile.Keys = strings.Split(names, ",")
	}
	if cover != "" {
		lane, err := settings.ParseCover(cover)
		if err != nil {
			return c, warm, err
		}
		c.Profile.Cover = &lane
	}
	if f.NArg() != 0 {
		return c, warm, fmt.Errorf("unexpected game arguments: %v", f.Args())
	}
	return c, warm, nil
}
