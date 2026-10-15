package multi

import (
	"crypto/pbkdf2"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"fmt"

	"ggosu/internal/multi"
)

const passwordIterations = 600000

type password struct {
	salt [16]byte
	sum  [32]byte
}

// admission is checked outside the room lock, then matched to the same room
// during dispatch. A deleted room or changed membership cannot reuse it.
type admission struct {
	room     *room
	password *password
}

func (s *Server) checkPassword(c *client, q multi.Request) (admission, error) {
	if !multi.ValidPassword(q.Password) {
		return admission{}, fmt.Errorf("비밀번호는 제어 문자를 제외한 %d자까지 사용할 수 있습니다", multi.PasswordLimit)
	}
	s.mu.Lock()
	if q.Type == "create" {
		valid := c.name != "" && c.room == "" && c.owner == "" && multi.ValidText(q.Name, 80) && q.Capacity >= 2 && q.Capacity <= 8 && len(s.rooms) < s.config.MaxRooms
		s.mu.Unlock()
		if q.Password == "" {
			return admission{}, nil
		}
		if !valid {
			return admission{}, fmt.Errorf("room unavailable; leave your room and choose a valid name and capacity")
		}
		p := &password{}
		if _, err := rand.Read(p.salt[:]); err != nil {
			return admission{}, fmt.Errorf("create room password: %w", err)
		}
		key, err := s.passwordKey(c, q.Password, p.salt[:])
		if err != nil {
			return admission{}, err
		}
		copy(p.sum[:], key)
		return admission{password: p}, nil
	}
	r := s.rooms[q.RoomID]
	if r == nil || c.name == "" || c.room != "" || c.owner != "" || r.connected() >= r.Capacity {
		s.mu.Unlock()
		return admission{}, nil
	}
	p := r.password
	s.mu.Unlock()
	if p == nil {
		return admission{room: r}, nil
	}
	if q.Password == "" {
		return admission{}, fmt.Errorf("비밀번호가 필요하거나 일치하지 않습니다")
	}
	key, err := s.passwordKey(c, q.Password, p.salt[:])
	if err != nil {
		return admission{}, err
	}
	if subtle.ConstantTimeCompare(p.sum[:], key) != 1 {
		return admission{}, fmt.Errorf("비밀번호가 필요하거나 일치하지 않습니다")
	}
	return admission{room: r}, nil
}

// Bound expensive derivations without holding Server.mu. Game ticks, chat,
// room lists and shutdown remain independent of password work.
func (s *Server) passwordKey(c *client, value string, salt []byte) ([]byte, error) {
	select {
	case s.passwords <- struct{}{}:
	case <-s.stop:
		return nil, fmt.Errorf("server closed")
	case <-c.stop:
		return nil, fmt.Errorf("connection closed")
	}
	defer func() { <-s.passwords }()
	return pbkdf2.Key(sha256.New, value, salt, passwordIterations, 32)
}
