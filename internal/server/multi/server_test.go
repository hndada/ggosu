package multi

import (
	"encoding/json"
	"fmt"
	"ggosu/internal/multi"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	"ggosu/internal/piano"
)

type fixture struct {
	t     *testing.T
	s     *Server
	http  *httptest.Server
	clock atomic.Int64
}

type peer struct {
	t    *testing.T
	conn *websocket.Conn
	id   string
	next int
	room *multi.Room
}

func newFixture(t *testing.T, config Config) *fixture {
	t.Helper()
	s, err := NewServer(config)
	if err != nil {
		t.Fatal(err)
	}
	f := &fixture{t: t, s: s}
	f.clock.Store(time.Date(2026, 10, 9, 0, 0, 0, 0, time.UTC).UnixNano())
	s.mu.Lock()
	s.now = func() time.Time { return time.Unix(0, f.clock.Load()) }
	origin := f.clock.Load()
	s.tick = func() time.Duration { return time.Duration(f.clock.Load() - origin) }
	s.mu.Unlock()
	f.http = httptest.NewServer(s)
	t.Cleanup(func() {
		if err := s.Close(); err != nil {
			t.Error(err)
		}
		f.http.Close()
	})
	return f
}

func (f *fixture) peer(name string) *peer {
	f.t.Helper()
	c, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(f.http.URL, "http")+"/ws", nil)
	if err != nil {
		f.t.Fatal(err)
	}
	p := &peer{t: f.t, conn: c}
	f.t.Cleanup(func() { _ = c.Close() })
	e := p.read()
	if e.Type != "welcome" || e.Version != multi.Version || e.PlayerID == "" {
		f.t.Fatalf("invalid welcome: %+v", e)
	}
	p.id = e.PlayerID
	p.ok(multi.Request{Type: "hello", Name: name})
	return p
}

func (p *peer) read() multi.Event {
	p.t.Helper()
	if err := p.conn.SetReadDeadline(time.Now().Add(3 * time.Second)); err != nil {
		p.t.Fatal(err)
	}
	var e multi.Event
	if err := p.conn.ReadJSON(&e); err != nil {
		p.t.Fatal(err)
	}
	if e.Room != nil {
		p.room = e.Room
	}
	return e
}

func (p *peer) request(q multi.Request) multi.Event {
	p.t.Helper()
	p.next++
	q.ID = fmt.Sprint(p.next)
	if (q.Type == "progress" || q.Type == "finish") && q.ScoreAtNS == 0 {
		q.ScoreAtNS = q.PositionNS
	}
	if err := p.conn.WriteJSON(q); err != nil {
		p.t.Fatal(err)
	}
	for {
		e := p.read()
		if e.ID == q.ID {
			return e
		}
	}
}

func (p *peer) ok(q multi.Request) multi.Event {
	p.t.Helper()
	e := p.request(q)
	if e.Type == "error" {
		p.t.Fatalf("%s failed: %s", q.Type, e.Error)
	}
	return e
}

func (p *peer) reject(q multi.Request) {
	p.t.Helper()
	if e := p.request(q); e.Type != "error" {
		p.t.Fatalf("%s accepted: %+v", q.Type, e)
	}
}

func selection() multi.Selection {
	return multi.Selection{Title: "Test", ChartHash: strings.Repeat("a", 64), Lanes: 4,
		Rules: "ggosu-keys-v3", ScoreVersion: 2, DurationNS: int64(10 * time.Second), Judgements: 10, Rate: 1}
}

func (f *fixture) match() (*peer, *peer, multi.Room) {
	f.t.Helper()
	host, guest := f.peer("host"), f.peer("guest")
	host.ok(multi.Request{Type: "create", Name: "room", Capacity: 2})
	guest.ok(multi.Request{Type: "join", RoomID: host.room.ID})
	c := selection()
	host.ok(multi.Request{Type: "select", Selection: &c})
	f.ready(host)
	f.ready(guest)
	host.ok(multi.Request{Type: "start"})
	f.arm(host)
	f.arm(guest)
	host.ok(multi.Request{Type: "clock"})
	return host, guest, *host.room
}

func (f *fixture) arm(p *peer) {
	f.t.Helper()
	f.s.mu.Lock()
	r := f.s.rooms[pRoom(f.s, p.id)]
	q := multi.Request{Type: "armed", MatchID: r.MatchID, StartNS: r.StartNS}
	f.s.mu.Unlock()
	p.ok(q)
}

func (f *fixture) ready(p *peer) {
	f.t.Helper()
	f.s.mu.Lock()
	r := f.s.rooms[pRoom(f.s, p.id)]
	q := multi.Request{Type: "ready", Ready: true, Revision: r.Revision, ChartHash: r.Selection.ChartHash}
	f.s.mu.Unlock()
	p.ok(q)
}

func pRoom(s *Server, id string) string { return s.clients[id].room }

func TestMatchLifecycle(t *testing.T) {
	f := newFixture(t, Config{})
	host, guest, match := f.match()
	if match.Phase != "playing" || match.MatchID == "" || len(match.Members) != 2 || match.StartAt.Sub(time.Unix(0, f.clock.Load())) != 3*time.Second {
		t.Fatalf("invalid start: %+v", match)
	}
	late := f.peer("late")
	late.reject(multi.Request{Type: "join", RoomID: match.ID})
	guest.reject(multi.Request{Type: "start"})
	host.reject(multi.Request{Type: "start"})
	q := multi.Request{Type: "progress", MatchID: match.MatchID, Sequence: 1, Score: kool(0)}
	host.reject(q) // Countdown: no report accepted yet.
	f.clock.Store(match.StartAt.Add(5 * time.Second).UnixNano())
	q.PositionNS, q.Score = int64(5*time.Second), kool(5)
	q.MatchID = "old-match"
	host.reject(q)
	q.MatchID = match.MatchID
	q.PositionNS = int64(9 * time.Second)
	host.reject(q) // A structurally valid score cannot advance the clock.
	q.PositionNS = int64(5 * time.Second)
	q.Score.Points++
	host.reject(q)
	q.Score = kool(5)
	host.ok(q)
	host.reject(q) // Duplicate sequence.
	q.Sequence, q.PositionNS = 2, int64(4*time.Second)
	host.reject(q)
	q.PositionNS, q.Score = int64(5*time.Second), kool(4)
	host.reject(q) // Judgements and earned units cannot go backwards.
	q.Score = kool(5)
	q.Type = "finish"
	host.reject(q) // Partial play is not a completed result.
	f.clock.Store(match.StartAt.Add(10 * time.Second).UnixNano())
	q.PositionNS, q.Score = int64(10*time.Second), kool(10)
	host.ok(q)
	if host.room.Phase != "playing" || host.room.Members[0].Status != "finished" {
		t.Fatalf("first finish lost: %+v", host.room)
	}
	q.Sequence = 1
	guest.ok(q)
	if guest.room.Phase != "finished" || guest.room.Members[1].Score.Points != piano.MaxScore {
		t.Fatalf("results lost: %+v", guest.room)
	}
	guest.reject(multi.Request{Type: "reset"})
	host.ok(multi.Request{Type: "reset"})
	if host.room.Phase != "lobby" || host.room.MatchID != "" || host.room.Members[0].Ready || host.room.Members[0].Score.Total != 0 {
		t.Fatalf("reset failed: %+v", host.room)
	}
	f.ready(host)
	f.ready(guest)
	host.ok(multi.Request{Type: "start"})
	f.arm(host)
	f.arm(guest)
	if host.room.MatchID == match.MatchID {
		t.Fatal("match ID reused")
	}
	f.clock.Store(host.room.StartAt.Add(10 * time.Second).UnixNano())
	host.reject(q) // A delayed result from the previous round.
}

func TestSelectionReadinessAndAuthority(t *testing.T) {
	f := newFixture(t, Config{MaxRooms: 1})
	host, guest, extra := f.peer("host"), f.peer("guest"), f.peer("extra")
	host.ok(multi.Request{Type: "create", Name: "room", Capacity: 2})
	host.reject(multi.Request{Type: "start"})
	extra.reject(multi.Request{Type: "create", Name: "other", Capacity: 2})
	guest.ok(multi.Request{Type: "join", RoomID: host.room.ID})
	extra.reject(multi.Request{Type: "join", RoomID: host.room.ID})
	c := selection()
	guest.reject(multi.Request{Type: "select", Selection: &c})
	host.ok(multi.Request{Type: "select", Selection: &c})
	host.reject(multi.Request{Type: "start"})
	f.ready(host)
	guest.reject(multi.Request{Type: "ready", Ready: true, Revision: 1, ChartHash: strings.Repeat("b", 64)})
	f.ready(guest)
	host.ok(multi.Request{Type: "select", Selection: &c})
	if host.room.Revision != 2 || host.room.Members[0].Ready || host.room.Members[1].Ready {
		t.Fatal("new selection did not invalidate readiness")
	}
	guest.reject(multi.Request{Type: "ready", Ready: true, Revision: 1, ChartHash: c.ChartHash})
	guest.reject(multi.Request{Type: "kick", PlayerID: host.id})
	host.reject(multi.Request{Type: "kick", PlayerID: host.id})
	host.ok(multi.Request{Type: "kick", PlayerID: guest.id})
	extra.ok(multi.Request{Type: "join", RoomID: host.room.ID})
	extra.ok(multi.Request{Type: "leave"})
	host.ok(multi.Request{Type: "leave"})
	if e := extra.ok(multi.Request{Type: "list"}); len(e.Rooms) != 0 {
		t.Fatalf("empty room leaked: %+v", e.Rooms)
	}
}

func TestDepartureAndHostMigration(t *testing.T) {
	f := newFixture(t, Config{})
	host, guest, match := f.match()
	host.ok(multi.Request{Type: "leave"})
	e := guest.ok(multi.Request{Type: "clock"}) // Drain the room event before the clock reply.
	if e.Type != "clock" || guest.room.HostID != guest.id || guest.room.Members[0].Status != "left" || guest.room.Members[0].Connected {
		t.Fatalf("departure/migration failed: %+v", guest.room)
	}
	guest.ok(multi.Request{Type: "abort", MatchID: match.MatchID})
	if guest.room.Phase != "finished" {
		t.Fatal("departed player prevented completion")
	}
	guest.ok(multi.Request{Type: "reset"})
	if len(guest.room.Members) != 1 || guest.room.Members[0].ID != guest.id {
		t.Fatal("departed participant not removed on reset")
	}
	host.ok(multi.Request{Type: "join", RoomID: match.ID})
	if err := guest.conn.Close(); err != nil {
		t.Fatal(err)
	}
	for {
		e := host.read()
		if e.Room != nil && e.Room.HostID == host.id {
			break
		}
	}
	if len(host.room.Members) != 1 {
		t.Fatalf("disconnected lobby member remained: %+v", host.room)
	}
}

func TestTimeoutAndClock(t *testing.T) {
	f := newFixture(t, Config{})
	host, guest, match := f.match()
	f.clock.Store(match.Deadline.UnixNano())
	e := host.ok(multi.Request{Type: "clock"})
	if !e.ServerTime.Equal(match.Deadline) || host.room.Phase != "finished" || host.room.Members[0].Status != "timeout" {
		t.Fatalf("expiry/clock failed: %+v %+v", e, host.room)
	}
	guest.reject(multi.Request{Type: "finish", MatchID: match.MatchID, Sequence: 1, PositionNS: int64(10 * time.Second), Score: kool(10)})
	// The background sweep also expires a match with no subsequent requests.
	host.ok(multi.Request{Type: "reset"})
	f.ready(host)
	f.ready(guest)
	host.ok(multi.Request{Type: "start"})
	f.arm(host)
	f.arm(guest)
	f.clock.Store(host.room.Deadline.UnixNano())
	for {
		e := guest.read()
		if e.Room != nil && e.Room.Phase == "finished" {
			break
		}
	}
}

func TestRateAndNegativeLeadIn(t *testing.T) {
	f := newFixture(t, Config{})
	a, b := f.peer("a"), f.peer("b")
	a.ok(multi.Request{Type: "create", Name: "room", Capacity: 2})
	b.ok(multi.Request{Type: "join", RoomID: a.room.ID})
	c := selection()
	c.BeginNS, c.Rate = -int64(2*time.Second), 2
	a.ok(multi.Request{Type: "select", Selection: &c})
	f.ready(a)
	f.ready(b)
	a.ok(multi.Request{Type: "start"})
	f.arm(a)
	f.arm(b)
	a.ok(multi.Request{Type: "clock"})
	r := *a.room
	if r.Deadline.Sub(r.StartAt) != 36*time.Second {
		t.Fatalf("bad rate/lead-in deadline: %s", r.Deadline.Sub(r.StartAt))
	}
	f.clock.Store(r.StartAt.Add(time.Second).UnixNano())
	a.ok(multi.Request{Type: "progress", MatchID: r.MatchID, Sequence: 1, PositionNS: 0, Score: kool(0)})
	a.reject(multi.Request{Type: "progress", MatchID: r.MatchID, Sequence: 2, PositionNS: int64(time.Second), Score: kool(0)})
}

func TestSelectionRateBounds(t *testing.T) {
	s := selection()
	for _, rate := range []float64{.5, 1, 2} {
		s.Rate = rate
		if err := s.Validate(); err != nil {
			t.Fatalf("rate %g: %v", rate, err)
		}
	}
	for _, rate := range []float64{.25, .49, 2.01, 4, math.NaN(), math.Inf(1)} {
		s.Rate = rate
		if err := s.Validate(); err == nil {
			t.Fatalf("accepted rate %g", rate)
		}
	}
}

func TestOriginLimitsAndShutdown(t *testing.T) {
	f := newFixture(t, Config{MaxClients: 2})
	endpoint := "ws" + strings.TrimPrefix(f.http.URL, "http") + "/ws"
	_, response, err := websocket.DefaultDialer.Dial(endpoint, http.Header{"Origin": {"https://unrelated.example"}})
	if err == nil || response == nil || response.StatusCode != http.StatusForbidden {
		t.Fatalf("expected origin rejection; response=%v error=%v", response, err)
	}
	a, b := f.peer("a"), f.peer("b")
	_, response, err = websocket.DefaultDialer.Dial(endpoint, nil)
	if err == nil || response == nil || response.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("expected capacity rejection; response=%v error=%v", response, err)
	}
	a.ok(multi.Request{Type: "create", Name: "room", Capacity: 2})
	b.ok(multi.Request{Type: "join", RoomID: a.room.ID})
	if err := f.s.Close(); err != nil {
		t.Fatal(err)
	}
	f.s.mu.Lock()
	if len(f.s.clients) != 0 || len(f.s.rooms) != 0 || f.s.pending != 0 {
		t.Errorf("shutdown leaked state: clients=%d rooms=%d pending=%d", len(f.s.clients), len(f.s.rooms), f.s.pending)
	}
	f.s.mu.Unlock()
	response, err = http.Get(f.http.URL + "/health")
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusServiceUnavailable {
		t.Fatal("closed server reports healthy")
	}
}

func TestSameOriginAndExplicitOrigin(t *testing.T) {
	for _, config := range []Config{{}, {Origins: []string{"https://gosu.example"}}} {
		f := newFixture(t, config)
		origin := f.http.URL
		if len(config.Origins) > 0 {
			origin = config.Origins[0]
		}
		c, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(f.http.URL, "http")+"/ws", http.Header{"Origin": {origin}})
		if err != nil {
			t.Fatal(err)
		}
		if err := c.Close(); err != nil {
			t.Fatal(err)
		}
	}
}

func TestMalformedAndFlood(t *testing.T) {
	f := newFixture(t, Config{})
	p := f.peer("p")
	for _, raw := range []string{`{"type":"clock","unknown":1}`, `{"type":"clock"} {"type":"start"}`, `{`, `null`} {
		if err := p.conn.WriteMessage(websocket.TextMessage, []byte(raw)); err != nil {
			t.Fatal(err)
		}
		if e := p.read(); e.Type != "error" {
			t.Fatalf("malformed request accepted: %s", raw)
		}
	}
	// Oversized input closes the connection instead of allocating an unbounded frame.
	if err := p.conn.WriteMessage(websocket.TextMessage, []byte(strings.Repeat("x", 17<<10))); err != nil {
		t.Fatal(err)
	}
	if err := p.conn.SetReadDeadline(time.Now().Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	if _, _, err := p.conn.ReadMessage(); err == nil {
		t.Fatal("oversized message accepted")
	}
	p = f.peer("flood")
	for range 50 {
		if err := p.conn.WriteJSON(multi.Request{Type: "clock"}); err != nil {
			break // The server may have already disconnected the sender.
		}
	}
	if err := p.conn.SetReadDeadline(time.Now().Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	n := 0
	for {
		if _, _, err := p.conn.ReadMessage(); err != nil {
			break
		}
		n++
	}
	if n > 40 {
		t.Fatalf("flood limit not enforced: %d", n)
	}
}

func TestSelectionValidation(t *testing.T) {
	mutations := []func(*multi.Selection){
		func(c *multi.Selection) { c.ChartHash = strings.Repeat("z", 64) },
		func(c *multi.Selection) { c.ChartHash = strings.Repeat("A", 64) },
		func(c *multi.Selection) { c.Rules = "other" },
		func(c *multi.Selection) { c.ScoreVersion = 1 },
		func(c *multi.Selection) { c.Lanes = 19 },
		func(c *multi.Selection) { c.Rate = 0 },
		func(c *multi.Selection) { c.Title = "bad\nname" },
		func(c *multi.Selection) { c.DurationNS = int64(time.Hour) },
		func(c *multi.Selection) { c.Judgements = 0 },
		func(c *multi.Selection) { c.Mods.Kind = "random"; c.Mods.Mapping = []int{0, 0, 2, 3} },
	}
	for i, mutate := range mutations {
		c := selection()
		mutate(&c)
		if err := c.Validate(); err == nil {
			t.Errorf("invalid selection %d accepted", i)
		}
	}
}

func TestHTTPAndConfig(t *testing.T) {
	for _, c := range []Config{{MaxClients: -1}, {MaxRooms: -1}, {StartDelay: -1}, {FinishGrace: -1}, {Origins: []string{"https://example/path"}}} {
		if s, err := NewServer(c); err == nil {
			_ = s.Close()
			t.Fatalf("invalid config accepted: %+v", c)
		}
	}
	f := newFixture(t, Config{})
	for _, path := range []string{"/", "/client.js", "/health", "/api/rooms"} {
		r, err := http.Get(f.http.URL + path)
		if err != nil {
			t.Fatal(err)
		}
		if r.StatusCode != http.StatusOK || r.Header.Get("Cache-Control") != "no-store" {
			t.Errorf("bad response %s: %d", path, r.StatusCode)
		}
		if path == "/api/rooms" {
			var rooms []multi.Summary
			if err := json.NewDecoder(r.Body).Decode(&rooms); err != nil {
				t.Error(err)
			}
		}
		if err := r.Body.Close(); err != nil {
			t.Fatal(err)
		}
	}
}

func TestConcurrentRoomsAndClose(t *testing.T) {
	f := newFixture(t, Config{})
	var wg sync.WaitGroup
	for range 6 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			a, b, r := f.match()
			a.ok(multi.Request{Type: "abort", MatchID: r.MatchID})
			b.ok(multi.Request{Type: "abort", MatchID: r.MatchID})
			if b.room.Phase != "finished" {
				t.Error("concurrent room did not finish")
			}
		}()
	}
	wg.Wait()
	f.s.mu.Lock()
	if len(f.s.rooms) != 6 || len(f.s.clients) != 12 {
		t.Errorf("room isolation failed: rooms=%d clients=%d", len(f.s.rooms), len(f.s.clients))
	}
	f.s.mu.Unlock()
	for range 4 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := f.s.Close(); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	f.s.mu.Lock()
	defer f.s.mu.Unlock()
	if len(f.s.clients) != 0 || len(f.s.rooms) != 0 || f.s.pending != 0 {
		t.Error("concurrent Close leaked rooms or connections")
	}
}

func TestUnconfirmedStartIsCanceled(t *testing.T) {
	f := newFixture(t, Config{})
	a, b := f.peer("host"), f.peer("guest")
	a.ok(multi.Request{Type: "create", Name: "room", Capacity: 2})
	b.ok(multi.Request{Type: "join", RoomID: a.room.ID})
	c := selection()
	a.ok(multi.Request{Type: "select", Selection: &c})
	f.ready(a)
	f.ready(b)
	a.ok(multi.Request{Type: "start"})
	r := *a.room
	if r.Phase != "arming" {
		t.Fatal("game committed before any client confirmed")
	}
	a.reject(multi.Request{Type: "progress", MatchID: r.MatchID, Sequence: 1, Score: kool(0)})
	b.reject(multi.Request{Type: "armed", MatchID: "old", StartNS: r.StartNS})
	f.arm(a)
	f.clock.Store(r.StartAt.Add(-500 * time.Millisecond).UnixNano())
	b.reject(multi.Request{Type: "armed", MatchID: r.MatchID, StartNS: r.StartNS})
	if b.room.Phase != "lobby" || b.room.Notice == "" || b.room.MatchID != "" || b.room.Members[0].Ready {
		t.Fatalf("unconfirmed match was not canceled: %+v", b.room)
	}
}

func TestClockWireTimestampsAndScoreTime(t *testing.T) {
	f := newFixture(t, Config{})
	a, b, r := f.match()
	e := a.ok(multi.Request{Type: "clock", SentNS: 123})
	if e.Clock == nil || e.Clock.ClientSentNS != 123 || e.Clock.ServerSentNS < e.Clock.ServerRecvNS {
		t.Fatalf("incomplete clock exchange: %+v", e)
	}
	f.clock.Store(r.StartAt.Add(time.Second).UnixNano())
	a.reject(multi.Request{Type: "progress", MatchID: r.MatchID, Sequence: 1, PositionNS: int64(time.Second), ScoreAtNS: -1, Score: kool(1)})
	b.ok(multi.Request{Type: "abort", MatchID: r.MatchID})
}

func kool(n int) *piano.Score {
	return &piano.Score{Kool: n, Total: n, Possible: 10, Combo: n, MaxCombo: n,
		Earned: int64(n) * 2200, Points: n * 110000, Flow: 50, Precision: 20}
}
