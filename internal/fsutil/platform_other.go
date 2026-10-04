//go:build !unix && !windows

package fsutil

import "os"

func replaceFile(oldpath, newpath string) error {
	return os.Rename(oldpath, newpath)
}

func syncDirectory(string) error { return nil }
