// Package sm converts StepMania .sm charts.
package sm

import (
	"fmt"
	"strings"

	"ggosu/internal/chart"
	"ggosu/internal/chart/internal/parse"
)

// Format reads StepMania charts.
type Format struct{}

func init() { chart.Register(Format{}) }

func (Format) Extensions() []string { return []string{".sm"} }

func (Format) Parse(b []byte) ([]*chart.Chart, error) {
	var clean strings.Builder
	for _, line := range strings.Split(strings.TrimPrefix(string(b), "\ufeff"), "\n") {
		line, _, _ = strings.Cut(line, "//")
		clean.WriteString(line)
		clean.WriteByte('\n')
	}
	tags := map[string]string{}
	var blocks []string
	rest := clean.String()
	for {
		i := strings.IndexByte(rest, '#')
		if i < 0 {
			break
		}
		rest = rest[i+1:]
		record, tail, ok := strings.Cut(rest, ";")
		if !ok {
			return nil, fmt.Errorf("unterminated SM tag")
		}
		rest = tail
		key, value, ok := strings.Cut(record, ":")
		if !ok {
			return nil, fmt.Errorf("malformed SM tag")
		}
		key = strings.ToUpper(strings.TrimSpace(key))
		if key == "NOTES" {
			blocks = append(blocks, value)
		} else {
			tags[key] = strings.TrimSpace(value)
		}
	}
	for _, key := range []string{"WARPS", "DELAYS", "SPEEDS", "SCROLLS", "FAKES", "TIMESIGNATURES"} {
		if tags[key] != "" {
			return nil, fmt.Errorf("SM %s is unsupported", key)
		}
	}
	n := parse.Numbers{}
	offset := 0.0
	if tags["OFFSET"] != "" {
		offset = n.Float(tags["OFFSET"])
	}
	start, err := chart.Seconds(-offset)
	if err != nil {
		return nil, err
	}
	bpm := 0.0
	var changes []chart.Change
	for _, s := range strings.Split(tags["BPMS"], ",") {
		beat, value, ok := strings.Cut(strings.TrimSpace(s), "=")
		if !ok {
			return nil, fmt.Errorf("SM requires BPMS")
		}
		a, v := n.Float(beat), n.Float(value)
		if a == 0 {
			bpm = v
		}
		if v <= 0 {
			return nil, fmt.Errorf("SM negative BPM/warps unsupported")
		}
		changes = append(changes, chart.Change{Beat: a, BPM: v})
	}
	if tags["STOPS"] != "" {
		for _, s := range strings.Split(tags["STOPS"], ",") {
			a, v, ok := strings.Cut(strings.TrimSpace(s), "=")
			if !ok {
				return nil, fmt.Errorf("malformed SM stop")
			}
			stop, err := chart.Seconds(n.Float(v))
			if err != nil {
				return nil, err
			}
			changes = append(changes, chart.Change{Beat: n.Float(a), Stop: stop})
		}
	}
	if n.Err != nil {
		return nil, n.Err
	}
	clock, err := chart.NewClock(bpm, start, changes)
	if err != nil {
		return nil, err
	}
	var out []*chart.Chart
	for _, block := range blocks {
		f := strings.SplitN(block, ":", 6)
		if len(f) != 6 {
			return nil, fmt.Errorf("malformed SM NOTES")
		}
		for i := range f {
			f[i] = strings.TrimSpace(f[i])
		}
		lanes, ok := map[string]int{"dance-single": 4, "dance-double": 8, "dance-solo": 6, "pump-single": 5, "pump-double": 10, "kb7-single": 7, "beat-single5": 6, "beat-single7": 8, "beat-double5": 12, "beat-double7": 16}[f[0]]
		if !ok {
			return nil, fmt.Errorf("unsupported SM step type %q", f[0])
		}
		c := chart.New(lanes)
		c.Metadata = chart.Metadata{Title: tags["TITLE"], Artist: tags["ARTIST"], Creator: f[1], Difficulty: f[2], Level: f[3]}
		c.Timing, err = clock.Timing()
		if err != nil {
			return nil, err
		}
		if tags["SAMPLESTART"] != "" {
			c.Metadata.Preview, err = chart.Seconds(n.Float(tags["SAMPLESTART"]))
			if err != nil {
				return nil, err
			}
		}
		if tags["MUSIC"] != "" {
			c.Audio = append(c.Audio, chart.Audio{Sound: chart.Sound{Asset: parse.Asset(c, "music", tags["MUSIC"], "audio"), Volume: 1}})
		}
		if tags["BACKGROUND"] != "" {
			c.Storyboard = append(c.Storyboard, chart.StoryEvent{Kind: "background", Asset: parse.Asset(c, "background", tags["BACKGROUND"], "image")})
		}
		if tags["BGCHANGES"] != "" {
			c.Warnings = append(c.Warnings, "SM BGCHANGES are not rendered")
		}
		pending := make([]int, lanes)
		for i := range pending {
			pending[i] = -1
		}
		for measure, part := range strings.Split(f[5], ",") {
			rows := strings.Fields(part)
			if len(rows) == 0 {
				return nil, fmt.Errorf("empty SM measure")
			}
			for row, v := range rows {
				if len(v) != lanes {
					return nil, fmt.Errorf("SM measure %d row %d: expected %d columns", measure, row, lanes)
				}
				at, err := clock.At(float64(measure)*4 + float64(row)*4/float64(len(rows)))
				if err != nil {
					return nil, err
				}
				for lane, ch := range v {
					if ch == '0' {
						continue
					}
					if ch == '3' {
						i := pending[lane]
						if i < 0 {
							return nil, fmt.Errorf("orphan SM hold tail")
						}
						c.Notes[i].End = at
						pending[lane] = -1
						continue
					}
					kind, ok := map[rune]string{'1': "tap", '2': "hold", '4': "roll", 'M': "mine", 'L': "lift", 'F': "fake"}[ch]
					if !ok {
						return nil, fmt.Errorf("unsupported SM note %q", ch)
					}
					if pending[lane] >= 0 {
						return nil, fmt.Errorf("SM note inside hold")
					}
					if ch == '2' || ch == '4' {
						pending[lane] = len(c.Notes)
					}
					c.Notes = append(c.Notes, chart.Note{At: at, End: at, Lane: lane, Kind: kind})
				}
			}
		}
		for _, p := range pending {
			if p >= 0 {
				return nil, fmt.Errorf("unclosed SM hold")
			}
		}
		out = append(out, c)
	}
	if n.Err != nil {
		return nil, n.Err
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("no SM charts")
	}
	return out, nil
}
