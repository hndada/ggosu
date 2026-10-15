//go:build linux || darwin

package keyboard

import (
	"errors"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	"ggosu/internal/clock"
)

// Source owns an OS event loop independent of Update and Draw. Both physical
// events and synthetic test messages are delivered by that loop, in order.
type Source struct {
	native    native
	clock     *clock.Counter
	receive   func(Event)
	done      chan struct{}
	errors    chan error
	active    atomic.Bool
	closeOnce sync.Once
	closeErr  error
	seq       uint64 // Only the native event loop changes this.
	notice    string // Published at startup, then immutable.
}

func Start(counter *clock.Counter, receive func(Event)) (*Source, error) {
	if counter == nil || receive == nil {
		return nil, errors.New("keyboard: nil counter or callback")
	}
	s := &Source{clock: counter, receive: receive, done: make(chan struct{}), errors: make(chan error, 1)}
	ready := make(chan error, 1)
	go func() {
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
		defer close(s.done)
		s.run(ready)
	}()
	if err := <-ready; err != nil {
		<-s.done
		return nil, err
	}
	return s, nil
}

func (s *Source) Now() time.Duration    { return s.clock.Now() }
func (s *Source) Errors() <-chan error  { return s.errors }
func (s *Source) SetActive(active bool) { s.active.Store(active) }
func (s *Source) Notice() string        { return s.notice }

func (s *Source) Post(key uint16, down bool) error {
	select {
	case <-s.done:
		return errors.New("keyboard receiver is closed")
	default:
		return s.post(key, down)
	}
}

func (s *Source) Close() error {
	s.closeOnce.Do(func() {
		select {
		case <-s.done:
		default:
			if err := s.stop(); err != nil {
				s.closeErr = err
				return
			}
			select {
			case <-s.done:
			case <-time.After(3 * time.Second):
				s.closeErr = errors.New("keyboard receiver did not stop within 3 seconds")
				return
			}
		}
		select {
		case err := <-s.errors:
			s.closeErr = err
		default:
		}
	})
	return s.closeErr
}

func (s *Source) fail(err error) {
	select {
	case s.errors <- err:
	default:
	}
}

func (s *Source) deliver(e Event) {
	s.seq++
	e.Seq = s.seq
	s.receive(e)
}
