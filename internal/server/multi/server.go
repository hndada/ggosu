package multi

import (
	"bytes"
	"crypto/rand"
	"embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"ggosu/internal/multi"
	"io"
	"io/fs"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/gorilla/websocket"

	"ggosu/internal/chart"
	"ggosu/internal/clock"
	"ggosu/internal/piano"
	"ggosu/scenes"
)

//go:embed static/*
var assets embed.FS

type Config struct {
	MaxClients  int
	MaxRooms    int
	StartDelay  time.Duration
	FinishGrace time.Duration
	// Origins are exact browser origins, e.g. https://gosu.example.com.
	// Empty allows same-host HTTP/HTTPS origins and native clients without Origin.
	Origins []string
	// Charts are the server's canonical catalogue. Nil keeps results provisional.
	// With a catalogue, unknown selections and finishes without replays are rejected.
	Charts []*chart.Chart
}

type Server struct {
	passwords  chan struct{}
	mu         sync.Mutex
	config     Config
	clients    map[string]*client
	rooms      map[string]*room
	pending    int
	closed     bool
	stop       chan struct{}
	done       chan struct{}
	wg         sync.WaitGroup
	mux        *http.ServeMux
	now        func() time.Time
	tick       func() time.Duration
	charts     map[string]*chart.Chart
	selections map[string]multi.Selection
}

type client struct {
	id, name, room string
	conn           *websocket.Conn
	out            chan multi.Event
	stop           chan struct{}
	once           sync.Once
	window         time.Time
	requests       int
	chatAt         time.Duration
	chatCount      int
	owner          string // game connection attached to a persistent lobby member
	game           *client
	ticket         string
	ticketUntil    time.Duration
	ticketRevision uint64
}

// NewServer starts a timeout sweep. The owner must call Close; HTTP shutdown
// alone does not close upgraded WebSocket connections.
func NewServer(config Config) (*Server, error) {
	if config.MaxClients == 0 {
		config.MaxClients = 256
	}
	if config.MaxRooms == 0 {
		config.MaxRooms = 128
	}
	if config.StartDelay == 0 {
		config.StartDelay = 3 * time.Second
	}
	if config.FinishGrace == 0 {
		config.FinishGrace = 30 * time.Second
	}
	if config.MaxClients < 2 || config.MaxClients > 4096 || config.MaxRooms < 1 || config.MaxRooms > 4096 || config.StartDelay < 0 || config.StartDelay > 30*time.Second || config.FinishGrace < 0 || config.FinishGrace > 5*time.Minute {
		return nil, fmt.Errorf("multi: invalid limits or timing")
	}
	config.Origins = append([]string(nil), config.Origins...)
	for _, origin := range config.Origins {
		u, err := url.Parse(origin)
		if err != nil || u.Host == "" || u.Scheme != "http" && u.Scheme != "https" || u.User != nil || u.Path != "" || u.RawQuery != "" || u.Fragment != "" {
			return nil, fmt.Errorf("multi: invalid browser origin %q", origin)
		}
	}
	counter, err := clock.New()
	if err != nil {
		return nil, fmt.Errorf("multi: create server clock: %w", err)
	}
	s := &Server{passwords: make(chan struct{}, 4), config: config, clients: map[string]*client{}, rooms: map[string]*room{}, stop: make(chan struct{}), done: make(chan struct{}), now: time.Now, tick: counter.Now}
	if err := s.loadCharts(config.Charts); err != nil {
		return nil, err
	}
	s.config.Charts = nil
	s.mux = http.NewServeMux()
	s.mux.HandleFunc("GET /ws", s.connect)
	s.mux.HandleFunc("GET /health", s.health)
	s.mux.HandleFunc("GET /api/rooms", s.catalog)
	s.mux.Handle("GET /audio/", http.FileServer(http.FS(scenes.Assets)))
	s.mux.Handle("GET /ui/", http.FileServer(http.FS(scenes.Assets)))
	s.mux.Handle("GET /preferences/", http.FileServer(http.FS(scenes.Assets)))
	for route, name := range map[string]string{"/{$}": "index.html", "/client.js": "client.js", "/lobby/chat.js": "lobby/chat.js", "/lobby/chat.css": "lobby/chat.css", "/theme.css": "theme.css", "/settings/chat.js": "settings/chat.js", "/preferences/chat.js": "preferences/chat.js"} {
		s.mux.HandleFunc("GET "+route, func(w http.ResponseWriter, r *http.Request) {
			var b []byte
			var err error
			if name == "index.html" || name == "client.js" {
				b, err = assets.ReadFile("static/" + name)
			} else {
				b, err = fs.ReadFile(scenes.Assets, name)
			}
			if err != nil {
				http.Error(w, "missing test client", http.StatusInternalServerError)
				return
			}
			mime := "text/html; charset=utf-8"
			if strings.HasSuffix(name, ".js") {
				mime = "text/javascript; charset=utf-8"
			} else if strings.HasSuffix(name, ".css") {
				mime = "text/css; charset=utf-8"
			}
			w.Header().Set("Content-Type", mime)
			if _, err := w.Write(b); err != nil {
				return
			}
		})
	}
	go s.sweep()
	return s, nil
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Security-Policy", "default-src 'self'; connect-src 'self'; frame-ancestors 'none'; base-uri 'none'")
	s.mux.ServeHTTP(w, r)
}

func (s *Server) health(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	closed := s.closed
	s.mu.Unlock()
	if closed {
		http.Error(w, "shutting down", http.StatusServiceUnavailable)
		return
	}
	writeJSON(w, struct {
		Version int    `json:"version"`
		Status  string `json:"status"`
	}{multi.Version, "ok"})
}

func (s *Server) catalog(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	rooms := s.list()
	s.mu.Unlock()
	writeJSON(w, rooms)
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	if err := json.NewEncoder(w).Encode(v); err != nil {
		return
	}
}

func newID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", fmt.Errorf("multi: generate ID: %w", err)
	}
	return hex.EncodeToString(b[:]), nil
}

func (s *Server) origin(r *http.Request) bool {
	values := r.Header.Values("Origin")
	if len(values) == 0 {
		return true // Native clients; identity is still scoped to this connection.
	}
	if len(values) != 1 {
		return false
	}
	origin := values[0]
	if len(s.config.Origins) > 0 {
		for _, allowed := range s.config.Origins {
			if origin == allowed {
				return true
			}
		}
		return false
	}
	u, err := url.Parse(origin)
	return err == nil && (u.Scheme == "http" || u.Scheme == "https") && u.User == nil && u.Path == "" && u.RawQuery == "" && u.Fragment == "" && strings.EqualFold(u.Host, r.Host)
}

func (s *Server) connect(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	if s.closed || len(s.clients)+s.pending >= s.config.MaxClients {
		s.mu.Unlock()
		http.Error(w, "server unavailable or full", http.StatusServiceUnavailable)
		return
	}
	s.pending++
	s.wg.Add(1)
	s.mu.Unlock()
	reserved := true
	defer s.wg.Done()
	defer func() {
		s.mu.Lock()
		if reserved {
			s.pending--
		}
		s.mu.Unlock()
	}()
	id, err := newID()
	if err != nil {
		http.Error(w, "cannot create connection", http.StatusInternalServerError)
		return
	}
	u := websocket.Upgrader{HandshakeTimeout: 5 * time.Second, ReadBufferSize: 4096, WriteBufferSize: 4096, CheckOrigin: s.origin}
	conn, err := u.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	c := &client{id: id, conn: conn, out: make(chan multi.Event, 64), stop: make(chan struct{})}
	s.mu.Lock()
	s.pending--
	reserved = false
	if s.closed {
		s.mu.Unlock()
		c.close()
		return
	}
	s.clients[id] = c
	s.send(c, multi.Event{Type: "welcome", Version: multi.Version, PlayerID: id})
	s.mu.Unlock()
	writerDone := make(chan struct{})
	go func() {
		defer close(writerDone)
		s.write(c)
	}()
	s.read(c)
	c.close()
	<-writerDone
	s.mu.Lock()
	s.leave(c)
	delete(s.clients, id)
	s.mu.Unlock()
}

func (c *client) close() {
	c.once.Do(func() {
		close(c.stop)
		// Close is allowed concurrently with the single reader/writer.
		if err := c.conn.Close(); err != nil {
			return
		}
	})
}

// send never waits on a peer while holding the room lock. A saturated queue
// disconnects the slow peer rather than silently losing authoritative events.
func (s *Server) send(c *client, e multi.Event) {
	e.ServerTime = s.now().UTC()
	select {
	case <-c.stop:
		return
	default:
	}
	select {
	case c.out <- e:
	default:
		c.close()
	}
}

func (s *Server) broadcast(r *room) {
	v := r.snapshot()
	for _, id := range r.order {
		if c := s.clients[id]; c != nil && c.room == r.ID {
			s.send(c, multi.Event{Type: "room", Room: &v})
			if c.game != nil {
				s.send(c.game, multi.Event{Type: "room", Room: &v})
			}
		}
	}
}

func (s *Server) read(c *client) {
	c.conn.SetReadLimit(16 << 10)
	if len(s.charts) > 0 {
		c.conn.SetReadLimit(32 << 20)
	}
	if err := c.conn.SetReadDeadline(time.Now().Add(60 * time.Second)); err != nil {
		return
	}
	c.conn.SetPongHandler(func(string) error { return c.conn.SetReadDeadline(time.Now().Add(60 * time.Second)) })
	for {
		kind, b, err := c.conn.ReadMessage()
		received := s.tick()
		if err != nil {
			return
		}
		now := time.Now()
		if now.Sub(c.window) >= time.Second {
			c.window, c.requests = now, 0
		}
		c.requests++
		if c.requests > 40 {
			return
		}
		if kind != websocket.TextMessage || !utf8.Valid(b) {
			return
		}
		var q multi.Request
		d := json.NewDecoder(bytes.NewReader(b))
		d.DisallowUnknownFields()
		err = d.Decode(&q)
		if err == nil {
			var extra any
			if d.Decode(&extra) != io.EOF {
				err = fmt.Errorf("expected one JSON request")
			}
		}
		var verified *piano.Score
		var access admission
		if err == nil && len(b) > 16<<10 && q.Type != "finish" {
			err = fmt.Errorf("only finish proofs may exceed 16 KiB")
		}
		if err == nil && q.Type == "finish" && len(s.charts) > 0 {
			verified, err = s.verifyFinish(c, q)
		}
		if err == nil && (q.Type == "create" || q.Type == "join") {
			access, err = s.checkPassword(c, q)
		}
		q.Password = ""
		s.mu.Lock()
		if s.closed {
			s.mu.Unlock()
			return
		}
		s.expire()
		if err == nil {
			err = s.dispatch(c, q, int64(received), verified, access)
		}
		if err != nil {
			s.send(c, multi.Event{Type: "error", ID: q.ID, Error: err.Error()})
			if (q.Type == "finish" || q.Type == "progress") && s.rooms[c.room] != nil {
				v := s.rooms[c.room].snapshot()
				s.send(c, multi.Event{Type: "room", Room: &v})
			}
		} else if q.Type != "clock" && q.Type != "list" {
			s.send(c, multi.Event{Type: "ack", ID: q.ID})
		}
		s.mu.Unlock()
	}
}

func (s *Server) write(c *client) {
	ping := time.NewTicker(20 * time.Second)
	defer ping.Stop()
	defer c.close()
	for {
		select {
		case <-c.stop:
			return
		case e := <-c.out:
			e.ServerNS = int64(s.tick())
			if e.Clock != nil {
				e.Clock.ServerSentNS = e.ServerNS
			}
			if err := c.conn.SetWriteDeadline(time.Now().Add(5 * time.Second)); err != nil {
				return
			}
			if err := c.conn.WriteJSON(e); err != nil {
				return
			}
		case <-ping.C:
			if err := c.conn.WriteControl(websocket.PingMessage, nil, time.Now().Add(5*time.Second)); err != nil {
				return
			}
		}
	}
}

func (s *Server) sweep() {
	defer close(s.done)
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-s.stop:
			return
		case <-ticker.C:
			s.mu.Lock()
			s.expire()
			s.mu.Unlock()
		}
	}
}

func (s *Server) Close() error {
	s.mu.Lock()
	if !s.closed {
		s.closed = true
		close(s.stop)
		for _, c := range s.clients {
			c.close()
		}
	}
	s.mu.Unlock()
	<-s.done
	s.wg.Wait()
	return nil
}
