package difficulty

import (
	"fmt"
	"math"
	"slices"
	"time"

	"ggosu/internal/chart"
)

type Cost struct {
	Movement    float64 `json:"movement"`
	Interaction float64 `json:"interaction"`
	Repeat      float64 `json:"repeat"`
}

func (c Cost) Total() float64 { return c.Movement + c.Interaction + c.Repeat }

type State struct {
	Action        Action         `json:"action"`
	PositionStart float64        `json:"position_start"`
	PositionEnd   float64        `json:"position_end"`
	Note          int            `json:"note"` // -1 when resting.
	LastMotionEnd *time.Duration `json:"last_motion_end_ns,omitempty"`
	Speed         float64        `json:"speed"` // 0 while stationary.
	LoadStart     float64        `json:"load_start"`
	LoadEnd       float64        `json:"load_end"`
}

type Contribution struct {
	A       int     `json:"a"`
	B       int     `json:"b"`
	AStrain float64 `json:"a_strain"`
	BStrain float64 `json:"b_strain"`
}

// Segment ends at a phase or window boundary. Costs are integrated over this
// interval; splitting a segment never adds another copy of its movement cost.
type Segment struct {
	At           time.Duration  `json:"at_ns"`
	End          time.Duration  `json:"end_ns"`
	States       []State        `json:"states"`
	Fingers      []Cost         `json:"fingers"`
	Interactions []Contribution `json:"interactions,omitempty"`
	Total        float64        `json:"total"`
}

type Window struct {
	At       time.Duration `json:"at_ns"`
	End      time.Duration `json:"end_ns"`
	Fingers  []Cost        `json:"fingers"`
	Total    float64       `json:"total"`
	Rank     int           `json:"rank"`
	Weight   float64       `json:"weight"`
	Weighted float64       `json:"weighted"`
}

type Report struct {
	Model      string        `json:"model"`
	Profile    Profile       `json:"profile"`
	Options    Options       `json:"options"`
	Origin     time.Duration `json:"origin_ns"`
	Notes      []int         `json:"notes_per_finger"`
	Fingers    []Cost        `json:"fingers"`
	Strain     float64       `json:"strain"`
	Difficulty float64       `json:"difficulty"`
	Segments   []Segment     `json:"segments"`
	Windows    []Window      `json:"windows"` // Chronological, with rank metadata.
}

// Analyze never mutates the chart or profile. It plans motions in audio time
// divided by Rate; original chart timestamps remain untouched.
func Analyze(c *chart.Chart, p Profile, o Options) (*Report, error) {
	if err := c.Validate(); err != nil {
		return nil, fmt.Errorf("difficulty: %w", err)
	}
	if err := p.validate(c.Lanes); err != nil {
		return nil, err
	}
	if err := o.validate(); err != nil {
		return nil, err
	}
	for _, lane := range c.Layout {
		if lane.Role == "scratch" {
			return nil, fmt.Errorf("difficulty: scratch movement is outside the finger POC")
		}
	}
	phases, notes, err := schedule(c, p, o)
	if err != nil {
		return nil, err
	}
	r := &Report{Model: Model, Profile: p, Options: o, Notes: notes, Fingers: make([]Cost, c.Lanes)}
	var cuts []time.Duration
	first := true
	for _, lane := range phases {
		for _, ph := range lane {
			if ph.action == Press && (first || ph.end < r.Origin) {
				r.Origin, first = ph.end, false
			}
		}
	}
	if first {
		return r, nil
	}
	for _, lane := range phases {
		for _, ph := range lane {
			cuts = append(cuts, ph.start, ph.end)
			if !moving(ph.action) {
				continue // No passive duration cost in this POC.
			}
			// Insert only boundaries crossed by actual movement, not a long
			// empty lead-in, silent suffix or an isolated sustained hold.
			for at := r.Origin + time.Duration(bin(ph.start-r.Origin, o.Window)+1)*o.Window; at < ph.end; at += o.Window {
				cuts = append(cuts, at)
				if len(cuts) > 2_000_000 {
					return nil, fmt.Errorf("difficulty: too many movement/window boundaries")
				}
			}
		}
	}
	slices.Sort(cuts)
	cuts = slices.Compact(cuts)
	indices := make([]int, c.Lanes)
	last := make([]*time.Duration, c.Lanes)
	loads := make([]float64, c.Lanes)
	windows := make(map[int64]int)
	for k := 0; k+1 < len(cuts); k++ {
		at, end := cuts[k], cuts[k+1]
		seg := Segment{At: at, End: end, States: make([]State, c.Lanes), Fingers: make([]Cost, c.Lanes)}
		intensity := make([]float64, c.Lanes)
		for i, lane := range phases {
			for indices[i] < len(lane) && lane[indices[i]].end <= at {
				ph := lane[indices[i]]
				if moving(ph.action) {
					completed := ph.end
					last[i] = &completed
				}
				indices[i]++
			}
			st := State{Action: Rest, Note: -1, LastMotionEnd: last[i]}
			if indices[i] < len(lane) {
				ph := lane[indices[i]]
				if ph.start <= at {
					st.Action, st.Note, st.Speed = ph.action, ph.note, ph.factor
					st.PositionStart, st.PositionEnd = ph.position(at), ph.position(end)
					if moving(ph.action) {
						// Integrated over the entire phase this is one corrected
						// movement, regardless of how many report cuts split it.
						intensity[i] = ph.factor / float64(ph.end-ph.start)
						base := p.Fingers[i].PressCost
						if ph.action == Release {
							base = p.Fingers[i].ReleaseCost
						}
						seg.Fingers[i].Movement = base * intensity[i] * float64(end-at)
					}
				}
			}
			seg.States[i] = st
		}
		for _, pair := range p.Pairs {
			share := pair.Costs[seg.States[pair.A].Action][seg.States[pair.B].Action]
			// A held recipient inherits the mover's time constraint. For
			// two movers, max intensity is the explicitly chosen POC rule.
			amount := max(intensity[pair.A], intensity[pair.B]) * float64(end-at)
			a, b := share.A*amount, share.B*amount
			seg.Fingers[pair.A].Interaction += a
			seg.Fingers[pair.B].Interaction += b
			if a != 0 || b != 0 {
				seg.Interactions = append(seg.Interactions, Contribution{pair.A, pair.B, a, b})
			}
		}
		for i := range seg.Fingers {
			cost, state := &seg.Fingers[i], &seg.States[i]
			state.LoadStart = loads[i]
			work, demand := cost.rates(end-at, o)
			loads[i], cost.Repeat = advanceLoad(loads[i], work, demand, state.Action, end-at, o)
			state.LoadEnd = loads[i]
			seg.Total += cost.Total()
		}
		if !nonnegative(seg.Total) {
			return nil, fmt.Errorf("difficulty: strain overflow at %s", at)
		}
		if seg.Total == 0 {
			continue
		}
		r.Segments = append(r.Segments, seg)
		id := bin(at-r.Origin, o.Window)
		wi, ok := windows[id]
		if !ok {
			wi = len(r.Windows)
			windows[id] = wi
			start := r.Origin + time.Duration(id)*o.Window
			r.Windows = append(r.Windows, Window{At: start, End: start + o.Window, Fingers: make([]Cost, c.Lanes)})
		}
		w := &r.Windows[wi]
		for i, cost := range seg.Fingers {
			w.Fingers[i].Movement += cost.Movement
			w.Fingers[i].Interaction += cost.Interaction
			w.Fingers[i].Repeat += cost.Repeat
			r.Fingers[i].Movement += cost.Movement
			r.Fingers[i].Interaction += cost.Interaction
			r.Fingers[i].Repeat += cost.Repeat
		}
		w.Total += seg.Total
		r.Strain += seg.Total
	}
	r.Difficulty = Rank(r.Windows, o.Weight)
	if !nonnegative(r.Difficulty) || !nonnegative(r.Strain) {
		return nil, fmt.Errorf("difficulty: aggregate strain overflow")
	}
	return r, nil
}

// Rank annotates chronological windows without reordering them. Equal totals
// are ordered by time, so note/file order never affects ranking.
func Rank(windows []Window, decay float64) float64 {
	order := make([]int, len(windows))
	for i := range order {
		order[i] = i
	}
	slices.SortFunc(order, func(a, b int) int {
		if windows[a].Total > windows[b].Total {
			return -1
		}
		if windows[a].Total < windows[b].Total {
			return 1
		}
		if windows[a].At < windows[b].At {
			return -1
		}
		if windows[a].At > windows[b].At {
			return 1
		}
		return 0
	})
	var total float64
	for rank, i := range order {
		w := &windows[i]
		w.Rank, w.Weight = rank+1, math.Pow(decay, float64(rank))
		w.Weighted = w.Total * w.Weight
		total += w.Weighted
	}
	return total
}

func bin(t, width time.Duration) int64 {
	n := int64(t / width)
	if t%width < 0 {
		n--
	}
	return n
}
