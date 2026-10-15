package keyboard

import (
	"errors"
	"fmt"
	"unsafe"

	"github.com/ebitengine/purego"
)

// These are the only foreign APIs needed by the macOS receiver. Keep the
// binding/setup code out of the input callback.
type hidAPI struct {
	libs              []uintptr
	keyCallbacks      uintptr
	valueCallbacks    uintptr
	managerCreate     func(uintptr, uint32) uintptr
	checkAccess       func(uint32) uint32
	managerOpen       func(uintptr, uint32) int32
	managerClose      func(uintptr, uint32) int32
	managerMatch      func(uintptr, uintptr)
	managerSchedule   func(uintptr, uintptr, uintptr)
	managerUnschedule func(uintptr, uintptr, uintptr)
	managerCallback   func(uintptr, uintptr, uintptr)
	valueElement      func(uintptr) uintptr
	valueInteger      func(uintptr) int64
	elementPage       func(uintptr) uint32
	elementUsage      func(uintptr) uint32
	elementDevice     func(uintptr) uintptr
	stringCreate      func(uintptr, string, uint32) uintptr
	numberCreate      func(uintptr, int64, unsafe.Pointer) uintptr
	dictionaryCreate  func(uintptr, int64, uintptr, uintptr) uintptr
	dictionarySet     func(uintptr, uintptr, uintptr)
	release           func(uintptr)
	loopCurrent       func() uintptr
	loopRun           func()
	loopStop          func(uintptr)
	loopWake          func(uintptr)
	loopAddSource     func(uintptr, uintptr, uintptr)
	loopRemoveSource  func(uintptr, uintptr, uintptr)
	sourceCreate      func(uintptr, int64, unsafe.Pointer) uintptr
	sourceSignal      func(uintptr)
	objectSend        func(uintptr, uintptr) uintptr
	activeSend        func(uintptr, uintptr) bool
}

func openHID() (*hidAPI, error) {
	a := new(hidAPI)
	paths := [...]string{
		"/System/Library/Frameworks/IOKit.framework/IOKit",
		"/System/Library/Frameworks/CoreFoundation.framework/CoreFoundation",
		"/System/Library/Frameworks/AppKit.framework/AppKit",
	}
	for _, path := range paths {
		lib, err := purego.Dlopen(path, purego.RTLD_NOW|purego.RTLD_GLOBAL)
		if err != nil {
			return nil, errors.Join(fmt.Errorf("load %s: %w", path, err), a.close())
		}
		a.libs = append(a.libs, lib)
	}
	bindings := []struct {
		target any
		name   string
		lib    uintptr
	}{
		{&a.managerCreate, "IOHIDManagerCreate", a.libs[0]},
		{&a.checkAccess, "IOHIDCheckAccess", a.libs[0]},
		{&a.managerOpen, "IOHIDManagerOpen", a.libs[0]},
		{&a.managerClose, "IOHIDManagerClose", a.libs[0]},
		{&a.managerMatch, "IOHIDManagerSetDeviceMatching", a.libs[0]},
		{&a.managerSchedule, "IOHIDManagerScheduleWithRunLoop", a.libs[0]},
		{&a.managerUnschedule, "IOHIDManagerUnscheduleFromRunLoop", a.libs[0]},
		{&a.managerCallback, "IOHIDManagerRegisterInputValueCallback", a.libs[0]},
		{&a.valueElement, "IOHIDValueGetElement", a.libs[0]},
		{&a.valueInteger, "IOHIDValueGetIntegerValue", a.libs[0]},
		{&a.elementPage, "IOHIDElementGetUsagePage", a.libs[0]},
		{&a.elementUsage, "IOHIDElementGetUsage", a.libs[0]},
		{&a.elementDevice, "IOHIDElementGetDevice", a.libs[0]},
		{&a.stringCreate, "CFStringCreateWithCString", a.libs[1]},
		{&a.numberCreate, "CFNumberCreate", a.libs[1]},
		{&a.dictionaryCreate, "CFDictionaryCreateMutable", a.libs[1]},
		{&a.dictionarySet, "CFDictionarySetValue", a.libs[1]},
		{&a.release, "CFRelease", a.libs[1]},
		{&a.loopCurrent, "CFRunLoopGetCurrent", a.libs[1]},
		{&a.loopRun, "CFRunLoopRun", a.libs[1]},
		{&a.loopStop, "CFRunLoopStop", a.libs[1]},
		{&a.loopWake, "CFRunLoopWakeUp", a.libs[1]},
		{&a.loopAddSource, "CFRunLoopAddSource", a.libs[1]},
		{&a.loopRemoveSource, "CFRunLoopRemoveSource", a.libs[1]},
		{&a.sourceCreate, "CFRunLoopSourceCreate", a.libs[1]},
		{&a.sourceSignal, "CFRunLoopSourceSignal", a.libs[1]},
		{&a.objectSend, "objc_msgSend", purego.RTLD_DEFAULT},
		{&a.activeSend, "objc_msgSend", purego.RTLD_DEFAULT},
	}
	for _, binding := range bindings {
		proc, err := purego.Dlsym(binding.lib, binding.name)
		if err != nil {
			return nil, errors.Join(fmt.Errorf("resolve %s: %w", binding.name, err), a.close())
		}
		purego.RegisterFunc(binding.target, proc)
	}
	var err error
	a.keyCallbacks, err = purego.Dlsym(a.libs[1], "kCFTypeDictionaryKeyCallBacks")
	if err != nil {
		return nil, errors.Join(err, a.close())
	}
	a.valueCallbacks, err = purego.Dlsym(a.libs[1], "kCFTypeDictionaryValueCallBacks")
	if err != nil {
		return nil, errors.Join(err, a.close())
	}
	return a, nil
}

func (a *hidAPI) close() error {
	// Frameworks are process libraries too; only release our own dlopen references.
	var result error
	for i := len(a.libs) - 1; i >= 0; i-- {
		result = errors.Join(result, purego.Dlclose(a.libs[i]))
	}
	return result
}

func (a *hidAPI) keyboardMatch() (uintptr, error) {
	dict := a.dictionaryCreate(0, 2, a.keyCallbacks, a.valueCallbacks)
	if dict == 0 {
		return 0, fmt.Errorf("create HID matching dictionary")
	}
	for _, entry := range []struct {
		name  string
		value int32
	}{{"DeviceUsagePage", 1}, {"DeviceUsage", 6}} {
		key := a.stringCreate(0, entry.name, 0x08000100)             // UTF-8
		number := a.numberCreate(0, 3, unsafe.Pointer(&entry.value)) // kCFNumberSInt32Type
		if key == 0 || number == 0 {
			if key != 0 {
				a.release(key)
			}
			if number != 0 {
				a.release(number)
			}
			a.release(dict)
			return 0, fmt.Errorf("create HID keyboard matching value")
		}
		a.dictionarySet(dict, key, number)
		a.release(key)
		a.release(number)
	}
	return dict, nil
}
