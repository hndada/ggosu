package chart_test

import (
	"bytes"
	"ggosu/internal/chart"
	_ "ggosu/internal/chart/bms"
	_ "ggosu/internal/chart/ojn"
	_ "ggosu/internal/chart/osu"
	_ "ggosu/internal/chart/sm"
	"strings"
	"testing"
	"time"
)

const osuFixture = "osu file format v14\n[General]\nMode:3\n[Metadata]\nTitle:Test\n[Difficulty]\nCircleSize:4\n[TimingPoints]\n0,500,4,1,0,80,1,0\n1000,-50,4,1,0,70,0,1\n2000,250,4,1,0,90,1,0\n[Events]\n0,0,\"bg,wide.png\",0,0\nSprite,Foreground,Centre,\"dot.png\",320,240\n F,0,0,2000,0,1\n M,0,0,2000,0,0,640,480\n[HitObjects]\n512,192,1000,1,0,0:0:0:0:hit.wav\n64,192,1500,128,0,2500:0:0:0:100:hold.wav\n"

func TestOSU(t *testing.T) {
	cs, err := chart.Decode("TEST.OSU", strings.NewReader(osuFixture))
	if err != nil {
		t.Fatal(err)
	}
	c := cs[0]
	if c.Lanes != 4 || c.Notes[0].Lane != 3 || c.Notes[1].End != 2500*time.Millisecond || c.Timing[1].Scroll != 2 || c.Timing[2].Scroll != 1 || len(c.Storyboard) != 2 || len(c.Storyboard[1].Commands) != 2 || c.Assets["bg,wide.png"].Path != "bg,wide.png" {
		t.Fatalf("bad conversion: %+v", c)
	}
	if c.Notes[0].Sounds[0].Volume != .7 {
		t.Fatal("did not inherit volume")
	}
}
func TestBMS(t *testing.T) {
	data := "#TITLE Test\n#BPM 120\n#BPM01 240\n#STOP01 48\n#WAV01 hit.wav\n#WAV02 hold.wav\n#00002:0.5\n#00008:0001\n#00109:01\n#00011:0100\n#00151:0202\n#00211:01\n"
	cs, err := chart.Decode("test.bms", strings.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	c := cs[0]
	if c.Lanes != 6 || c.Notes[1].At != 750*time.Millisecond || c.Notes[1].End != 1500*time.Millisecond || c.Notes[2].At != 2*time.Second {
		t.Fatalf("bad BMS timing: %+v", c.Notes)
	}
}
func TestSM(t *testing.T) {
	data := "#TITLE:Test;\n#OFFSET:0.5;\n#BPMS:0=120,2=240;\n#STOPS:4=1;\n#NOTES:dance-single:Author:Easy:2:0,0,0,0,0:\n0000\n2000\n0000\n3000\n,\n0100\n0000\n0010\n0000\n;\n#NOTES:kb7-single:Author:Hard:4:0,0,0,0,0:0001000;"
	cs, err := chart.Decode("test.sm", strings.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	if len(cs) != 2 || cs[1].Lanes != 7 || cs[0].Notes[0].At != 0 || cs[0].Notes[0].End != 750*time.Millisecond || cs[0].Notes[1].At != time.Second || cs[0].Notes[2].At != 2500*time.Millisecond {
		t.Fatalf("bad SM: %+v", cs[0].Notes)
	}
}
func TestMalformed(t *testing.T) {
	if _, err := chart.Decode("a.bms", strings.NewReader("#BPM 120\n#-0111:01")); err == nil {
		t.Fatal("accepted negative BMS measure")
	}
	for name, data := range map[string]string{"a.osu": strings.Replace(osuFixture, "2500:0:", "1000:0:", 1), "a.sm": "#BPMS:0=120;#NOTES:dance-single:a:Easy:2:x:3000;", "a.bms": "#BPM 120\n#RANDOM 2\n", "a.ojn": "bad"} {
		if _, err := chart.Decode(name, strings.NewReader(data)); err == nil {
			t.Errorf("accepted %s", name)
		}
	}
}
func FuzzRead(f *testing.F) {
	f.Add("a.osu", []byte(osuFixture))
	f.Add("a.ojn", []byte("ojn"))
	f.Add("a.sm", []byte("#BPMS:0=120;"))
	f.Add("a.bms", []byte("#BPM 120"))
	f.Fuzz(func(t *testing.T, name string, b []byte) {
		if len(b) > 1<<20 {
			return
		}
		cs, err := chart.Decode(name, bytes.NewReader(b))
		if err == nil {
			for _, c := range cs {
				if err := c.Validate(); err != nil {
					t.Fatal(err)
				}
			}
		}
	})
}
