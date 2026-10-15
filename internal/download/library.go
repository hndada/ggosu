package download

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"
)

type Result struct {
	Source  string   `json:"source"`
	ID      string   `json:"id"`
	Title   string   `json:"title"`
	Charts  int      `json:"charts"`
	Hashes  []string `json:"hashes"`
	Already bool     `json:"already"`
}

type State struct {
	Phase    string  `json:"phase"` // idle, download, extract, verify, done, failed, cancelled
	Title    string  `json:"title"`
	Received int64   `json:"received"`
	Total    int64   `json:"total"`
	Files    int     `json:"files"`
	Result   *Result `json:"result,omitempty"`
	Error    string  `json:"error,omitempty"`
}

type Info struct {
	Archive      string `json:"archive"`
	CanInstall   bool   `json:"canInstall"`
	CanConfigure bool   `json:"canConfigure"`
}

// Library owns one cancellable installation; Close cancels and joins it.
// Search and status reads do not hold its lock during network requests.
type Library struct {
	mu                    sync.Mutex
	root, archive, direct string
	client                *http.Client
	offers                map[string]offer
	state                 State
	busy, closed          bool
	cancel                context.CancelFunc
	done                  chan struct{}
}

func New(root, archive string) (*Library, error) {
	if archive == "" {
		archive = DefaultArchive
	}
	if err := ValidateServer(archive); err != nil {
		return nil, err
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	return &Library{root: abs, archive: strings.TrimRight(archive, "/"), direct: DefaultDirect, client: &http.Client{Timeout: 0}, offers: map[string]offer{}, state: State{Phase: "idle"}}, nil
}

func (l *Library) Archive() string { l.mu.Lock(); defer l.mu.Unlock(); return l.archive }

func (l *Library) SetArchive(server string) error {
	if server == "" {
		server = DefaultArchive
	}
	if err := ValidateServer(server); err != nil {
		return err
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.busy || l.closed {
		return fmt.Errorf("설치 종료 후 주소를 변경해 주세요")
	}
	l.archive = strings.TrimRight(server, "/")
	l.offers = map[string]offer{}
	return nil
}

func (l *Library) State() State {
	l.mu.Lock()
	defer l.mu.Unlock()
	s := l.state
	if s.Result != nil {
		r := *s.Result
		r.Hashes = slices.Clone(r.Hashes)
		s.Result = &r
	}
	return s
}

func (l *Library) Start(key string) (State, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	o, ok := l.offers[key]
	if !ok {
		return l.state, fmt.Errorf("검색 결과에서 곡을 다시 선택해 주세요")
	}
	if !o.Available {
		return l.state, fmt.Errorf("%s", o.Reason)
	}
	if l.closed || l.busy {
		return l.state, fmt.Errorf("진행 중인 설치를 먼저 마쳐 주세요")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	done := make(chan struct{})
	l.cancel, l.done, l.busy = cancel, done, true
	l.state = State{Phase: "download", Title: o.Title, Total: o.Size}
	go func() {
		defer close(done)
		defer cancel()
		r, err := l.install(ctx, o)
		l.mu.Lock()
		defer l.mu.Unlock()
		l.busy = false
		if err == nil {
			l.state.Phase, l.state.Result = "done", &r
		} else {
			l.state.Phase, l.state.Error = "failed", err.Error()
			if errors.Is(err, context.Canceled) {
				l.state.Phase, l.state.Error = "cancelled", "설치를 취소했습니다"
			}
		}
	}()
	return l.state, nil
}

func (l *Library) Cancel() {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.cancel != nil {
		l.cancel()
	}
}

func (l *Library) Close() {
	l.mu.Lock()
	l.closed = true
	if l.cancel != nil {
		l.cancel()
	}
	done := l.done
	l.mu.Unlock()
	if done != nil {
		<-done
	}
}

func (l *Library) update(phase string, received, total int64, files int) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.state.Phase, l.state.Received, l.state.Total, l.state.Files = phase, received, total, files
}

func (l *Library) installed(source, id string) bool {
	r, err := readResult(filepath.Join(l.root, source+"-"+id, ".ggosu-install.json"))
	return err == nil && r.Source == source && r.ID == id && r.Charts > 0
}

func readResult(path string) (Result, error) {
	var r Result
	f, err := os.Open(path)
	if err != nil {
		return r, err
	}
	err = json.NewDecoder(f).Decode(&r)
	return r, errors.Join(err, f.Close())
}
