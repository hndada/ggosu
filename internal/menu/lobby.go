package menu

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"sync"
	"time"

	"ggosu/internal/chart"
	"ggosu/internal/clock"
	"ggosu/internal/multi"
)

type lobby struct {
	mu                      sync.Mutex
	client                  *multi.Client
	endpoint, name, id, err string
	room                    *multi.Room
	rooms                   []multi.Summary
	events                  []LobbyEvent
	sequence                uint64
	pending                 map[string]chan multi.Event
	stop, done              chan struct{}
	once                    sync.Once
	launching               bool
}

type LobbyEvent struct {
	Sequence uint64      `json:"sequence"`
	Event    multi.Event `json:"event"`
}

type LobbyState struct {
	Connected bool            `json:"connected"`
	PlayerID  string          `json:"player_id"`
	Room      *multi.Room     `json:"room,omitempty"`
	Rooms     []multi.Summary `json:"rooms"`
	Events    []LobbyEvent    `json:"events"`
	Sequence  uint64          `json:"sequence"`
	Error     string          `json:"error"`
	Running   bool            `json:"running"`
}

func (a *App) MultiConnect(endpoint, name string) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.closed || a.lobby != nil {
		return fmt.Errorf("기존 멀티 연결을 종료해 주세요")
	}
	counter, err := clock.New()
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	c, err := multi.Dial(ctx, endpoint, counter.Now)
	if err != nil {
		return err
	}
	l := &lobby{client: c, endpoint: endpoint, name: name, pending: map[string]chan multi.Event{}, stop: make(chan struct{}), done: make(chan struct{})}
	a.lobby = l
	go l.run()
	return nil
}

func (l *lobby) run() {
	defer close(l.done)
	defer l.client.Close()
	for {
		select {
		case <-l.stop:
			return
		case err := <-l.client.Errors():
			l.mu.Lock()
			l.err = err.Error()
			l.mu.Unlock()
			return
		case received := <-l.client.Events():
			e := received.Event
			l.mu.Lock()
			switch e.Type {
			case "welcome":
				l.id = e.PlayerID
			case "room":
				l.room = e.Room
			case "rooms":
				l.rooms = e.Rooms
			}
			l.sequence++
			l.events = append(l.events, LobbyEvent{Sequence: l.sequence, Event: e})
			if len(l.events) > 200 {
				l.events = l.events[len(l.events)-200:]
			}
			if ch := l.pending[e.ID]; ch != nil && (e.Type == "ticket" || e.Type == "error") {
				ch <- e
				delete(l.pending, e.ID)
			}
			l.mu.Unlock()
			if e.Type == "welcome" {
				if e.Version != multi.Version {
					l.mu.Lock()
					l.err = "멀티 프로토콜 버전이 다릅니다"
					l.mu.Unlock()
					return
				}
				if err := l.client.Send(multi.Request{Type: "hello", Name: l.name}); err != nil {
					l.mu.Lock()
					l.err = err.Error()
					l.mu.Unlock()
					return
				}
				if err := l.client.Send(multi.Request{Type: "list"}); err != nil {
					l.mu.Lock()
					l.err = err.Error()
					l.mu.Unlock()
					return
				}
			}
		}
	}
}

func (l *lobby) close() {
	l.once.Do(func() { close(l.stop) })
	<-l.done
}

func (a *App) MultiDisconnect() error {
	a.mu.Lock()
	l := a.lobby
	a.lobby = nil
	a.mu.Unlock()
	if l != nil {
		l.close()
	}
	return nil
}

func (a *App) multi() (*lobby, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.lobby == nil || a.closed {
		return nil, fmt.Errorf("멀티 서버에 접속해 주세요")
	}
	return a.lobby, nil
}

func (a *App) MultiState(after uint64) LobbyState {
	l, err := a.multi()
	if err != nil {
		return LobbyState{}
	}
	a.mu.Lock()
	running := a.state.Running
	a.mu.Unlock()
	l.mu.Lock()
	defer l.mu.Unlock()
	v := LobbyState{Connected: l.err == "", PlayerID: l.id, Room: l.room, Rooms: l.rooms, Sequence: l.sequence, Error: l.err, Running: running}
	for _, e := range l.events {
		if e.Sequence > after {
			v.Events = append(v.Events, e)
		}
	}
	return v
}

func (a *App) MultiSend(q multi.Request) error {
	l, err := a.multi()
	if err != nil {
		return err
	}
	switch q.Type {
	case "create", "join", "leave", "list", "have", "select", "propose", "start", "reset", "cancel", "chat", "kick", "wait":
	default:
		return fmt.Errorf("대기실 요청이 아닙니다")
	}
	if err := l.client.Send(q); err != nil {
		return err
	}
	if q.Type == "leave" {
		l.mu.Lock()
		l.room = nil
		l.mu.Unlock()
	}
	return nil
}

func (a *App) MultiChoose(s Selection, propose bool) error {
	e, err := a.songs.Lookup(s)
	if err != nil {
		return err
	}
	charts, err := chart.Load(e.Path)
	if err != nil {
		return err
	}
	if e.Index < 0 || e.Index >= len(charts) {
		return fmt.Errorf("난이도가 변경됐습니다")
	}
	c := charts[e.Index]
	if s.Rate == 0 {
		s.Rate = 1
	}
	seed := uint32(0)
	if s.Mod == "random" || s.Mod == "mirror+random" {
		if s.Seed != nil {
			seed = *s.Seed
		} else {
			var b [4]byte
			if _, err := rand.Read(b[:]); err != nil {
				return err
			}
			seed = binary.LittleEndian.Uint32(b[:])
		}
	}
	mods, err := chart.NewMod(c, s.Mod, seed)
	if err != nil {
		return err
	}
	mods.Visibility = s.Visibility
	audio := e.AudioDuration
	if audio == 0 {
		// Charts from extra song folders are scanned without audio lengths.
		audio = c.AudioDuration(nil)
	}
	selected, err := multi.Describe(c, mods, s.Rate, audio)
	if err != nil {
		return err
	}
	selected.Pitch = s.Pitch == nil || *s.Pitch
	typ := "select"
	if propose {
		typ = "propose"
	}
	return a.MultiSend(multi.Request{Type: typ, Selection: &selected})
}

// MultiPlay loads a matching local chart. The server's one-use attachment leaves
// the lobby connection alive after this child exits, including abnormal exits.
func (a *App) MultiPlay() (string, error) {
	l, err := a.multi()
	if err != nil {
		return "", err
	}
	l.mu.Lock()
	if l.launching {
		l.mu.Unlock()
		return "플레이를 준비 중입니다", nil
	}
	l.launching = true
	defer func() { l.mu.Lock(); l.launching = false; l.mu.Unlock() }()
	r := l.room
	if r == nil || r.Selection == nil {
		l.mu.Unlock()
		return "", fmt.Errorf("선택된 채보가 없습니다")
	}
	selected := *r.Selection
	pitch := selected.Pitch
	roomID, revision := r.ID, r.Revision
	id := fmt.Sprintf("ticket-%d", l.sequence)
	ch := make(chan multi.Event, 1)
	l.pending[id] = ch
	l.mu.Unlock()
	defer func() { l.mu.Lock(); delete(l.pending, id); l.mu.Unlock() }()
	a.mu.Lock()
	if a.state.Running {
		a.mu.Unlock()
		return "플레이 창이 실행 중입니다", nil
	}
	a.mu.Unlock()
	e, ok := a.songs.Find(selected.ChartHash)
	if !ok {
		return "", fmt.Errorf("선택된 채보를 가지고 있지 않습니다")
	}
	s := Selection{Folder: e.Folder, ChartID: e.Path, Index: e.Index, Mode: "live", Mod: selected.Mods.Kind, Visibility: selected.Mods.Visibility, Rate: selected.Rate, Pitch: &pitch}
	if s.Mod == "random" || s.Mod == "mirror+random" {
		seed := selected.Mods.Seed
		s.Seed = &seed
	}
	if err := l.client.Send(multi.Request{Type: "ticket", ID: id, Revision: revision}); err != nil {
		return "", err
	}
	timer := time.NewTimer(5 * time.Second)
	defer timer.Stop()
	select {
	case e := <-ch:
		if e.Type == "error" {
			return "", fmt.Errorf("게임 연결: %s", e.Error)
		}
		return a.play(s, []string{"-multi", l.endpoint, "-room", roomID, "-multi-token", e.Token, "-multi-revision", fmt.Sprint(revision)})
	case <-l.done:
		return "", fmt.Errorf("멀티 연결이 종료됐습니다")
	case <-timer.C:
		return "", fmt.Errorf("게임 연결 준비 시간이 초과됐습니다")
	}
}
