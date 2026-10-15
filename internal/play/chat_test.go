//go:build !js

package play

import (
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

func TestChatTypingDoesNotReachGameplayShortcuts(t *testing.T) {
	g := &scene{network: &network{chatFocus: true}}
	for _, key := range []ebiten.Key{ebiten.KeyEscape, ebiten.KeyF5, ebiten.KeyF6, ebiten.KeyPageUp} {
		if g.pressed(key) {
			t.Fatalf("chat typing reached game shortcut %v", key)
		}
	}
	if !g.chatTyping() {
		t.Fatal("native lane input was not gated")
	}
}
