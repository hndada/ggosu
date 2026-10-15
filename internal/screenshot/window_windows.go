package screenshot

import (
	"errors"
	"image"
	"sync"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	user32                  = windows.NewLazySystemDLL("user32.dll")
	gdi32                   = windows.NewLazySystemDLL("gdi32.dll")
	procEnumWindows         = user32.NewProc("EnumWindows")
	procGetWindowThreadPID  = user32.NewProc("GetWindowThreadProcessId")
	procIsWindowVisible     = user32.NewProc("IsWindowVisible")
	procGetClientRect       = user32.NewProc("GetClientRect")
	procGetDC               = user32.NewProc("GetDC")
	procReleaseDC           = user32.NewProc("ReleaseDC")
	procPrintWindow         = user32.NewProc("PrintWindow")
	procCreateCompatibleDC  = gdi32.NewProc("CreateCompatibleDC")
	procCreateCompatibleBMP = gdi32.NewProc("CreateCompatibleBitmap")
	procSelectObject        = gdi32.NewProc("SelectObject")
	procDeleteObject        = gdi32.NewProc("DeleteObject")
	procDeleteDC            = gdi32.NewProc("DeleteDC")
	procGetDIBits           = gdi32.NewProc("GetDIBits")
)

type rect struct{ Left, Top, Right, Bottom int32 }

type bitmapInfoHeader struct {
	Size          uint32
	Width, Height int32
	Planes        uint16
	BitCount      uint16
	Compression   uint32
	SizeImage     uint32
	XPels, YPels  int32
	ClrUsed       uint32
	ClrImportant  uint32
}

// Windows allows only a limited number of callbacks per process, so one is made once
// and reports into find, which mu guards.
var (
	mu   sync.Mutex
	find struct {
		pid  uint32
		best uintptr
		area int32
	}
	enum = syscall.NewCallback(func(hwnd, _ uintptr) uintptr {
		var owner uint32
		procGetWindowThreadPID.Call(hwnd, uintptr(unsafe.Pointer(&owner)))
		if owner != find.pid {
			return 1
		}
		if v, _, _ := procIsWindowVisible.Call(hwnd); v == 0 {
			return 1
		}
		var r rect
		procGetClientRect.Call(hwnd, uintptr(unsafe.Pointer(&r)))
		if a := (r.Right - r.Left) * (r.Bottom - r.Top); a > find.area {
			find.best, find.area = hwnd, a
		}
		return 1
	})
)

// Window captures the client area of this process's largest visible window.
// PrintWindow with PW_RENDERFULLCONTENT also captures WebView2 content.
func Window() (image.Image, error) {
	mu.Lock()
	find.pid, find.best, find.area = windows.GetCurrentProcessId(), 0, 0
	procEnumWindows.Call(enum, 0)
	best := find.best
	mu.Unlock()
	if best == 0 {
		return nil, errors.New("screenshot: no visible window")
	}
	var r rect
	procGetClientRect.Call(best, uintptr(unsafe.Pointer(&r)))
	w, h := r.Right-r.Left, r.Bottom-r.Top
	screen, _, _ := procGetDC.Call(0)
	if screen == 0 {
		return nil, errors.New("screenshot: GetDC failed")
	}
	defer procReleaseDC.Call(0, screen)
	mem, _, _ := procCreateCompatibleDC.Call(screen)
	defer procDeleteDC.Call(mem)
	bmp, _, _ := procCreateCompatibleBMP.Call(screen, uintptr(w), uintptr(h))
	if bmp == 0 {
		return nil, errors.New("screenshot: CreateCompatibleBitmap failed")
	}
	defer procDeleteObject.Call(bmp)
	old, _, _ := procSelectObject.Call(mem, bmp)
	const pwClientOnly, pwRenderFullContent = 1, 2
	ok, _, _ := procPrintWindow.Call(best, mem, pwClientOnly|pwRenderFullContent)
	procSelectObject.Call(mem, old)
	if ok == 0 {
		return nil, errors.New("screenshot: PrintWindow failed")
	}
	// A negative height asks GetDIBits for top-down rows; 32-bit pixels arrive as BGRA.
	hdr := bitmapInfoHeader{Width: w, Height: -h, Planes: 1, BitCount: 32}
	hdr.Size = uint32(unsafe.Sizeof(hdr))
	img := image.NewRGBA(image.Rect(0, 0, int(w), int(h)))
	if n, _, _ := procGetDIBits.Call(mem, bmp, 0, uintptr(h), uintptr(unsafe.Pointer(&img.Pix[0])), uintptr(unsafe.Pointer(&hdr)), 0); n == 0 {
		return nil, errors.New("screenshot: GetDIBits failed")
	}
	for i := 0; i < len(img.Pix); i += 4 {
		img.Pix[i], img.Pix[i+2], img.Pix[i+3] = img.Pix[i+2], img.Pix[i], 255
	}
	return img, nil
}
