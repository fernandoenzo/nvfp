//go:build !windows

package main

import "errors"

// nvidiaModelsDir is only implemented on Windows: the NGX models folder is an
// artifact of the NVIDIA App, and none of its paths exist elsewhere.
func nvidiaModelsDir() (string, error) {
	return "", errors.New("the NGX models folder is only available on Windows")
}
