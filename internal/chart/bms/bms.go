// Package bms converts BMS (.bms, .bme, .bml) charts.
package bms

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"

	"golang.org/x/text/encoding/japanese"

	"ggosu/internal/chart"
	"ggosu/internal/chart/internal/parse"
)

// Format reads BMS charts.
type Format struct{}

func init() { chart.Register(Format{}) }

func (Format) Extensions() []string { return []string{".bms", ".bme", ".bml"} }

type bmsRecord struct {
	measure, channel int
	data             string
}
type bmsEvent struct {
	beat    float64
	channel int
	value   string
}

func (Format) Parse(b []byte) ([]*chart.Chart, error) {
	if !utf8.Valid(b) {
		var err error
		b, err = japanese.ShiftJIS.NewDecoder().Bytes(b)
		if err != nil {
			return nil, err
		}
	}
	c := chart.New(0)
	tags := map[string]string{}
	var records []bmsRecord
	ratios := map[int]float64{}
	maxMeasure := 0
	n := parse.Numbers{}
	for i, line := range strings.Split(strings.TrimPrefix(string(b), "\ufeff"), "\n") {
		v := strings.TrimSpace(line)
		if !strings.HasPrefix(v, "#") {
			continue
		}
		if len(v) >= 7 && v[6] == ':' {
			m, err := strconv.Atoi(v[1:4])
			if err != nil {
				return nil, fmt.Errorf("BMS line %d: %w", i+1, err)
			}
			if m < 0 || m > 999 {
				return nil, fmt.Errorf("BMS measure out of range")
			}
			ch, err := strconv.ParseInt(v[4:6], 16, 32)
			if err != nil {
				return nil, err
			}
			data := strings.TrimSpace(v[7:])
			maxMeasure = max(maxMeasure, m)
			if ch == 2 {
				r := n.Float(data)
				if r <= 0 || r > 100 {
					return nil, fmt.Errorf("invalid BMS measure length")
				}
				ratios[m] = r
			} else {
				if len(data)%2 != 0 || len(data) == 0 {
					return nil, fmt.Errorf("invalid BMS channel data")
				}
				records = append(records, bmsRecord{m, int(ch), data})
			}
			continue
		}
		f := strings.Fields(v)
		key := strings.ToUpper(strings.TrimPrefix(f[0], "#"))
		value := strings.TrimSpace(strings.TrimPrefix(v, f[0]))
		switch key {
		case "RANDOM", "SETRANDOM", "IF", "ELSE", "ELSEIF", "ENDIF", "ENDRANDOM":
			return nil, fmt.Errorf("BMS random branches require an explicit seed; unsupported")
		}
		tags[key] = value
	}
	if n.Err != nil {
		return nil, n.Err
	}
	if tags["LNTYPE"] != "" && tags["LNTYPE"] != "1" {
		return nil, fmt.Errorf("only BMS LNTYPE 1 is supported")
	}
	c.Metadata = chart.Metadata{Title: tags["TITLE"], Artist: tags["ARTIST"], Creator: tags["SUBARTIST"], Difficulty: tags["DIFFICULTY"], Level: tags["PLAYLEVEL"]}
	if c.Metadata.Difficulty == "" {
		c.Metadata.Difficulty = "BMS"
	}
	starts := make([]float64, maxMeasure+2)
	for m := 0; m <= maxMeasure; m++ {
		ratio := 1.0
		if r, ok := ratios[m]; ok {
			ratio = r
		}
		starts[m+1] = starts[m] + 4*ratio
	}
	var events []bmsEvent
	var changes []chart.Change
	seven, double := false, tags["PLAYER"] == "2" || tags["PLAYER"] == "3"
	for _, r := range records {
		for j := 0; j < len(r.data); j += 2 {
			value := strings.ToUpper(r.data[j : j+2])
			if value == "00" {
				continue
			}
			if _, err := strconv.ParseInt(value, 36, 32); err != nil {
				return nil, err
			}
			beat := starts[r.measure] + float64(j/2)/float64(len(r.data)/2)*(starts[r.measure+1]-starts[r.measure])
			ch := r.channel
			switch ch {
			case 3:
				v, err := strconv.ParseInt(value, 16, 32)
				if err != nil || v <= 0 {
					return nil, fmt.Errorf("invalid hexadecimal BMS BPM")
				}
				changes = append(changes, chart.Change{Beat: beat, BPM: float64(v)})
			case 8:
				v, ok := tags["BPM"+value]
				if !ok {
					return nil, fmt.Errorf("undefined BMS BPM %s", value)
				}
				bpm := n.Float(v)
				if bpm <= 0 {
					return nil, fmt.Errorf("invalid BMS BPM")
				}
				changes = append(changes, chart.Change{Beat: beat, BPM: bpm})
			case 9: // STOP values use 1/48 beat; convert after tempo lookup.
				events = append(events, bmsEvent{beat, ch, value})
			default:
				events = append(events, bmsEvent{beat, ch, value})
				base := ch
				if base >= 0x50 && base <= 0x69 {
					base -= 0x40
				}
				if base >= 0x21 && base <= 0x29 {
					double = true
				}
				if base&15 == 8 || base&15 == 9 {
					seven = true
				}
			}
		}
	}
	bpm := n.Float(tags["BPM"])
	if n.Err != nil {
		return nil, n.Err
	}
	sort.SliceStable(changes, func(i, j int) bool { return changes[i].Beat < changes[j].Beat })
	for _, e := range events {
		if e.channel != 9 {
			continue
		}
		value, ok := tags["STOP"+e.value]
		if !ok {
			return nil, fmt.Errorf("undefined BMS STOP %s", e.value)
		}
		tempo := bpm
		for _, p := range changes {
			if p.Beat > e.beat {
				break
			}
			if p.BPM > 0 {
				tempo = p.BPM
			}
		}
		stop, err := chart.Seconds(n.Float(value) / 48 * 60 / tempo)
		if err != nil {
			return nil, err
		}
		changes = append(changes, chart.Change{Beat: e.beat, Stop: stop})
	}
	if n.Err != nil {
		return nil, n.Err
	}
	clock, err := chart.NewClock(bpm, 0, changes)
	if err != nil {
		return nil, err
	}
	c.Timing, err = clock.Timing()
	if err != nil {
		return nil, err
	}
	order := []int{0x16, 0x11, 0x12, 0x13, 0x14, 0x15}
	if seven {
		order = append(order, 0x18, 0x19)
	}
	if double {
		first := append([]int(nil), order...)
		for _, v := range first {
			order = append(order, v+0x10)
		}
	}
	c.Lanes = len(order)
	lanes := map[int]int{}
	for i, ch := range order {
		lanes[ch] = i
		role := "key"
		if ch&15 == 6 {
			role = "scratch"
		}
		group := 0
		if ch >= 0x20 {
			group = 1
		}
		c.Layout = append(c.Layout, chart.Lane{Name: fmt.Sprintf("%02X", ch), Role: role, Group: group})
	}
	pending := make([]int, c.Lanes)
	last := make([]int, c.Lanes)
	for i := range pending {
		pending[i] = -1
		last[i] = -1
	}
	for key, path := range tags {
		if strings.HasPrefix(key, "WAV") && len(key) == 5 {
			parse.Asset(c, key, path, "audio")
		}
		if strings.HasPrefix(key, "BMP") && len(key) == 5 {
			parse.Asset(c, key, path, "image")
		}
	}
	if tags["STAGEFILE"] != "" {
		c.Storyboard = append(c.Storyboard, chart.StoryEvent{Kind: "background", Asset: parse.Asset(c, "stage", tags["STAGEFILE"], "image")})
	}
	sort.SliceStable(events, func(i, j int) bool { return events[i].beat < events[j].beat })
	warned := map[int]bool{}
	for _, e := range events {
		at, err := clock.At(e.beat)
		if err != nil {
			return nil, err
		}
		ch := e.channel
		if ch == 9 {
			continue
		}
		if ch == 4 || ch == 6 || ch == 7 {
			id := "BMP" + e.value
			if _, ok := c.Assets[id]; !ok {
				return nil, fmt.Errorf("undefined BMS image %s", id)
			}
			kind := "background"
			if ch != 4 {
				kind = map[int]string{6: "bga-poor", 7: "bga-overlay"}[ch]
				if !warned[ch] {
					c.Warnings = append(c.Warnings, fmt.Sprintf("BMS %s events preserved but not rendered", kind))
					warned[ch] = true
				}
			}
			c.Storyboard = append(c.Storyboard, chart.StoryEvent{Kind: kind, At: at, End: at, Asset: id})
			continue
		}
		long := ch >= 0x51 && ch <= 0x69
		if long {
			ch -= 0x40
		}
		lane, playable := lanes[ch]
		if e.channel != 1 && !playable {
			if !warned[e.channel] {
				c.Warnings = append(c.Warnings, fmt.Sprintf("unsupported BMS channel %02X", e.channel))
				warned[e.channel] = true
			}
			continue
		}
		if playable && !long && tags["LNOBJ"] != "" && e.value == strings.ToUpper(tags["LNOBJ"]) {
			i := last[lane]
			if i < 0 || c.Notes[i].Kind != "tap" {
				return nil, fmt.Errorf("orphan BMS LNOBJ")
			}
			c.Notes[i].Kind = "hold"
			c.Notes[i].End = at
			last[lane] = -1
			continue
		}
		id := "WAV" + e.value
		if _, ok := c.Assets[id]; !ok {
			return nil, fmt.Errorf("undefined BMS sound %s", id)
		}
		sound := chart.Sound{Asset: id, Volume: 1}
		if e.channel == 1 {
			c.Audio = append(c.Audio, chart.Audio{At: at, Sound: sound})
			continue
		}
		if long && pending[lane] >= 0 {
			i := pending[lane]
			c.Notes[i].End = at
			pending[lane] = -1
			continue
		}
		kind := "tap"
		if long {
			kind = "hold"
			pending[lane] = len(c.Notes)
		}
		last[lane] = len(c.Notes)
		c.Notes = append(c.Notes, chart.Note{At: at, End: at, Lane: lane, Kind: kind, Sounds: []chart.Sound{sound}})
	}
	for _, p := range pending {
		if p >= 0 {
			return nil, fmt.Errorf("unclosed BMS hold")
		}
	}
	return []*chart.Chart{c}, nil
}
