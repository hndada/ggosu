// Command chat generates room conversations through the real multiplayer protocol.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"sync"
	"time"

	"ggosu/internal/multi"
)

type options struct {
	server, room, password string
	names                  []string
	interval               time.Duration
	count                  int
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	f := flag.NewFlagSet("chat", flag.ContinueOnError)
	var o options
	var names string
	f.StringVar(&o.server, "server", "ws://127.0.0.1:8090/ws", "multiplayer WebSocket endpoint")
	f.StringVar(&o.room, "room", "", "existing room ID (empty creates a DEBUG room)")
	f.StringVar(&o.password, "password", "", "existing room password")
	f.StringVar(&names, "names", "DEBUG 민수,DEBUG mint,DEBUG 긴닉네임", "comma-separated bot names, 1-7")
	f.DurationVar(&o.interval, "interval", 2*time.Second, "time between messages, at least 1100ms per bot")
	f.IntVar(&o.count, "count", 0, "number of messages, 0 runs until Ctrl+C")
	if err := f.Parse(os.Args[1:]); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	if f.NArg() != 0 {
		return fmt.Errorf("unexpected positional arguments")
	}
	for _, name := range strings.Split(names, ",") {
		o.names = append(o.names, strings.TrimSpace(name))
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	return generate(ctx, o, os.Stdout)
}

type bot struct {
	client *multi.Client
	name   string
}

type received struct {
	bot   int
	event multi.Event
	err   error
}

func (o options) validate() error {
	if !strings.HasPrefix(o.server, "ws://") && !strings.HasPrefix(o.server, "wss://") {
		return fmt.Errorf("server must use ws:// or wss://")
	}
	if len(o.names) < 1 || len(o.names) > 7 || o.count < 0 || o.interval <= 0 || o.interval < 1100*time.Millisecond/time.Duration(len(o.names)) {
		return fmt.Errorf("use 1-7 bots, a nonnegative count, and at least 1100ms between each bot's messages")
	}
	seen := make(map[string]bool)
	for _, name := range o.names {
		if !multi.ValidText(name, 32) || seen[name] {
			return fmt.Errorf("bot names must be unique printable names of 1-32 characters")
		}
		seen[name] = true
	}
	if !multi.ValidPassword(o.password) || o.password != "" && o.room == "" {
		return fmt.Errorf("password requires an existing room and at most 64 printable characters")
	}
	return nil
}

func generate(ctx context.Context, o options, out io.Writer) (err error) {
	if err := o.validate(); err != nil {
		return err
	}
	ctx, cancel := context.WithCancel(ctx)
	var bots []bot
	var readers sync.WaitGroup
	defer func() {
		cancel()
		if errors.Is(err, context.Canceled) {
			err = nil
		}
		for _, b := range bots {
			err = errors.Join(err, b.client.Close())
		}
		readers.Wait()
	}()
	start := time.Now()
	now := func() time.Duration { return time.Since(start) }
	for _, name := range o.names {
		setup, stop := context.WithTimeout(ctx, 5*time.Second)
		b, room, e := connect(setup, o, name, now)
		stop()
		if e != nil {
			return fmt.Errorf("connect %s: %w", name, e)
		}
		bots = append(bots, b)
		o.room = room
	}
	if _, err := fmt.Fprintf(out, "DEBUG chat: room=%s bots=%d interval=%s\nJoin this room in the lobby. Ctrl+C stops and disconnects the bots.\n", o.room, len(bots), o.interval); err != nil {
		return err
	}
	inbox := make(chan received, 64)
	for i, b := range bots {
		readers.Add(1)
		go func() {
			defer readers.Done()
			for {
				var r received
				r.bot = i
				select {
				case <-ctx.Done():
					return
				case e := <-b.client.Events():
					r.event = e.Event
				case e := <-b.client.Errors():
					r.err = e
				}
				select {
				case inbox <- r:
				case <-ctx.Done():
					return
				}
				if r.err != nil {
					return
				}
			}
		}()
	}
	// One outstanding request: report accepted sends, surface rate/room errors,
	// and fail a stalled connection instead of accumulating unsent messages.
	timer := time.NewTimer(0)
	defer timer.Stop()
	sent, sender, pending := 0, 0, ""
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-timer.C:
			if pending != "" {
				return fmt.Errorf("%s: chat acknowledgment timed out", bots[sender].name)
			}
			sender = sent % len(bots)
			pending = fmt.Sprintf("debug-chat-%d", sent+1)
			if err := bots[sender].client.Send(multi.Request{Type: "chat", ID: pending, Text: message(sent)}); err != nil {
				return fmt.Errorf("send %s: %w", bots[sender].name, err)
			}
			timer.Reset(5 * time.Second)
		case r := <-inbox:
			if r.err != nil {
				return fmt.Errorf("%s: %w", bots[r.bot].name, r.err)
			}
			e := r.event
			if e.Type == "error" {
				return fmt.Errorf("%s: %s", bots[r.bot].name, e.Error)
			}
			if r.bot != sender || pending == "" || e.ID != pending {
				continue
			}
			if e.Type == "chat" && e.Message != nil {
				if _, err := fmt.Fprintf(out, "%s %s: %s\n", e.Message.Time.Local().Format("15:04:05"), e.Message.Name, e.Message.Text); err != nil {
					return err
				}
			}
			if e.Type == "ack" {
				sent++
				pending = ""
				if o.count > 0 && sent >= o.count {
					return nil
				}
				timer.Stop()
				timer.Reset(o.interval)
			}
		}
	}
}

func connect(ctx context.Context, o options, name string, now func() time.Duration) (b bot, room string, err error) {
	c, err := multi.Dial(ctx, o.server, now)
	if err != nil {
		return b, "", err
	}
	defer func() {
		if err != nil {
			err = errors.Join(err, c.Close())
		}
	}()
	if _, err = await(ctx, c, "welcome", ""); err != nil {
		return b, "", err
	}
	if err = c.Send(multi.Request{Type: "hello", ID: "name", Name: name}); err != nil {
		return b, "", err
	}
	if _, err = await(ctx, c, "ack", "name"); err != nil {
		return b, "", err
	}
	q := multi.Request{Type: "join", ID: "room", RoomID: o.room, Password: o.password}
	if o.room == "" {
		q = multi.Request{Type: "create", ID: "room", Name: "DEBUG 채팅", Capacity: 8}
	}
	if err = c.Send(q); err != nil {
		return b, "", err
	}
	room, err = await(ctx, c, "ack", "room")
	return bot{client: c, name: name}, room, err
}

func await(ctx context.Context, c *multi.Client, kind, id string) (string, error) {
	var room string
	for {
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case err := <-c.Errors():
			return "", err
		case e := <-c.Events():
			if e.Type == "welcome" && e.Version != multi.Version {
				return "", fmt.Errorf("multiplayer protocol version mismatch")
			}
			if e.Type == "error" {
				return "", fmt.Errorf("%s", e.Error)
			}
			if e.Room != nil {
				room = e.Room.ID
			}
			if e.Type == kind && e.ID == id {
				return room, nil
			}
		}
	}
}

func message(i int) string {
	texts := []string{
		"안녕하세요! 다음 곡 같이 해요.",
		"좋아요. 7키로 할까요? 🎵",
		"잠깐만요, 스크롤 속도 조절하고 있어요.",
		"준비됐어요! Ready when you are.",
		"이번에는 관전할게요.\n끝나면 다음 곡 추천해 주세요.",
		"긴 문장 확인: " + strings.Repeat("채팅을 접어도 대화는 계속 이어지고, 이전 메시지를 읽는 동안에는 새 메시지가 와도 읽던 위치가 유지되어야 해요. ", 4),
		"줄바꿈 확인\n두 번째 줄입니다.\n세 번째 줄도 잘 보이나요?",
		"<b>이 문장은 HTML이 아닌 텍스트로 보여야 해요.</b> & \"따옴표\"",
		strings.Repeat("가", multi.MaxChatRunes),
		"수고하셨습니다! 한 판 더 할까요?",
	}
	return strings.TrimSpace(texts[i%len(texts)])
}
