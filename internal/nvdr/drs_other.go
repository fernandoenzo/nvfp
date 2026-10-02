//go:build !windows

package nvdr

import "errors"

// Apply is only implemented on Windows: the driver profile database is written
// through NVAPI, which ships with the NVIDIA driver for Windows.
func Apply(reqs []Request) ([]Result, error) {
	return nil, errors.New("driver profile patching is only supported on Windows")
}

// Diagnose is only implemented on Windows, alongside Apply.
func Diagnose(reqs []Request) ([][]LookupAttempt, error) {
	return nil, errors.New("driver profile diagnostics are only supported on Windows")
}
