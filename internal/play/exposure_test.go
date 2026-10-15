package play

import (
	"math"
	"testing"
	"time"

	"ggosu/internal/chart"
	"ggosu/internal/piano"
	"ggosu/internal/settings"
)

func TestExposureTracksScrollRateAndFieldReach(t *testing.T) {
	c := chart.New(1)
	c.Timing = []chart.Timing{{BPM: 120, Scroll: 1, Meter: 4}}
	s, err := piano.NewSession(c)
	if err != nil {
		t.Fatal(err)
	}
	p := &view{session: s, field: newField((settings.Profile{SpeedChange: 1}).Visual(c), c, "argon")}
	p.Update(viewState{Rate: 1, Scroll: 110})
	if p.hud.exposureText != "5608ms" {
		t.Fatal("initial exposure missing", p.hud.exposureText)
	}
	p.Update(viewState{Rate: 1, Scroll: 220})
	if p.hud.exposureText != "2804ms" {
		t.Fatal("scroll change did not update exposure", p.hud.exposureText)
	}
	p.Update(viewState{Rate: 2, Scroll: 220})
	if p.hud.exposureText != "1402ms" {
		t.Fatal("rate change did not update exposure", p.hud.exposureText)
	}
	p.field.layout.line = 500
	p.Update(p.state)
	if p.hud.exposureText != "1136ms" {
		t.Fatal("hit line change did not update exposure", p.hud.exposureText)
	}
	p.config.Profile = &settings.Profile{ScrollMode: "exposure", ExposureMS: 750}
	p.Update(viewState{Rate: 2, Scroll: 220})
	if math.Abs(p.exposureMS()-750) > 1e-9 || p.hud.exposureText != "750ms" {
		t.Fatal("exposure mode displayed stale speed", p.hud.exposureText)
	}
}

func TestViewScrollIsIndependentOfBPM(t *testing.T) {
	for _, bpm := range []float64{60, 120, 240} {
		c := chart.New(1)
		// The base speed ignores absolute BPM; the later change only sets the main BPM aside.
		c.Timing = []chart.Timing{{BPM: bpm, Scroll: 1, Meter: 4}, {At: 6 * time.Second, BPM: bpm * 2, Scroll: 1, Meter: 4}}
		c.Notes = []chart.Note{{At: 3 * time.Second, End: 5 * time.Second, Kind: "hold"}}
		profile, err := (settings.Config{}).Profile(1)
		if err != nil {
			t.Fatal(err)
		}
		for _, mode := range []string{"default", "speed", "exposure"} {
			profile.ScrollMode = mode
			config := viewConfig{Keys: profile.Keys}
			if mode != "default" {
				config.Profile = &profile
			}
			s, err := piano.NewSession(c)
			if err != nil {
				t.Fatal(err)
			}
			p, err := newView(c, s, nil, config)
			if err != nil {
				t.Fatal(err)
			}
			for _, rate := range []float64{.5, 1, 2} {
				p.Update(viewState{Position: 500 * time.Millisecond, Rate: rate, Scroll: 200})
				n := p.field.notes[0][0]
				head := p.field.y(n.head, p.state.Scroll)
				length := head - p.field.y(n.tail, p.state.Scroll)
				step := 500 * time.Millisecond
				p.Update(viewState{Position: 500*time.Millisecond + time.Duration(float64(step)*rate), Rate: rate, Scroll: 200})
				want := 200 * step.Seconds() * rate
				wantLength := 400.0
				if mode == "exposure" {
					speed := float64(p.field.layout.line-p.field.layout.top) * 1000 / float64(profile.ExposureMS)
					want = speed * step.Seconds()
					wantLength = 2 * speed / rate
					if math.Abs(p.exposureMS()-float64(profile.ExposureMS)) > 1e-6 {
						t.Fatal("exposure changed with BPM or rate")
					}
				}
				if got := float64(p.field.y(n.head, p.state.Scroll) - head); math.Abs(got-want) > 1e-3 {
					t.Fatalf("BPM %v, mode %s, rate %v: moved %v px, want %v", bpm, mode, rate, got, want)
				}
				if math.Abs(float64(length)-wantLength) > 1e-3 {
					t.Fatalf("BPM %v, mode %s, rate %v: hold length %v, want %v", bpm, mode, rate, length, wantLength)
				}
			}
		}
	}
}
