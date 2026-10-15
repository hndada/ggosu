package keyboard

import (
	"errors"
	"fmt"
	"os"
	"runtime"
	"sync"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"

	"ggosu/internal/clock"
)

const (
	wmDestroy   = 0x0002
	wmClose     = 0x0010
	wmInput     = 0x00ff
	wmSynthetic = 0x8001
	ridInput    = 0x10000003
	ridevSink   = 0x00000100
	ridevRemove = 0x00000001
)

var (
	user32          = windows.NewLazySystemDLL("user32.dll")
	kernel32        = windows.NewLazySystemDLL("kernel32.dll")
	registerClass   = user32.NewProc("RegisterClassExW")
	unregisterClass = user32.NewProc("UnregisterClassW")
	createWindow    = user32.NewProc("CreateWindowExW")
	destroyWindow   = user32.NewProc("DestroyWindow")
	defWindowProc   = user32.NewProc("DefWindowProcW")
	getMessage      = user32.NewProc("GetMessageW")
	translateMsg    = user32.NewProc("TranslateMessage")
	dispatchMsg     = user32.NewProc("DispatchMessageW")
	postMessage     = user32.NewProc("PostMessageW")
	postQuit        = user32.NewProc("PostQuitMessage")
	registerInput   = user32.NewProc("RegisterRawInputDevices")
	getRawInput     = user32.NewProc("GetRawInputData")
	foreground      = user32.NewProc("GetForegroundWindow")
	windowPID       = user32.NewProc("GetWindowThreadProcessId")
	moduleHandle    = kernel32.NewProc("GetModuleHandleW")
)

type windowClass struct {
	Size, Style  uint32
	Proc         uintptr
	ClassExtra   int32
	WindowExtra  int32
	Instance     uintptr
	Icon, Cursor uintptr
	Background   uintptr
	Menu, Name   *uint16
	SmallIcon    uintptr
}

type message struct {
	Window  uintptr
	ID      uint32
	WParam  uintptr
	LParam  uintptr
	Time    uint32
	X, Y    int32
	Private uint32
}

type rawHeader struct {
	Kind, Size     uint32
	Device, WParam uintptr
}

type rawDevice struct {
	Page, Usage uint16
	Flags       uint32
	Window      uintptr
}

// Source owns a message-only window on a dedicated OS thread. Its callback
// runs there immediately after a message is read, independently of Ebitengine.
// Keep the callback short and thread-safe. Close stops and joins the thread.
type Source struct {
	window    uintptr // Published by the startup handshake; immutable afterwards.
	clock     *clock.Counter
	receive   func(Event)
	done      chan struct{}
	errors    chan error
	closeOnce sync.Once
	closeErr  error
	seq       uint64 // Receiver thread only.
	destroyed bool
	buffer    [32]uint64 // GetRawInputData requires DWORD-aligned storage.
}

func Start(counter *clock.Counter, receive func(Event)) (*Source, error) {
	if unsafe.Sizeof(uintptr(0)) != 8 {
		return nil, errors.New("keyboard POC requires 64-bit Windows")
	}
	if counter == nil || receive == nil {
		return nil, errors.New("keyboard: nil counter or callback")
	}
	s := &Source{
		clock: counter, receive: receive,
		done: make(chan struct{}), errors: make(chan error, 1),
	}
	ready := make(chan error, 1)
	go s.run(ready)
	if err := <-ready; err != nil {
		<-s.done
		return nil, err
	}
	return s, nil
}

func (s *Source) Errors() <-chan error { return s.errors }

// Windows checks the actual foreground process on every raw message.
func (*Source) SetActive(bool)  {}
func (*Source) Backend() string { return "Windows Raw Input" }
func (*Source) Notice() string  { return "" }

func (s *Source) Now() time.Duration { return s.clock.Now() }

func (s *Source) Close() error {
	s.closeOnce.Do(func() {
		select {
		case <-s.done:
			select {
			case err := <-s.errors:
				s.closeErr = err
			default:
			}
			return
		default:
		}
		r, _, err := postMessage.Call(s.window, wmClose, 0, 0)
		if r == 0 {
			s.closeErr = callError("post keyboard close", err)
			return
		}
		select {
		case <-s.done:
			select {
			case err := <-s.errors:
				s.closeErr = err
			default:
			}
		case <-time.After(3 * time.Second):
			s.closeErr = errors.New("keyboard receiver did not stop within 3 seconds")
		}
	})
	return s.closeErr
}

// Post sends a clearly marked synthetic event through the receiver's Windows
// message loop. It does not inject OS keyboard input or exercise WM_INPUT.
func (s *Source) Post(key uint16, down bool) error {
	select {
	case <-s.done:
		return errors.New("keyboard receiver is closed")
	default:
	}
	value := uintptr(key)
	if down {
		value |= 1 << 16
	}
	r, _, err := postMessage.Call(s.window, wmSynthetic, value, uintptr(s.clock.Now()))
	if r == 0 {
		return callError("post synthetic key", err)
	}
	return nil
}

func (s *Source) fail(err error) {
	select {
	case s.errors <- err:
	default:
	}
}

func (s *Source) run(ready chan<- error) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	defer close(s.done)
	// Resolve the hot-path APIs before the first real key arrives, so lazy DLL
	// lookup does not become the first event's measured delivery latency.
	for _, proc := range []*windows.LazyProc{foreground, windowPID, getRawInput} {
		if err := proc.Find(); err != nil {
			ready <- fmt.Errorf("resolve keyboard API %s: %w", proc.Name, err)
			return
		}
	}
	name, err := windows.UTF16PtrFromString(fmt.Sprintf("ggosu.keyboard.%d", os.Getpid()))
	if err != nil {
		ready <- err
		return
	}
	instance, _, err := moduleHandle.Call(0)
	if instance == 0 {
		ready <- callError("get module handle", err)
		return
	}
	class := windowClass{Size: uint32(unsafe.Sizeof(windowClass{})),
		Proc: syscall.NewCallback(s.windowProc), Instance: instance, Name: name}
	r, _, err := registerClass.Call(uintptr(unsafe.Pointer(&class)))
	if r == 0 {
		ready <- callError("register keyboard window class", err)
		return
	}
	defer func() {
		r, _, err := unregisterClass.Call(uintptr(unsafe.Pointer(name)), instance)
		if r == 0 {
			s.fail(callError("unregister keyboard window class", err))
		}
	}()
	// HWND_MESSAGE is -3: no visible window, taskbar entry, or focus stealing.
	s.window, _, err = createWindow.Call(0, uintptr(unsafe.Pointer(name)), 0, 0,
		0, 0, 0, 0, ^uintptr(2), 0, instance, 0)
	if s.window == 0 {
		ready <- callError("create keyboard receiver window", err)
		return
	}
	defer func() {
		if !s.destroyed {
			r, _, err := destroyWindow.Call(s.window)
			if r == 0 {
				s.fail(callError("destroy keyboard window during cleanup", err))
			}
		}
	}()
	device := rawDevice{Page: 1, Usage: 6, Flags: ridevSink, Window: s.window}
	r, _, err = registerInput.Call(uintptr(unsafe.Pointer(&device)), 1, unsafe.Sizeof(device))
	if r == 0 {
		ready <- callError("register raw keyboard", err)
		return
	}
	defer func() {
		device.Flags, device.Window = ridevRemove, 0
		r, _, err := registerInput.Call(uintptr(unsafe.Pointer(&device)), 1, unsafe.Sizeof(device))
		if r == 0 {
			s.fail(callError("unregister raw keyboard", err))
		}
	}()
	ready <- nil
	var msg message
	for {
		r, _, err := getMessage.Call(uintptr(unsafe.Pointer(&msg)), 0, 0, 0)
		if int32(r) == -1 {
			s.fail(callError("read keyboard message", err))
			return
		}
		if r == 0 {
			return
		}
		translateMsg.Call(uintptr(unsafe.Pointer(&msg)))
		dispatchMsg.Call(uintptr(unsafe.Pointer(&msg)))
	}
}

func (s *Source) windowProc(hwnd, msg, wParam, lParam uintptr) uintptr {
	switch msg {
	case wmInput:
		at := s.clock.Now()
		// INPUTSINK is necessary for the message-only window. Discard background
		// input before reading the keyboard data; only our foreground app is used.
		active, _, _ := foreground.Call()
		var pid uint32
		windowPID.Call(active, uintptr(unsafe.Pointer(&pid)))
		if active != 0 && pid == uint32(os.Getpid()) {
			s.read(lParam, at)
		}
		// DefWindowProc performs the required foreground raw-input cleanup.
	case wmSynthetic:
		s.deliver(Event{At: s.clock.Now(), PostedAt: time.Duration(lParam),
			Key: uint16(wParam), Down: wParam&(1<<16) != 0, Synthetic: true})
		return 0
	case wmClose:
		r, _, err := destroyWindow.Call(hwnd)
		if r == 0 {
			s.fail(callError("destroy keyboard receiver", err))
			postQuit.Call(1)
		}
		return 0
	case wmDestroy:
		s.destroyed = true
		postQuit.Call(0)
		return 0
	}
	r, _, _ := defWindowProc.Call(hwnd, msg, wParam, lParam)
	return r
}

func (s *Source) read(handle uintptr, at time.Duration) {
	size := uint32(unsafe.Sizeof(s.buffer))
	r, _, err := getRawInput.Call(handle, ridInput, uintptr(unsafe.Pointer(&s.buffer[0])),
		uintptr(unsafe.Pointer(&size)), unsafe.Sizeof(rawHeader{}))
	if uint32(r) == ^uint32(0) {
		s.fail(callError("read raw keyboard", err))
		return
	}
	if r > unsafe.Sizeof(s.buffer) {
		s.fail(errors.New("raw keyboard packet exceeds receiver buffer"))
		return
	}
	data := unsafe.Slice((*byte)(unsafe.Pointer(&s.buffer[0])), int(r))
	e, ok, err := decode(data, int(unsafe.Sizeof(rawHeader{})))
	if err != nil {
		s.fail(err)
		return
	}
	if ok {
		e.At = at
		s.deliver(e)
	}
}

func (s *Source) deliver(e Event) {
	s.seq++
	e.Seq = s.seq
	s.receive(e)
}

func callError(op string, err error) error {
	if err == nil || errors.Is(err, syscall.Errno(0)) {
		return fmt.Errorf("%s failed (Windows returned no error code)", op)
	}
	return fmt.Errorf("%s: %w", op, err)
}
