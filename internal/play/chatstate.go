package play

import (
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"ggosu/internal/multi"
	"ggosu/internal/settings"
)

type chatAlert struct {
	Message      multi.Message
	Count        int
	Until        time.Duration
	Popup, Sound bool
}

// chatState retains a native drawer's state independently of game phase and snapshots.
type chatState struct {
	RoomID                 string
	Open                   bool
	Unread                 int
	Draft                  string
	Scroll                 int // lines up from the latest native message
	Messages               []multi.Message
	Ticker, Popup, Pending *chatAlert
	Playing, restoreOpen   bool
}

func (c *chatState) SetRoom(id string) {
	if c.RoomID == id {
		return
	}
	c.RoomID, c.Unread, c.Scroll = id, 0, 0
	c.Messages = nil
	c.Ticker, c.Popup, c.Pending = nil, nil, nil
}

func (c *chatState) Append(m multi.Message, self string) {
	if m.RoomID != c.RoomID || len(c.Messages) > 0 && m.ID <= c.Messages[len(c.Messages)-1].ID {
		return
	}
	c.Messages = append(c.Messages, m)
	if len(c.Messages) > 100 {
		c.Messages = c.Messages[len(c.Messages)-100:]
	}
	if c.Scroll > 0 {
		c.Scroll = min(c.Scroll+1, len(c.Messages)-1)
	}
	if m.PlayerID != self && (!c.Open || c.Scroll > 0) {
		c.Unread++
	}
}

func (c *chatState) Show() {
	c.Open = true
	if c.Scroll == 0 {
		c.Unread = 0
		c.Ticker, c.Popup, c.Pending = nil, nil, nil
	}
}

func (c *chatState) Hide() { c.Open = false }

func mentioned(text, name string) bool {
	name = strings.ToLower(strings.TrimSpace(name))
	if name == "" {
		return false
	}
	text = strings.ToLower(text)
	word := func(r rune) bool { return unicode.IsLetter(r) || unicode.IsNumber(r) || r == '_' }
	for from := 0; from < len(text); {
		i := strings.Index(text[from:], name)
		if i < 0 {
			return false
		}
		i += from
		end := i + len(name)
		left, _ := utf8.DecodeLastRuneInString(text[:i])
		right, _ := utf8.DecodeRuneInString(text[end:])
		if (i == 0 || !word(left)) && (end == len(text) || !word(right)) {
			return true
		}
		from = end
	}
	return false
}

// Receive returns whether an immediate mention sound should play.
func (c *chatState) Receive(m multi.Message, self, name string, prefs settings.Chat, playing bool, now time.Duration) bool {
	if m.RoomID != c.RoomID || len(c.Messages) > 0 && m.ID <= c.Messages[len(c.Messages)-1].ID {
		return false
	}
	c.Append(m, self)
	if m.PlayerID == self || c.Open && c.Scroll == 0 {
		return false
	}
	if prefs.Ticker && !c.Open {
		c.Ticker = &chatAlert{Message: m, Until: now + 5*time.Second}
	}
	mention := mentioned(m.Text, name)
	popup := mention && prefs.MentionPopup || !mention && prefs.MessagePopup
	sound := mention && prefs.MentionSound
	if playing && !prefs.Instant && (popup || sound) {
		count := 1
		if c.Pending != nil {
			count += c.Pending.Count
			popup = popup || c.Pending.Popup
			sound = sound || c.Pending.Sound
		}
		c.Pending = &chatAlert{Message: m, Count: count, Popup: popup, Sound: sound}
		return false
	}
	if popup {
		c.Popup = &chatAlert{Message: m, Count: 1, Until: now + 5*time.Second}
	}
	return sound
}

// Tick expires alerts and flushes deferred alerts after play. F8 can reopen a
// drawer hidden at match start; auto-hide is applied only on that transition.
func (c *chatState) Tick(prefs settings.Chat, playing bool, now time.Duration) bool {
	if playing && !c.Playing && prefs.AutoHide {
		c.restoreOpen = c.Open
		c.Hide()
	}
	if !playing && c.Playing && c.restoreOpen {
		c.Show()
		c.restoreOpen = false
	}
	c.Playing = playing
	if c.Ticker != nil && (!prefs.Ticker || now >= c.Ticker.Until) {
		c.Ticker = nil
	}
	if c.Popup != nil && ((!prefs.MessagePopup && !prefs.MentionPopup) || now >= c.Popup.Until) {
		c.Popup = nil
	}
	if !playing && c.Pending != nil {
		p := c.Pending
		c.Pending = nil
		if p.Popup && (prefs.MessagePopup || prefs.MentionPopup) {
			p.Until = now + 5*time.Second
			c.Popup = p
		}
		return p.Sound && prefs.MentionSound
	}
	return false
}
