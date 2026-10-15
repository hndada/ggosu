package chart

import (
	"reflect"
	"slices"
	"testing"
	"time"
)

func fixture() *Chart {
	c := New(8)
	c.Timing = []Timing{{BPM: 120, Scroll: 1, Meter: 4}}
	c.Layout = []Lane{{Role: "scratch"}, {Role: "key"}, {Role: "key"}, {Role: "key"}, {Role: "scratch", Group: 1}, {Role: "key", Group: 1}, {Role: "key", Group: 1}, {Role: "key", Group: 1}}
	for lane := range c.Lanes {
		c.Notes = append(c.Notes, Note{At: time.Second, End: 2 * time.Second, Lane: lane, Kind: "hold"})
	}
	return c
}

func TestPermutationsPreserveSourceAndLayout(t *testing.T) {
	c := fixture()
	hash, err := c.Digest()
	if err != nil {
		t.Fatal(err)
	}
	for _, kind := range []string{"", "mirror", "random", "mirror+random"} {
		m, err := NewMod(c, kind, 4294967295)
		if err != nil {
			t.Fatal(err)
		}
		x, err := m.Apply(c)
		if err != nil {
			t.Fatal(err)
		}
		if len(x.Notes) != len(c.Notes) || !reflect.DeepEqual(x.Layout, c.Layout) {
			t.Fatal("lost notes or changed controls")
		}
		for i, n := range x.Notes {
			if n.At != c.Notes[i].At || n.End != c.Notes[i].End || n.Kind != "hold" {
				t.Fatal("changed hold timing")
			}
		}
		if kind != "" {
			if m.Mapping[0] != 0 || m.Mapping[4] != 4 {
				t.Fatal("moved scratch")
			}
			for lane, dest := range m.Mapping {
				if c.Layout[lane].Group != c.Layout[dest].Group {
					t.Fatal("crossed player groups")
				}
			}
			if kind == "mirror" && !slices.Equal(m.Mapping, []int{0, 3, 2, 1, 4, 7, 6, 5}) {
				t.Fatal("incorrect mirror")
			}
		}
	}
	got, err := c.Digest()
	if err != nil || got != hash {
		t.Fatal("mutated source")
	}
}

func TestMirrorRandomComposition(t *testing.T) {
	plain := New(7)
	plain.Timing = []Timing{{BPM: 120, Scroll: 1, Meter: 4}}
	for _, c := range []*Chart{fixture(), plain} {
		for _, seed := range []uint32{0, 123, 4294967295} {
			r, err := NewMod(c, "random", seed)
			if err != nil {
				t.Fatal(err)
			}
			mirror, err := NewMod(c, "mirror", 0)
			if err != nil {
				t.Fatal(err)
			}
			both, err := NewMod(c, "mirror+random", seed)
			if err != nil {
				t.Fatal(err)
			}
			for lane, dest := range r.Mapping {
				if both.Mapping[lane] != mirror.Mapping[dest] {
					t.Fatalf("seed %d lane %d: combined layout does not mirror Random", seed, lane)
				}
			}
			if both.Seed != seed {
				t.Fatal("lost combined seed")
			}
			// Saved mappings take precedence over seed metadata during playback.
			x, err := both.Apply(c)
			if err != nil {
				t.Fatal(err)
			}
			both.Seed++
			y, err := both.Apply(c)
			if err != nil || !reflect.DeepEqual(x, y) {
				t.Fatal("regenerated combined mapping from seed", err)
			}
		}
	}
}

func TestRandomRetryAndStoredMapping(t *testing.T) {
	c := fixture()
	a, err := NewMod(c, "random", 123)
	if err != nil {
		t.Fatal(err)
	}
	b, err := NewMod(c, "random", 123)
	if err != nil || !reflect.DeepEqual(a, b) {
		t.Fatal("same seed produced different layouts")
	}
	// Playback consumes the mapping even when seed metadata changes.
	b.Seed = 456
	x, err := a.Apply(c)
	if err != nil {
		t.Fatal(err)
	}
	y, err := b.Apply(c)
	if err != nil || !reflect.DeepEqual(x, y) {
		t.Fatal("regenerated saved mapping from seed")
	}
	for _, mapping := range [][]int{{0, 1}, {0, 0, 2, 3, 4, 5, 6, 7}, {1, 0, 2, 3, 4, 5, 6, 7}, {0, 5, 2, 3, 4, 1, 6, 7}} {
		if _, err := (Mod{Kind: "random", Mapping: mapping}).Apply(c); err == nil {
			t.Fatalf("accepted invalid layout: %v", mapping)
		}
	}
}
