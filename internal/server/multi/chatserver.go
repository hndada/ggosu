package multi

import (
	"fmt"
	"ggosu/internal/multi"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

func chatText(text string) (string, error) {
	text = strings.TrimSpace(strings.ReplaceAll(text, "\r\n", "\n"))
	if !utf8.ValidString(text) || text == "" || utf8.RuneCountInString(text) > multi.MaxChatRunes {
		return "", fmt.Errorf("chat: enter 1-%d characters", multi.MaxChatRunes)
	}
	for _, r := range text {
		if unicode.IsControl(r) && r != '\n' {
			return "", fmt.Errorf("chat: control characters are not allowed")
		}
	}
	return text, nil
}

func (s *Server) chat(c *client, r *room, q multi.Request) error {
	text, err := chatText(q.Text)
	if err != nil {
		return err
	}
	now := s.tick()
	p := c
	if c.owner != "" {
		p = s.clients[c.owner]
	}
	if now-p.chatAt >= 5*time.Second {
		p.chatAt, p.chatCount = now, 0
	}
	if p.chatCount >= 5 {
		return fmt.Errorf("chat: wait before sending more messages")
	}
	p.chatCount++
	r.nextChat++
	m := multi.Message{ID: r.nextChat, RoomID: r.ID, PlayerID: c.id, Name: c.name, Time: s.now().UTC(), Text: text}
	r.chat = append(r.chat, m)
	if len(r.chat) > 100 {
		copy(r.chat, r.chat[len(r.chat)-100:])
		r.chat = r.chat[:100]
	}
	for _, id := range r.order {
		p := s.clients[id]
		if p == nil || p.room != r.ID {
			continue
		}
		e := multi.Event{Type: "chat", RoomID: r.ID, Message: &m}
		if p == c {
			e.ID = q.ID
		}
		s.send(p, e)
		if p.game != nil {
			s.send(p.game, e)
		}
	}
	return nil
}

// Twelve messages keep even escaped 300-rune texts inside the client frame cap.
func (s *Server) chatHistory(c *client, r *room) {
	start := max(0, len(r.chat)-12)
	s.send(c, multi.Event{Type: "chat_history", RoomID: r.ID, Messages: append([]multi.Message(nil), r.chat[start:]...)})
}
