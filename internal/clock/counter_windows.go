package clock

import (
	"fmt"
	"runtime"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	kernel32       = windows.NewLazySystemDLL("kernel32.dll")
	queryCounter   = kernel32.NewProc("QueryPerformanceCounter")
	queryFrequency = kernel32.NewProc("QueryPerformanceFrequency")
)

// Counter is an immutable QPC origin shared by the receiver and the game.
// All event and latency timestamps use this one high-resolution time domain.
type Counter struct {
	origin int64
	freq   int64
	proc   uintptr
}

func New() (*Counter, error) {
	c := new(Counter)
	r, _, err := queryFrequency.Call(uintptr(unsafe.Pointer(&c.freq)))
	if r == 0 || c.freq <= 0 {
		return nil, fmt.Errorf("read performance counter frequency: %w", err)
	}
	r, _, err = queryCounter.Call(uintptr(unsafe.Pointer(&c.origin)))
	if r == 0 {
		return nil, fmt.Errorf("read performance counter origin: %w", err)
	}
	c.proc = queryCounter.Addr()
	return c, nil
}

func (c *Counter) Frequency() int64 { return c.freq }

func (c *Counter) Now() time.Duration {
	var ticks int64
	// Microsoft guarantees success on Windows XP+ with a valid pointer. This
	// POC targets Windows amd64; no fallback clock is mixed into measurements.
	// SyscallN keeps this synchronous output on the stack. LazyProc.Call's
	// uintptrescapes wrapper would allocate a buffer for every clock sample.
	syscall.SyscallN(c.proc, uintptr(unsafe.Pointer(&ticks)))
	runtime.KeepAlive(&ticks)
	ticks -= c.origin
	// Split before multiplying so a long session cannot overflow ticks*1e9.
	return time.Duration(ticks/c.freq)*time.Second + time.Duration((ticks%c.freq)*int64(time.Second)/c.freq)
}
