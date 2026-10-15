// Package ojn converts O2Jam .ojn charts and decodes .ojm sound banks.
package ojn

import (
	"bytes"
	"cmp"
	"encoding/binary"
	"fmt"
	"maps"
	"math"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"golang.org/x/text/encoding/korean"

	"ggosu/internal/chart"
)

// Format reads O2Jam charts and their .ojm sample banks.
type Format struct{}

func init() { chart.Register(Format{}) }

func (Format) Extensions() []string     { return []string{".ojn"} }
func (Format) BankExtensions() []string { return []string{".ojm"} }

// Layout follows gosu's o2jam.Header. Conversion is direct and preserves
// fractional measures, intra-measure BPM changes, all difficulties and sounds.
type ojnHeader struct {
	ID                                uint32
	Signature                         [4]byte
	Version                           float32
	Genre                             uint32
	BPM                               float32
	Level                             [4]uint16
	Events, Notes, Measures, Packages [3]uint32
	OldVersion, OldID                 uint16
	OldGenre                          [20]byte
	BMPSize, FileVersion              uint32
	Title                             [64]byte
	Artist, Creator, Bank             [32]byte
	CoverSize                         uint32
	Duration, Offset                  [3]uint32
	CoverOffset                       uint32
}
type ojnEvent struct {
	measure          int
	fraction         float64
	channel          int
	value            float64
	sample           int
	flags, volumePan byte
	beat             float64
}

func decodeKorean(b []byte) (string, error) {
	if i := bytes.IndexByte(b, 0); i >= 0 {
		b = b[:i]
	}
	out, err := korean.EUCKR.NewDecoder().Bytes(b)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

func (Format) Parse(b []byte) ([]*chart.Chart, error) {
	var h ojnHeader
	if err := binary.Read(bytes.NewReader(b), binary.LittleEndian, &h); err != nil {
		return nil, fmt.Errorf("OJN header: %w", err)
	}
	if h.Signature != [4]byte{'o', 'j', 'n', 0} {
		return nil, fmt.Errorf("invalid OJN signature")
	}
	text := decodeText(h.Title[:], h.Artist[:], h.Creator[:])
	title, artist, creator := text[0], text[1], text[2]
	bank, err := decodeKorean(h.Bank[:])
	if err != nil {
		return nil, err
	}
	// One broken difficulty leaves the others playable, as O2Mania does.
	var out []*chart.Chart
	var skipped []string
	var first error
	for diff := 0; diff < 3; diff++ {
		c, err := parseDifficulty(b, h, diff, title, artist, creator, bank)
		if err == nil {
			c.Normalize()
			err = c.Validate()
		}
		if err != nil {
			err = fmt.Errorf("OJN difficulty %d: %w", diff, err)
			first = cmp.Or(first, err)
			skipped = append(skipped, err.Error())
			continue
		}
		out = append(out, c)
	}
	// The cover (JPEG) sits after the charts, followed by an 80x80 BMP thumbnail.
	cover, size := int64(h.CoverOffset), int64(h.CoverSize)
	if size == 0 {
		cover, size = cover+int64(h.CoverSize), int64(h.BMPSize)
	}
	if h.CoverOffset >= uint32(binary.Size(h)) && size > 0 && cover+size <= int64(len(b)) {
		for _, c := range out {
			c.Assets["ojn:cover"] = chart.Asset{Kind: "image", Offset: cover, Size: size}
			c.Storyboard = append(c.Storyboard, chart.StoryEvent{Kind: "background", Asset: "ojn:cover"})
		}
	}
	if len(out) == 0 {
		return nil, first
	}
	for _, c := range out {
		c.Fixes = append(c.Fixes, skipped...)
	}
	return out, nil
}

// parseDifficulty converts one of the three charts in an OJN file.
func parseDifficulty(b []byte, h ojnHeader, diff int, title, artist, creator, bank string) (*chart.Chart, error) {
	{
		c := chart.New(7)
		c.Metadata = chart.Metadata{Title: title, Artist: artist, Creator: creator, Difficulty: []string{"Easy", "Normal", "Hard"}[diff], Level: strconv.Itoa(int(h.Level[diff]))}
		offset := int(h.Offset[diff])
		end := len(b)
		if diff < 2 && h.Offset[diff+1] > 0 {
			end = int(h.Offset[diff+1])
		} else if h.CoverOffset > 0 {
			end = int(h.CoverOffset)
		}
		if offset < binary.Size(h) || offset > end || end > len(b) || uint64(h.Packages[diff]) > uint64(end-offset)/8 {
			return nil, fmt.Errorf("invalid section bounds")
		}
		var events []ojnEvent
		ratios := map[int]float64{}
		maxMeasure := 0
		for p := uint32(0); p < h.Packages[diff]; p++ {
			if end-offset < 8 {
				return nil, fmt.Errorf("truncated OJN package")
			}
			measure := int(binary.LittleEndian.Uint32(b[offset:]))
			ch := int(binary.LittleEndian.Uint16(b[offset+4:]))
			count := int(binary.LittleEndian.Uint16(b[offset+6:]))
			offset += 8
			// Channels from 9 up are background sound; O2Mania charts go past 22.
			if measure > 100000 || count > (end-offset)/4 {
				return nil, fmt.Errorf("invalid OJN package %d", p)
			}
			maxMeasure = max(maxMeasure, measure)
			for j := 0; j < count; j++ {
				raw := b[offset : offset+4]
				offset += 4
				e := ojnEvent{measure: measure, fraction: float64(j) / float64(count), channel: ch}
				if ch < 2 {
					e.value = float64(math.Float32frombits(binary.LittleEndian.Uint32(raw)))
					if e.value == 0 {
						continue
					}
					if ch == 0 {
						if j != 0 || e.value <= 0 || e.value > 100 || math.IsNaN(e.value) {
							return nil, fmt.Errorf("invalid OJN measure ratio")
						}
						ratios[measure] = e.value
						continue
					}
				} else {
					e.sample = int(binary.LittleEndian.Uint16(raw))
					if e.sample == 0 {
						continue
					}
					e.flags = raw[3]
					e.volumePan = raw[2]
				}
				events = append(events, e)
			}
		}
		starts := make([]float64, maxMeasure+2)
		for m := 0; m <= maxMeasure; m++ {
			ratio := 1.0
			if r, ok := ratios[m]; ok {
				ratio = r
			}
			starts[m+1] = starts[m] + 4*ratio
		}
		var changes []chart.Change
		for i := range events {
			e := &events[i]
			e.beat = starts[e.measure] + e.fraction*(starts[e.measure+1]-starts[e.measure])
			if e.channel == 1 {
				if e.value <= 0 {
					return nil, fmt.Errorf("invalid OJN BPM")
				}
				changes = append(changes, chart.Change{Beat: e.beat, BPM: e.value})
			}
		}
		clock, err := chart.NewClock(float64(h.BPM), 0, changes)
		if err != nil {
			return nil, err
		}
		c.Timing, err = clock.Timing()
		if err != nil {
			return nil, err
		}
		sort.SliceStable(events, func(i, j int) bool { return events[i].beat < events[j].beat })
		pending := [7]int{-1, -1, -1, -1, -1, -1, -1}
		// O2Mania plays charts with stray long-note marks; keep the playable notes and note what was fixed.
		fixed := map[string]int{}
		for _, e := range events {
			if e.channel < 2 {
				continue
			}
			at, err := clock.At(e.beat)
			if err != nil {
				return nil, err
			}
			sample := e.sample
			if e.flags%8 > 3 {
				sample += 1000
			}
			id := fmt.Sprintf("ojn:%d", sample)
			c.Assets[id] = chart.Asset{Path: bank, Kind: "bank", Sample: sample}
			volume := float64(e.volumePan>>4) / 16
			if volume == 0 {
				volume = 1
			}
			pan := int(e.volumePan & 15)
			if pan == 0 {
				pan = 8
			}
			sound := chart.Sound{Asset: id, Volume: volume, Pan: float64(pan-8) / 8}
			if e.channel >= 9 {
				c.Audio = append(c.Audio, chart.Audio{At: at, Sound: sound})
				continue
			}
			lane := e.channel - 2
			typ := e.flags % 4
			switch typ {
			case 0, 1:
				if pending[lane] >= 0 {
					fixed["tap inside a hold dropped"]++
					continue
				}
				c.Notes = append(c.Notes, chart.Note{At: at, End: at, Lane: lane, Kind: "tap", Sounds: []chart.Sound{sound}})
			case 2:
				if pending[lane] >= 0 {
					fixed["second hold head dropped"]++
					continue
				}
				pending[lane] = len(c.Notes)
				c.Notes = append(c.Notes, chart.Note{At: at, End: at, Lane: lane, Kind: "hold", Sounds: []chart.Sound{sound}})
			case 3:
				i := pending[lane]
				if i < 0 {
					fixed["hold tail without head dropped"]++
					continue
				}
				c.Notes[i].End = at
				c.Notes[i].TailSounds = []chart.Sound{sound}
				pending[lane] = -1
			}
		}
		for _, p := range pending {
			if p >= 0 {
				c.Notes[p].Kind = "tap"
				fixed["unclosed hold played as a tap"]++
			}
		}
		// Notes stacked in one lane play as one, and a hold with no length plays as a tap.
		kept := c.Notes[:0]
		heads, ends := map[int]time.Duration{}, map[int]time.Duration{}
		for _, n := range c.Notes {
			if n.Kind == "hold" && n.End <= n.At {
				n.Kind, n.End, n.TailSounds = "tap", n.At, nil
				fixed["empty hold played as a tap"]++
			}
			if e, ok := ends[n.Lane]; ok && (n.At < e || n.At == heads[n.Lane]) {
				fixed["overlapping note dropped"]++
				continue
			}
			heads[n.Lane], ends[n.Lane] = n.At, n.End
			kept = append(kept, n)
		}
		c.Notes = kept
		for _, k := range slices.Sorted(maps.Keys(fixed)) {
			c.Fixes = append(c.Fixes, fmt.Sprintf("OJN %s: %s ×%d", c.Metadata.Difficulty, k, fixed[k]))
		}
		return c, nil
	}
}
