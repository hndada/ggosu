//go:build !js

package play

import (
	"cmp"
	"fmt"
	"image"
	"strings"
	"unicode/utf8"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/exp/textinput"
	"github.com/hajimehoshi/ebiten/v2/inpututil"

	"ggosu/internal/multi"
)

func (g *scene) initChat() {
	n := g.network
	n.chatInput.OnNewSession = func() *textinput.SessionOptions {
		if !n.chatFocus || g.view == nil {
			return nil
		}
		b := g.view.ChatBounds()
		return &textinput.SessionOptions{CaretBounds: image.Rect(b.Min.X+14, 616, b.Max.X-14, 637), TextBeforeCaret: n.chat.Draft}
	}
	n.chatInput.OnComposition = func(c *textinput.Composition) { n.chatComposition = c.Text() }
	n.chatInput.OnCommit = func(c *textinput.Commit) {
		text := n.chat.Draft + c.Text()
		if before, after := c.IsSurroundingTextReplaced(); before || after {
			left, right := c.SurroundingText()
			text = left + c.Text() + right
		}
		if utf8.RuneCountInString(text) <= multi.MaxChatRunes {
			n.chat.Draft = text
		} else {
			n.chatStatus = "Maximum 300 characters"
		}
	}
	n.chatInput.OnEndByUser = func() { n.chatFocus = false }
}

func (g *scene) updateChat() error {
	n := g.network
	if n == nil || g.view == nil {
		return nil
	}
	n.chatHandled = false
	// The attached game's lobby delivers deferred alerts after its window returns.
	if n.token != "" && !g.chatPlaying() && !g.chatPrefs.Instant {
		n.chat.Pending = nil
	}
	wasOpen := n.chat.Open
	if n.chat.Tick(g.chatPrefs, g.chatPlaying(), g.clock.Now()) {
		g.chatSound()
	}
	if wasOpen && !n.chat.Open {
		n.chatInput.Confirm()
		n.chatFocus = false
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyF8) {
		if n.chat.Open {
			g.closeChat()
		} else {
			n.chat.Show()
		}
		n.chatHandled = true
	}
	if inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonLeft) {
		point := g.cursor()
		if point.In(g.view.ChatToggleBounds(n.chat.Open)) {
			if n.chat.Open {
				g.closeChat()
			} else {
				n.chat.Show()
			}
		} else if n.chat.Open {
			b := g.view.ChatBounds()
			focus := point.In(image.Rect(b.Min.X+8, 600, b.Max.X-8, 666))
			if focus && !n.chatFocus && g.started {
				if err := g.release(g.position); err != nil {
					return err
				}
			}
			if !focus && n.chatFocus {
				n.chatInput.Confirm()
			}
			n.chatFocus = focus
		}
	}
	if n.chat.Open {
		if n.chatFocus && inpututil.IsKeyJustPressed(ebiten.KeyEscape) {
			g.closeChat()
			n.chatHandled = true
		}
		// Ctrl+wheel moves the lane cover instead.
		if g.cursor().In(g.view.ChatBounds()) && !ebiten.IsKeyPressed(ebiten.KeyControl) {
			_, dy := ebiten.Wheel()
			n.chat.Scroll = min(max(0, n.chat.Scroll+int(dy)), max(0, len(n.chat.Messages)-1))
			if n.chat.Scroll == 0 {
				n.chat.Unread = 0
			}
		}
	}
	if n.chatFocus {
		handled, err := n.chatInput.Update()
		if err != nil {
			return err
		}
		if !handled && n.chatComposition == "" {
			if inpututil.IsKeyJustPressed(ebiten.KeyBackspace) && n.chat.Draft != "" {
				n.chatInput.Confirm()
				_, size := utf8.DecodeLastRuneInString(n.chat.Draft)
				n.chat.Draft = n.chat.Draft[:len(n.chat.Draft)-size]
			}
			if !ebiten.IsKeyPressed(ebiten.KeyAlt) && inpututil.IsKeyJustPressed(ebiten.KeyEnter) && n.chatPending == "" && strings.TrimSpace(n.chat.Draft) != "" {
				n.chatInput.Confirm()
				n.chatSeq++
				n.chatPending, n.chatSent = fmt.Sprintf("chat-%d", n.chatSeq), strings.TrimSpace(n.chat.Draft)
				if err := n.client.Send(multi.Request{Type: "chat", ID: n.chatPending, Text: n.chatSent}); err != nil {
					n.chatPending = ""
					n.chatStatus = err.Error()
				}
			}
		}
	}
	return nil
}

func (g *scene) closeChat() {
	n := g.network
	n.chatInput.Confirm()
	n.chatFocus = false
	n.chat.Hide()
	n.chat.restoreOpen = false
}
func (g *scene) chatTyping() bool { return g.network != nil && g.network.chatFocus }

func (g *scene) chatPlaying() bool { return g.started && !g.ended && !g.finishing }

func (g *scene) chatSound() {
	if g.bank != nil {
		if err := g.bank.Effect("mention", 1); err != nil {
			g.network.chatStatus = "알림음: " + err.Error()
		}
	}
}

func (g *scene) chatState(s *viewState) {
	n := g.network
	if n == nil {
		return
	}
	c := &chat{Open: n.chat.Open, Unread: n.chat.Unread, Focused: n.chatFocus, Draft: n.chat.Draft + n.chatComposition, Status: n.chatStatus}
	if n.chat.Ticker != nil && g.clock.Now() < n.chat.Ticker.Until {
		c.Ticker = n.chat.Ticker
	}
	if n.chat.Popup != nil && g.clock.Now() < n.chat.Popup.Until {
		c.Popup = n.chat.Popup
	}
	if n.room != nil {
		c.Room = n.room.Name
	}
	if !n.chat.Open {
		s.Chat = c
		return
	}
	end := max(0, len(n.chat.Messages)-n.chat.Scroll)
	var minute string
	for _, m := range n.chat.Messages[:end] {
		line := chatLine{Name: m.Name, Text: m.Text, Colour: chatNameColour(cmp.Or(m.PlayerID, m.Name))}
		if t := m.Time.Local().Format("15:04"); t != minute {
			line.Time, minute = t, t
		}
		c.Messages = append(c.Messages, line)
	}
	s.Chat = c
}
