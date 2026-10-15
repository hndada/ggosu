package multi

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"ggosu/internal/multi"
)

func TestPasswordRoomRejectsBadEntriesAndKeepsPasswordAfterHostMigration(t *testing.T) {
	f := newFixture(t, Config{})
	h, g, outsider := f.peer("host"), f.peer("guest"), f.peer("outsider")
	secret := "  비밀-Case123  "
	h.ok(multi.Request{Type: "create", Name: "private room", Capacity: 3, Password: secret})
	if !h.room.Locked {
		t.Fatal("password room was public")
	}
	id := h.room.ID
	for _, value := range []string{"", "wrong", strings.TrimSpace(secret), "  비밀-case123  "} {
		g.reject(multi.Request{Type: "join", RoomID: id, Password: value})
		if g.room != nil {
			t.Fatal("rejected guest received room state")
		}
	}
	g.reject(multi.Request{Type: "chat", Text: "not a member"})
	listed := outsider.ok(multi.Request{Type: "list"})
	if len(listed.Rooms) != 1 || !listed.Rooms[0].Locked || listed.Rooms[0].Players != 1 {
		t.Fatal("bad attempts changed membership", listed)
	}
	assertNoPassword(t, listed, secret)
	assertNoPassword(t, h.room, secret)
	resp, err := http.Get(f.http.URL + "/api/rooms")
	if err != nil {
		t.Fatal(err)
	}
	b, readErr := io.ReadAll(resp.Body)
	closeErr := resp.Body.Close()
	if readErr != nil || closeErr != nil || !strings.Contains(string(b), `"locked":true`) || strings.Contains(string(b), secret) || strings.Contains(string(b), `"password"`) {
		t.Fatal("HTTP catalogue exposed a password", readErr, closeErr)
	}
	g.ok(multi.Request{Type: "join", RoomID: id, Password: secret})
	if !g.room.Locked || len(g.room.Members) != 2 {
		t.Fatal("correct password did not admit guest")
	}
	h.ok(multi.Request{Type: "leave"})
	g.ok(multi.Request{Type: "clock"})
	if g.room.HostID != g.id || !g.room.Locked {
		t.Fatal("host migration removed the lock")
	}
	outsider.reject(multi.Request{Type: "join", RoomID: id})
	outsider.ok(multi.Request{Type: "join", RoomID: id, Password: secret})
	if len(outsider.room.Members) != 2 {
		t.Fatal("password stopped working after migration")
	}
}

func assertNoPassword(t *testing.T, value any, secret string) {
	t.Helper()
	b, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	for _, text := range []string{secret, `"password"`, `"salt"`, `"sum"`} {
		if strings.Contains(string(b), text) {
			t.Fatal("public state exposed secret material")
		}
	}
}

func TestPasswordSaltAndValidation(t *testing.T) {
	f := newFixture(t, Config{})
	h1, h2 := f.peer("host1"), f.peer("host2")
	for _, bad := range []string{strings.Repeat("가", 65), "line\nbreak", "tab\tkey"} {
		h1.reject(multi.Request{Type: "create", Name: "invalid", Capacity: 2, Password: bad})
	}
	h1.ok(multi.Request{Type: "create", Name: "one", Capacity: 2, Password: "same password"})
	h2.ok(multi.Request{Type: "create", Name: "two", Capacity: 2, Password: "same password"})
	f.s.mu.Lock()
	p1, p2 := f.s.rooms[h1.room.ID].password, f.s.rooms[h2.room.ID].password
	f.s.mu.Unlock()
	if p1 == nil || p2 == nil || p1.salt == p2.salt || p1.sum == p2.sum {
		t.Fatal("rooms reused password material")
	}
}

func TestPasswordRoomGameUsesTicketWithoutPassword(t *testing.T) {
	f := newFixture(t, Config{})
	h, g := f.peer("host"), f.peer("guest")
	h.ok(multi.Request{Type: "create", Name: "private", Capacity: 2, Password: "secret"})
	g.ok(multi.Request{Type: "join", RoomID: h.room.ID, Password: "secret"})
	sel := selection()
	h.ok(multi.Request{Type: "select", Selection: &sel})
	h.ok(multi.Request{Type: "have", Revision: h.room.Revision, ChartHash: sel.ChartHash, HasChart: true})
	ticket := h.ok(multi.Request{Type: "ticket", Revision: h.room.Revision})
	game := f.peer("game")
	game.reject(multi.Request{Type: "attach", Token: "fake", Password: "secret", RoomID: h.room.ID, Revision: h.room.Revision})
	attached := game.ok(multi.Request{Type: "attach", Token: ticket.Token, RoomID: h.room.ID, Revision: h.room.Revision})
	if attached.Type != "attached" || !attached.Room.Locked || len(attached.Room.Members) != 2 || attached.PlayerID != h.id {
		t.Fatal("password room duplicated or rejected game attachment", attached)
	}
	assertNoPassword(t, attached, "secret")
	reuse := f.peer("reuse")
	reuse.reject(multi.Request{Type: "attach", Token: ticket.Token, RoomID: h.room.ID, Revision: h.room.Revision})
}

func TestPasswordWorkDoesNotBlockRoomLockAndShutdown(t *testing.T) {
	f := newFixture(t, Config{})
	h, g, observer := f.peer("host"), f.peer("guest"), f.peer("observer")
	h.ok(multi.Request{Type: "create", Name: "private", Capacity: 2, Password: "secret"})
	// Fill the derivation slots to make the join wait deterministically.
	for i := 0; i < cap(f.s.passwords); i++ {
		f.s.passwords <- struct{}{}
	}
	if err := g.conn.WriteJSON(multi.Request{Type: "join", ID: "blocked", RoomID: h.room.ID, Password: "secret"}); err != nil {
		t.Fatal(err)
	}
	observer.ok(multi.Request{Type: "clock"})
	observer.ok(multi.Request{Type: "list"})
	if err := f.s.Close(); err != nil {
		t.Fatal("shutdown did not cancel waiting password checks", err)
	}
}
