package main

import (
	"bytes"
	"context"
	"io"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"ggosu/internal/multi"
	server "ggosu/internal/server/multi"
)

func endpoint(t *testing.T) string {
	t.Helper()
	s, err := server.NewServer(server.Config{})
	if err != nil {
		t.Fatal(err)
	}
	h := httptest.NewServer(s)
	t.Cleanup(func() {
		if err := s.Close(); err != nil {
			t.Error(err)
		}
		h.Close()
	})
	return "ws" + strings.TrimPrefix(h.URL, "http") + "/ws"
}

func observer(t *testing.T, address string) (bot, string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	start := time.Now()
	b, room, err := connect(ctx, options{server: address}, "Observer", func() time.Duration { return time.Since(start) })
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := b.client.Close(); err != nil {
			t.Error(err)
		}
	})
	return b, room
}

func TestConversationThroughServer(t *testing.T) {
	address := endpoint(t)
	watcher, room := observer(t, address)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	o := options{server: address, room: room, names: []string{"DEBUG A", "DEBUG B"}, interval: 550 * time.Millisecond, count: 10}
	var output bytes.Buffer
	if err := generate(ctx, o, &output); err != nil {
		t.Fatal(err)
	}
	var messages []multi.Message
	for len(messages) < o.count {
		select {
		case e := <-watcher.client.Events():
			if e.Type == "chat" && e.Message != nil {
				messages = append(messages, *e.Message)
			}
		case err := <-watcher.client.Errors():
			t.Fatal(err)
		case <-ctx.Done():
			t.Fatal("observer did not receive the generated conversation")
		}
	}
	for i, m := range messages {
		if m.RoomID != room || m.ID != uint64(i+1) || m.Name != o.names[i%2] || m.PlayerID == "" || m.Time.IsZero() || m.Text != message(i) {
			t.Fatalf("server message %d: %+v", i, m)
		}
	}
	if !strings.Contains(messages[6].Text, "\n") || !strings.Contains(messages[7].Text, "<b>") || utf8.RuneCountInString(messages[8].Text) != multi.MaxChatRunes {
		t.Fatal("missing multiline, literal HTML, or maximum-length sample")
	}
	if !strings.Contains(output.String(), "room="+room) || strings.Count(output.String(), "DEBUG A:") != 5 || strings.Count(output.String(), "DEBUG B:") != 5 {
		t.Fatalf("accepted message output: %s", output.String())
	}
	// A finite run must disconnect every bot and preserve the human's room.
	for {
		select {
		case e := <-watcher.client.Events():
			if e.Room != nil {
				connected := 0
				for _, m := range e.Room.Members {
					if m.Connected {
						connected++
					}
				}
				if connected == 1 && e.Room.HostID == e.Room.Members[0].ID {
					return
				}
			}
		case <-ctx.Done():
			t.Fatal("bots remained connected after completion")
		}
	}
}

func TestCancellationDisconnectsBots(t *testing.T) {
	address := endpoint(t)
	watcher, room := observer(t, address)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() {
		done <- generate(ctx, options{server: address, room: room, names: []string{"DEBUG"}, interval: 2 * time.Second}, io.Discard)
	}()
	timeout := time.NewTimer(5 * time.Second)
	defer timeout.Stop()
	for {
		select {
		case e := <-watcher.client.Events():
			if e.Type != "chat" {
				continue
			}
			cancel()
			select {
			case err := <-done:
				if err != nil {
					t.Fatal(err)
				}
				for {
					select {
					case e := <-watcher.client.Events():
						if e.Room == nil {
							continue
						}
						connected := 0
						for _, m := range e.Room.Members {
							if m.Connected {
								connected++
							}
						}
						if connected == 1 {
							return
						}
					case <-timeout.C:
						t.Fatal("canceled bots remained in the room")
					}
				}
			case <-timeout.C:
				t.Fatal("canceled generator did not stop")
			}
		case err := <-done:
			t.Fatalf("generator stopped before cancellation: %v", err)
		case <-timeout.C:
			t.Fatal("generator did not send its first message")
		}
	}
}

func TestCreatesDebugRoom(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var output bytes.Buffer
	if err := generate(ctx, options{server: endpoint(t), names: []string{"DEBUG A", "DEBUG B", "DEBUG C"}, interval: 400 * time.Millisecond, count: 3}, &output); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"DEBUG A", "DEBUG B", "DEBUG C"} {
		if !strings.Contains(output.String(), name+":") {
			t.Fatalf("bot did not join created room: %s", output.String())
		}
	}
}

func TestJoinFailureDisconnectsEarlierBots(t *testing.T) {
	address := endpoint(t)
	watcher, room := observer(t, address)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	// Leaving/recreating lets this fixture use a two-person room.
	if err := watcher.client.Send(multi.Request{Type: "leave", ID: "leave"}); err != nil {
		t.Fatal(err)
	}
	if _, err := await(ctx, watcher.client, "ack", "leave"); err != nil {
		t.Fatal(err)
	}
	if err := watcher.client.Send(multi.Request{Type: "create", ID: "small", Name: "small", Capacity: 2}); err != nil {
		t.Fatal(err)
	}
	var err error
	room, err = await(ctx, watcher.client, "ack", "small")
	if err != nil {
		t.Fatal(err)
	}
	err = generate(ctx, options{server: address, room: room, names: []string{"DEBUG A", "DEBUG B"}, interval: time.Second}, io.Discard)
	if err == nil || !strings.Contains(err.Error(), "room unavailable") {
		t.Fatalf("expected full-room failure: %v", err)
	}
	for {
		select {
		case e := <-watcher.client.Events():
			if e.Room != nil && e.Room.ID == room && len(e.Room.Members) == 1 {
				return
			}
		case <-ctx.Done():
			t.Fatal("partial setup left a bot connected")
		}
	}
}

func TestInvalidOptions(t *testing.T) {
	valid := options{server: "ws://127.0.0.1:8090/ws", names: []string{"DEBUG"}, interval: 2 * time.Second}
	for _, mutate := range []func(*options){
		func(o *options) { o.interval = time.Millisecond },
		func(o *options) { o.count = -1 },
		func(o *options) { o.names = nil },
		func(o *options) { o.names = []string{"same", "same"} },
		func(o *options) { o.names = []string{"\n"} },
		func(o *options) { o.names = make([]string, 8) },
		func(o *options) { o.server = "http://localhost" },
		func(o *options) { o.password = "password" },
	} {
		o := valid
		mutate(&o)
		if err := o.validate(); err == nil {
			t.Fatalf("accepted invalid options: %+v", o)
		}
	}
}
