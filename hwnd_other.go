//go:build !windows

package main

import "unsafe"

// mainWindowHandle is only needed on Windows, where the native menu attaches
// to an HWND; the macOS menu bar is application-global and Linux has none.
func mainWindowHandle() unsafe.Pointer { return nil }
