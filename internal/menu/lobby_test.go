package menu

import (
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"ggosu/internal/multi"
	multiserver "ggosu/internal/server/multi"
)

func lobbyWait(t *testing.T, a *App, want func(LobbyState) bool) LobbyState {
	t.Helper()
	until := time.Now().Add(3 * time.Second)
	for time.Now().Before(until) {
		v := a.MultiState(0)
		if want(v) {
			return v
		}
		if v.Error != "" {
			t.Fatal(v.Error)
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("lobby event timed out")
	return LobbyState{}
}

func TestDesktopLobbyLifecycle(t *testing.T) {
	server, err := multiserver.NewServer(multiserver.Config{})
	if err != nil {
		t.Fatal(err)
	}
	http := httptest.NewServer(server)
	t.Cleanup(func() {
		if err := server.Close(); err != nil {
			t.Error(err)
		}
		http.Close()
	})
	endpoint := "ws" + strings.TrimPrefix(http.URL, "http") + "/ws"
	h, g := &App{}, &App{}
	for _, a := range []*App{h, g} {
		t.Cleanup(func() {
			if err := a.close(); err != nil {
				t.Error(err)
			}
		})
		if err := a.MultiConnect(endpoint, "Player"); err != nil {
			t.Fatal(err)
		}
		lobbyWait(t, a, func(v LobbyState) bool { return v.PlayerID != "" })
	}
	if err := h.MultiConnect(endpoint, "duplicate"); err == nil {
		t.Fatal("duplicate persistent connection allowed")
	}
	if err := h.MultiSend(multi.Request{Type: "create", Name: "persistent", Capacity: 2, Password: "lobby-secret"}); err != nil {
		t.Fatal(err)
	}
	r := lobbyWait(t, h, func(v LobbyState) bool { return v.Room != nil }).Room
	if !r.Locked {
		t.Fatal("desktop lobby lost the room lock")
	}
	if err := g.MultiSend(multi.Request{Type: "join", ID: "bad-password", RoomID: r.ID, Password: "wrong"}); err != nil {
		t.Fatal(err)
	}
	lobbyWait(t, g, func(v LobbyState) bool {
		for _, e := range v.Events {
			if e.Event.ID == "bad-password" && e.Event.Type == "error" {
				return true
			}
		}
		return false
	})
	if g.MultiState(0).Room != nil {
		t.Fatal("desktop admitted wrong password")
	}
	if err := g.MultiSend(multi.Request{Type: "join", RoomID: r.ID, Password: "lobby-secret"}); err != nil {
		t.Fatal(err)
	}
	lobbyWait(t, h, func(v LobbyState) bool { return len(v.Room.Members) == 2 })
	if err := g.MultiSend(multi.Request{Type: "chat", ID: "chat-check", Text: "room connection survives panel state"}); err != nil {
		t.Fatal(err)
	}
	v := lobbyWait(t, h, func(v LobbyState) bool {
		for _, e := range v.Events {
			if e.Event.Type == "chat" {
				return true
			}
		}
		return false
	})
	if len(h.MultiState(v.Sequence).Events) != 0 {
		t.Fatal("cursor replayed delivered events")
	}
	if err := g.MultiSend(multi.Request{Type: "attach", Token: "fake"}); err == nil {
		t.Fatal("lobby could send game-only request")
	}
	if err := g.MultiDisconnect(); err != nil {
		t.Fatal(err)
	}
	lobbyWait(t, h, func(v LobbyState) bool { return v.Room != nil && len(v.Room.Members) == 1 })
	if g.MultiState(0).Connected {
		t.Fatal("disconnected lobby remained active")
	}
	if err := g.MultiConnect(endpoint, "New name"); err != nil {
		t.Fatal(err)
	}
	lobbyWait(t, g, func(v LobbyState) bool { return v.PlayerID != "" })
	if err := g.MultiSend(multi.Request{Type: "join", RoomID: r.ID, Password: "lobby-secret"}); err != nil {
		t.Fatal(err)
	}
	lobbyWait(t, h, func(v LobbyState) bool { return len(v.Room.Members) == 2 })
	if err := h.MultiSend(multi.Request{Type: "leave"}); err != nil {
		t.Fatal(err)
	}
	lobbyWait(t, g, func(v LobbyState) bool { return v.Room != nil && v.Room.HostID == v.PlayerID })
}
