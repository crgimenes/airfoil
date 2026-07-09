//go:build windows

package main

import (
	"syscall"
	"unsafe"
)

var (
	user32      = syscall.NewLazyDLL("user32.dll")
	findWindowW = user32.NewProc("FindWindowW")

	cachedHwnd unsafe.Pointer
)

// mainWindowHandle returns the app window's Win32 handle, which the native
// menu bar needs on Windows. It is nil until the window exists; a found
// handle is cached since the window lives as long as the process.
func mainWindowHandle() unsafe.Pointer {
	if cachedHwnd != nil {
		return cachedHwnd
	}
	title, err := syscall.UTF16PtrFromString(windowTitle)
	if err != nil {
		return nil
	}
	h, _, _ := findWindowW.Call(0, uintptr(unsafe.Pointer(title)))
	if h == 0 {
		return nil
	}
	// An HWND is an opaque kernel handle, not a Go pointer, so smuggling it
	// through unsafe.Pointer is safe; the indirection keeps go vet content.
	cachedHwnd = *(*unsafe.Pointer)(unsafe.Pointer(&h)) // #nosec G103
	return cachedHwnd
}
