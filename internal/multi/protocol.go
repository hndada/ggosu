// Package multi defines the multiplayer protocol and shared play conditions,
// connects clients, and estimates server clock offsets. Judgement stays in the
// game; reported scores are provisional, not proof of an authentic play.
package multi

import (
	"encoding/hex"
	"fmt"
	"math"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"ggosu/internal/chart"
	"ggosu/internal/piano"
)

const Version = 6

const PasswordLimit = 64

// ValidPassword preserves spaces and case. Empty means a public room.
func ValidPassword(password string) bool {
	if !utf8.ValidString(password) || utf8.RuneCountInString(password) > PasswordLimit {
		return false
	}
	for _, r := range password {
		if unicode.IsControl(r) {
			return false
		}
	}
	return true
}

// Selection fixes the shared game conditions. Times are game-1x nanoseconds,
// including any negative lead-in. DurationNS is the end, including audio outro.
type Selection struct {
	Title        string    `json:"title"`
	ChartHash    string    `json:"chart_hash"`
	Lanes        int       `json:"lanes"`
	Rules        string    `json:"rules"`
	ScoreVersion int       `json:"score_version"`
	BeginNS      int64     `json:"begin_ns"`
	DurationNS   int64     `json:"duration_ns"`
	Judgements   int       `json:"judgements"`
	Rate         float64   `json:"rate"`
	Pitch        bool      `json:"pitch"`
	Mods         chart.Mod `json:"mods"`
}

func (s Selection) Validate() error {
	if !ValidText(s.Title, 160) || len(s.ChartHash) != 64 || s.ChartHash != strings.ToLower(s.ChartHash) {
		return fmt.Errorf("invalid chart title or SHA-256 hash")
	}
	if _, err := hex.DecodeString(s.ChartHash); err != nil {
		return fmt.Errorf("invalid chart hash")
	}
	if s.Lanes < 1 || s.Lanes > 18 || s.Rules != piano.Rules || s.ScoreVersion != piano.RecordVersion {
		return fmt.Errorf("unsupported lanes, judgement rules or score version")
	}
	if s.BeginNS > 0 || s.BeginNS < -int64(30*time.Second) || s.DurationNS <= 0 || s.DurationNS > int64(30*time.Minute) || s.Judgements < 1 || s.Judgements > 1_000_000 {
		return fmt.Errorf("invalid chart duration or judgement count")
	}
	if math.IsNaN(s.Rate) || math.IsInf(s.Rate, 0) || s.Rate < .5 || s.Rate > 2 {
		return fmt.Errorf("rate must be between 0.5 and 2.0")
	}
	return s.Mods.Validate(s.Lanes)
}

// ValidText reports whether s is printable, non-blank text of at most limit runes.
func ValidText(s string, limit int) bool {
	if !utf8.ValidString(s) || strings.TrimSpace(s) == "" || utf8.RuneCountInString(s) > limit {
		return false
	}
	for _, r := range s {
		if unicode.IsControl(r) {
			return false
		}
	}
	return true
}

// Request is one JSON text message. ID is an optional correlation value.
// Ready acknowledges Revision and ChartHash after loading the entire Selection.
type Request struct {
	Password   string        `json:"password,omitempty"` // Used only for create/join; never echoed in events.
	Type       string        `json:"type"`
	ID         string        `json:"id,omitempty"`
	Name       string        `json:"name,omitempty"`
	RoomID     string        `json:"room_id,omitempty"`
	PlayerID   string        `json:"player_id,omitempty"`
	Capacity   int           `json:"capacity,omitempty"`
	Selection  *Selection    `json:"selection,omitempty"`
	Revision   uint64        `json:"revision,omitempty"`
	ChartHash  string        `json:"chart_hash,omitempty"`
	Ready      bool          `json:"ready,omitempty"`
	MatchID    string        `json:"match_id,omitempty"`
	Sequence   uint64        `json:"sequence,omitempty"`
	PositionNS int64         `json:"position_ns,omitempty"`
	Score      *piano.Score  `json:"score,omitempty"`
	SentNS     int64         `json:"sent_ns,omitempty"`
	StartNS    int64         `json:"start_ns,omitempty"`
	OffsetNS   int64         `json:"offset_ns,omitempty"`
	ScoreAtNS  int64         `json:"score_at_ns,omitempty"`
	Text       string        `json:"text,omitempty"`
	Token      string        `json:"token,omitempty"`
	HasChart   bool          `json:"has_chart,omitempty"`
	Replay     *piano.Replay `json:"replay,omitempty"` // Finish proof when the room requests verification.
}

type Member struct {
	ID          string      `json:"id"`
	Name        string      `json:"name"`
	Connected   bool        `json:"connected"`
	Ready       bool        `json:"ready"`
	HasChart    bool        `json:"has_chart"`
	Participant bool        `json:"participant"`
	Status      string      `json:"status"` // waiting, playing, finished, aborted, left, timeout
	Sequence    uint64      `json:"sequence"`
	PositionNS  int64       `json:"position_ns"`
	Score       piano.Score `json:"score"`
	ScoreAtNS   int64       `json:"score_at_ns"`
	OffsetNS    int64       `json:"offset_ns"`
	Armed       bool        `json:"armed"`
	Verified    bool        `json:"verified"` // Consistent with inputs and server chart, not proof of human input.
}

type Room struct {
	Locked        bool       `json:"locked"`
	ID            string     `json:"id"`
	Name          string     `json:"name"`
	HostID        string     `json:"host_id"`
	Capacity      int        `json:"capacity"`
	Phase         string     `json:"phase"` // lobby, preparing, arming, playing (includes countdown), finished
	Revision      uint64     `json:"revision"`
	Selection     *Selection `json:"selection,omitempty"`
	MatchID       string     `json:"match_id,omitempty"`
	StartAt       time.Time  `json:"start_at,omitzero"`
	Deadline      time.Time  `json:"deadline,omitzero"`
	StartNS       int64      `json:"start_ns"`
	ArmByNS       int64      `json:"arm_by_ns"`
	EndNS         int64      `json:"end_ns"`
	Notice        string     `json:"notice,omitempty"`
	VerifyResults bool       `json:"verify_results"`
	Members       []Member   `json:"members"`
	Proposals     []Proposal `json:"proposals,omitempty"`
}

type Proposal struct {
	PlayerID  string    `json:"player_id"`
	Name      string    `json:"name"`
	Selection Selection `json:"selection"`
}

type Summary struct {
	Locked   bool   `json:"locked"`
	ID       string `json:"id"`
	Name     string `json:"name"`
	Phase    string `json:"phase"`
	Players  int    `json:"players"`
	Capacity int    `json:"capacity"`
	Title    string `json:"title,omitempty"`
}

type Event struct {
	Type       string    `json:"type"` // welcome, ack, error, room, rooms, clock
	ID         string    `json:"id,omitempty"`
	Version    int       `json:"version,omitempty"`
	PlayerID   string    `json:"player_id,omitempty"`
	ServerTime time.Time `json:"server_time"`
	Error      string    `json:"error,omitempty"`
	Room       *Room     `json:"room,omitempty"`
	Rooms      []Summary `json:"rooms,omitempty"`
	Clock      *Stamp    `json:"clock,omitempty"`
	ServerNS   int64     `json:"server_ns"`
	Message    *Message  `json:"message,omitempty"`
	Messages   []Message `json:"messages,omitempty"`
	RoomID     string    `json:"room_id,omitempty"`
	Token      string    `json:"token,omitempty"`
}

// Stamp contains the first three of the four synchronization timestamps.
// ServerSentNS is sampled by the socket writer, after any queueing delay.
type Stamp struct {
	ClientSentNS int64 `json:"client_sent_ns"`
	ServerRecvNS int64 `json:"server_recv_ns"`
	ServerSentNS int64 `json:"server_sent_ns"`
}
