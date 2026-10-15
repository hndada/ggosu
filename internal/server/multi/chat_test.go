package multi

import (
	"encoding/json"
	"ggosu/internal/multi"
	"strings"
	"testing"
	"time"
)

func TestRoomChatBroadcastHistoryAndIsolation(t *testing.T) {
	f := newFixture(t, Config{})
	a, b, other := f.peer("host"), f.peer("guest"), f.peer("elsewhere")
	a.ok(multi.Request{Type: "create", Name: "room", Capacity: 4})
	id := a.room.ID
	b.ok(multi.Request{Type: "join", RoomID: id})
	other.ok(multi.Request{Type: "create", Name: "other", Capacity: 2})
	a.ok(multi.Request{Type: "chat", Name: "spoofed", RoomID: other.room.ID, Text: "  안녕하세요\n같은 방 대화예요  "})
	var message *multi.Message
	for {
		e := b.read()
		if e.Type == "chat" {
			message = e.Message
			break
		}
	}
	if message == nil || message.PlayerID != a.id || message.Name != "host" || message.RoomID != id || message.Text != "안녕하세요\n같은 방 대화예요" {
		t.Fatalf("chat sender/scope incorrect: %+v", message)
	}
	f.s.mu.Lock()
	if len(f.s.rooms[other.room.ID].chat) != 0 || len(f.s.rooms[id].chat) != 1 {
		t.Error("chat leaked to a different room")
	}
	f.s.mu.Unlock()
	c := f.peer("late")
	c.ok(multi.Request{Type: "join", RoomID: id})
	// The history precedes the join ack; inspect it independently of room state.
	f.s.mu.Lock()
	stored := f.s.rooms[id].chat
	if len(stored) != 1 || stored[0].ID != message.ID {
		t.Error("history missing")
	}
	f.s.mu.Unlock()
	a.ok(multi.Request{Type: "leave"})
	a.reject(multi.Request{Type: "chat", Text: "not in a room"})
}

func TestChatDuringGameAndAfterReset(t *testing.T) {
	f := newFixture(t, Config{})
	a, b, r := f.match()
	a.ok(multi.Request{Type: "chat", Text: "경기 중에도 방 채팅"})
	b.ok(multi.Request{Type: "abort", MatchID: r.MatchID})
	b.ok(multi.Request{Type: "chat", Text: "포기 후에도 같은 방"})
	a.ok(multi.Request{Type: "abort", MatchID: r.MatchID})
	a.ok(multi.Request{Type: "reset"})
	f.s.mu.Lock()
	defer f.s.mu.Unlock()
	if len(f.s.rooms[r.ID].chat) != 2 {
		t.Fatal("phase/reset erased the room chat")
	}
}

func TestChatLimitsAndHistoryFrame(t *testing.T) {
	f := newFixture(t, Config{})
	p := f.peer("sender")
	p.ok(multi.Request{Type: "create", Name: "room", Capacity: 2})
	for _, text := range []string{" ", "a\x00b", strings.Repeat("가", multi.MaxChatRunes+1)} {
		p.reject(multi.Request{Type: "chat", Text: text})
	}
	for range 5 {
		p.ok(multi.Request{Type: "chat", Text: "message"})
	}
	p.reject(multi.Request{Type: "chat", Text: "rate limited"})
	f.clock.Add(int64(5 * time.Second))
	p.ok(multi.Request{Type: "chat", Text: "allowed again"})
	f.s.mu.Lock()
	room := f.s.rooms[p.room.ID]
	for range 110 {
		room.nextChat++
		room.chat = append(room.chat, multi.Message{ID: room.nextChat, RoomID: room.ID, Name: strings.Repeat("<", 32), Text: strings.Repeat("<", multi.MaxChatRunes)})
	}
	page := room.chat[len(room.chat)-12:]
	b, err := json.Marshal(multi.Event{Type: "chat_history", RoomID: room.ID, Messages: page})
	f.s.mu.Unlock()
	if err != nil || len(b) >= 32<<10 {
		t.Fatalf("chat history exceeds native frame cap: %d %v", len(b), err)
	}
}
