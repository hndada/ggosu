package multi

import (
	"fmt"
	"ggosu/internal/multi"
	"slices"
	"sort"
	"strings"
	"time"

	"ggosu/internal/piano"
)

type room struct {
	password *password
	multi.Room
	members  map[string]*multi.Member
	order    []string
	chat     []multi.Message
	nextChat uint64
}

// All room mutations and snapshots run under Server.mu.
func (r *room) snapshot() multi.Room {
	v := r.Room
	v.Members = make([]multi.Member, 0, len(r.order))
	for _, id := range r.order {
		v.Members = append(v.Members, *r.members[id])
	}
	if v.Selection != nil {
		s := *v.Selection
		s.Mods.Mapping = slices.Clone(s.Mods.Mapping)
		v.Selection = &s
	}
	v.Proposals = slices.Clone(r.Proposals)
	for i := range v.Proposals {
		v.Proposals[i].Selection.Mods.Mapping = slices.Clone(v.Proposals[i].Selection.Mods.Mapping)
	}
	return v
}

func (r *room) connected() int {
	n := 0
	for _, m := range r.members {
		if m.Connected {
			n++
		}
	}
	return n
}

func (r *room) remove(id string) {
	delete(r.members, id)
	r.order = slices.DeleteFunc(r.order, func(v string) bool { return v == id })
}

func (r *room) complete() {
	if r.Phase != "playing" {
		return
	}
	for _, m := range r.members {
		if m.Status == "playing" {
			return
		}
	}
	r.Phase = "finished"
}

func (s *Server) list() []multi.Summary {
	out := make([]multi.Summary, 0, len(s.rooms))
	for _, r := range s.rooms {
		v := multi.Summary{Locked: r.Locked, ID: r.ID, Name: r.Name, Phase: r.Phase, Players: r.connected(), Capacity: r.Capacity}
		if r.Selection != nil {
			v.Title = r.Selection.Title
		}
		out = append(out, v)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

func (s *Server) dispatch(c *client, q multi.Request, receivedNS int64, verified *piano.Score, access admission) error {
	if len(q.ID) > 64 {
		return fmt.Errorf("request ID too long")
	}
	if q.Type == "clock" {
		if q.SentNS < 0 {
			return fmt.Errorf("negative client clock timestamp")
		}
		s.send(c, multi.Event{Type: "clock", ID: q.ID, Clock: &multi.Stamp{ClientSentNS: q.SentNS, ServerRecvNS: receivedNS}})
		return nil
	}
	if q.Type == "attach" {
		return s.attach(c, q)
	}
	if c.owner != "" {
		switch q.Type {
		case "ready", "armed", "progress", "finish", "abort", "chat":
		default:
			return fmt.Errorf("game connection cannot control the lobby")
		}
	}
	if q.Type == "hello" {
		if c.room != "" || !multi.ValidText(q.Name, 32) {
			return fmt.Errorf("choose a name of 1-32 characters before joining a room")
		}
		c.name = strings.TrimSpace(q.Name)
		return nil
	}
	if c.name == "" {
		return fmt.Errorf("send hello with a name first")
	}
	switch q.Type {
	case "list":
		s.send(c, multi.Event{Type: "rooms", ID: q.ID, Rooms: s.list()})
		return nil
	case "create":
		if c.room != "" || !multi.ValidText(q.Name, 80) || q.Capacity < 2 || q.Capacity > 8 {
			return fmt.Errorf("leave your room first; room name and capacity 2-8 are required")
		}
		if len(s.rooms) >= s.config.MaxRooms {
			return fmt.Errorf("room limit reached")
		}
		id, err := newID()
		if err != nil {
			return err
		}
		r := &room{password: access.password, Room: multi.Room{Locked: access.password != nil, ID: id, Name: strings.TrimSpace(q.Name), HostID: c.id, Capacity: q.Capacity, Phase: "lobby", VerifyResults: len(s.charts) > 0}, members: map[string]*multi.Member{}}
		s.rooms[id] = r
		s.join(c, r)
		s.broadcast(r)
		s.chatHistory(c, r)
		return nil
	case "join":
		r := s.rooms[q.RoomID]
		if c.room != "" || r == nil || r.connected() >= r.Capacity {
			return fmt.Errorf("room unavailable, full or already playing; leave your room first")
		}
		if access.room != r {
			return fmt.Errorf("방이 변경됐습니다. 다시 입장해 주세요")
		}
		s.join(c, r)
		s.broadcast(r)
		s.chatHistory(c, r)
		return nil
	case "leave":
		if c.room == "" {
			return fmt.Errorf("not in a room")
		}
		s.leave(c)
		return nil
	}
	r := s.rooms[c.room]
	if r == nil {
		return fmt.Errorf("join a room first")
	}
	m := r.members[c.id]
	if m == nil || !m.Connected || c.owner != "" && (s.clients[c.owner] == nil || s.clients[c.owner].game != c || s.clients[c.owner].room != r.ID) {
		return fmt.Errorf("room membership ended")
	}
	switch q.Type {
	case "ticket":
		if c.owner != "" || c.game != nil || r.Selection == nil || q.Revision != r.Revision || !m.HasChart || m.Participant && r.Phase == "playing" {
			return fmt.Errorf("game attachment unavailable for this selection")
		}
		token, err := newID()
		if err != nil {
			return err
		}
		c.ticket, c.ticketUntil = token, s.tick()+30*time.Second
		c.ticketRevision = r.Revision
		s.send(c, multi.Event{Type: "ticket", ID: q.ID, Token: token})
		return nil
	case "chat":
		return s.chat(c, r, q)
	case "propose":
		if r.Phase != "lobby" || q.Selection == nil {
			return fmt.Errorf("propose a chart in the lobby")
		}
		if err := q.Selection.Validate(); err != nil {
			return err
		}
		if err := s.checkSelection(*q.Selection); err != nil {
			return err
		}
		v := *q.Selection
		v.Mods.Mapping = slices.Clone(v.Mods.Mapping)
		r.Proposals = slices.DeleteFunc(r.Proposals, func(p multi.Proposal) bool { return p.PlayerID == c.id })
		r.Proposals = append(r.Proposals, multi.Proposal{PlayerID: c.id, Name: c.name, Selection: v})
	case "select":
		if c.id != r.HostID || r.Phase != "lobby" || q.Selection == nil {
			return fmt.Errorf("only the host can select a chart in the lobby")
		}
		if err := q.Selection.Validate(); err != nil {
			return err
		}
		if err := s.checkSelection(*q.Selection); err != nil {
			return err
		}
		v := *q.Selection
		v.Mods.Mapping = slices.Clone(v.Mods.Mapping)
		r.Selection = &v
		r.Proposals = nil
		r.Revision++
		for _, p := range r.members {
			p.Ready, p.HasChart = false, false
			if owner := s.clients[p.ID]; owner != nil {
				owner.ticket = ""
			}
		}
	case "have":
		if r.Phase != "lobby" || r.Selection == nil || q.Revision != r.Revision || q.ChartHash != r.Selection.ChartHash {
			return fmt.Errorf("acknowledge chart possession for the current revision")
		}
		m.HasChart = q.HasChart
		if !m.HasChart {
			m.Ready = false
		}
	case "ready":
		if (r.Phase != "lobby" && r.Phase != "preparing") || r.Selection == nil || q.Revision != r.Revision || q.ChartHash != r.Selection.ChartHash || r.Phase == "preparing" && !m.Participant {
			return fmt.Errorf("load and acknowledge the current selection revision and chart hash")
		}
		if q.OffsetNS < -int64(2*time.Second) || q.OffsetNS > int64(2*time.Second) {
			return fmt.Errorf("invalid judgement offset")
		}
		m.Ready = q.Ready
		m.HasChart = true
		m.OffsetNS = q.OffsetNS
		if r.Phase == "preparing" && r.loaded() {
			s.arm(r)
		}
	case "start":
		if c.id != r.HostID || r.Phase != "lobby" || r.Selection == nil || r.connected() < 2 {
			return fmt.Errorf("only the host can start; at least two players and a selection are required")
		}
		count := 0
		for _, p := range r.members {
			if p.Connected && p.HasChart {
				count++
			}
		}
		if count == 0 {
			return fmt.Errorf("at least one chart holder is required")
		}
		id, err := newID()
		if err != nil {
			return err
		}
		r.MatchID, r.Phase, r.Notice = id, "preparing", ""
		r.ArmByNS = int64(s.tick() + 30*time.Second)
		for _, p := range r.members {
			p.Participant = p.Connected && p.HasChart
			p.Status, p.Armed = "waiting", false
			p.Verified = false
			p.Sequence, p.PositionNS = 0, r.Selection.BeginNS
			p.ScoreAtNS = r.Selection.BeginNS - p.OffsetNS
			p.Score = piano.Score{Possible: r.Selection.Judgements, Flow: 50, Precision: 20}
		}
		if r.loaded() {
			s.arm(r)
		}
	case "armed":
		if !m.Participant || r.Phase != "arming" || q.MatchID != r.MatchID || q.StartNS != r.StartNS || int64(s.tick()) >= r.ArmByNS {
			return fmt.Errorf("start reservation expired or does not match")
		}
		m.Armed = true
		all := true
		for _, p := range r.members {
			all = all && (!p.Participant || p.Armed)
		}
		if all {
			r.Phase = "playing"
			for _, p := range r.members {
				if p.Participant {
					p.Status = "playing"
				}
			}
		}
	case "progress", "finish":
		if r.Phase != "playing" || q.MatchID != r.MatchID || m.Status != "playing" {
			return fmt.Errorf("not an active participant in this match")
		}
		now := int64(s.tick())
		if now < r.StartNS || now >= r.EndNS {
			return fmt.Errorf("outside match reporting window")
		}
		if q.Sequence <= m.Sequence || q.Sequence > 1_000_000_000 || q.PositionNS < m.PositionNS || q.PositionNS > r.Selection.DurationNS {
			return fmt.Errorf("stale sequence or invalid game position")
		}
		maxAt := r.Selection.BeginNS + int64(float64(now-r.StartNS+int64(250*time.Millisecond))*r.Selection.Rate)
		if q.PositionNS > maxAt {
			return fmt.Errorf("game position is ahead of the server timeline")
		}
		if q.ScoreAtNS != q.PositionNS-m.OffsetNS || q.ScoreAtNS < m.ScoreAtNS {
			return fmt.Errorf("score timestamp must use the original chart timeline")
		}
		if q.Score == nil {
			return fmt.Errorf("score required")
		}
		if err := q.Score.Validate(); err != nil {
			return err
		}
		a, b := m.Score, *q.Score
		if b.Possible != r.Selection.Judgements || b.Total < a.Total || b.Kool < a.Kool || b.Cool < a.Cool || b.Good < a.Good || b.Miss < a.Miss || b.Earned < a.Earned || b.Points < a.Points || b.MaxCombo < a.MaxCombo {
			return fmt.Errorf("score must match the chart and retain previous judgements")
		}
		if q.Type == "finish" && (b.Total != b.Possible || q.PositionNS != r.Selection.DurationNS) {
			return fmt.Errorf("finish requires all judgements and the chart end position")
		}
		if q.Type == "finish" && r.VerifyResults {
			if verified == nil || *verified != b {
				return fmt.Errorf("verified finish required")
			}
			m.Verified = true
		}
		m.Sequence, m.PositionNS, m.Score = q.Sequence, q.PositionNS, b
		m.ScoreAtNS = q.ScoreAtNS
		if q.Type == "finish" {
			m.Status = "finished"
			r.complete()
		}
	case "abort", "wait":
		if r.Phase != "playing" || q.MatchID != r.MatchID || m.Status != "playing" {
			return fmt.Errorf("not an active participant in this match")
		}
		m.Status = "aborted"
		m.Ready = false
		r.complete()
	case "cancel":
		if c.id != r.HostID || (r.Phase != "preparing" && r.Phase != "arming") {
			return fmt.Errorf("only the host can cancel preparation")
		}
		s.reset(r, "start canceled by host")
	case "reset":
		if c.id != r.HostID || r.Phase != "finished" {
			return fmt.Errorf("only the host can return a finished match to the lobby")
		}
		for _, p := range r.members {
			if owner := s.clients[p.ID]; owner != nil && owner.game != nil {
				return fmt.Errorf("wait for game processes to return to the lobby")
			}
		}
		s.reset(r, "")
	case "kick":
		if c.id != r.HostID || r.Phase != "lobby" || q.PlayerID == c.id || r.members[q.PlayerID] == nil {
			return fmt.Errorf("only the host can remove another player in the lobby")
		}
		p := s.clients[q.PlayerID]
		s.leave(p)
		s.send(p, multi.Event{Type: "error", Error: "removed from room by host"})
		return nil
	default:
		return fmt.Errorf("unknown request type")
	}
	s.broadcast(r)
	return nil
}

func (s *Server) join(c *client, r *room) {
	c.room = r.ID
	r.members[c.id] = &multi.Member{ID: c.id, Name: c.name, Connected: true, Status: "waiting"}
	r.order = append(r.order, c.id)
}

func (s *Server) leave(c *client) {
	if c.owner != "" {
		s.detach(c)
		return
	}
	r := s.rooms[c.room]
	c.ticket = ""
	if c.game != nil {
		c.game.close()
		c.game = nil
	}
	c.room = ""
	if r == nil {
		return
	}
	if (r.Phase == "arming" || r.Phase == "preparing") && r.members[c.id].Participant {
		s.reset(r, "start canceled: participant left before confirmation")
	}
	if r.Phase == "lobby" {
		r.remove(c.id)
	} else {
		p := r.members[c.id]
		p.Connected, p.Ready = false, false
		if p.Status == "playing" {
			p.Status = "left"
		}
		r.complete()
	}
	if r.connected() == 0 {
		delete(s.rooms, r.ID)
		return
	}
	if r.HostID == c.id {
		for _, id := range r.order {
			if r.members[id].Connected {
				r.HostID = id
				break
			}
		}
	}
	s.broadcast(r)
}

func (s *Server) expire() {
	for _, r := range s.rooms {
		if (r.Phase == "arming" || r.Phase == "preparing") && int64(s.tick()) >= r.ArmByNS {
			s.reset(r, "start canceled: confirmation timed out")
			s.broadcast(r)
			continue
		}
		if r.Phase != "playing" || int64(s.tick()) < r.EndNS {
			continue
		}
		for _, p := range r.members {
			if p.Status == "playing" {
				p.Status = "timeout"
			}
		}
		r.complete()
		s.broadcast(r)
	}
}

func (s *Server) reset(r *room, notice string) {
	r.Phase, r.MatchID, r.Notice = "lobby", "", notice
	r.StartAt, r.Deadline = time.Time{}, time.Time{}
	r.StartNS, r.ArmByNS, r.EndNS = 0, 0, 0
	r.Revision++
	for _, id := range slices.Clone(r.order) {
		p := r.members[id]
		if owner := s.clients[id]; owner != nil {
			owner.ticket = ""
		}
		if !p.Connected {
			r.remove(id)
			continue
		}
		*p = multi.Member{ID: p.ID, Name: p.Name, Connected: true, HasChart: p.HasChart, Status: "waiting"}
	}
}
