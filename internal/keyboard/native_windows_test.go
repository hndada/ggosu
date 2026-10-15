package keyboard

import (
	"testing"
	"time"
	"unsafe"

	"ggosu/internal/clock"
)

func TestNativeABI(t *testing.T) {
	if unsafe.Sizeof(uintptr(0)) != 8 {
		t.Skip("POC runtime targets 64-bit Windows")
	}
	for _, item := range []struct {
		name      string
		got, want uintptr
	}{
		{"WNDCLASSEX", unsafe.Sizeof(windowClass{}), 80},
		{"MSG", unsafe.Sizeof(message{}), 48},
		{"RAWINPUTHEADER", unsafe.Sizeof(rawHeader{}), 24},
		{"RAWINPUTDEVICE", unsafe.Sizeof(rawDevice{}), 16},
	} {
		if item.got != item.want {
			t.Errorf("%s: %d bytes, want %d", item.name, item.got, item.want)
		}
	}
}

func TestRawBufferAlignment(t *testing.T) {
	s := new(Source)
	if uintptr(unsafe.Pointer(&s.buffer[0]))%8 != 0 {
		t.Fatal("raw packet buffer is not aligned for the native header")
	}
}

func TestSourceMessageLoopAndClose(t *testing.T) {
	// Only one raw-input window per keyboard device class can be registered in
	// a process. Keep these lifecycle tests sequential.
	for run := 0; run < 3; run++ {
		counter, err := clock.New()
		if err != nil {
			t.Fatal(err)
		}
		events := make(chan Event, 1024)
		s, err := Start(counter, func(e Event) { events <- e })
		if err != nil {
			t.Fatal(err)
		}
		for i := 0; i < 1000; i++ {
			if err := s.Post('D', i%2 == 0); err != nil {
				_ = s.Close()
				t.Fatal(err)
			}
		}
		timer := time.NewTimer(3 * time.Second)
		for i := 0; i < 1000; i++ {
			select {
			case e := <-events:
				if e.Seq != uint64(i+1) || e.Down != (i%2 == 0) || e.Key != 'D' || !e.Synthetic || e.At < e.PostedAt {
					_ = s.Close()
					t.Fatalf("event %d: %+v", i, e)
				}
			case <-timer.C:
				_ = s.Close()
				t.Fatalf("receiver timed out at event %d", i)
			}
		}
		timer.Stop()
		if err := s.Close(); err != nil {
			t.Fatal(err)
		}
		if err := s.Close(); err != nil {
			t.Fatalf("second Close: %v", err)
		}
		if err := s.Post('D', true); err == nil {
			t.Fatal("Post succeeded after Close")
		}
		select {
		case err := <-s.Errors():
			t.Fatal(err)
		default:
		}
	}
}
