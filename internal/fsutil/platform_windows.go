//go:build windows

package fsutil

import (
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
	if err := windows.MoveFileEx(oldPtr, newPtr, windows.MOVEFILE_REPLACE_EXISTING|windows.MOVEFILE_WRITE_THROUGH); err != nil {
		return &os.LinkError{Op: "rename", Old: oldpath, New: newpath, Err: err}
	}
	return nil
}

func extendedPath(path string) (string, error) {
	if strings.HasPrefix(path, `\\?\`) {
		return path, nil
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	if strings.HasPrefix(absolute, `\\`) {
		return `\\?\UNC\` + strings.TrimPrefix(absolute, `\\`), nil
	}
	return `\\?\` + absolute, nil
}

// MoveFileEx uses MOVEFILE_WRITE_THROUGH for the replacement. Windows does not
// expose a portable directory-fsync operation through os.File.
func syncDirectory(string) error { return nil }
