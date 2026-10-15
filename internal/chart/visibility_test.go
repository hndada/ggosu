package chart

import (
	"reflect"
	"testing"
)

func TestVisibilityPreservesChartAndCombinesWithLaneMods(t *testing.T) {
	c := fixture()
	hash, err := c.Digest()
	if err != nil {
		t.Fatal(err)
	}
	for _, kind := range []string{"", "mirror", "random", "mirror+random"} {
		m, err := NewMod(c, kind, 123)
		if err != nil {
			t.Fatal(err)
		}
		want, err := m.Apply(c)
		if err != nil {
			t.Fatal(err)
		}
		for _, visibility := range []string{"", "hidden", "fadein", "cover", "flashlight"} {
			m.Visibility = visibility
			got, err := m.Apply(c)
			if err != nil || !reflect.DeepEqual(got, want) {
				t.Fatalf("%s/%s changed the chart: %v", kind, visibility, err)
			}
		}
	}
	if got, err := c.Digest(); err != nil || got != hash {
		t.Fatal("visibility changed source identity", err)
	}
	for _, kind := range []string{"invalid", "hidden+fadein", "Hidden"} {
		if err := (Mod{Visibility: kind}).Validate(c.Lanes); err == nil {
			t.Fatalf("accepted %q", kind)
		}
	}
}
