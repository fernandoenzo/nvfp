package main

import (
	"fmt"
	"path/filepath"

	"github.com/fernandoenzo/nvfp/internal/fsutil"
)

// restoreDB overwrites the working fingerprint.db with the pristine copy kept
// under the DAO directory, undoing every patch applied by this tool.
func restoreDB() error {
	src, err := findDAOFingerprintDB()
	if err != nil {
		return fmt.Errorf("finding the original fingerprint.db: %w", err)
	}
	dst, err := getFingerprintDBPath()
	if err != nil {
		return err
	}
	if dryRun {
		fmt.Printf("Would restore %s\n  from %s\n", dst, src)
		return nil
	}
	// The working copy may be gone entirely; recreate its directory.
	if err := fsutil.MkdirAllSync(filepath.Dir(dst), 0o755); err != nil {
		return fmt.Errorf("creating %s: %w", filepath.Dir(dst), err)
	}
	if err := fsutil.CopyFile(src, dst); err != nil {
		return fmt.Errorf("restoring %s: %w", dst, err)
	}
	fmt.Printf("Restored %s\n  from %s\n", dst, src)
	return nil
}
