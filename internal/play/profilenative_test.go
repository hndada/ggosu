//go:build !js

package play

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"ggosu/internal/settings"
)

func TestCloseKeepsProfileWithoutScore(t *testing.T) {
	for _, mode := range []string{"live", "auto", "section", "medley", "replay"} {
		t.Run(mode, func(t *testing.T) {
			g := pauseGame(t, 1)
			g.mode, g.initialRate = "live", 1
			g.settingsPath = filepath.Join(t.TempDir(), "settings.json")
			p, err := (settings.Config{}).Profile(1)
			if err != nil {
				t.Fatal(err)
			}
			p.Keys = []string{"A"}
			p.ScrollOffset, p.ExposureOffset = 1.5, 120
			c := settings.Config{Name: "Player", GlobalMS: 17}
			c.SetProfile(1, p)
			if err := settings.Save(g.settingsPath, c); err != nil {
				t.Fatal(err)
			}
			g.profile = p.Effective()
			switch mode {
			case "live":
				g.result = filepath.Join(filepath.Dir(g.settingsPath), "records", "canceled.json")
			case "auto":
				g.mode, g.autoplay = "auto", true
			case "section":
				g.sectionStart, g.sectionEnd = 9*time.Second, 11*time.Second
			case "replay":
				g.mode = "replay"
			}
			if err := g.prepare(); err != nil {
				t.Fatal(err)
			}
			g.scroll, g.profile.Scroll, g.profile.ExposureMS = 20, 20, 600
			cover := g.profile.LaneCover()
			cover.SuddenOn, cover.Sudden = true, .4
			g.profile.Cover = &cover
			g.position = time.Second
			if err := g.save(); err != nil {
				t.Fatal(err)
			}
			if err := g.close(); err != nil {
				t.Fatal(err)
			}
			if mode == "live" {
				if _, err := os.Stat(g.result); !os.IsNotExist(err) {
					t.Fatalf("canceled run wrote a record: %v", err)
				}
			}
			got, err := settings.Load(g.settingsPath)
			if err != nil {
				t.Fatal(err)
			}
			profile, err := got.Profile(1)
			if err != nil {
				t.Fatal(err)
			}
			want := p
			if mode != "replay" {
				want.Scroll, want.ExposureMS, want.Cover = 18.5, 480, &cover
			}
			if !reflect.DeepEqual(profile, want) || got.Name != c.Name || got.GlobalMS != c.GlobalMS {
				t.Fatalf("profile/other settings changed: got %+v, want %+v", profile, want)
			}
			// A resident host may close twice; effective offsets must not be applied again.
			if err := g.close(); err != nil {
				t.Fatal(err)
			}
			again, err := settings.Load(g.settingsPath)
			if err != nil || !reflect.DeepEqual(again, got) {
				t.Fatal("second close changed preferences", err)
			}
		})
	}
}

func TestCoverSaveKeepsClampedScrollValues(t *testing.T) {
	g := pauseGame(t, 1)
	g.settingsPath = filepath.Join(t.TempDir(), "settings.json")
	p, err := (settings.Config{}).Profile(1)
	if err != nil {
		t.Fatal(err)
	}
	p.Scroll, p.ScrollOffset, p.ExposureMS, p.ExposureOffset = 38, 4, 100, -120
	var c settings.Config
	c.SetProfile(1, p)
	if err := settings.Save(g.settingsPath, c); err != nil {
		t.Fatal(err)
	}
	g.profile = p.Effective()
	if err := g.prepare(); err != nil {
		t.Fatal(err)
	}
	cover := g.profile.LaneCover()
	cover.SuddenOn = true
	g.profile.Cover = &cover
	if err := g.close(); err != nil {
		t.Fatal(err)
	}
	c, err = settings.Load(g.settingsPath)
	if err != nil {
		t.Fatal(err)
	}
	got, err := c.Profile(1)
	if err != nil || got.Scroll != p.Scroll || got.ExposureMS != p.ExposureMS || got.LaneCover() != cover {
		t.Fatal("cover save changed unchanged clamped speed/exposure", got, err)
	}
}
