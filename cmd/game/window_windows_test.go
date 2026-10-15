package main

import (
	"strings"
	"testing"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"

	"ggosu/internal/settings"
)

// Exercise the OS window state as well as the resident pipe. The next song
// must restore an idle window before applying its own size and display mode.
func TestResidentWindowStates(t *testing.T) {
	u := windows.NewLazySystemDLL("user32.dll")
	enum := u.NewProc("EnumWindows")
	pidOf := u.NewProc("GetWindowThreadProcessId")
	classOf := u.NewProc("GetClassNameW")
	titleOf := u.NewProc("GetWindowTextW")
	show := u.NewProc("ShowWindow")
	iconic := u.NewProc("IsIconic")
	zoomed := u.NewProc("IsZoomed")
	clientRect := u.NewProc("GetClientRect")
	dpiOf := u.NewProc("GetDpiForWindow")
	for _, state := range []struct {
		name string
		cmd  uintptr
		is   *windows.LazyProc
	}{
		{"minimized", 2, iconic}, // SW_SHOWMINIMIZED
		{"maximized", 3, zoomed}, // SW_SHOWMAXIMIZED
	} {
		t.Run(state.name, func(t *testing.T) {
			var hwnd uintptr
			var pid int
			find := windows.NewCallback(func(w, _ uintptr) uintptr {
				var owner uint32
				pidOf.Call(w, uintptr(unsafe.Pointer(&owner)))
				if owner != uint32(pid) {
					return 1
				}
				var class [64]uint16
				classOf.Call(w, uintptr(unsafe.Pointer(&class[0])), uintptr(len(class)))
				if windows.UTF16ToString(class[:]) != "GLFW30" {
					return 1
				}
				// GLFW's hidden message window uses the same class. Only the
				// actual game window should be minimized or maximized.
				var title [256]uint16
				titleOf.Call(w, uintptr(unsafe.Pointer(&title[0])), uintptr(len(title)))
				if !strings.HasPrefix(windows.UTF16ToString(title[:]), "gosu") {
					return 1
				}
				hwnd = w
				return 0
			})
			testResidentSongs(t, func(process int, mode, resolution string, ready bool) {
				if !ready {
					pid, hwnd = process, 0
					enum.Call(find, 0)
					if hwnd == 0 {
						t.Fatal("resident game window not found")
					}
					show.Call(hwnd, state.cmd)
					deadline := time.Now().Add(2 * time.Second)
					for {
						if v, _, _ := state.is.Call(hwnd); v != 0 {
							break
						}
						if time.Now().After(deadline) {
							t.Fatalf("window did not become %s", state.name)
						}
						time.Sleep(10 * time.Millisecond)
					}
					show.Call(hwnd, 0) // SW_HIDE: match the resident idle state.
					return
				}
				if v, _, _ := iconic.Call(hwnd); v != 0 {
					t.Fatal("ready window is still minimized")
				}
				if mode == "windowed" {
					if v, _, _ := zoomed.Call(hwnd); v != 0 {
						t.Fatal("windowed song retained the previous maximized state")
					}
					var rect struct{ left, top, right, bottom int32 }
					if ok, _, err := clientRect.Call(hwnd, uintptr(unsafe.Pointer(&rect))); ok == 0 {
						t.Fatal("read client size", err)
					}
					dpi, _, _ := dpiOf.Call(hwnd)
					if dpi == 0 {
						t.Fatal("read window DPI")
					}
					w, h := (settings.Display{Resolution: resolution}).WindowSize()
					wantW, wantH := w*int(dpi)/96, h*int(dpi)/96
					if dw, dh := int(rect.right)-wantW, int(rect.bottom)-wantH; dw < -1 || dw > 1 || dh < -1 || dh > 1 {
						t.Fatalf("%s window is %dx%d at %d DPI; want %dx%d", resolution, rect.right, rect.bottom, dpi, wantW, wantH)
					}
				}
			})
		})
	}
}
