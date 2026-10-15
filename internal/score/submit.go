// Package score submits saved plays and describes their server receipts.
package score

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"ggosu/internal/piano"
	"ggosu/internal/settings"
)

type Request struct {
	Player string       `json:"player"`
	Record piano.Record `json:"record"`
}

type Receipt struct {
	ID       string    `json:"id"`
	Received time.Time `json:"received"`
	Verified bool      `json:"verified"`
}

type Status struct {
	State   string   `json:"state"` // pending, sent, failed, disabled
	Error   string   `json:"error,omitempty"`
	Receipt *Receipt `json:"receipt,omitempty"`
}

func Eligible(r piano.Record) bool {
	return r.Mode == "live" && r.Version == piano.RecordVersion && r.Replay.Rules == piano.Rules && r.Retained()
}

func ValidPlayer(name string) bool {
	if !utf8.ValidString(name) || strings.TrimSpace(name) != name || name == "" || utf8.RuneCountInString(name) > 32 {
		return false
	}
	for _, r := range name {
		if unicode.IsControl(r) {
			return false
		}
	}
	return true
}

// Submit sends only retained live plays. The caller owns retries and local storage.
func Submit(ctx context.Context, server string, q Request) (Receipt, error) {
	if err := settings.ValidateScoreServer(server); err != nil {
		return Receipt{}, err
	}
	if server == "" || !Eligible(q.Record) || !ValidPlayer(q.Player) {
		return Receipt{}, fmt.Errorf("score: server, player or play is unavailable")
	}
	b, err := json.Marshal(q)
	if err != nil {
		return Receipt{}, err
	}
	if len(b) > 32<<20 {
		return Receipt{}, fmt.Errorf("score: replay exceeds 32 MiB")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(server, "/")+"/api/scores", bytes.NewReader(b))
	if err != nil {
		return Receipt{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Gosu-Score", "1")
	client := &http.Client{Timeout: 10 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	resp, err := client.Do(req)
	if err != nil {
		return Receipt{}, fmt.Errorf("점수 제출: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		b, err := io.ReadAll(io.LimitReader(resp.Body, 4096))
		if err != nil {
			return Receipt{}, fmt.Errorf("점수 제출 응답 읽기: %w", err)
		}
		return Receipt{}, fmt.Errorf("점수 제출 HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(b)))
	}
	var receipt Receipt
	if err := json.NewDecoder(io.LimitReader(resp.Body, 64<<10)).Decode(&receipt); err != nil {
		return Receipt{}, fmt.Errorf("점수 제출 응답: %w", err)
	}
	if receipt.ID != q.Record.ID || receipt.Received.IsZero() || !receipt.Verified {
		return Receipt{}, fmt.Errorf("점수 제출 확인 정보가 기록과 다릅니다")
	}
	return receipt, nil
}
