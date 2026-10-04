//go:build unix

package fsutil

import (
	"errors"
	"fmt"
	"os"
	"syscall"
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
		// Filesystems without directory sync (some network and FUSE mounts)
		// must not fail the write: the rename has already happened.
		if errors.Is(err, syscall.EINVAL) || errors.Is(err, syscall.ENOTSUP) || errors.Is(err, syscall.ENOSYS) {
			return errDirSyncUnsupported
		}
		return fmt.Errorf("flushing directory %s: %w", path, err)
	}
	return nil
}
