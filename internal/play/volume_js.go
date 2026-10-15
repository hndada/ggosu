package play

import (
	"encoding/json"
	"fmt"
	"syscall/js"
	"time"
)

func (g *scene) readVolumeKeys(now time.Duration) {
	for _, key := range web.volumeKeys {
		g.volumeKey(key, now)
	}
	web.volumeKeys = web.volumeKeys[:0]
}

func (g *scene) persistAudio() (err error) {
	b, err := json.Marshal(g.audio)
	if err != nil {
		return err
	}
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("browser preferences: %v", r)
		}
	}()
	storage := js.Global().Get("localStorage")
	storage.Call("setItem", "ggosu:audio:backup", string(b))
	storage.Call("setItem", "ggosu:audio", string(b))
	post("audio", g.audio)
	return nil
}
