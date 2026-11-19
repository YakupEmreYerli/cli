//go:build !windows

package ui

import "os"

// EnableVT reports whether f understands ANSI escape sequences. Terminals
// on Unix-like systems always do.
func EnableVT(*os.File) bool { return true }
