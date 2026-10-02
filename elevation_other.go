//go:build !windows

package main

import (
	"errors"
	"os"
)

// errElevationCancelled reports that the user dismissed the UAC prompt; off
// Windows there is never a prompt to answer.
var errElevationCancelled = errors.New("elevation cancelled")

// isElevated always reports true off Windows: there is no UAC to satisfy and
// the driver-profile step is skipped on other platforms anyway.
func isElevated() (bool, error) { return true, nil }

// relaunchElevated is only implemented on Windows.
func relaunchElevated() (int, error) {
	return 0, errors.New("elevation is only supported on Windows")
}

// stdoutIsPiped reports whether the output is redirected; off Windows there is
// no console window to hold open, so the pause is a no-op anyway.
func stdoutIsPiped() bool {
	info, err := os.Stdout.Stat()
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeCharDevice == 0
}

// pauseBeforeExit is a no-op off Windows.
func pauseBeforeExit() {}
