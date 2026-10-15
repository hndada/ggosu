package settings

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestProfileReadsLegacySVScale(t *testing.T) {
	var p Profile
	if err := json.Unmarshal([]byte(`{"keys":["D"],"scroll":260,"scrollMode":"speed","exposureMS":750,"svScale":0.8}`), &p); err != nil || p.SpeedChange != .8 {
		t.Fatal(p, err)
	}
	// The former px/s scroll converts to osu!lazer mania speed: 420 px/s is about 8.
	if err := json.Unmarshal([]byte(`{"keys":["D"],"scroll":420,"scrollOffset":-52.5,"scrollMode":"speed","exposureMS":750}`), &p); err != nil || p.Scroll != 8 || p.ScrollOffset != -1 {
		t.Fatal("legacy px/s scroll not converted", p, err)
	}
	if err := json.Unmarshal([]byte(`{"keys":["D"],"scroll":10,"scrollMode":"speed","exposureMS":750}`), &p); err != nil || p.Scroll != MinScroll {
		t.Fatal("legacy slow scroll not clamped", p, err)
	}
	var s Scroll
	if err := json.Unmarshal([]byte(`{"mode":"speed","speed":1050,"exposureMS":750,"speedChange":1}`), &s); err != nil || s.Speed != 20 {
		t.Fatal("legacy shared scroll not converted", s, err)
	}
	if json.Unmarshal([]byte(`{"keys":["D"],"bogus":1}`), &p) == nil {
		t.Fatal("accepted unknown profile field")
	}
}

func TestKeys(t *testing.T) {
	for n := 1; n <= 18; n++ {
		p, err := (Config{}).Profile(n)
		if err != nil || p.Validate(n) != nil {
			t.Fatalf("%dK default invalid: %v", n, err)
		}
	}
	for _, keys := range [][]string{{"D", "D", "J", "K"}, {"D", "P", "J", "K"}, {"D", "ESCAPE", "J", "K"}, {"D", "NUMPAD1", "J", "K"}} {
		if (Profile{Keys: keys}).Validate(4) == nil {
			t.Fatalf("accepted invalid keys %v", keys)
		}
	}
	if (Profile{Keys: []string{"1", "2", "3", "4"}}).Validate(4) != nil {
		t.Fatal("rejected number keys")
	}
}

// Default keys follow hndada/gosu's KeyMappings.
func TestDefaultKeysFollowHndada(t *testing.T) {
	for lanes, want := range map[int]string{6: "S D F J K L", 8: "A S D F SPACE J K L", 9: "A S D F SPACE J K L SEMICOLON", 10: "A S D F V N J K L SEMICOLON"} {
		keys, err := DefaultKeys(lanes)
		if err != nil || strings.Join(keys, " ") != want {
			t.Fatal(lanes, keys, err)
		}
		if err := (Profile{Keys: keys}).Validate(lanes); err != nil {
			t.Fatal(lanes, err)
		}
	}
}
