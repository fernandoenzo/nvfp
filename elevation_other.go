//go:build !windows

package main

import "errors"

// errElevationCancelled reports that the user dismissed the UAC prompt; off
// Windows there is never a prompt to answer.
var errElevationCancelled = errors.New("elevation cancelled")

// isElevated always reports true off Windows: there is no UAC to satisfy and
// the driver-profile step is skipped on other platforms anyway.
func isElevated() bool { return true }

// relaunchElevated is only implemented on Windows.
func relaunchElevated() (int, error) {
	return 0, errors.New("elevation is only supported on Windows")
}
