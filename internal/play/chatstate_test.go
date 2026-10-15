package play

import (
	"testing"

	"ggosu/internal/multi"
	"ggosu/internal/settings"
	"time"
)

func TestChatAlertsDeferAndAutoHideOnlyOnce(t *testing.T) {
	c := chatState{RoomID: "r", Open: true}
	p := settings.Chat{Ticker: true, AutoHide: true, MentionPopup: true, MessagePopup: false, MentionSound: true}
	c.Tick(p, true, 0)
	if c.Open {
		t.Fatal("chat did not hide at match start")
	}
	m := multi.Message{ID: 1, RoomID: "r", PlayerID: "other", Name: "친구", Text: "@mint 준비됐어?"}
	if c.Receive(m, "me", "mint", p, true, time.Second) || c.Popup != nil || c.Pending == nil || c.Ticker == nil {
		t.Fatal("gameplay alert was not deferred")
	}
	c.Show()
	c.Tick(p, true, 2*time.Second)
	if !c.Open || c.Pending != nil {
		t.Fatal("manual F8 reopen was hidden again or retained read alerts")
	}
	c.Hide()
	c.restoreOpen = false
	m.ID = 2
	if c.Receive(m, "me", "mint", p, true, 3*time.Second) {
		t.Fatal("sound played during gameplay")
	}
	if !c.Tick(p, false, 4*time.Second) || c.Popup == nil || c.Pending != nil {
		t.Fatal("deferred alert was not flushed")
	}
	c.Tick(p, false, 10*time.Second)
	if c.Popup != nil || c.Ticker != nil {
		t.Fatal("alerts did not expire")
	}
	p.Instant = true
	m.ID = 3
	if !c.Receive(m, "me", "mint", p, true, 11*time.Second) || c.Popup == nil {
		t.Fatal("instant mention did not alert")
	}
	if c.Receive(m, "me", "mint", p, true, 12*time.Second) {
		t.Fatal("duplicate message alerted again")
	}
	m.ID = 4
	m.PlayerID = "me"
	if c.Receive(m, "me", "mint", p, true, 13*time.Second) {
		t.Fatal("own message alerted")
	}
}

func TestChatMentionBoundaries(t *testing.T) {
	for _, tt := range []struct {
		text, name string
		want       bool
	}{{"@MINT!", "mint", true}, {"chocomint", "mint", false}, {"민수야", "민수", false}, {"@민수 hi", "민수", true}, {"@mint+", "mint+", true}, {"text", "", false}} {
		if got := mentioned(tt.text, tt.name); got != tt.want {
			t.Fatalf("%q in %q: %t", tt.name, tt.text, got)
		}
	}
}

func TestChatDrawerPreservesState(t *testing.T) {
	var c chatState
	c.SetRoom("room")
	c.Draft = "작성 중"
	c.Append(multi.Message{ID: 1, RoomID: "room", PlayerID: "other", Text: "first"}, "me")
	if c.Open || c.Unread != 1 {
		t.Fatal("incoming message opened the drawer or failed to count unread")
	}
	c.Append(multi.Message{ID: 2, RoomID: "room", PlayerID: "other", Text: "second"}, "me")
	c.Show()
	c.Scroll = 1
	c.Hide()
	c.Append(multi.Message{ID: 3, RoomID: "room", PlayerID: "other", Text: "third"}, "me")
	c.Show()
	if c.Draft != "작성 중" || c.Scroll != 2 || c.Unread != 1 {
		t.Fatal("toggling lost draft, scroll or unread state")
	}
	c.Append(multi.Message{ID: 3, RoomID: "room", PlayerID: "other"}, "me")
	c.Append(multi.Message{ID: 4, RoomID: "elsewhere", PlayerID: "other"}, "me")
	if len(c.Messages) != 3 {
		t.Fatal("duplicate or other-room message rendered")
	}
}
