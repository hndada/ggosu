//go:build !js

package play

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"strings"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/exp/textinput"

	"ggosu/internal/multi"
	"ggosu/internal/piano"
	"ggosu/internal/settings"
)

type network struct {
	password                        string
	client                          *multi.Client
	clock                           multi.Clock
	history                         history
	selection                       multi.Selection
	room                            *multi.Room
	id, name, join, roomName        string
	armed, finalID, report          string
	autoStart, created, ready, sent bool
	localStart, nextProbe, nextSend time.Duration
	finalAt, offset, firstTick      time.Duration
	sequence                        uint64
	estimate                        multi.Estimate
	trace                           networkTrace
	shown                           map[string]uint64
	rejected                        int
	token                           string
	revision                        uint64
	chat                            chatState
	chatInput                       textinput.Composer
	chatFocus, chatHandled          bool
	chatComposition, chatStatus     string
	chatSeq                         uint64
	chatPending, chatSent           string
}

type comparison struct {
	PlayerID string        `json:"player_id"`
	At       time.Duration `json:"at_ns"`
	Local    int           `json:"local_points"`
	Remote   int           `json:"remote_points"`
	Diff     int           `json:"difference"`
}

type networkTrace struct {
	PlayerID    string        `json:"player_id"`
	RoomID      string        `json:"room_id"`
	MatchID     string        `json:"match_id"`
	ServerStart int64         `json:"server_start_ns"`
	LocalStart  time.Duration `json:"local_start_ns"`
	FirstTick   time.Duration `json:"first_tick_ns"`
	StartLag    time.Duration `json:"start_lag_ns"`
	RTT         time.Duration `json:"rtt_ns"`
	Uncertainty time.Duration `json:"uncertainty_ns"`
	Drift       time.Duration `json:"estimated_drift_ns"`
	FinalAck    bool          `json:"final_ack"`
	Points      []point       `json:"points"`
	Comparisons []comparison  `json:"comparisons"`
}

func (g *scene) connect(endpoint, name, roomID, roomName, report, token string, revision uint64, autoStart bool) error {
	if !strings.HasPrefix(endpoint, "ws://") && !strings.HasPrefix(endpoint, "wss://") {
		return fmt.Errorf("multiplayer endpoint must use ws:// or wss://")
	}
	if g.mode == "replay" {
		return fmt.Errorf("saved replay playback is not a multiplayer participant")
	}
	n := &network{password: g.roomPassword, name: name, join: roomID, roomName: roomName, autoStart: autoStart, report: report, offset: g.offsets.Total(), shown: map[string]uint64{}}
	n.token, n.revision = token, revision
	if g.bank != nil {
		if err := g.bank.PrepareMusic(g.chart.Audio); err != nil {
			return err
		}
	}
	var err error
	n.selection, err = multi.Describe(g.chart, g.mods, g.speed, g.chart.AudioDuration(nil))
	if err != nil {
		return err
	}
	n.selection.Pitch = g.pitch
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	n.client, err = multi.Dial(ctx, endpoint, g.clock.Now)
	if err != nil {
		return err
	}
	g.network = n
	g.initChat()
	begin := time.Duration(n.selection.BeginNS)
	g.preroll, g.last, g.position = -begin, begin, begin
	return nil
}

func (g *scene) pumpNetwork() error {
	n := g.network
	if n == nil {
		return nil
	}
	select {
	case err := <-n.client.Errors():
		return err
	default:
	}
	for {
		select {
		case e := <-n.client.Events():
			switch e.Type {
			case "welcome":
				if e.Version != multi.Version {
					return fmt.Errorf("multiplayer protocol version mismatch")
				}
				n.id = e.PlayerID
				if n.token != "" {
					if err := n.client.Send(multi.Request{Type: "attach", Token: n.token, RoomID: n.join, Revision: n.revision}); err != nil {
						return err
					}
					continue
				}
				if err := n.client.Send(multi.Request{Type: "hello", Name: n.name}); err != nil {
					return err
				}
			case "attached":
				n.id, n.created, n.room = e.PlayerID, true, e.Room
				if n.room == nil {
					return fmt.Errorf("multiplayer: missing attached room")
				}
				n.chat.SetRoom(n.room.ID)
			case "chat":
				if e.Message != nil {
					if n.chat.Receive(*e.Message, n.id, n.name, g.chatPrefs, g.chatPlaying(), g.clock.Now()) {
						g.chatSound()
					}
				}
			case "chat_history":
				if e.RoomID == n.chat.RoomID {
					n.chat.Messages = append([]multi.Message(nil), e.Messages...)
				}
			case "clock":
				if e.Clock != nil {
					if err := n.clock.Observe(*e.Clock, e.At); err != nil {
						if n.rejected < 3 {
							fmt.Fprintln(os.Stderr, "discarded sync sample:", err)
						}
						n.rejected++
					}
				}
			case "error":
				if strings.HasPrefix(e.ID, "chat-") {
					n.chatPending = ""
					n.chatStatus = e.Error
					continue
				}
				return fmt.Errorf("multiplayer: %s", e.Error)
			case "ack":
				if e.ID == n.chatPending && n.chatPending != "" {
					if n.chatComposition == "" && strings.TrimSpace(n.chat.Draft) == n.chatSent {
						n.chatInput.Cancel()
						n.chat.Draft = ""
					}
					n.chatPending, n.chatStatus = "", ""
				}
				if e.ID == n.finalID && n.finalID != "" {
					n.trace.FinalAck = true
				}
			case "room":
				if e.Room == nil {
					return fmt.Errorf("multiplayer: missing room state")
				}
				if g.started && e.Room.MatchID != n.trace.MatchID {
					return fmt.Errorf("multiplayer: match canceled or reset")
				}
				n.room = e.Room
				n.chat.SetRoom(e.Room.ID)
				for _, m := range e.Room.Members {
					if m.ID == n.id && m.Status == "aborted" {
						g.completed, g.finishing = false, true
						n.finalID, n.trace.FinalAck = "lobby-wait", true
					}
				}
				if !g.started && n.room.Phase == "lobby" && n.room.Notice != "" {
					return fmt.Errorf("multiplayer: %s", n.room.Notice)
				}
			}
		default:
			return nil
		}
	}
}

func (g *scene) networkUpdate(now time.Duration) (bool, error) {
	n := g.network
	if n == nil {
		return false, nil
	}
	if err := g.pumpNetwork(); err != nil {
		return true, err
	}
	if g.finishing {
		return true, nil
	}
	if n.id != "" && now >= n.nextProbe {
		if err := n.client.Send(multi.Request{Type: "clock"}); err != nil {
			return true, err
		}
		n.nextProbe = now + 150*time.Millisecond
		if _, ok := n.clock.Estimate(now); ok {
			n.nextProbe = now + 5*time.Second
		}
	}
	if !g.announced || n.id == "" {
		g.networkWait("Loading multiplayer")
		return true, nil
	}
	if !n.created && n.token != "" {
		return true, nil
	}
	if !n.created {
		q := multi.Request{Type: "join", RoomID: n.join, Password: n.password}
		if n.join == "" {
			q = multi.Request{Type: "create", Name: n.roomName, Capacity: 8, Password: n.password}
		}
		n.created = true
		n.password, g.roomPassword = "", ""
		return true, n.client.Send(q)
	}
	r := n.room
	if r == nil {
		g.networkWait("Joining room")
		return true, nil
	}
	if r.Selection == nil {
		g.networkWait("Waiting for chart selection")
		if r.HostID == n.id && !n.sent {
			n.sent = true
			return true, n.client.Send(multi.Request{Type: "select", Selection: &n.selection})
		}
		return true, nil
	}
	want, got := n.selection, *r.Selection
	want.Title = got.Title // Titles are descriptive; the digest is authoritative.
	if !reflect.DeepEqual(want, got) {
		return true, fmt.Errorf("multiplayer: chart, duration, rate or mods differ from the room selection")
	}
	if est, ok := n.clock.Estimate(now); ok {
		n.estimate = est
		if g.started {
			n.trace.Drift = est.Offset - time.Duration(n.trace.ServerStart) + n.localStart
		}
	} else if !g.started {
		if n.rejected >= 10 {
			return true, fmt.Errorf("multiplayer: unable to establish a valid clock estimate")
		}
		g.networkWait("Synchronizing clocks")
		return true, nil
	}
	if r.Phase == "lobby" || r.Phase == "preparing" {
		g.networkWait(fmt.Sprintf("Room %s / F5 Start (%d players)", r.ID, len(r.Members)))
		if !n.ready {
			if n.estimate.Uncertainty > 100*time.Millisecond {
				return true, fmt.Errorf("multiplayer: clock uncertainty too large")
			}
			n.ready = true
			fmt.Println("multi room:", r.ID)
			return true, n.client.Send(multi.Request{Type: "ready", Ready: true, Revision: r.Revision, ChartHash: got.ChartHash, OffsetNS: int64(n.offset)})
		}
		all := len(r.Members) >= 2
		for _, m := range r.Members {
			all = all && m.Ready
		}
		if n.token == "" && r.Phase == "lobby" && r.HostID == n.id && all && (g.pressed(ebiten.KeyF5) || n.autoStart) {
			n.autoStart = false
			return true, n.client.Send(multi.Request{Type: "start"})
		}
		return true, nil
	}
	if r.Phase == "arming" && n.armed != r.MatchID {
		at, err := n.estimate.LocalStart(r.StartNS, now)
		if err != nil {
			return true, err
		}
		n.localStart, n.armed = at, r.MatchID
		n.trace = networkTrace{PlayerID: n.id, RoomID: r.ID, MatchID: r.MatchID, ServerStart: r.StartNS,
			LocalStart: at, RTT: n.estimate.RTT, Uncertainty: n.estimate.Uncertainty}
		return true, n.client.Send(multi.Request{Type: "armed", MatchID: r.MatchID, StartNS: r.StartNS})
	}
	if r.Phase == "arming" || r.Phase == "playing" && now < n.localStart {
		g.networkWait(fmt.Sprintf("Reserved start in %.2f s", max(time.Duration(0), n.localStart-now).Seconds()))
		return true, nil
	}
	if !g.started {
		if n.armed != r.MatchID || r.Phase != "playing" || now-n.localStart > 100*time.Millisecond {
			return true, fmt.Errorf("multiplayer: start commitment arrived too late")
		}
		n.firstTick = now
		n.trace.FirstTick, n.trace.StartLag = now, now-n.localStart
		n.history.Reset(time.Duration(got.BeginNS)-n.offset, g.session.Score)
		g.session.OnScore = func(at time.Duration, score piano.Score) {
			if err := n.history.Append(at-n.offset, score); err != nil {
				g.err = err
			}
		}
	}
	if g.started && g.player == nil && !ebiten.IsFocused() && g.runFor == 0 && g.limit == 0 {
		g.completed, g.finishing = false, true
		return true, nil
	}
	return false, nil
}

func (g *scene) networkWait(message string) {
	if g.view != nil {
		s := viewState{Position: -g.preroll, Rate: g.speed, Scroll: g.scroll * settings.PixelsPerScroll, Online: message}
		g.chatState(&s)
		g.view.Update(s)
	}
	// Waiting inputs must not leak into the first frame of the match.
	g.events, _ = g.box.Drain(g.events)
}

func (g *scene) gameStart(now time.Duration) time.Duration {
	if g.network != nil {
		return g.network.localStart
	}
	return now
}

func (g *scene) networkAdvance() error {
	if g.err != nil {
		return g.err
	}
	n := g.network
	if n == nil {
		return nil
	}
	if err := n.history.Advance(g.position - n.offset); err != nil {
		return err
	}
	if g.clock.Now() < n.nextSend || n.finalID != "" {
		return nil
	}
	n.nextSend = g.clock.Now() + 100*time.Millisecond
	n.sequence++
	score := g.session.Score
	return n.client.Send(multi.Request{Type: "progress", MatchID: n.trace.MatchID, Sequence: n.sequence,
		PositionNS: int64(g.position), ScoreAtNS: int64(g.position - n.offset), Score: &score})
}

func (g *scene) networkState(s *viewState) {
	n := g.network
	if n == nil || n.room == nil {
		return
	}
	s.Online = fmt.Sprintf("MULTI / %s / RTT %.1f ms", n.room.Phase, n.estimate.RTT.Seconds()*1000)
	g.chatState(s)
	for _, m := range n.room.Members {
		if m.ID == n.id || !m.Participant {
			continue
		}
		at := time.Duration(m.ScoreAtNS)
		own, ok := n.history.At(at)
		s.Rivals = append(s.Rivals, rival{Name: m.Name, At: at, Points: m.Score.Points,
			Difference: own.Points - m.Score.Points, Available: ok, Status: m.Status})
		if ok && n.shown[m.ID] != m.Sequence {
			n.shown[m.ID] = m.Sequence
			if len(n.trace.Comparisons) < 10000 {
				n.trace.Comparisons = append(n.trace.Comparisons, comparison{PlayerID: m.ID, At: at, Local: own.Points, Remote: m.Score.Points, Diff: own.Points - m.Score.Points})
			}
		}
	}
}

func (g *scene) networkEnabled() bool { return g.network != nil }

func (g *scene) networkPosition(at time.Duration) time.Duration {
	if g.network != nil {
		return min(at, time.Duration(g.network.selection.DurationNS))
	}
	return at
}

func (g *scene) networkFinishReady() bool {
	return g.network == nil || g.position >= time.Duration(g.network.selection.DurationNS)
}

func (g *scene) networkFinished() (bool, error) {
	n := g.network
	if n == nil {
		return true, nil
	}
	now := g.clock.Now()
	if err := g.pumpNetwork(); err != nil {
		return false, err
	}
	if n.finalID == "" {
		if g.completed {
			if err := n.history.Finish(time.Duration(n.selection.DurationNS) + 2*time.Second); err != nil {
				return false, err
			}
		}
		n.finalID, n.finalAt = "final", now
		n.sequence++
		q := multi.Request{Type: "abort", ID: n.finalID, MatchID: n.trace.MatchID}
		if g.completed {
			score := g.session.Score
			q.Type, q.Sequence, q.PositionNS, q.ScoreAtNS, q.Score = "finish", n.sequence,
				n.selection.DurationNS, n.selection.DurationNS-int64(n.offset), &score
			if n.room.VerifyResults {
				g.recording.Duration = time.Duration(n.selection.DurationNS)
				if err := g.release(g.recording.Duration); err != nil {
					return false, err
				}
				q.Replay = &g.recording
				score = g.session.Score
			}
		}
		return false, n.client.Send(q)
	}
	if g.view != nil {
		s := viewState{Position: g.position, Rate: g.speed, Scroll: g.scroll * settings.PixelsPerScroll, Finished: g.completed}
		g.networkState(&s)
		g.view.Update(s)
	}
	if !n.trace.FinalAck && now-n.finalAt > 3*time.Second {
		return false, fmt.Errorf("multiplayer: final result was not acknowledged")
	}
	return n.trace.FinalAck, nil
}

func (g *scene) saveNetwork() error {
	n := g.network
	if n == nil || n.report == "" {
		return nil
	}
	n.trace.Points = n.history.Points()
	return writeFile(n.report, func(f *os.File) error { return json.NewEncoder(f).Encode(n.trace) })
}
