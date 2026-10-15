package chart

import (
	"cmp"
	"fmt"
	"slices"
	"time"
)

// Bars returns bar starts up to end; see Measures.
func (c *Chart) Bars(end time.Duration) []time.Duration {
	var bars []time.Duration
	for _, m := range c.Measures(end) {
		bars = append(bars, m.At)
	}
	return append(bars, end)
}

// Measure is one bar start. Major marks every Meter-th bar of a BPM section,
// as osu!lazer's BarLineGenerator.
type Measure struct {
	At    time.Duration
	Major bool
}

// Measures returns bar starts before end. A BPM or Meter change (4 when unset)
// starts a new bar; points that only change scroll, volume or highlight do not.
// Stops are ignored.
func (c *Chart) Measures(end time.Duration) []Measure {
	var bars []Measure
	for i := 0; i < len(c.Timing); {
		t := c.Timing[i]
		j := i + 1
		for ; j < len(c.Timing) && (c.Timing[j].At == t.At || c.Timing[j].BPM == t.BPM && c.Timing[j].Meter == t.Meter); j++ {
			if c.Timing[j].At == t.At {
				t = c.Timing[j]
			}
		}
		next := end
		if j < len(c.Timing) {
			next = min(end, c.Timing[j].At)
		}
		i = j
		meter := t.Meter
		if meter <= 0 {
			meter = 4
		}
		if t.BPM <= 0 {
			continue
		}
		bar := time.Duration(float64(meter) * 60 / t.BPM * float64(time.Second))
		for n, at := 0, t.At; bar > 0 && at < next; n, at = n+1, at+bar {
			bars = append(bars, Measure{at, n%meter == 0})
		}
	}
	return bars
}

// beat is the beat length in effect at at.
func (c *Chart) beat(at time.Duration) time.Duration {
	bpm := 120.0
	for _, t := range c.Timing {
		if t.At > at {
			break
		}
		if t.BPM > 0 {
			bpm = t.BPM
		}
	}
	return time.Duration(60 / bpm * float64(time.Second))
}

func (c *Chart) lastNote() time.Duration {
	var end time.Duration
	for _, n := range c.Notes {
		if n.Kind != "fake" {
			end = max(end, n.End)
		}
	}
	return end
}

func playable(n Note) bool { return n.Kind != "mine" && n.Kind != "fake" }

// Highlight is the chorus as bar indices [from, to) into bars: the longest
// Highlight (osu kiai) span, or else the 16 bars with the most playable notes.
func (c *Chart) Highlight(bars []time.Duration) (from, to int) {
	index := func(at time.Duration) int {
		i, _ := slices.BinarySearch(bars, at)
		if i > 0 && (i == len(bars) || at-bars[i-1] < bars[i]-at) {
			i--
		}
		return min(i, len(bars)-1)
	}
	var start time.Duration
	on := false
	for i, t := range c.Timing {
		if t.Highlight && !on {
			start, on = t.At, true
		}
		last := i+1 == len(c.Timing)
		if on && (!t.Highlight || last) {
			end := t.At
			if last && t.Highlight {
				end = bars[len(bars)-1]
			}
			on = false
			if a, b := index(start), index(end); b-a > to-from {
				from, to = a, b
			}
		}
	}
	if to-from >= 8 {
		return from, to
	}
	count := make([]int, len(bars))
	for _, n := range c.Notes {
		if playable(n) {
			if i, ok := slices.BinarySearch(bars, n.At); ok {
				count[i]++
			} else if i > 0 {
				count[i-1]++
			}
		}
	}
	span := min(16, len(bars)-1)
	best := -1
	for i := 0; i+span < len(bars); i++ {
		total := 0
		for _, k := range count[i : i+span] {
			total += k
		}
		if total > best {
			best, from = total, i
		}
	}
	return from, from + span
}

// MedleyPart plays [From, To) of a chart; To at or after the last note keeps the
// rest of the song, ending on its own.
type MedleyPart struct {
	Chart    *Chart
	From, To time.Duration
}

// PlanMedley arranges one to three charts of one lane count as 기승전결: the lightest
// chorus opens (8 build-up bars + 8 chorus), the middle rises (8 + 16), the
// heaviest peaks (up to 8 + the whole chorus), and the opener returns for its ending.
// With two charts there is no middle; a single chart plays its whole chorus, then its ending.
func PlanMedley(cs []*Chart) ([]MedleyPart, error) {
	if len(cs) < 1 || len(cs) > 3 {
		return nil, fmt.Errorf("chart: a medley takes one to three charts")
	}
	type song struct {
		c        *Chart
		bars     []time.Duration
		from, to int
		density  float64
	}
	songs := make([]song, len(cs))
	for i, c := range cs {
		if err := c.Validate(); err != nil {
			return nil, err
		}
		if c.Lanes != cs[0].Lanes {
			return nil, fmt.Errorf("chart: medley charts need one lane count")
		}
		bars := c.Bars(c.lastNote())
		if len(bars) < 18 {
			return nil, fmt.Errorf("chart: %q is too short for a medley", c.Metadata.Title)
		}
		from, to := c.Highlight(bars)
		notes := 0
		for _, n := range c.Notes {
			if playable(n) && n.At >= bars[from] && n.At < bars[to] {
				notes++
			}
		}
		songs[i] = song{c, bars, from, to, float64(notes) / max(bars[to]-bars[from], time.Second).Seconds()}
	}
	slices.SortStableFunc(songs, func(a, b song) int { return cmp.Compare(a.density, b.density) })
	part := func(s song, build, chorus int) MedleyPart {
		from := max(0, s.from-build)
		to := s.to
		if chorus > 0 {
			to = min(s.to, s.from+chorus)
		}
		return MedleyPart{s.c, s.bars[from], s.bars[to]}
	}
	a := songs[0]
	end := MedleyPart{a.c, a.bars[max(0, len(a.bars)-1-12)], a.bars[len(a.bars)-1]}
	switch len(songs) {
	case 1:
		return []MedleyPart{part(a, 8, 0), end}, nil
	case 2:
		return []MedleyPart{part(a, 8, 8), part(songs[1], 8, 0), end}, nil
	}
	return []MedleyPart{part(a, 8, 8), part(songs[1], 8, 16), part(songs[2], 8, 0), end}, nil
}

// Medley joins parts end to end. Each part starts at its own downbeat; the
// outgoing audio fades over its last bar and rings one beat into the next part,
// whose first beat has no notes so the player can hear the new song first.
func Medley(parts []MedleyPart) (*Chart, error) {
	if len(parts) == 0 {
		return nil, fmt.Errorf("chart: empty medley")
	}
	first := parts[0].Chart
	m := &Chart{Version: Version, Metadata: first.Metadata, Source: first.Source, Lanes: first.Lanes, Layout: first.Layout, Assets: map[string]Asset{}}
	m.Metadata = Metadata{Title: "Medley", Difficulty: fmt.Sprintf("%dK", first.Lanes)}
	m.Source.SHA256, m.Source.MD5 = "", ""
	var offset time.Duration
	for i, p := range parts {
		c := p.Chart
		if c.Lanes != m.Lanes || p.To <= p.From {
			return nil, fmt.Errorf("chart: invalid medley part %d", i)
		}
		final := i+1 == len(parts)
		prefix := fmt.Sprintf("%d/", i)
		for id, a := range c.Assets {
			if a.Kind != "audio" && a.Kind != "bank" {
				continue
			}
			if a.Source == "" {
				a.Source = c.Source.Path
			}
			m.Assets[prefix+id] = a
		}
		beat := c.beat(p.From)
		bars := c.Bars(p.To + 1)
		fadeAt := p.To - beat*4
		if i := slices.Index(bars, p.To); i > 0 {
			fadeAt = bars[i-1]
		}
		stop := p.To + c.beat(p.To)
		shift := offset - p.From
		// clip maps a sound starting at at into the part, or reports it silent.
		clip := func(s Sound, at time.Duration) (Audio, bool) {
			s.Asset = prefix + s.Asset
			if !final && at >= stop {
				return Audio{}, false
			}
			if at < p.From {
				s.From += p.From - at
				if s.Length > 0 && s.Length <= p.From-at {
					return Audio{}, false
				}
				if s.Length > 0 {
					s.Length -= p.From - at
				}
				at = p.From
				if i > 0 {
					s.FadeIn = beat
				}
			}
			if !final {
				left := stop - at
				if s.Length == 0 || s.Length > left {
					s.Length = left
				}
				if at >= fadeAt {
					// Start quieter by the fade already done, then fade the rest.
					s.Volume *= float64(stop-at) / float64(stop-fadeAt)
					s.FadeOut = s.Length
				} else {
					s.FadeOut = min(s.Length, stop-fadeAt)
				}
				if s.FadeIn+s.FadeOut > s.Length {
					s.FadeIn = s.Length - s.FadeOut
				}
			}
			return Audio{At: at + shift, Sound: s}, true
		}
		// Of the sounds already started, keep the recent ones and any backing track
		// (an asset played at most twice); earlier short samples have ended.
		uses := map[string]int{}
		for _, a := range c.Audio {
			uses[a.Sound.Asset]++
		}
		for _, a := range c.Audio {
			if a.At < p.From && p.From-a.At > 20*time.Second && uses[a.Sound.Asset] > 2 {
				continue
			}
			if x, ok := clip(a.Sound, a.At); ok {
				m.Audio = append(m.Audio, x)
			}
		}
		lead := p.From
		if i > 0 {
			lead += beat
		}
		for _, n := range c.Notes {
			if n.At < p.From || !final && n.At >= p.To {
				continue
			}
			if n.At < lead || !final && n.End >= p.To && n.End != n.At {
				// Unplayed heads keep their sounds as background audio.
				if playable(n) {
					for _, s := range n.Sounds {
						if x, ok := clip(s, n.At); ok {
							m.Audio = append(m.Audio, x)
						}
					}
				}
				continue
			}
			n.Sounds = prefixed(n.Sounds, prefix)
			n.TailSounds = prefixed(n.TailSounds, prefix)
			n.At += shift
			n.End += shift
			m.Notes = append(m.Notes, n)
		}
		var active *Timing
		for _, t := range c.Timing {
			if t.At <= p.From {
				active = &t
				continue
			}
			if !final && t.At >= p.To {
				break
			}
			if active != nil {
				a := *active
				a.At = offset
				m.Timing = append(m.Timing, a)
				active = nil
			}
			t.At += shift
			m.Timing = append(m.Timing, t)
		}
		if active != nil {
			a := *active
			a.At = offset
			m.Timing = append(m.Timing, a)
		}
		offset += p.To - p.From
	}
	m.Normalize()
	if err := m.Validate(); err != nil {
		return nil, err
	}
	return m, nil
}

func prefixed(sounds []Sound, prefix string) []Sound {
	if len(sounds) == 0 {
		return sounds
	}
	out := slices.Clone(sounds)
	for i := range out {
		out[i].Asset = prefix + out[i].Asset
	}
	return out
}
