//go:build !windows

package ngx

import "errors"

// Root is only implemented on Windows: the NGX OTA cache is an artifact of the
// NVIDIA App, and none of its paths exist elsewhere.
func Root() (string, error) {
	return "", errors.New("the NGX OTA manifest is only available on Windows")
}
