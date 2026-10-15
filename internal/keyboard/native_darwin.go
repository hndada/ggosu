package keyboard

import (
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"unsafe"

	"github.com/ebitengine/purego"
	"github.com/ebitengine/purego/objc"
)

type native struct {
	api      *hidAPI
	loop     uintptr
	source   uintptr
	posted   chan Event
	mu       sync.Mutex // Only protects shutdown against Post.
	closed   bool
	stopping atomic.Bool
}

type loopSourceContext struct {
	Version                                                                    int64
	Info, Retain, Release, Description, Equal, Hash, Schedule, Cancel, Perform uintptr
}

func (*Source) Backend() string { return "macOS IOHIDManager / CFRunLoop" }

func (s *Source) run(ready chan<- error) {
	a, err := openHID()
	if err != nil {
		ready <- err
		return
	}
	defer func() {
		if err := a.close(); err != nil {
			s.fail(err)
		}
	}()
	n := &s.native
	n.api, n.loop, n.posted = a, a.loopCurrent(), make(chan Event, 32768)
	mode := a.stringCreate(0, "kCFRunLoopDefaultMode", 0x08000100)
	if mode == 0 {
		ready <- errors.New("create input run-loop mode")
		return
	}
	defer a.release(mode)
	poolClass := objc.GetClass("NSAutoreleasePool")
	appClass := objc.GetClass("NSRunningApplication")
	if poolClass == 0 || appClass == 0 {
		ready <- errors.New("AppKit application classes unavailable")
		return
	}
	pool := a.objectSend(uintptr(poolClass), uintptr(objc.RegisterName("new")))
	defer a.objectSend(pool, uintptr(objc.RegisterName("drain")))
	application := a.objectSend(uintptr(appClass), uintptr(objc.RegisterName("currentApplication")))
	active := uintptr(objc.RegisterName("isActive"))
	perform := purego.NewCallback(func(uintptr) {
		if n.stopping.Load() {
			a.loopStop(n.loop)
			return
		}
		for {
			select {
			case e := <-n.posted:
				e.At = s.Now()
				s.deliver(e)
			default:
				return
			}
		}
	})
	context := loopSourceContext{Perform: perform}
	n.source = a.sourceCreate(0, 0, unsafe.Pointer(&context))
	if n.source == 0 {
		ready <- errors.New("create input test wake source")
		return
	}
	a.loopAddSource(n.loop, n.source, mode)
	defer func() {
		n.mu.Lock()
		defer n.mu.Unlock()
		n.closed = true
		a.loopRemoveSource(n.loop, n.source, mode)
		a.release(n.source)
	}()
	manager := a.managerCreate(0, 0)
	if manager == 0 {
		ready <- errors.New("create HID manager")
		return
	}
	defer a.release(manager)
	matching, err := a.keyboardMatch()
	if err != nil {
		ready <- err
		return
	}
	a.managerMatch(manager, matching)
	a.release(matching)
	callback := purego.NewCallback(func(_ uintptr, result int32, _ uintptr, value uintptr) {
		// Neither the run-loop callback nor this input path uses timers/FPS.
		if result != 0 {
			s.fail(fmt.Errorf("HID input status: 0x%x", uint32(result)))
			return
		}
		if !s.active.Load() || !a.activeSend(application, active) {
			return
		}
		at := s.Now()
		element := a.valueElement(value)
		if a.elementPage(element) != 7 {
			return
		} // Keyboard/Keypad usage page.
		usage := a.elementUsage(element)
		key := hidKey(usage)
		if key == 0 {
			return
		}
		e := Event{At: at, Key: key, Scan: uint16(usage),
			Device: a.elementDevice(element), Down: a.valueInteger(value) != 0}
		if !e.Down {
			e.Flags = 1
		}
		s.deliver(e)
	})
	a.managerCallback(manager, callback, 0)
	a.managerSchedule(manager, n.loop, mode)
	defer a.managerUnschedule(manager, n.loop, mode)
	if a.checkAccess(1) != 0 {
		s.notice = "IOHID capture inactive: allow Input Monitoring for this app and restart. Synthetic tests still work."
	} else if result := a.managerOpen(manager, 0); result != 0 {
		s.notice = fmt.Sprintf("IOHID open failed (0x%x); native capture inactive. Synthetic tests still work.", uint32(result))
	} else {
		defer func() {
			if status := a.managerClose(manager, 0); status != 0 {
				s.fail(fmt.Errorf("close HID manager: 0x%x", uint32(status)))
			}
		}()
	}
	ready <- nil
	a.loopRun() // Dedicated OS thread; Close stops and wakes this run loop.
}

func (s *Source) post(key uint16, down bool) error {
	n := &s.native
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.closed {
		return errors.New("keyboard receiver is closed")
	}
	select {
	case n.posted <- Event{PostedAt: s.Now(), Key: key, Down: down, Synthetic: true}:
		n.api.sourceSignal(n.source)
		n.api.loopWake(n.loop)
		return nil
	default:
		return errors.New("input test wake queue is full")
	}
}

func (s *Source) stop() error {
	n := &s.native
	n.mu.Lock()
	defer n.mu.Unlock()
	if !n.closed {
		n.stopping.Store(true)
		n.api.sourceSignal(n.source)
		n.api.loopWake(n.loop)
	}
	return nil
}
