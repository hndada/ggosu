// Package osu converts osu!mania .osu beatmaps and .osb storyboards to charts.
package osu

import (
	"bufio"
	"bytes"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"ggosu/internal/chart"
	"ggosu/internal/chart/internal/parse"
)

// Format reads osu!mania beatmaps and merges .osb storyboards beside them.
type Format struct{}

func init() { chart.Register(Format{}) }

func (Format) Extensions() []string      { return []string{".osu"} }
func (Format) MergeExtensions() []string { return []string{".osb"} }

type osuPoint struct {
	at                 time.Duration
	set, index, volume int
}

func (Format) Parse(b []byte) ([]*chart.Chart, error) {
	c := chart.New(0)
	section := ""
	mode := 0
	defaultSet := 1
	header := false
	line := 0
	var points []osuPoint
	var objects []string
	var events []string
	s := bufio.NewScanner(bytes.NewReader(b))
	s.Buffer(make([]byte, 4096), 1<<20)
	for s.Scan() {
		line++
		v := strings.TrimSpace(strings.TrimPrefix(s.Text(), "\ufeff"))
		if v == "" || strings.HasPrefix(v, "//") {
			continue
		}
		if !header {
			if !strings.HasPrefix(v, "osu file format v") {
				return nil, fmt.Errorf("line %d: missing osu header", line)
			}
			header = true
			continue
		}
		if strings.HasPrefix(v, "[") {
			section = v
			continue
		}
		n := parse.Numbers{}
		switch section {
		case "[General]", "[Metadata]", "[Difficulty]":
			key, value, ok := strings.Cut(v, ":")
			if !ok {
				return nil, fmt.Errorf("line %d: expected key:value", line)
			}
			value = strings.TrimSpace(value)
			switch key {
			case "SampleSet":
				switch value {
				case "Soft":
					defaultSet = 2
				case "Drum":
					defaultSet = 3
				case "Normal", "None":
					// Legacy None inherits the normal bank unless timing overrides it.
					defaultSet = 1
				default:
					return nil, fmt.Errorf("invalid osu sample set %q", value)
				}
			case "Mode":
				mode = n.Int(value)
			case "CircleSize":
				f := n.Float(value)
				if f != math.Trunc(f) {
					return nil, fmt.Errorf("fractional key count")
				}
				c.Lanes = int(f)
			case "AudioFilename":
				if value != "" && value != "virtual" {
					c.Audio = append(c.Audio, chart.Audio{Sound: chart.Sound{Asset: parse.Asset(c, "music", value, "audio"), Volume: 1}})
				}
			case "Title":
				c.Metadata.Title = value
			case "TitleUnicode":
				if value != "" {
					c.Metadata.Title = value
				}
			case "Artist":
				c.Metadata.Artist = value
			case "ArtistUnicode":
				if value != "" {
					c.Metadata.Artist = value
				}
			case "Creator":
				c.Metadata.Creator = value
			case "Version":
				c.Metadata.Difficulty = value
			case "OverallDifficulty":
				c.Metadata.Level = value
			case "PreviewTime":
				c.Metadata.Preview = n.Millis(value)
			case "AudioLeadIn":
				c.Metadata.LeadIn = n.Millis(value)
			}
		case "[TimingPoints]":
			f, err := parse.Fields(v)
			if err != nil || len(f) < 8 {
				return nil, fmt.Errorf("line %d: malformed timing point", line)
			}
			at, err := chart.Seconds(n.Float(f[0]) / 1000)
			if err != nil {
				return nil, err
			}
			beat := n.Float(f[1])
			meter := n.Int(f[2])
			set := n.Int(f[3])
			index := n.Int(f[4])
			volume := n.Int(f[5])
			base := n.Int(f[6])
			effects := n.Int(f[7])
			if n.Err != nil {
				return nil, n.Err
			}
			if volume < 0 || volume > 100 || set < 0 || set > 3 || index < 0 {
				return nil, fmt.Errorf("line %d: invalid sample settings", line)
			}
			t := chart.Timing{At: at, Scroll: 1, Meter: meter, Highlight: effects&1 != 0}
			if len(c.Timing) > 0 {
				prev := c.Timing[len(c.Timing)-1]
				if at < prev.At {
					return nil, fmt.Errorf("unordered osu timing")
				}
				t.BPM = prev.BPM
				t.Meter = prev.Meter
			}
			if base == 1 {
				if beat <= 0 {
					return nil, fmt.Errorf("invalid beat length")
				}
				t.BPM = 60000 / beat
				t.Meter = meter
			} else if base == 0 {
				if beat >= 0 || t.BPM == 0 {
					return nil, fmt.Errorf("inherited timing without base BPM")
				}
				t.Scroll = min(10, max(.1, -100/beat))
			} else {
				return nil, fmt.Errorf("invalid timing inheritance")
			}
			c.Timing = append(c.Timing, t)
			points = append(points, osuPoint{at, set, index, volume})
		case "[HitObjects]":
			objects = append(objects, v)
		case "[Events]":
			events = append(events, s.Text())
		}
		if n.Err != nil {
			return nil, fmt.Errorf("line %d: %w", line, n.Err)
		}
	}
	if err := s.Err(); err != nil {
		return nil, err
	}
	if !header || mode != 3 {
		return nil, fmt.Errorf("only osu!mania mode 3 is supported")
	}
	if c.Lanes < 1 || c.Lanes > 18 {
		return nil, fmt.Errorf("invalid mania key count")
	}
	if len(points) == 0 {
		return nil, fmt.Errorf("no base timing point")
	}
	for i, v := range objects {
		f, err := parse.Fields(v)
		if err != nil || len(f) < 5 {
			return nil, fmt.Errorf("hit object %d: malformed record", i)
		}
		n := parse.Numbers{}
		x := n.Int(f[0])
		at, err := chart.Seconds(n.Float(f[2]) / 1000)
		if err != nil {
			return nil, err
		}
		typ := n.Int(f[3])
		hit := n.Int(f[4])
		note := chart.Note{At: at, End: at, Lane: min(c.Lanes-1, max(0, min(512, max(0, x))*c.Lanes/512)), Kind: "tap"}
		sample := "0:0:0:0:"
		if typ&128 != 0 {
			if len(f) < 6 {
				return nil, fmt.Errorf("hold without end time")
			}
			end, rest, ok := strings.Cut(f[5], ":")
			if !ok {
				return nil, fmt.Errorf("hold without sample fields")
			}
			note.End, err = chart.Seconds(n.Float(end) / 1000)
			if err != nil {
				return nil, err
			}
			note.Kind = "hold"
			sample = rest
		} else if typ&1 != 0 {
			if len(f) > 5 {
				sample = f[5]
			}
		} else {
			return nil, fmt.Errorf("unsupported mania object type %d", typ)
		}
		p := points[0]
		for _, next := range points {
			if next.at > at {
				break
			}
			p = next
		}
		hs := strings.SplitN(sample, ":", 5)
		if len(hs) != 5 {
			return nil, fmt.Errorf("invalid hit sample")
		}
		set := n.Int(hs[0])
		add := n.Int(hs[1])
		index := n.Int(hs[2])
		volume := n.Int(hs[3])
		if n.Err != nil {
			return nil, n.Err
		}
		if set == 0 {
			set = p.set
		}
		if set == 0 {
			set = defaultSet
		}
		if add == 0 {
			add = set
		}
		if index == 0 {
			index = p.index
		}
		if volume == 0 {
			volume = p.volume
		}
		if set < 1 || set > 3 || add < 1 || add > 3 || index < 0 || volume < 0 || volume > 100 {
			return nil, fmt.Errorf("invalid hit sample values")
		}
		if hs[4] != "" {
			note.Sounds = []chart.Sound{{Asset: parse.Asset(c, hs[4], hs[4], "audio"), Volume: float64(volume) / 100}}
		} else {
			suffix := ""
			if index > 1 {
				suffix = strconv.Itoa(index)
			}
			sets := []string{"", "normal", "soft", "drum"}
			kinds := []string{"normal", "whistle", "finish", "clap"}
			for j, kind := range kinds {
				if j == 0 && hit != 0 && hit&1 == 0 {
					continue
				}
				if j > 0 && hit&(1<<j) == 0 {
					continue
				}
				bank := set
				if j > 0 {
					bank = add
				}
				path := sets[bank] + "-hit" + kind + suffix + ".wav"
				note.Sounds = append(note.Sounds, chart.Sound{Asset: parse.Asset(c, path, path, "audio"), Volume: float64(volume) / 100})
			}
		}
		c.Notes = append(c.Notes, note)
	}
	if err := osuStoryboard(c, events); err != nil {
		return nil, err
	}
	return []*chart.Chart{c}, nil
}

// Merge adds an external .osb storyboard to c into the chart without changing source
// identity. Digest still includes it, so replay binding detects storyboard changes.
func (Format) Merge(c *chart.Chart, b []byte) error {
	if len(b) > chart.MaxSize {
		return fmt.Errorf("storyboard too large")
	}
	var lines []string
	active := false
	for _, s := range strings.Split(string(b), "\n") {
		if strings.TrimSpace(s) == "[Events]" {
			active = true
			continue
		}
		if strings.HasPrefix(strings.TrimSpace(s), "[") {
			active = false
		}
		if active {
			lines = append(lines, s)
		}
	}
	if err := osuStoryboard(c, lines); err != nil {
		return err
	}
	c.Normalize()
	return c.Validate()
}

func osuStoryboard(c *chart.Chart, lines []string) error {
	current := -1
	skipDepth := -1
	for _, raw := range lines {
		v := strings.TrimSpace(raw)
		if v == "" || strings.HasPrefix(v, "//") {
			continue
		}
		f, err := parse.Fields(v)
		if err != nil || len(f) == 0 {
			return fmt.Errorf("malformed storyboard: %s", v)
		}
		n := parse.Numbers{}
		if strings.HasPrefix(raw, " ") || strings.HasPrefix(raw, "_") || strings.HasPrefix(raw, "\t") {
			if current < 0 {
				return fmt.Errorf("orphan storyboard command")
			}
			depth := len(raw) - len(strings.TrimLeft(raw, " _\t"))
			if skipDepth >= 0 && depth > skipDepth {
				c.Storyboard[current].Unsupported = append(c.Storyboard[current].Unsupported, raw)
				continue
			}
			skipDepth = -1
			kind := strings.TrimLeft(f[0], "_")
			dims := map[string]int{"F": 1, "M": 2, "MX": 1, "MY": 1, "S": 1, "V": 2, "R": 1, "C": 3}
			dim, ok := dims[kind]
			if !ok {
				c.Warnings = append(c.Warnings, "unsupported storyboard command: "+v)
				c.Storyboard[current].Unsupported = append(c.Storyboard[current].Unsupported, raw)
				skipDepth = depth
				continue
			}
			if len(f) < 4+dim {
				return fmt.Errorf("short storyboard command: %s", v)
			}
			cmd := chart.Command{Kind: kind, Easing: n.Int(f[1]), At: n.Millis(f[2])}
			cmd.End = cmd.At
			if f[3] != "" {
				cmd.End = n.Millis(f[3])
			}
			for _, s := range f[4:] {
				cmd.Values = append(cmd.Values, n.Float(s))
			}
			if len(cmd.Values) != dim && len(cmd.Values) != dim*2 {
				return fmt.Errorf("invalid command arity: %s", v)
			}
			if cmd.Easing < 0 || cmd.Easing > 2 {
				c.Warnings = append(c.Warnings, fmt.Sprintf("storyboard easing %d rendered linearly", cmd.Easing))
			}
			event := &c.Storyboard[current]
			if len(event.Commands) == 0 {
				event.At = cmd.At
			}
			event.At = min(event.At, cmd.At)
			event.End = max(event.End, cmd.End)
			event.Commands = append(event.Commands, cmd)
		} else {
			current = -1
			skipDepth = -1
			switch f[0] {
			case "0", "Background", "1", "Video":
				if len(f) < 3 {
					return fmt.Errorf("short background event")
				}
				kind := "background"
				if f[0] == "1" || f[0] == "Video" {
					kind = "video"
					c.Warnings = append(c.Warnings, "video events preserved; video decoder is not implemented")
				}
				at := n.Millis(f[1])
				event := chart.StoryEvent{At: at, End: at, Kind: kind, Asset: parse.Asset(c, f[2], f[2], map[bool]string{true: "video", false: "image"}[kind == "video"])}
				if len(f) >= 5 {
					event.X = n.Float(f[3])
					event.Y = n.Float(f[4])
				}
				c.Storyboard = append(c.Storyboard, event)
			case "2", "Break":
				if len(f) < 3 {
					return fmt.Errorf("short break")
				}
				c.Storyboard = append(c.Storyboard, chart.StoryEvent{Kind: "break", At: n.Millis(f[1]), End: n.Millis(f[2])})
			case "Sprite":
				if len(f) < 6 {
					return fmt.Errorf("short sprite")
				}
				c.Storyboard = append(c.Storyboard, chart.StoryEvent{Kind: "sprite", Asset: parse.Asset(c, f[3], f[3], "image"), Layer: f[1], Origin: f[2], X: n.Float(f[4]), Y: n.Float(f[5])})
				current = len(c.Storyboard) - 1
			case "Sample":
				if len(f) < 5 {
					return fmt.Errorf("short storyboard sample")
				}
				c.Audio = append(c.Audio, chart.Audio{At: n.Millis(f[1]), Sound: chart.Sound{Asset: parse.Asset(c, f[3], f[3], "audio"), Volume: n.Float(f[4]) / 100}})
			default:
				c.Warnings = append(c.Warnings, "unsupported storyboard event: "+v)
				c.Storyboard = append(c.Storyboard, chart.StoryEvent{Kind: "unsupported", Unsupported: []string{raw}})
				current = len(c.Storyboard) - 1
				skipDepth = 0
			}
		}
		if n.Err != nil {
			return n.Err
		}
	}
	return nil
}
