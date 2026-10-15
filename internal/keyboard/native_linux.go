package keyboard

import (
	"encoding/binary"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
	"unsafe"

	"golang.org/x/sys/unix"
)

type native struct {
	epoll   int
	wake    int
	pipe    [2]int
	devices []int
	mu      sync.Mutex // Post and descriptor cleanup only; never held during callbacks.
	closed  bool
}

func (*Source) Backend() string { return "Linux evdev / epoll" }

func (s *Source) run(ready chan<- error) {
	if unsafe.Sizeof(uintptr(0)) != 8 {
		ready <- errors.New("evdev POC requires a 64-bit Linux target")
		return
	}
	n := &s.native
	fd, err := unix.EpollCreate1(unix.EPOLL_CLOEXEC)
	if err != nil {
		ready <- fmt.Errorf("create input epoll: %w", err)
		return
	}
	n.epoll = fd
	defer func() {
		if err := unix.Close(fd); err != nil {
			s.fail(err)
		}
	}()
	wake, err := unix.Eventfd(0, unix.EFD_CLOEXEC|unix.EFD_NONBLOCK)
	if err != nil {
		ready <- fmt.Errorf("create input wake event: %w", err)
		return
	}
	n.wake = wake
	defer func() {
		if err := unix.Close(wake); err != nil {
			s.fail(err)
		}
	}()
	if err := unix.Pipe2(n.pipe[:], unix.O_CLOEXEC|unix.O_NONBLOCK); err != nil {
		ready <- fmt.Errorf("create input test pipe: %w", err)
		return
	}
	defer func() {
		n.mu.Lock()
		defer n.mu.Unlock()
		n.closed = true
		for _, fd := range append(n.devices, n.pipe[:]...) {
			if err := unix.Close(fd); err != nil {
				s.fail(err)
			}
		}
	}()
	add := func(fd int) error {
		return unix.EpollCtl(n.epoll, unix.EPOLL_CTL_ADD, fd, &unix.EpollEvent{Events: unix.EPOLLIN, Fd: int32(fd)})
	}
	if err := add(wake); err != nil {
		ready <- err
		return
	}
	if err := add(n.pipe[0]); err != nil {
		ready <- err
		return
	}
	paths, err := filepath.Glob("/dev/input/event*")
	if err != nil {
		ready <- err
		return
	}
	if named := os.Getenv("GGOSU_INPUT_DEVICES"); named != "" {
		paths = filepath.SplitList(named)
	}
	var denied bool
	for _, path := range paths {
		fd, err := unix.Open(path, unix.O_RDONLY|unix.O_NONBLOCK|unix.O_CLOEXEC, 0)
		if err != nil {
			denied = denied || errors.Is(err, unix.EACCES)
			continue
		}
		if !keyboardDevice(fd) {
			if err := unix.Close(fd); err != nil {
				s.fail(err)
			}
			continue
		}
		if err := add(fd); err != nil {
			ready <- errors.Join(fmt.Errorf("watch %s: %w", path, err), unix.Close(fd))
			return
		}
		n.devices = append(n.devices, fd)
	}
	if len(n.devices) == 0 {
		s.notice = "No readable evdev keyboards; native capture inactive (synthetic tests still work)."
		if denied {
			s.notice += " Grant read access to the selected /dev/input/event device, then restart."
		}
	}
	ready <- nil
	var events [32]unix.EpollEvent
	var buffer [6144]byte // Whole multiples of both 24-byte evdev and 16-byte test records.
	for {
		count, err := unix.EpollWait(n.epoll, events[:], -1)
		if errors.Is(err, unix.EINTR) {
			continue
		}
		if err != nil {
			s.fail(fmt.Errorf("wait for input: %w", err))
			return
		}
		for _, event := range events[:count] {
			fd := int(event.Fd)
			if fd == n.wake {
				return
			}
			if event.Events&(unix.EPOLLHUP|unix.EPOLLERR) != 0 {
				s.fail(errors.New("input device disconnected; restart the POC"))
				return
			}
			for {
				bytes, err := unix.Read(fd, buffer[:])
				if errors.Is(err, unix.EAGAIN) {
					break
				}
				if errors.Is(err, unix.EINTR) {
					continue
				}
				if err != nil {
					s.fail(fmt.Errorf("read input: %w", err))
					return
				}
				if bytes == 0 {
					s.fail(errors.New("input stream closed"))
					return
				}
				size := 24
				if fd == n.pipe[0] {
					size = 16
				}
				if bytes%size != 0 {
					s.fail(errors.New("partial input record"))
					return
				}
				for offset := 0; offset < bytes; offset += size {
					data, at := buffer[offset:offset+size], s.Now()
					if size == 16 {
						s.deliver(Event{At: at, PostedAt: time.Duration(binary.NativeEndian.Uint64(data)),
							Key: binary.NativeEndian.Uint16(data[8:]), Down: data[10] != 0, Synthetic: true})
						continue
					}
					if !s.active.Load() {
						continue
					}
					e, ok, err := decodeEvdev(data)
					if err != nil {
						s.fail(err)
						return
					}
					if ok {
						e.At, e.Device = at, uintptr(fd)
						s.deliver(e)
					}
				}
			}
		}
	}
}

// EVIOCGBIT(EV_KEY) identifies keyboard-capable devices, avoiding mice/buttons.
func keyboardDevice(fd int) bool {
	var bits [96]byte
	const request = 0x80604521 // _IOC(_IOC_READ, 'E', 0x21, 96)
	_, _, errno := unix.Syscall(unix.SYS_IOCTL, uintptr(fd), request, uintptr(unsafe.Pointer(&bits[0])))
	if errno != 0 {
		return false
	}
	return bits[30/8]&(1<<(30%8)) != 0 && bits[57/8]&(1<<(57%8)) != 0 // KEY_A and SPACE
}

func (s *Source) post(key uint16, down bool) error {
	n := &s.native
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.closed {
		return errors.New("keyboard receiver is closed")
	}
	var record [16]byte
	binary.NativeEndian.PutUint64(record[:], uint64(s.Now()))
	binary.NativeEndian.PutUint16(record[8:], key)
	if down {
		record[10] = 1
	}
	count, err := unix.Write(n.pipe[1], record[:])
	if err != nil {
		return fmt.Errorf("post input test event: %w", err)
	}
	if count != len(record) {
		return errors.New("short input test write")
	}
	return nil
}

func (s *Source) stop() error {
	var one = [8]byte{1}
	if _, err := unix.Write(s.native.wake, one[:]); err != nil {
		return fmt.Errorf("wake input receiver: %w", err)
	}
	return nil
}
