package piano

import (
	"fmt"
	"math"
	"sort"
	"time"

	"ggosu/internal/chart"
)

const Rules = "ggosu-keys-v3"

// StrictReleaseRules judged hold releases with the head windows.
const StrictReleaseRules = "ggosu-keys-v2"
const LegacyRules = "ggosu-keys-v1"

// Known reports judgement rules this build can replay.
func Known(rules string) bool {
	return rules == Rules || rules == StrictReleaseRules || rules == LegacyRules
}

const Window = 135 * time.Millisecond
const RollGap = 200 * time.Millisecond

type Grade string

const (
	Kool Grade = "kool"
	Cool Grade = "cool"
	Good Grade = "good"
	Miss Grade = "miss"
)

type Result struct {
	Note  int
	Tail  bool
	At    time.Duration
	Delta time.Duration
	Grade Grade
}
type state struct {
	head, tail bool
	active     bool
	failed     bool
	rollAt     time.Duration
}
type Session struct {
	Health  Health
	Rules   string // The stored rule version when replaying historical input.
	Chart   *chart.Chart
	Score   Score
	Held    []bool
	Results []Result
	Sounds  []chart.Sound // Drained by the game after Input/Advance.
	Music   []chart.Sound // Scheduled background audio, separate from key sounds.
	// OnScore is an optional, synchronous observer. At is the operation's game
	// time, not a backdated note time. It must not block or mutate this session.
	OnScore func(at time.Duration, score Score)
	states  []state
	lanes   [][]int
	next    []int
	audio   int
	last    time.Duration
	pending []Result
	o2jam   *o2jam // The O2Jam event mod, or nil.
}

func NewSession(c *chart.Chart) (*Session, error) {
	if err := c.Validate(); err != nil {
		return nil, err
	}
	s := &Session{Rules: Rules, Chart: c, Held: make([]bool, c.Lanes), states: make([]state, len(c.Notes)), lanes: make([][]int, c.Lanes), next: make([]int, c.Lanes), last: time.Duration(math.MinInt64)}
	s.Score.Flow, s.Score.Precision = 50, 20
	for i, n := range c.Notes {
		if n.Kind == "fake" {
			s.states[i] = state{head: true, tail: true}
			continue
		}
		s.lanes[n.Lane] = append(s.lanes[n.Lane], i)
		s.Score.Possible++
		if n.Kind == "hold" || n.Kind == "roll" {
			s.Score.Possible++
		}
	}
	return s, nil
}

// HitError averages the timing error of every judgement except Miss, or nil
// when nothing was hit.
func (s *Session) HitError() *HitError {
	var sum time.Duration
	n, fast, slow := 0, 0, 0
	for _, r := range s.Results {
		if r.Grade != Miss {
			sum += r.Delta
			n++
			if r.Grade == Kool {
				continue
			}
			if r.Delta < 0 {
				fast++
			} else if r.Delta > 0 {
				slow++
			}
		}
	}
	if n == 0 {
		return nil
	}
	return &HitError{MeanMS: float64(sum) / float64(n) / float64(time.Millisecond), Count: n, Fast: fast, Slow: slow}
}

func grade(delta time.Duration) Grade {
	if delta < 0 {
		delta = -delta
	}
	switch {
	case delta <= 22*time.Millisecond:
		return Kool
	case delta <= 45*time.Millisecond:
		return Cool
	case delta <= 90*time.Millisecond:
		return Good
	default:
		return Miss
	}
}

// releaseGrade widens every window by 1.5x, as osu!lazer's
// HoldNoteTail.RELEASE_WINDOW_LENIENCE does for hold tails.
func releaseGrade(delta time.Duration) Grade {
	if delta < 0 {
		delta = -delta
	}
	switch {
	case 2*delta <= 3*22*time.Millisecond:
		return Kool
	case 2*delta <= 3*45*time.Millisecond:
		return Cool
	case 2*delta <= 3*90*time.Millisecond:
		return Good
	default:
		return Miss
	}
}
func (s *Session) judge(i int, tail bool, at, delta time.Duration, g Grade) {
	if s.Health.Failed {
		return
	}
	if s.o2jam != nil {
		g = s.o2jam.pill(g)
	}
	if g == Miss {
		s.states[i].failed = true
	}
	s.Results = append(s.Results, Result{i, tail, at, delta, g})
	s.addScore(g)
	s.Health.add(g, at)
	if s.OnScore != nil {
		s.OnScore(s.last, s.Score)
	}
	if tail {
		s.states[i].tail = true
	} else {
		s.states[i].head = true
	}
}

// Advance is monotonic and independent of frame cadence. Late inputs must be
// passed to Input in timestamp order before advancing to the current frame.
func (s *Session) Advance(at time.Duration) error {
	if at < s.last {
		return fmt.Errorf("play: time moved backwards; create a new session to seek")
	}
	s.last = at
	if s.Health.Failed {
		return nil
	}
	s.pending = s.pending[:0]
	queue := func(i int, tail bool, at, delta time.Duration, g Grade) {
		s.pending = append(s.pending, Result{i, tail, at, delta, g})
		if tail {
			s.states[i].tail = true
		} else {
			s.states[i].head = true
		}
	}
	for lane, indices := range s.lanes {
		for s.next[lane] < len(indices) {
			i := indices[s.next[lane]]
			n := s.Chart.Notes[i]
			st := &s.states[i]
			early, late := s.window(i, false)
			if !st.head {
				if n.Kind == "mine" {
					if at >= early && s.Held[lane] {
						queue(i, false, early, 0, Miss)
					} else if at > late {
						queue(i, false, late, 0, Kool)
					} else {
						break
					}
				}
				if !st.head && at > late {
					queue(i, false, late, late-n.At, Miss)
				}
				if !st.head {
					break
				}
			}
			if n.Kind == "hold" || n.Kind == "roll" {
				if !st.tail {
					_, tailLate := s.window(i, true)
					if !st.active {
						queue(i, true, late, 0, Miss)
					} else if n.Kind == "roll" && at > st.rollAt+RollGap && st.rollAt+RollGap < n.End {
						st.active = false
						queue(i, true, st.rollAt+RollGap, 0, Miss)
					} else if (n.Kind == "roll" || s.Rules == LegacyRules) && at >= n.End {
						st.active = false
						queue(i, true, n.End, 0, Kool)
					} else if at > tailLate {
						st.active = false
						queue(i, true, tailLate, tailLate-n.End, Miss)
					} else {
						break
					}
				}
			}
			s.next[lane]++
		}
	}
	sort.SliceStable(s.pending, func(i, j int) bool {
		a, b := s.pending[i], s.pending[j]
		if a.At != b.At {
			return a.At < b.At
		}
		if a.Note != b.Note {
			return a.Note < b.Note
		}
		return !a.Tail && b.Tail
	})
	for _, r := range s.pending {
		s.judge(r.Note, r.Tail, r.At, r.Delta, r.Grade)
	}
	for s.audio < len(s.Chart.Audio) && s.Chart.Audio[s.audio].At <= at {
		s.Music = append(s.Music, s.Chart.Audio[s.audio].Sound)
		s.audio++
	}
	return nil
}

func (s *Session) Input(at time.Duration, lane int, down bool) error {
	if lane < 0 || lane >= len(s.Held) {
		return fmt.Errorf("play: lane %d out of range", lane)
	}
	if err := s.Advance(at); err != nil {
		return err
	}
	if s.Health.Failed {
		return nil
	}
	if s.Held[lane] == down {
		return nil
	}
	s.Held[lane] = down
	indices := s.lanes[lane]
	if s.next[lane] >= len(indices) {
		return nil
	}
	i := indices[s.next[lane]]
	n := s.Chart.Notes[i]
	st := &s.states[i]
	if st.active {
		if n.Kind == "roll" && down {
			st.rollAt = at
			return nil
		}
		if !down && n.Kind == "hold" {
			st.active = false
			delta := at - n.End
			g := grade(delta)
			if s.Rules == Rules {
				g = releaseGrade(delta)
			}
			if delta < -Window {
				g = Miss
			}
			if s.o2jam != nil {
				g = s.o2Grade(i, true, at)
			}
			s.judge(i, true, at, delta, g)
		}
		return nil
	}
	if early, late := s.window(i, false); st.head || at < early || at > late {
		return nil
	}
	if n.Kind == "mine" {
		if down {
			s.judge(i, false, at, at-n.At, Miss)
		}
		return nil
	}
	if (n.Kind == "lift") == down {
		return nil
	}
	delta := at - n.At
	g := grade(delta)
	if s.o2jam != nil {
		g = s.o2Grade(i, false, at)
	}
	s.judge(i, false, at, delta, g)
	s.Sounds = append(s.Sounds, n.Sounds...)
	if n.End > n.At {
		if g == Miss {
			s.judge(i, true, at, 0, Miss)
		} else {
			st.active = true
			st.rollAt = at
		}
	}
	return nil
}

func (s *Session) Visible(i int) bool {
	n := s.Chart.Notes[i]
	if n.Kind == "fake" {
		return s.last <= n.At+Window
	}
	st := s.states[i]
	if n.End > n.At && st.failed {
		return s.last < n.End
	}
	return !st.head || (n.End > n.At && !st.tail)
}

// Active reports a successfully started hold or roll that has not ended.
func (s *Session) Active(i int) bool { return s.states[i].active }

// Failed reports a missed head or broken long note, even after its judgement.
func (s *Session) Failed(i int) bool { return s.states[i].failed }
func (s *Session) Done() bool {
	return s.Judged() && s.audio == len(s.Chart.Audio)
}

// Judged reports every note judged, ignoring audio still scheduled after them.
func (s *Session) Judged() bool {
	for lane, list := range s.lanes {
		if s.next[lane] < len(list) {
			return false
		}
	}
	return true
}
