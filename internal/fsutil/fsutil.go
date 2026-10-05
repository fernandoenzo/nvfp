// Package fsutil provides file-copy and atomic-replacement helpers.
package fsutil

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
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
	for _, dir := range slices.Backward(missing) {
		parent := filepath.Dir(dir)
		if err := syncDir(parent); err != nil && !errors.Is(err, errDirSyncUnsupported) {
			return fmt.Errorf("syncing directory %s: %w", parent, err)
		}
	}
	return nil
}

// errDirSyncUnsupported reports a directory flush the filesystem does not
// implement. The directory entry is already created or replaced, so this is
// only about crash durability and never fails the operation.
var errDirSyncUnsupported = errors.New("directory sync not supported")

// syncDir flushes a directory entry. It is a variable so the tests can pin the
// unsupported-filesystem path, which no regular filesystem can reproduce.
var syncDir = syncDirectory

// syncReplacedDirectory flushes the directory entry after a successful
// replacement. Filesystems without directory sync are tolerated: the rename
// already happened, so only crash durability is best-effort.
func syncReplacedDirectory(dir string) error {
	if err := syncDir(dir); err != nil && !errors.Is(err, errDirSyncUnsupported) {
		return fmt.Errorf("syncing directory %s: %w", dir, err)
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

// stagedWrite is a temporary sibling file being prepared to replace a
// destination: it is flushed and closed before the replacement, and removed
// unless the replacement was committed.
type stagedWrite struct {
	file      *os.File
	path      string
	closed    bool
	committed bool
}

// newStagedWrite creates the unique temporary beside path.
func newStagedWrite(path string) (*stagedWrite, error) {
	file, err := os.CreateTemp(filepath.Dir(path), ".nvfp-tmp-*")
	if err != nil {
		return nil, fmt.Errorf("creating temporary file beside %s: %w", path, err)
	}
	return &stagedWrite{file: file, path: file.Name()}, nil
}

// stage streams src into the temporary, applies the mode and flushes the file
// before closing it: everything that must succeed before the replacement.
func (s *stagedWrite) stage(src io.Reader, perm os.FileMode) error {
	if _, err := io.Copy(s.file, src); err != nil {
		return fmt.Errorf("writing %s: %w", s.path, err)
	}
	if err := s.file.Chmod(perm.Perm()); err != nil {
		return fmt.Errorf("setting permissions on %s: %w", s.path, err)
	}
	if err := s.file.Sync(); err != nil {
		return fmt.Errorf("syncing %s: %w", s.path, err)
	}
	if err := s.file.Close(); err != nil {
		s.closed = true
		return fmt.Errorf("closing %s: %w", s.path, err)
	}
	s.closed = true
	return nil
}

// discard closes and removes the temporary unless the write was committed. It
// is deferred so every failure path leaves no temporary behind.
func (s *stagedWrite) discard() error {
	var retErr error
	if !s.closed {
		if err := s.file.Close(); err != nil {
			retErr = errors.Join(retErr, fmt.Errorf("closing %s: %w", s.path, err))
		}
	}
	if !s.committed {
		if err := os.Remove(s.path); err != nil && !errors.Is(err, os.ErrNotExist) {
			retErr = errors.Join(retErr, fmt.Errorf("removing temporary file %s: %w", s.path, err))
		}
	}
	return retErr
}

func writeAtomic(path string, src io.Reader, perm os.FileMode) (retErr error) {
	if path == "" {
		return errors.New("empty destination path")
	}
	staged, err := newStagedWrite(path)
	if err != nil {
		return err
	}
	defer func() { retErr = errors.Join(retErr, staged.discard()) }()
	if err := staged.stage(src, perm); err != nil {
		return err
	}
	if err := replaceFile(staged.path, path); err != nil {
		return fmt.Errorf("replacing %s: %w", path, err)
	}
	staged.committed = true
	return syncReplacedDirectory(filepath.Dir(path))
}
