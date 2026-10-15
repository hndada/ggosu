package menu

import (
	"slices"
	"strconv"
	"testing"

	"ggosu/internal/settings"
	"ggosu/internal/songselect"
)

func TestCombinedModLaunchKeepsSeed(t *testing.T) {
	a := &App{dataDir: t.TempDir()}
	if _, err := a.SaveDisplay(settings.Display{TimingThreshold: "good", Resolution: "1280x720"}); err != nil {
		t.Fatal(err)
	}
	catalog, err := a.Catalog()
	if err != nil || len(catalog.Entries) == 0 {
		t.Fatal("catalog unavailable", err)
	}
	e := catalog.Entries[0]
	s := Selection{Folder: e.Folder, ChartID: e.Path, Index: e.Index, Mode: "live", Mod: "mirror+random", Visibility: "hidden", Rate: 1}
	for _, seed := range []*uint32{nil, new(uint32)} {
		s.Seed = seed
		first, err := a.prepare(s, nil)
		if err != nil {
			t.Fatal(err)
		}
		if first.selection.Seed == nil || seed != nil && *first.selection.Seed != *seed {
			t.Fatal("combined play lost its seed")
		}
		for flag, want := range map[string]string{"-resolution": "1280x720", "-timing-threshold": "good", "-visibility": "hidden", "-mod": "mirror+random", "-seed": strconv.FormatUint(uint64(*first.selection.Seed), 10)} {
			i := slices.Index(first.args, flag)
			if i < 0 || i+1 >= len(first.args) || first.args[i+1] != want {
				t.Fatalf("launch %s: %v", flag, first.args)
			}
		}
		retry, err := a.prepare(first.selection, nil)
		if err != nil || retry.selection.Seed == nil || *retry.selection.Seed != *first.selection.Seed {
			t.Fatal("retry changed combined seed", err)
		}
	}
}

func TestPracticeSectionLaunchLeavesNoRecord(t *testing.T) {
	a := &App{dataDir: t.TempDir()}
	catalog, err := a.Catalog()
	if err != nil || len(catalog.Entries) == 0 {
		t.Fatal("catalog unavailable", err)
	}
	e := catalog.Entries[0]
	s := Selection{Folder: e.Folder, ChartID: e.Path, Index: e.Index, Mode: "live", Rate: 1, Section: &songselect.Range{Start: 0, End: 600}}
	p, err := a.prepare(s, nil)
	if err != nil {
		t.Fatal(err)
	}
	i := slices.Index(p.args, "-section-end")
	if p.resultPath != "" || slices.Contains(p.args, "-result") || i < 0 || p.args[i+1] != "10m0s" {
		t.Fatalf("section launch: %v", p.args)
	}
	s.Section = &songselect.Range{Start: 590, End: 600}
	if _, err := a.prepare(s, nil); err == nil {
		t.Fatal("section without notes accepted")
	}
	s.Section = &songselect.Range{Start: 5, End: 5}
	if _, err := a.prepare(s, nil); err == nil {
		t.Fatal("empty section accepted")
	}
}
