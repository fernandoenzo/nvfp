//go:build !windows

package main

import "errors"

func ngxDlssRegistry() dlssRegistry {
	fail := errors.New("the DLSS debug indicator is only available on Windows")
	return dlssRegistry{
		ReadValue:  func() (uint32, bool, error) { return 0, false, fail },
		WriteValue: func(uint32) error { return fail },
	}
}
