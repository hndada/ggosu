package multi

import (
	"fmt"
	"ggosu/internal/multi"
	"time"
)

// A lobby owns membership. A single-use ticket lets its game report under that
// identity without creating a second member or taking ownership of the room.
func (s *Server) attach(c *client, q multi.Request) error {
	if c.room != "" || c.owner != "" || q.Token == "" {
		return fmt.Errorf("invalid game attachment")
	}
	var owner *client
	for _, p := range s.clients {
		if p.ticket == q.Token && p.owner == "" && p.room == q.RoomID && s.tick() < p.ticketUntil {
			owner = p
			break
		}
	}
	if owner == nil || owner.game != nil {
		return fmt.Errorf("game ticket expired or already used")
	}
	r := s.rooms[owner.room]
	if r == nil || r.Selection == nil || q.Revision != r.Revision || owner.ticketRevision != r.Revision {
		return fmt.Errorf("game selection changed")
	}
	owner.ticket, owner.game = "", c
	c.owner, c.id, c.name, c.room = owner.id, owner.id, owner.name, owner.room
	v := r.snapshot()
	s.send(c, multi.Event{Type: "attached", ID: q.ID, PlayerID: owner.id, Room: &v})
	s.chatHistory(c, r)
	return nil
}

func (s *Server) detach(c *client) {
	owner := s.clients[c.owner]
	if owner == nil || owner.game != c {
		return
	}
	owner.game = nil
	r := s.rooms[owner.room]
	if r == nil {
		return
	}
	m := r.members[owner.id]
	m.Ready = false
	if m.Participant && (r.Phase == "preparing" || r.Phase == "arming") {
		s.reset(r, "start canceled: game closed before confirmation")
	} else if m.Status == "playing" {
		m.Status = "aborted"
		r.complete()
	}
	s.broadcast(r)
}

func (r *room) loaded() bool {
	for _, m := range r.members {
		if m.Participant && !m.Ready {
			return false
		}
	}
	return true
}

func (s *Server) arm(r *room) {
	r.Phase = "arming"
	r.StartAt = s.now().UTC().Add(s.config.StartDelay)
	r.StartNS = int64(s.tick() + s.config.StartDelay)
	r.ArmByNS = r.StartNS - int64(min(500*time.Millisecond, s.config.StartDelay/3))
	d := time.Duration(float64(r.Selection.DurationNS-r.Selection.BeginNS) / r.Selection.Rate)
	r.Deadline = r.StartAt.Add(d + s.config.FinishGrace)
	r.EndNS = r.StartNS + int64(d+s.config.FinishGrace)
}
