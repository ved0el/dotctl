// Package platform reports static host facts: OS, architecture, and home dir.
//
// It performs no process execution or capability probing — those concerns live
// with their consumers (e.g. package-manager availability lives in internal/pkg).
package platform

import (
	"os"
	"runtime"
)

// OS returns the operating system name (e.g. "darwin", "linux", "windows").
func OS() string { return runtime.GOOS }

// Arch returns the CPU architecture (e.g. "arm64", "amd64").
func Arch() string { return runtime.GOARCH }

// HomeDir returns the current user's home directory.
func HomeDir() (string, error) { return os.UserHomeDir() }
