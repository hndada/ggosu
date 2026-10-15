package play

import (
	"unsafe"

	"github.com/hajimehoshi/ebiten/v2"
	"golang.org/x/sys/windows"
)

var (
	displayDLL  = windows.NewLazySystemDLL("user32.dll")
	monitorAt   = displayDLL.NewProc("MonitorFromRect")
	monitorInfo = displayDLL.NewProc("GetMonitorInfoW")
	displayMode = displayDLL.NewProc("EnumDisplaySettingsW")
)

// These layouts match MONITORINFOEXW and DEVMODEW in winuser.h.
type monitorRect struct{ left, top, right, bottom int32 }
type monitorData struct {
	size          uint32
	monitor, work monitorRect
	flags         uint32
	device        [32]uint16
}
type modeData struct {
	device                     [32]uint16
	spec, driver, size, extra  uint16
	fields                     uint32
	position                   [4]int32
	colour, duplex, resolution int16
	tt, collate                int16
	form                       [32]uint16
	pixels                     uint16
	bits, width, height, flags uint32
	frequency                  uint32
	rest                       [8]uint32
}

func monitorHz() float64 {
	x, y := ebiten.WindowPosition()
	w, h := ebiten.WindowSize()
	r := monitorRect{int32(x), int32(y), int32(x + w), int32(y + h)}
	m, _, _ := monitorAt.Call(uintptr(unsafe.Pointer(&r)), 2) // nearest monitor
	info := monitorData{size: uint32(unsafe.Sizeof(monitorData{}))}
	if ok, _, _ := monitorInfo.Call(m, uintptr(unsafe.Pointer(&info))); ok == 0 {
		return 0
	}
	d := modeData{size: uint16(unsafe.Sizeof(modeData{}))}
	if ok, _, _ := displayMode.Call(uintptr(unsafe.Pointer(&info.device[0])), uintptr(0xffffffff), uintptr(unsafe.Pointer(&d))); ok == 0 || d.frequency <= 1 {
		return 0
	}
	return float64(d.frequency)
}
