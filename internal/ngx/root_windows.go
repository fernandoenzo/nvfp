//go:build windows

package ngx

import "golang.org/x/sys/windows/registry"

const (
	// regKey is where the NVIDIA App records the NGX OTA cache location.
	regKey = `SOFTWARE\NVIDIA Corporation\Global\NGXCore`
	// defaultRoot is the cache location when the registry points elsewhere.
	defaultRoot = `C:\ProgramData\NVIDIA\NGX\models`
)

// Root returns the NGX OTA cache directory, preferring the OTACachePath the
// NVIDIA App writes to the registry. A missing key or value yields the
// default, as the original PowerShell did.
func Root() (string, error) {
	key, err := registry.OpenKey(registry.LOCAL_MACHINE, regKey, registry.QUERY_VALUE)
	if err != nil {
		return defaultRoot, nil
	}
	defer key.Close()
	path, _, err := key.GetStringValue("OTACachePath")
	if err != nil || path == "" {
		return defaultRoot, nil
	}
	return path, nil
}
