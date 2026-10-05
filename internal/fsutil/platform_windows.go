//go:build windows

package fsutil

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/sys/windows"
)

func replaceFile(oldpath, newpath string) error {
	oldPath, err := extendedPath(oldpath)
	if err != nil {
		return fmt.Errorf("preparing %s: %w", oldpath, err)
	}
	newPath, err := extendedPath(newpath)
	if err != nil {
		return fmt.Errorf("preparing %s: %w", newpath, err)
	}
	oldPtr, err := windows.UTF16PtrFromString(oldPath)
	if err != nil {
		return fmt.Errorf("encoding %s: %w", oldpath, err)
	}
	newPtr, err := windows.UTF16PtrFromString(newPath)
	if err != nil {
		return fmt.Errorf("encoding %s: %w", newpath, err)
	}
	err = windows.MoveFileEx(oldPtr, newPtr, windows.MOVEFILE_REPLACE_EXISTING|windows.MOVEFILE_WRITE_THROUGH)
	if err != nil {
		return &os.LinkError{Op: "rename", Old: oldpath, New: newpath, Err: replaceHint(err)}
	}
	return nil
}

// replaceHint explains the two failures a locked destination produces, which
// are otherwise indistinguishable from a genuinely broken filesystem.
func replaceHint(err error) error {
	switch {
	case errors.Is(err, windows.ERROR_SHARING_VIOLATION):
		return fmt.Errorf("%w (the file is open in another program, e.g. NVIDIA App: close it and retry)", err)
	case errors.Is(err, windows.ERROR_ACCESS_DENIED):
		return fmt.Errorf("%w (access denied: an elevated process or antivirus may be holding the file)", err)
	default:
		return err
	}
}

func extendedPath(path string) (string, error) {
	if strings.HasPrefix(path, `\\?\`) {
		return path, nil
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	if rest, ok := strings.CutPrefix(absolute, `\\`); ok {
		return `\\?\UNC\` + rest, nil
	}
	return `\\?\` + absolute, nil
}

// MoveFileEx uses MOVEFILE_WRITE_THROUGH for the replacement. Windows does not
// expose a portable directory-fsync operation through os.File.
func syncDirectory(string) error { return nil }
