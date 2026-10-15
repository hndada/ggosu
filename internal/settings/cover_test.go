package settings

import (
	"encoding/json"
	"math"
	"testing"
)

func TestCoverDefaultsAndSharing(t *testing.T) {
	var p Profile
	if err := json.Unmarshal([]byte(`{"keys":["D"],"scroll":260,"scrollMode":"speed","exposureMS":750}`), &p); err != nil {
		t.Fatal(err)
	}
	if c := p.Normalized().LaneCover(); c != DefaultCover() {
		t.Fatalf("old profile cover %+v", c)
	}
	if s, h, l := DefaultCover().Amounts(); s != 0 || h != 0 || l != 0 {
		t.Fatal("default cover is on")
	}
	var c Config
	p4, _ := c.Profile(4)
	p4.Cover = &Cover{Sudden: .3, SuddenOn: true}
	c.SetProfile(4, p4)
	p7, err := c.Profile(7)
	if err != nil || p7.LaneCover() != *p4.Cover {
		t.Fatalf("cover not shared: %+v %v", p7.Cover, err)
	}
	for _, bad := range []float64{-.1, 1.1, math.NaN()} {
		p7.Cover = &Cover{Hidden: bad}
		if p7.Validate(7) == nil {
			t.Fatalf("accepted cover %v", bad)
		}
	}
}

// beatoraja moves SUDDEN+ while it is on or all are off, then LIFT, then HIDDEN+.
func TestCoverAdjustTarget(t *testing.T) {
	c := DefaultCover()
	c.Adjust(.01)
	if c.Sudden != .21 {
		t.Fatalf("all off: %+v", c)
	}
	c = Cover{Sudden: .2, Hidden: .1, Lift: .1, LiftOn: true, HiddenOn: true}
	c.Adjust(.01)
	if c.Lift != .09 || c.Hidden != .1 || c.Sudden != .2 {
		t.Fatalf("lift first: %+v", c)
	}
	c.LiftOn = false
	c.Adjust(-.5)
	if c.Hidden != .6 {
		t.Fatalf("hidden: %+v", c)
	}
	c.SuddenOn = true
	c.Adjust(-1)
	if c.Sudden != 0 {
		t.Fatalf("clamp: %+v", c)
	}
}

func TestCoverFlagRoundTrip(t *testing.T) {
	c := Cover{Sudden: .205, Hidden: .1, Lift: 0, SuddenOn: true, LiftOn: true}
	got, err := ParseCover(c.Flag())
	if err != nil || got != c || c.Flag() != "0.205+,0.1,0+" {
		t.Fatal(c.Flag(), got, err)
	}
	for _, bad := range []string{"", "0.2,0.1", "x,0,0", "2,0,0"} {
		if _, err := ParseCover(bad); err == nil {
			t.Fatalf("accepted %q", bad)
		}
	}
}
