package launch

import (
	"os"
	"sync"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	user32      = windows.NewLazySystemDLL("user32.dll")
	allowFocus  = user32.NewProc("AllowSetForegroundWindow")
	foreground  = user32.NewProc("SetForegroundWindow")
	enumWindow  = user32.NewProc("EnumWindows")
	windowPID   = user32.NewProc("GetWindowThreadProcessId")
	windowClass = user32.NewProc("GetClassNameW")
	visible     = user32.NewProc("IsWindowVisible")
	focusMu     sync.Mutex
	search      windowSearch
	findWindow  = windows.NewCallback(func(hwnd, _ uintptr) uintptr {
		var pid uint32
		windowPID.Call(hwnd, uintptr(unsafe.Pointer(&pid)))
		if pid != search.pid {
			return 1
		}
		if ok, _, _ := visible.Call(hwnd); ok == 0 {
			return 1
		}
		var class [64]uint16
		windowClass.Call(hwnd, uintptr(unsafe.Pointer(&class[0])), uintptr(len(class)))
		if windows.UTF16ToString(class[:]) != "GLFW30" {
			return 1
		}
		search.hwnd = hwnd
		return 0
	})
)

type windowSearch struct {
	pid  uint32
	hwnd uintptr
}

// AllowFocus lets the menu's existing game process receive foreground input
// when Play is clicked. Windows can decline if another app owns the foreground.
func AllowFocus(pid int) { allowFocus.Call(uintptr(pid)) }

// Focus activates only this process's Ebitengine window. Unlike GLFW's SetFocus
// wrapper, a declined foreground request does not terminate an otherwise valid
// song. Showing the window remains Ebitengine's responsibility.
func Focus() {
	focusMu.Lock()
	defer focusMu.Unlock()
	search = windowSearch{pid: uint32(os.Getpid())}
	enumWindow.Call(findWindow, 0)
	if search.hwnd != 0 {
		foreground.Call(search.hwnd)
	}
}
