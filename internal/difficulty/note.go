package difficulty

import "time"

// Share of one finger's segment cost charged to a note. Interaction lists the
// other fingers whose pair terms made up this finger's interaction cost.
type Part struct {
	At          time.Duration `json:"at_ns"`
	End         time.Duration `json:"end_ns"`
	Finger      int           `json:"finger"`
	Action      Action        `json:"action"`
	Speed       float64       `json:"speed"`
	LoadStart   float64       `json:"load_start"`
	LoadEnd     float64       `json:"load_end"`
	Cost        Cost          `json:"cost"`
	Interaction []Partner     `json:"interaction,omitempty"`
}

type Partner struct {
	Finger int     `json:"finger"`
	Action Action  `json:"action"`
	Amount float64 `json:"amount"`
}

type NoteStrain struct {
	Cost  Cost   `json:"cost"`
	Parts []Part `json:"parts,omitempty"`
}

// Attribute charges every finger's segment cost to the note that finger is
// moving for or holding. Repeat cost left while a finger rests goes to its
// last note, the one that built the load. The note totals sum to r.Strain;
// they are unweighted, so they do not sum to r.Difficulty.
func (r *Report) Attribute(notes int) []NoteStrain {
	out := make([]NoteStrain, notes)
	var last []int
	for _, seg := range r.Segments {
		if last == nil {
			last = make([]int, len(seg.States))
			for i := range last {
				last[i] = -1
			}
		}
		for i, cost := range seg.Fingers {
			st := seg.States[i]
			note := st.Note
			if note >= 0 {
				last[i] = note
			} else {
				note = last[i]
			}
			if note < 0 || note >= notes || cost.Total() == 0 {
				continue
			}
			part := Part{At: seg.At, End: seg.End, Finger: i, Action: st.Action, Speed: st.Speed,
				LoadStart: st.LoadStart, LoadEnd: st.LoadEnd, Cost: cost}
			for _, c := range seg.Interactions {
				if c.A == i && c.AStrain != 0 {
					part.Interaction = append(part.Interaction, Partner{c.B, seg.States[c.B].Action, c.AStrain})
				} else if c.B == i && c.BStrain != 0 {
					part.Interaction = append(part.Interaction, Partner{c.A, seg.States[c.A].Action, c.BStrain})
				}
			}
			n := &out[note]
			n.Cost.Movement += cost.Movement
			n.Cost.Interaction += cost.Interaction
			n.Cost.Repeat += cost.Repeat
			n.Parts = append(n.Parts, part)
		}
	}
	return out
}
