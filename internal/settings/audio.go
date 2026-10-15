package settings

import (
	"cmp"
	"encoding/json"
	"fmt"
	"slices"
)

// Audio volumes are percents. Master scales the others, as osu!'s master volume does.
// Hit and Guide follow ONGEKI's TAP成功音 and ガイド音: a sound on every key press and
// one at every note's judgement time. Both are off (zero) by default.
type Audio struct {
	Master   int    `json:"master"`
	Music    int    `json:"music"`
	Keys     int    `json:"keys"`
	Effects  int    `json:"effects"`
	Hit      int    `json:"hit,omitempty"`
	HitSound string `json:"hitSound,omitempty"` // Empty means click.
	Guide    int    `json:"guide,omitempty"`
}

var HitSounds = []string{"click", "wood", "glass", "pluck"}

// UnmarshalJSON keeps master at full for settings saved before it existed.
func (a *Audio) UnmarshalJSON(b []byte) error {
	type plain Audio
	v := plain{Master: 100}
	if err := json.Unmarshal(b, &v); err != nil {
		return err
	}
	*a = Audio(v)
	return nil
}

func (a Audio) Validate() error {
	for _, v := range []int{a.Master, a.Music, a.Keys, a.Effects, a.Hit, a.Guide} {
		if v < 0 || v > 100 {
			return fmt.Errorf("volume must be between 0 and 100")
		}
	}
	if a.HitSound != "" && !slices.Contains(HitSounds, a.HitSound) {
		return fmt.Errorf("hit sound must be one of %v", HitSounds)
	}
	return nil
}

// HitKind is the effect file of the selected hit sound.
func (a Audio) HitKind() string {
	return "hit-" + cmp.Or(a.HitSound, "click")
}

// Mixed returns music, keys and effects with master applied.
func (a Audio) Mixed() Audio {
	scale := func(v int) int { return (v*a.Master + 50) / 100 }
	return Audio{Master: 100, Music: scale(a.Music), Keys: scale(a.Keys), Effects: scale(a.Effects), Hit: scale(a.Hit), HitSound: a.HitSound, Guide: scale(a.Guide)}
}

func (c Config) AudioSettings() Audio {
	if c.Audio != nil {
		return *c.Audio
	}
	return Audio{Master: 100, Music: 100, Keys: 100, Effects: 50}
}
