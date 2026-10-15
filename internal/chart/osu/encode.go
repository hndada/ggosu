package osu

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"math"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"ggosu/internal/chart"
)

// Export writes c as an osu!mania v14 beatmap. Content osu! cannot hold is left out
// and counted in dropped, e.g. "mine 3". Events come verbatim from source when it is
// an .osu file, because sibling .osb storyboards merge into every beatmap again.
func Export(c *chart.Chart, source *chart.Source) (data []byte, dropped []string, err error) {
	counts := map[string]int{}
	order := []string{}
	drop := func(what string) {
		if counts[what] == 0 {
			order = append(order, what)
		}
		counts[what]++
	}
	var b bytes.Buffer
	line := func(format string, args ...any) { fmt.Fprintf(&b, format+"\r\n", args...) }

	music := "virtual"
	var samples []chart.Audio
	for i, a := range c.Audio {
		if p, ok := audioPath(c, a.Sound.Asset); ok && i == 0 && a.At == 0 && music == "virtual" {
			music = p
		} else {
			samples = append(samples, a)
		}
	}
	line("osu file format v14")
	line("")
	line("[General]")
	line("AudioFilename: %s", music)
	line("AudioLeadIn: %d", c.Metadata.LeadIn.Milliseconds())
	preview := int64(-1)
	if c.Metadata.Preview != 0 {
		preview = c.Metadata.Preview.Milliseconds()
	}
	line("PreviewTime: %d", preview)
	line("SampleSet: Normal")
	line("Mode: 3")
	line("")
	line("[Metadata]")
	for _, kv := range [][2]string{{"Title", c.Metadata.Title}, {"TitleUnicode", c.Metadata.Title}, {"Artist", c.Metadata.Artist}, {"ArtistUnicode", c.Metadata.Artist}, {"Creator", c.Metadata.Creator}, {"Version", c.Metadata.Difficulty}} {
		line("%s:%s", kv[0], oneLine(kv[1]))
	}
	line("")
	line("[Difficulty]")
	od := "8"
	if v, err := strconv.ParseFloat(c.Metadata.Level, 64); err == nil && v >= 0 && v <= 10 {
		od = c.Metadata.Level
	}
	line("HPDrainRate:8")
	line("CircleSize:%d", c.Lanes)
	line("OverallDifficulty:%s", od)
	line("ApproachRate:5")
	line("SliderMultiplier:1.4")
	line("SliderTickRate:1")
	line("")
	line("[Events]")
	if events, ok := sourceEvents(source); ok {
		for _, e := range events {
			line("%s", e)
		}
	} else {
		for _, e := range c.Storyboard {
			switch e.Kind {
			case "background", "video":
				p, ok := assetPath(c, e.Asset)
				if !ok {
					drop(e.Kind)
					continue
				}
				if e.Kind == "background" {
					line("0,%s,%q,%s,%s", ms(e.At), p, num(e.X), num(e.Y))
				} else {
					line("Video,%s,%q", ms(e.At), p)
				}
			case "break":
				line("2,%s,%s", ms(e.At), ms(e.End))
			default:
				drop("sprite")
			}
		}
		for _, a := range samples {
			p, ok := audioPath(c, a.Sound.Asset)
			if !ok {
				drop("keysound")
				continue
			}
			line("Sample,%s,0,%q,%d", ms(a.At), p, volume(a.Sound.Volume))
		}
	}
	line("")
	line("[TimingPoints]")
	for i, t := range c.Timing {
		effects := 0
		if t.Highlight {
			effects = 1
		}
		changed := i == 0 || t.BPM != c.Timing[i-1].BPM || t.Meter != c.Timing[i-1].Meter || c.Timing[i-1].Stop > 0
		if t.Stop > 0 {
			// osu! has no stops: hold the field nearly still, then restart the beat after it.
			line("%s,%s,%d,0,1,100,1,%d", ms(t.At), num(60000/t.BPM), t.Meter, effects)
			line("%s,-10000,%d,0,1,100,0,%d", ms(t.At), t.Meter, effects)
			if i+1 == len(c.Timing) || t.At+t.Stop < c.Timing[i+1].At {
				line("%s,%s,%d,0,1,100,1,%d", ms(t.At+t.Stop), num(60000/t.BPM), t.Meter, effects)
				if t.Scroll != 1 {
					line("%s,%s,%d,0,1,100,0,%d", ms(t.At+t.Stop), sv(t.Scroll, drop), t.Meter, effects)
				}
			}
			continue
		}
		if changed {
			line("%s,%s,%d,0,1,100,1,%d", ms(t.At), num(60000/t.BPM), t.Meter, effects)
		}
		if !changed || t.Scroll != 1 {
			line("%s,%s,%d,0,1,100,0,%d", ms(t.At), sv(t.Scroll, drop), t.Meter, effects)
		}
	}
	line("")
	line("[HitObjects]")
	for _, n := range c.Notes {
		kind := n.Kind
		switch kind {
		case "mine", "fake":
			drop(kind)
			continue
		case "lift":
			kind = "tap"
		case "roll":
			kind = "hold"
		}
		// Hit objects take whole milliseconds; the lane x maps back through the importer's x*lanes/512.
		x := (2*n.Lane + 1) * 256 / c.Lanes
		at, end := whole(n.At), whole(n.End)
		hit, sample := hitSample(c, n.Sounds, drop)
		if kind == "hold" && end > at {
			line("%d,192,%d,128,%d,%d:%s", x, at, hit, end, sample)
		} else {
			line("%d,192,%d,1,%d,%s", x, at, hit, sample)
		}
	}
	for _, what := range order {
		dropped = append(dropped, fmt.Sprintf("%s %d", what, counts[what]))
	}
	return b.Bytes(), dropped, nil
}

// SaveEdit exports the edited draft as a new .osu beside source, like chart.SaveEdit
// does for .ggc. The output must parse back as a valid chart before it is written.
func SaveEdit(source string, original, draft *chart.Chart) (path string, dropped []string, err error) {
	c, err := chart.CopyEdit(original, draft)
	if err != nil {
		return "", nil, err
	}
	data, dropped, err := Export(c, &original.Source)
	if err != nil {
		return "", nil, err
	}
	if _, err := chart.Decode("export.osu", bytes.NewReader(data)); err != nil {
		return "", nil, fmt.Errorf("osu export: %w", err)
	}
	path, err = chart.CreateEdit(source, ".osu", func(_ string, w io.Writer) error {
		_, err := w.Write(data)
		return err
	})
	return path, dropped, err
}

var defaultSample = regexp.MustCompile(`^(normal|soft|drum)-hit(normal|whistle|finish|clap)(\d*)\.wav$`)

// hitSample restores osu! default hitsound banks when every sound follows the skin
// file naming the importer produces; any other first sound becomes a custom filename.
func hitSample(c *chart.Chart, sounds []chart.Sound, drop func(string)) (int, string) {
	if len(sounds) == 0 {
		return 0, "0:0:0:0:"
	}
	banks := map[string]int{"normal": 1, "soft": 2, "drum": 3}
	bits := map[string]int{"normal": 1, "whistle": 2, "finish": 4, "clap": 8}
	hit, set, add, suffix, vol := 0, 0, 0, "", sounds[0].Volume
	fits := true
	for i, s := range sounds {
		p, ok := audioPath(c, s.Asset)
		m := defaultSample.FindStringSubmatch(p)
		if !ok || m == nil || s.Volume != vol || i > 0 && m[3] != suffix || hit&bits[m[2]] != 0 {
			fits = false
			break
		}
		suffix = m[3]
		hit |= bits[m[2]]
		if m[2] == "normal" {
			set = banks[m[1]]
		} else if add == 0 || add == banks[m[1]] {
			add = banks[m[1]]
		} else {
			fits = false
			break
		}
	}
	if fits {
		if set == 0 {
			set = add
		}
		if add == 0 {
			add = set
		}
		if hit == 1 {
			hit = 0
		}
		index := 0
		if suffix != "" {
			index, _ = strconv.Atoi(suffix)
		}
		return hit, fmt.Sprintf("%d:%d:%d:%d:", set, add, index, volume(vol))
	}
	p, ok := audioPath(c, sounds[0].Asset)
	for range sounds[1:] {
		drop("keysound")
	}
	if !ok || strings.ContainsAny(p, ":,") {
		drop("keysound")
		return 0, "0:0:0:0:"
	}
	return 0, fmt.Sprintf("0:0:0:%d:%s", volume(sounds[0].Volume), p)
}

// sourceEvents returns the [Events] lines of an .osu source file.
func sourceEvents(source *chart.Source) ([]string, bool) {
	if source == nil || source.Format != "osu" || source.Path == "" {
		return nil, false
	}
	b, err := os.ReadFile(source.Path)
	if err != nil {
		return nil, false
	}
	var events []string
	active := false
	s := bufio.NewScanner(bytes.NewReader(b))
	s.Buffer(make([]byte, 4096), 1<<20)
	for s.Scan() {
		v := strings.TrimSpace(s.Text())
		if strings.HasPrefix(v, "[") {
			active = v == "[Events]"
			continue
		}
		if active && v != "" {
			events = append(events, strings.TrimRight(s.Text(), "\r"))
		}
	}
	return events, s.Err() == nil
}

// assetPath gives a file path osu! can open beside the beatmap: no bank samples,
// embedded ranges or medley assets from other folders.
func assetPath(c *chart.Chart, id string) (string, bool) {
	a, ok := c.Assets[id]
	if !ok || a.Sample != 0 || a.Size != 0 || a.Offset != 0 || a.Source != "" || a.Path == "" || strings.Contains(a.Path, `"`) {
		return "", false
	}
	return a.Path, true
}

func audioPath(c *chart.Chart, id string) (string, bool) {
	if c.Assets[id].Kind != "audio" {
		return "", false
	}
	return assetPath(c, id)
}

// sv converts a scroll multiplier to an inherited beat length within osu!'s 0.01–10 range.
func sv(scroll float64, drop func(string)) string {
	if scroll < .01 || scroll > 10 {
		drop("scroll")
	}
	return num(-100 / min(10, max(.01, scroll)))
}

func ms(t time.Duration) string   { return num(float64(t) / float64(time.Millisecond)) }
func whole(t time.Duration) int64 { return int64(math.Round(float64(t) / float64(time.Millisecond))) }
func num(v float64) string        { return strconv.FormatFloat(v, 'f', -1, 64) }
func volume(v float64) int        { return max(1, min(100, int(math.Round(v*100)))) }
func oneLine(s string) string     { return strings.NewReplacer("\r", " ", "\n", " ").Replace(s) }
