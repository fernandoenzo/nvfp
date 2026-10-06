//go:build windows

package main

import "golang.org/x/sys/windows/registry"

const (
	// ngxRegKey is where the NVIDIA App records the NGX OTA cache location.
	ngxRegKey = `SOFTWARE\NVIDIA Corporation\Global\NGXCore`
	// defaultModelsDir is the cache location when the registry points elsewhere.
	defaultModelsDir = `C:\ProgramData\NVIDIA\NGX\models`
)

// nvidiaModelsDir returns the NGX OTA cache directory (the "models" folder),
// preferring the OTACachePath the NVIDIA App writes to the registry. A missing
// key or value yields the default.
func nvidiaModelsDir() (string, error) {
	key, err := registry.OpenKey(registry.LOCAL_MACHINE, ngxRegKey, registry.QUERY_VALUE)
	if err != nil {
		return defaultModelsDir, nil
	}
	defer key.Close()
	path, _, err := key.GetStringValue("OTACachePath")
	if err != nil || path == "" {
		return defaultModelsDir, nil
	}
	return path, nil
}
