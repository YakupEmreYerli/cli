//go:build windows

package ui

import (
	"os"

	"golang.org/x/sys/windows"
)

// EnableVT turns on ANSI escape sequence processing for a Windows console
// handle. It reports false when the console does not support it (very old
// Windows versions), in which case colors must stay off.
func EnableVT(f *os.File) bool {
	h := windows.Handle(f.Fd())
	var mode uint32
	if err := windows.GetConsoleMode(h, &mode); err != nil {
		return false
	}
	if mode&windows.ENABLE_VIRTUAL_TERMINAL_PROCESSING != 0 {
		return true
	}
	return windows.SetConsoleMode(h, mode|windows.ENABLE_VIRTUAL_TERMINAL_PROCESSING) == nil
}
