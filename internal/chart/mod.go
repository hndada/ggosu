// Mods change lane arrangement and note visibility without changing the source chart.
package chart

import (
	"fmt"
	"math/rand/v2"
	"slices"
)

// Mapping maps source lanes to destination lanes. Store it as well as Seed so
// saved replays do not depend on a future random-number implementation.
type Mod struct {
	Visibility string `json:"visibility,omitempty"`
	// O2Jam is the event mod: open2jam's beat judgement and life bar at easy, normal or hard.
	O2Jam   string `json:"o2jam,omitempty"`
	Pills   bool   `json:"pills,omitempty"` // O2Jam pills turn a later GOOD into a COOL.
	Kind    string `json:"kind,omitempty"`
	Seed    uint32 `json:"seed,omitempty"`
	Mapping []int  `json:"mapping,omitempty"`
}

func (m Mod) Validate(lanes int) error {
	if err := ValidateVisibility(m.Visibility); err != nil {
		return err
	}
	if err := ValidateO2Jam(m.O2Jam); err != nil {
		return err
	}
	if m.Pills && m.O2Jam == "" {
		return fmt.Errorf("mod: pills need the O2Jam mod")
	}
	if m.Kind == "" {
		if m.Seed != 0 || len(m.Mapping) != 0 {
			return fmt.Errorf("mod: unmodified play cannot have a seed or mapping")
		}
		return nil
	}
	if m.Kind != "mirror" && m.Kind != "random" && m.Kind != "mirror+random" || m.Kind == "mirror" && m.Seed != 0 || lanes < 1 || lanes > 18 || len(m.Mapping) != lanes {
		return fmt.Errorf("mod: invalid kind or lane count")
	}
	seen := make([]bool, lanes)
	for _, lane := range m.Mapping {
		if lane < 0 || lane >= lanes || seen[lane] {
			return fmt.Errorf("mod: mapping must be a lane permutation")
		}
		seen[lane] = true
	}
	return nil
}

// ValidateVisibility accepts one visibility effect; the empty value shows all notes.
func ValidateVisibility(kind string) error {
	switch kind {
	case "", "hidden", "fadein", "cover", "flashlight":
		return nil
	default:
		return fmt.Errorf("mod: unknown visibility %q", kind)
	}
}

// ValidateO2Jam accepts the O2Jam mod's life bar; the empty value turns it off.
func ValidateO2Jam(level string) error {
	switch level {
	case "", "easy", "normal", "hard":
		return nil
	default:
		return fmt.Errorf("mod: unknown O2Jam level %q", level)
	}
}

func groups(c *Chart) [][]int {
	var groups [][]int
	indices := map[int]int{}
	for i := 0; i < c.Lanes; i++ {
		group := 0
		if len(c.Layout) > 0 {
			if c.Layout[i].Role == "scratch" {
				continue
			}
			group = c.Layout[i].Group
		}
		index, ok := indices[group]
		if !ok {
			index = len(groups)
			indices[group] = index
			groups = append(groups, nil)
		}
		groups[index] = append(groups[index], i)
	}
	return groups
}

func NewMod(c *Chart, kind string, seed uint32) (Mod, error) {
	if err := c.Validate(); err != nil {
		return Mod{}, err
	}
	if kind == "" {
		return Mod{}, nil
	}
	if kind != "mirror" && kind != "random" && kind != "mirror+random" {
		return Mod{}, fmt.Errorf("mod: unknown kind %q", kind)
	}
	m := Mod{Kind: kind, Mapping: make([]int, c.Lanes)}
	for i := range m.Mapping {
		m.Mapping[i] = i
	}
	r := rand.New(rand.NewPCG(uint64(seed), 0x67676f7375))
	random := kind == "random" || kind == "mirror+random"
	if random {
		m.Seed = seed
	}
	for _, lanes := range groups(c) {
		dest := slices.Clone(lanes)
		if kind == "mirror" || kind == "mirror+random" {
			slices.Reverse(dest)
		}
		if random {
			// Shuffling mirrored destinations reflects the same seed's random layout.
			r.Shuffle(len(dest), func(i, j int) { dest[i], dest[j] = dest[j], dest[i] })
		}
		for i, lane := range lanes {
			m.Mapping[lane] = dest[i]
		}
	}
	return m, m.Validate(c.Lanes)
}

func (m Mod) Apply(c *Chart) (*Chart, error) {
	if err := c.Validate(); err != nil {
		return nil, err
	}
	if err := m.Validate(c.Lanes); err != nil {
		return nil, err
	}
	if m.Kind == "" {
		return c, nil
	}
	if m.Kind == "mirror" {
		expected, err := NewMod(c, "mirror", 0)
		if err != nil {
			return nil, err
		}
		if !slices.Equal(m.Mapping, expected.Mapping) {
			return nil, fmt.Errorf("mod: mirror mapping does not match the chart layout")
		}
	}
	for source, dest := range m.Mapping {
		if len(c.Layout) > 0 {
			a, b := c.Layout[source], c.Layout[dest]
			if a.Group != b.Group || a.Role != b.Role || a.Role == "scratch" && source != dest {
				return nil, fmt.Errorf("mod: mapping moves scratch or crosses player groups")
			}
		}
	}
	x := *c
	x.Notes = slices.Clone(c.Notes)
	for i := range x.Notes {
		x.Notes[i].Lane = m.Mapping[x.Notes[i].Lane]
	}
	slices.SortStableFunc(x.Notes, func(a, b Note) int {
		if a.At < b.At {
			return -1
		}
		if a.At > b.At {
			return 1
		}
		return a.Lane - b.Lane
	})
	return &x, x.Validate()
}
