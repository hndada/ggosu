package main

import (
	"errors"
	"fmt"
	"os"
	"unsafe"

	"golang.org/x/sys/windows"
)

var errForeground = errors.New("OS input check requires the POC foreground window")

var (
	checkUser32     = windows.NewLazySystemDLL("user32.dll")
	checkForeground = checkUser32.NewProc("GetForegroundWindow")
	checkWindowPID  = checkUser32.NewProc("GetWindowThreadProcessId")
	checkSendInput  = checkUser32.NewProc("SendInput")
)

// INPUT's union is 32 bytes on Windows amd64. Explicit storage keeps the
// KEYBDINPUT data at the native 8-byte offset without CGo.
type keyInput struct {
	Kind    uint32
	Padding uint32
	Data    [4]uint64
}

// sendPair is only used by the explicit -oscheck test. Refuse to send unless
// this POC owns the foreground window. It sends no shortcuts or system keys.
// Down and up are submitted together so the test never leaves a held key.
func sendPair(key uint16) error {
	hwnd, _, _ := checkForeground.Call()
	var pid uint32
	checkWindowPID.Call(hwnd, uintptr(unsafe.Pointer(&pid)))
	if hwnd == 0 || pid != uint32(os.Getpid()) {
		return errForeground
	}
	input := [2]keyInput{{Kind: 1}, {Kind: 1}} // INPUT_KEYBOARD
	input[0].Data[0] = uint64(key)
	input[1].Data[0] = uint64(key) | uint64(2)<<32 // KEYEVENTF_KEYUP
	n, _, err := checkSendInput.Call(2, uintptr(unsafe.Pointer(&input[0])), unsafe.Sizeof(input[0]))
	if n != 2 {
		return fmt.Errorf("send POC test key: %w", err)
	}
	return nil
}
