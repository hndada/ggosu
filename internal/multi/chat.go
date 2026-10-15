package multi

import "time"

const MaxChatRunes = 300

// Message identity is assigned by the server, scoped to a room.
type Message struct {
	ID       uint64    `json:"id"`
	RoomID   string    `json:"room_id"`
	PlayerID string    `json:"player_id"`
	Name     string    `json:"name"`
	Time     time.Time `json:"time"`
	Text     string    `json:"text"`
}
