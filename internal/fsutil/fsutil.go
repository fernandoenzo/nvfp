// Package fsutil holds the filesystem helpers shared across layers: the
// restore flow, the NGX repair and any future code that copies a file over
// another.
package fsutil

import (
	"fmt"
	"io"
	"os"
)

// CopyFile copies src over dst, overwriting dst if it exists, and syncs it to
// disk. Whether overwriting is acceptable is the caller's decision.
func CopyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("opening %s: %w", src, err)
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return fmt.Errorf("creating %s: %w", dst, err)
	}
	defer out.Close()

	if _, err := io.Copy(out, in); err != nil {
		return fmt.Errorf("copying %s to %s: %w", src, dst, err)
	}
	if err := out.Sync(); err != nil {
		return fmt.Errorf("syncing %s: %w", dst, err)
	}
	return nil
}
