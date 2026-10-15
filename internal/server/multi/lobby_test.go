package multi

import (
	"ggosu/internal/multi"
	"strings"
	"testing"
	"time"
)

func TestGoChartHoldersAndLateJoin(t *testing.T) {
	f := newFixture(t, Config{})
	h, g, w := f.peer("host"), f.peer("guest"), f.peer("waiting")
	h.ok(multi.Request{Type: "create", Name: "room", Capacity: 4})
	g.ok(multi.Request{Type: "join", RoomID: h.room.ID})
	w.ok(multi.Request{Type: "join", RoomID: h.room.ID})
	sel := selection()
	h.ok(multi.Request{Type: "select", Selection: &sel})
	for _, p := range []*peer{h, g} {
		p.ok(multi.Request{Type: "have", Revision: h.room.Revision, ChartHash: sel.ChartHash, HasChart: true})
	}
	h.ok(multi.Request{Type: "start"})
	if h.room.Phase != "preparing" || !h.room.Members[0].Participant || !h.room.Members[1].Participant || h.room.Members[2].Participant {
		t.Fatalf("wrong Go participants: %+v", h.room)
	}
	f.ready(h)
	if h.room.Phase != "preparing" {
		t.Fatal("started before guest loaded")
	}
	f.ready(g)
	f.arm(h)
	f.arm(g)
	if g.room.Phase != "playing" {
		t.Fatal("chartless waiter blocked start")
	}
	late := f.peer("late")
	late.ok(multi.Request{Type: "join", RoomID: h.room.ID})
	if late.room.Members[3].Participant {
		t.Fatal("late join entered active match")
	}
	late.reject(multi.Request{Type: "armed", MatchID: g.room.MatchID, StartNS: g.room.StartNS})
	f.clock.Store(g.room.StartAt.Add(time.Second).UnixNano())
	h.ok(multi.Request{Type: "wait", MatchID: g.room.MatchID})
	g.ok(multi.Request{Type: "abort", MatchID: g.room.MatchID})
	if g.room.Phase != "finished" || !g.room.Members[0].Connected || len(g.room.Members) != 4 {
		t.Fatalf("room lost after giving up: %+v", g.room)
	}
	h.ok(multi.Request{Type: "reset"})
	h.ok(multi.Request{Type: "start"})
	if h.room.Phase != "preparing" {
		t.Fatal("next round unavailable")
	}
	f.clock.Add(int64(31 * time.Second))
	h.ok(multi.Request{Type: "clock"})
	if h.room.Phase != "lobby" || h.room.Notice == "" {
		t.Fatal("loading timeout did not cancel")
	}
}

func TestProposalAuthorityAndReplacement(t *testing.T) {
	f := newFixture(t, Config{})
	h, g := f.peer("host"), f.peer("guest")
	h.ok(multi.Request{Type: "create", Name: "room", Capacity: 2})
	g.ok(multi.Request{Type: "join", RoomID: h.room.ID})
	sel := selection()
	g.ok(multi.Request{Type: "propose", Selection: &sel})
	sel.Title = "Other"
	sel.ChartHash = strings.Repeat("b", 64)
	g.ok(multi.Request{Type: "propose", Selection: &sel})
	if len(g.room.Proposals) != 1 || g.room.Selection != nil {
		t.Fatal("proposal changed selection or grew without bound")
	}
	g.reject(multi.Request{Type: "select", Selection: &sel})
	h.ok(multi.Request{Type: "select", Selection: &sel})
	if len(h.room.Proposals) != 0 || h.room.Selection.ChartHash != sel.ChartHash {
		t.Fatal("host adoption failed")
	}
}

func TestGameAttachmentKeepsLobbyAndIdentity(t *testing.T) {
	f := newFixture(t, Config{})
	h, g := f.peer("host"), f.peer("guest")
	h.ok(multi.Request{Type: "create", Name: "room", Capacity: 2})
	g.ok(multi.Request{Type: "join", RoomID: h.room.ID})
	sel := selection()
	h.ok(multi.Request{Type: "select", Selection: &sel})
	h.ok(multi.Request{Type: "have", Revision: h.room.Revision, ChartHash: sel.ChartHash, HasChart: true})
	ticket := h.ok(multi.Request{Type: "ticket", Revision: h.room.Revision})
	game := f.peer("unused")
	attached := game.ok(multi.Request{Type: "attach", Token: ticket.Token, RoomID: h.room.ID, Revision: h.room.Revision})
	if attached.Type != "attached" || attached.PlayerID != h.id || len(attached.Room.Members) != 2 {
		t.Fatalf("duplicated member: %+v", attached)
	}
	reuse := f.peer("reuse")
	reuse.reject(multi.Request{Type: "attach", Token: ticket.Token, RoomID: h.room.ID, Revision: h.room.Revision})
	game.reject(multi.Request{Type: "leave"})
	game.reject(multi.Request{Type: "select", Selection: &sel})
	f.ready(game)
	h.ok(multi.Request{Type: "start"})
	game.ok(multi.Request{Type: "armed", MatchID: h.room.MatchID, StartNS: h.room.StartNS})
	h.ok(multi.Request{Type: "clock"})
	if h.room.Phase != "playing" || !h.room.Members[0].Participant || h.room.Members[1].Participant {
		t.Fatal("attached game did not start as host")
	}
	game.ok(multi.Request{Type: "chat", Text: "hello from game"})
	if err := game.conn.Close(); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(time.Second)
	for {
		h.ok(multi.Request{Type: "clock"})
		if h.room.Phase == "finished" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("game close did not abort")
		}
		time.Sleep(time.Millisecond)
	}
	if !h.room.Members[0].Connected || h.room.HostID != h.id {
		t.Fatal("game close removed lobby owner")
	}
	h.ok(multi.Request{Type: "reset"})
	ticket = h.ok(multi.Request{Type: "ticket", Revision: h.room.Revision})
	game2 := f.peer("unused2")
	game2.ok(multi.Request{Type: "attach", Token: ticket.Token, RoomID: h.room.ID, Revision: h.room.Revision})
	if len(game2.room.Members) != 2 {
		t.Fatal("second game duplicated player")
	}
	h.ok(multi.Request{Type: "leave"})
	g.ok(multi.Request{Type: "clock"})
	if g.room.HostID != g.id {
		t.Fatal("host migration after lobby leave failed")
	}
}
