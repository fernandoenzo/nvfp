//go:build unix

package fsutil

import (
	"errors"
	"fmt"
	"os"
)

func replaceFile(oldpath, newpath string) error {
	return os.Rename(oldpath, newpath)
}

func syncDirectory(path string) (retErr error) {
	dir, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("opening directory %s: %w", path, err)
	}
	defer func() {
		if err := dir.Close(); err != nil {
			retErr = errors.Join(retErr, fmt.Errorf("closing directory %s: %w", path, err))
		}
	}()
	if err := dir.Sync(); err != nil {
		return fmt.Errorf("flushing directory %s: %w", path, err)
	}
	return nil
}
