// Package fsutil provides file-copy and atomic-replacement helpers.
package fsutil

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// CopyFile atomically copies src over dst, preserving the destination's mode
// or using the source's mode when dst does not exist.
func CopyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("opening %s: %w", src, err)
	}
	defer in.Close()

	info, err := in.Stat()
	if err != nil {
		return fmt.Errorf("stating %s: %w", src, err)
	}
	perm := info.Mode().Perm()
	if existing, err := os.Stat(dst); err == nil {
		if os.SameFile(info, existing) {
			return nil
		}
		perm = existing.Mode().Perm()
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("checking destination %s: %w", dst, err)
	}
	return writeAtomic(dst, in, perm)
}

// WriteFileAtomic replaces path from a unique temporary sibling. It syncs the
// file before replacement and the containing directory where the platform
// supports it; rename and crash-durability guarantees remain filesystem-specific.
// A directory-sync error after the rename does not undo the replacement.
func WriteFileAtomic(path string, data []byte, perm os.FileMode) error {
	mode, err := replacementMode(path, perm)
	if err != nil {
		return err
	}
	return writeAtomic(path, bytes.NewReader(data), mode)
}

// MkdirAllSync creates missing directories and syncs their parent entries on
// platforms that support directory synchronization.
func MkdirAllSync(path string, perm os.FileMode) error {
	var missing []string
	for current := filepath.Clean(path); ; current = filepath.Dir(current) {
		if _, err := os.Stat(current); err == nil {
			break
		} else if !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("checking %s: %w", current, err)
		}
		parent := filepath.Dir(current)
		if parent == current {
			break
		}
		missing = append(missing, current)
	}
	if err := os.MkdirAll(path, perm); err != nil {
		return fmt.Errorf("creating %s: %w", path, err)
	}
	for i := len(missing) - 1; i >= 0; i-- {
		parent := filepath.Dir(missing[i])
		if err := syncDirectory(parent); err != nil {
			return fmt.Errorf("syncing directory %s: %w", parent, err)
		}
	}
	return nil
}

func replacementMode(path string, fallback os.FileMode) (os.FileMode, error) {
	info, err := os.Stat(path)
	if err == nil {
		return info.Mode().Perm(), nil
	}
	if errors.Is(err, os.ErrNotExist) {
		return fallback.Perm(), nil
	}
	return 0, fmt.Errorf("checking destination %s: %w", path, err)
}

func writeAtomic(path string, src io.Reader, perm os.FileMode) (retErr error) {
	if path == "" {
		return errors.New("empty destination path")
	}
	dir := filepath.Dir(path)
	out, err := os.CreateTemp(dir, ".nvfp-tmp-*")
	if err != nil {
		return fmt.Errorf("creating temporary file beside %s: %w", path, err)
	}
	tmp := out.Name()
	closed, committed := false, false
	defer func() {
		if !closed {
			if err := out.Close(); err != nil {
				retErr = errors.Join(retErr, fmt.Errorf("closing %s: %w", tmp, err))
			}
		}
		if !committed {
			if err := os.Remove(tmp); err != nil && !errors.Is(err, os.ErrNotExist) {
				retErr = errors.Join(retErr, fmt.Errorf("removing temporary file %s: %w", tmp, err))
			}
		}
	}()

	if _, err := io.Copy(out, src); err != nil {
		return fmt.Errorf("writing %s: %w", path, err)
	}
	if err := out.Chmod(perm.Perm()); err != nil {
		return fmt.Errorf("setting permissions on %s: %w", tmp, err)
	}
	if err := out.Sync(); err != nil {
		return fmt.Errorf("syncing %s: %w", tmp, err)
	}
	if err := out.Close(); err != nil {
		closed = true
		return fmt.Errorf("closing %s: %w", tmp, err)
	}
	closed = true
	if err := replaceFile(tmp, path); err != nil {
		return fmt.Errorf("replacing %s: %w", path, err)
	}
	committed = true
	if err := syncDirectory(dir); err != nil {
		return fmt.Errorf("syncing directory %s: %w", dir, err)
	}
	return nil
}
