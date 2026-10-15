// Replays store key transitions on the game's fixed 1x time axis.
// Recording and playback rates affect how fast the game reaches a position,
// never the stored position itself.
package piano

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"slices"
	"time"

	"ggosu/internal/chart"
	"ggosu/internal/settings"
)

const (
	ReplayVersion = 2
	TimeBase      = "game-1x"
)

// Event.At is a position on the game's 1x time axis, in nanoseconds in JSON.
// For example, an input after 5 real seconds at 2x has At = 10 seconds.
// Slice order breaks ties, preserving a down and up within one frame.
type Event struct {
	At   time.Duration `json:"at_ns"`
	Key  uint16        `json:"key"`
	Down bool          `json:"down"`
}

// Pause records both fixed game position and real time since this run began.
type Pause struct {
	At      time.Duration `json:"at_ns"`
	Elapsed time.Duration `json:"elapsed_ns"`
	Action  string        `json:"action"`
	Reason  string        `json:"reason,omitempty"`
	Rate    float64       `json:"rate"`
}

type Replay struct {
	Health       int               `json:"health,omitempty"`
	HealthStages bool              `json:"health_stages,omitempty"`
	HealthFail   int               `json:"health_fail,omitempty"`
	HealthGauge  int               `json:"health_gauge,omitempty"`
	Pauses       []Pause           `json:"pauses,omitempty"`
	Version      int               `json:"version"`
	KeyEncoding  string            `json:"key_encoding"`
	TimeBase     string            `json:"time_base"`
	Duration     time.Duration     `json:"duration_ns"`
	Events       []Event           `json:"events"`
	ChartHash    string            `json:"chart_hash,omitempty"`
	Lanes        int               `json:"lanes,omitempty"`
	Rules        string            `json:"rules,omitempty"`
	Player       string            `json:"player,omitempty"`
	Offsets      settings.Offsets  `json:"offsets,omitzero"`
	Profile      *settings.Profile `json:"profile,omitempty"`
	Mods         chart.Mod         `json:"mods,omitzero"`
}

func NewReplay() Replay {
	return Replay{Version: ReplayVersion, KeyEncoding: "windows-vk", TimeBase: TimeBase}
}

func (f Replay) Validate() error {
	if f.Health < 0 || f.Health > 3 || f.HealthFail < 0 || f.HealthFail > 3 || f.Health > 0 && f.HealthFail > f.Health || !f.HealthStages && f.HealthFail != 0 || f.HealthGauge != 0 && f.HealthGauge != O2JamGauge && (f.HealthGauge != HealthGauge || !f.HealthStages) || len(f.Pauses) > 100000 {
		return fmt.Errorf("replay: invalid health or pause count")
	}
	// The O2Jam mod brings its own life bar at the mod's level.
	if (f.HealthGauge == O2JamGauge) != (f.Mods.O2Jam != "") || f.HealthGauge == O2JamGauge && (f.HealthStages || f.Health != O2JamLevel(f.Mods.O2Jam)) {
		return fmt.Errorf("replay: O2Jam mod and health do not match")
	}
	var elapsed time.Duration
	paused, countdown := false, false
	for i, e := range f.Pauses {
		if e.Elapsed < elapsed || e.Elapsed < 0 || e.At < -chart.MaxTime || e.At > f.Duration || math.IsNaN(e.Rate) || math.IsInf(e.Rate, 0) || e.Rate < .25 || e.Rate > 4 {
			return fmt.Errorf("replay: pause event %d is outside the timeline", i)
		}
		elapsed = e.Elapsed
		switch e.Action {
		case "pause":
			if paused {
				return fmt.Errorf("replay: duplicate pause")
			}
			paused = true
		case "resume-request":
			if !paused || countdown {
				return fmt.Errorf("replay: invalid resume request")
			}
			countdown = true
		case "resume-cancel":
			if !countdown {
				return fmt.Errorf("replay: no countdown to cancel")
			}
			countdown = false
		case "resume":
			if !paused || !countdown {
				return fmt.Errorf("replay: invalid resume")
			}
			paused, countdown = false, false
		case "end":
			if !paused || i != len(f.Pauses)-1 {
				return fmt.Errorf("replay: invalid paused end")
			}
			paused, countdown = false, false
		default:
			return fmt.Errorf("replay: unknown pause action %q", e.Action)
		}
	}
	if err := f.Mods.Validate(f.Lanes); err != nil {
		return err
	}
	if f.Mods.Kind != "" && f.KeyEncoding != "lane-index" {
		return fmt.Errorf("replay: lane mods require lane-index encoding")
	}
	if f.Profile != nil {
		if err := f.Profile.Validate(f.Lanes); err != nil {
			return err
		}
	}
	if err := f.Offsets.Validate(); err != nil {
		return err
	}
	if f.Version != ReplayVersion || (f.KeyEncoding != "windows-vk" && f.KeyEncoding != "lane-index") {
		return fmt.Errorf("replay: unsupported version or key encoding")
	}
	if f.TimeBase != TimeBase {
		return fmt.Errorf("replay: expected %s time base", TimeBase)
	}
	if f.Duration < 0 || f.Duration > chart.MaxTime || len(f.Events) > 1_000_000 {
		return fmt.Errorf("replay: duration or event count outside limits")
	}
	if f.KeyEncoding == "lane-index" && (f.Lanes < 1 || f.Lanes > 18 || len(f.ChartHash) != 64 || f.Rules == "") {
		return fmt.Errorf("replay: lane replay requires chart hash, lanes and rules")
	}
	var previous time.Duration
	if f.KeyEncoding == "lane-index" {
		previous = -chart.MaxTime
		if _, err := hex.DecodeString(f.ChartHash); err != nil {
			return fmt.Errorf("replay: invalid chart hash")
		}
	}
	for i, e := range f.Events {
		if e.At < previous || e.At > f.Duration {
			return fmt.Errorf("replay: event %d is outside the ordered game timeline", i)
		}
		previous = e.At
		if f.KeyEncoding == "lane-index" && int(e.Key) >= f.Lanes {
			return fmt.Errorf("replay: event %d lane out of range", i)
		}
	}
	return nil
}

func NewChartReplay(c *chart.Chart, rules string) (Replay, error) {
	hash, err := c.Digest()
	if err != nil {
		return Replay{}, err
	}
	f := NewReplay()
	f.KeyEncoding = "lane-index"
	f.ChartHash = hash
	f.Lanes = c.Lanes
	f.Rules = rules
	return f, nil
}

func (f Replay) Match(c *chart.Chart, rules string) error {
	if err := f.Validate(); err != nil {
		return err
	}
	hash, err := c.Digest()
	if err != nil {
		return err
	}
	if f.KeyEncoding != "lane-index" || f.ChartHash != hash || f.Lanes != c.Lanes || (f.Rules != rules && !(rules == Rules && Known(f.Rules))) {
		return fmt.Errorf("replay: chart, lane count or judgement rules do not match")
	}
	if _, err := f.Mods.Apply(c); err != nil {
		return err
	}
	return nil
}

// Auto builds lane transitions rather than bypassing the normal judgement path.
func Auto(c *chart.Chart, rules string) (Replay, error) {
	f, err := NewChartReplay(c, rules)
	if err != nil {
		return Replay{}, err
	}
	for _, n := range c.Notes {
		if n.Kind == "mine" || n.Kind == "fake" {
			continue
		}
		key := uint16(n.Lane)
		if n.Kind == "lift" {
			f.Events = append(f.Events, Event{At: n.At - time.Nanosecond, Key: key, Down: true}, Event{At: n.At, Key: key})
			continue
		}
		f.Events = append(f.Events, Event{At: n.At, Key: key, Down: true})
		end := n.End
		if end == n.At {
			end += time.Nanosecond
		}
		if n.Kind == "roll" {
			for at := n.At + 100*time.Millisecond; at < n.End; at += 100 * time.Millisecond {
				f.Events = append(f.Events, Event{At: at - time.Nanosecond, Key: key}, Event{At: at, Key: key, Down: true})
			}
		}
		f.Events = append(f.Events, Event{At: end, Key: key})
	}
	slices.SortStableFunc(f.Events, func(a, b Event) int {
		if a.At < b.At {
			return -1
		}
		if a.At > b.At {
			return 1
		}
		if a.Down == b.Down {
			return 0
		}
		if !a.Down {
			return -1
		}
		return 1
	})
	f.Duration = c.Duration() + time.Second
	return f, f.Validate()
}

func ReadReplay(r io.Reader) (Replay, error) {
	var f Replay
	d := json.NewDecoder(io.LimitReader(r, 128<<20))
	d.DisallowUnknownFields()
	if err := d.Decode(&f); err != nil {
		return Replay{}, fmt.Errorf("read replay: %w", err)
	}
	var extra any
	if err := d.Decode(&extra); err != io.EOF {
		return Replay{}, fmt.Errorf("replay: expected one JSON document")
	}
	// Version 1 already stored positions on the 1x axis, but did not name it.
	// Upgrade its metadata without rescaling any event or duration.
	if f.Version == 1 && f.TimeBase == "" {
		f.Version, f.TimeBase = ReplayVersion, TimeBase
	}
	if err := f.Validate(); err != nil {
		return Replay{}, err
	}
	return f, nil
}

func WriteReplay(w io.Writer, f Replay) error {
	if err := f.Validate(); err != nil {
		return err
	}
	e := json.NewEncoder(w)
	e.SetIndent("", "  ")
	if err := e.Encode(f); err != nil {
		return fmt.Errorf("write replay: %w", err)
	}
	return nil
}

// Player belongs to the game goroutine. Its zero value is an empty replay.
type Player struct {
	Rules        string
	Health       int
	HealthStages bool
	HealthFail   int
	HealthGauge  int
	events       []Event
	next         int
	last         time.Duration
	start        time.Duration
}

func NewPlayer(f Replay) (*Player, error) {
	if err := f.Validate(); err != nil {
		return nil, err
	}
	start := time.Duration(0)
	if f.KeyEncoding == "lane-index" {
		start = -chart.MaxTime - time.Second
	}
	return &Player{Rules: f.Rules, Health: f.Health, HealthStages: f.HealthStages, HealthFail: f.HealthFail, HealthGauge: f.HealthGauge, events: slices.Clone(f.Events), last: start, start: start}, nil
}

// HealthState restores the gauge policy stored with the input timeline.
func (p *Player) HealthState() Health {
	return replayHealth(p.Health, p.HealthFail, p.HealthStages, p.HealthGauge)
}

// AppendDue appends every transition up to at, once and in stored order. Call
// with the current 1x game position. The caller advances its game clock with
// the current playback rate; the player never rescales events. Repeated positions
// are safe, including while paused. Backward seeks require Reset and resetting
// the caller's game state before consuming again from zero.
func (p *Player) AppendDue(dst []Event, at time.Duration) ([]Event, error) {
	if at < p.last {
		return dst, fmt.Errorf("replay: game time moved backwards; reset before seeking")
	}
	p.last = at
	for p.next < len(p.events) && p.events[p.next].At <= at {
		dst = append(dst, p.events[p.next])
		p.next++
	}
	return dst, nil
}

func (p *Player) Reset() { p.next, p.last = 0, p.start }

func (p *Player) Done() bool { return p.next == len(p.events) }
