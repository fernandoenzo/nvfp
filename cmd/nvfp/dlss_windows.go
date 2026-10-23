//go:build windows

package main

import (
	"errors"
	"fmt"

	"golang.org/x/sys/windows/registry"
)

func ngxDlssRegistry() dlssRegistry {
	return dlssRegistry{ReadValue: readDLSSIndicator, WriteValue: writeDLSSIndicator}
}

var errDLSSNotDWORD = errors.New(dlssIndicatorName + " is not a DWORD: refusing to overwrite it")

// errNoNVIDIA stops the flow instead of fabricating the NVIDIA tree on a machine
// that has no NVIDIA software.
var errNoNVIDIA = errors.New(`HKLM\` + nvidiaRegGlobal + ` does not exist: is the NVIDIA driver or the NVIDIA App installed?`)

func openNVIDIAGlobal(access uint32) (registry.Key, error) {
	key, err := registry.OpenKey(registry.LOCAL_MACHINE, nvidiaRegGlobal, access)
	if err != nil {
		if errors.Is(err, registry.ErrNotExist) {
			return 0, errNoNVIDIA
		}
		return 0, fmt.Errorf("opening HKLM\\%s: %w", nvidiaRegGlobal, err)
	}
	return key, nil
}

func openNGXKey(parent registry.Key, access uint32) (registry.Key, bool, error) {
	key, err := registry.OpenKey(parent, ngxKeyName, access)
	switch {
	case errors.Is(err, registry.ErrNotExist):
		return 0, false, nil
	case err != nil:
		return 0, false, fmt.Errorf("opening HKLM\\%s: %w", ngxRegKey, err)
	}
	return key, true, nil
}

func readDLSSIndicator() (uint32, bool, error) {
	parent, err := openNVIDIAGlobal(registry.QUERY_VALUE)
	if err != nil {
		return 0, false, err
	}
	defer parent.Close()
	key, found, err := openNGXKey(parent, registry.QUERY_VALUE)
	if err != nil || !found {
		return 0, false, err
	}
	defer key.Close()
	return dlssValue(key)
}

func dlssValue(key registry.Key) (uint32, bool, error) {
	value, typ, err := key.GetIntegerValue(dlssIndicatorName)
	switch {
	case errors.Is(err, registry.ErrNotExist):
		return 0, false, nil
	case errors.Is(err, registry.ErrUnexpectedType):
		return 0, false, errDLSSNotDWORD
	case err != nil:
		return 0, false, fmt.Errorf("reading %s: %w", dlssIndicatorName, err)
	case typ != registry.DWORD:
		return 0, false, errDLSSNotDWORD
	}
	return uint32(value), true, nil
}

// writeDLSSIndicator creates the leaf under the parent handle: passing the whole
// path to CreateKey would invent every missing intermediate key.
func writeDLSSIndicator(value uint32) error {
	parent, err := openNVIDIAGlobal(registry.WRITE)
	if err != nil {
		return err
	}
	defer parent.Close()
	key, _, err := registry.CreateKey(parent, ngxKeyName, registry.WRITE)
	if err != nil {
		return fmt.Errorf("opening HKLM\\%s: %w", ngxRegKey, err)
	}
	defer key.Close()
	if err := key.SetDWordValue(dlssIndicatorName, value); err != nil {
		return fmt.Errorf("writing %s: %w", dlssIndicatorName, err)
	}
	return nil
}
