package main

import (
	"fmt"
	"os"
	"path/filepath"
)

// resetReloginHint follows a real models-folder reset: the folder comes back
// only at the next session.
const resetReloginHint = "Note: log out of Windows and sign back in (no reboot needed); NVIDIA rebuilds the folder at the next session."

// resetDLSSHint follows a real models-folder reset: the dlss-family payloads
// inside models are only created when a DLSS game first runs.
const resetDLSSHint = "Note: after signing back in, open any DLSS game once (until its main menu) and close it before playing: NVIDIA creates the dlss payloads in models only on that first run."

// runResetModels deletes the NGX models folder, holding the elevated child's
// console open when there is a real window to read. resolveDir is
// nvidiaModelsDir in production; tests inject a fixture directory instead.
func runResetModels(resolveDir func() (string, error)) error {
	if elevatedFlag && !stdoutIsPiped() {
		defer pauseBeforeExit()
	}
	return resetModels(resolveDir)
}

// resetModels deletes the NGX OTA cache directory so NVIDIA rebuilds it whole
// at the next session. The path must end in "models": anything else is refused
// before any check, so a misresolved path can never delete an unrelated tree.
func resetModels(resolveDir func() (string, error)) error {
	dir, err := resolveDir()
	if err != nil {
		return err
	}
	if filepath.Base(filepath.Clean(dir)) != "models" {
		return fmt.Errorf("refusing to delete %s: it is not an NGX models directory", dir)
	}
	if _, err := os.Stat(dir); err != nil {
		if os.IsNotExist(err) {
			fmt.Printf("Nothing to do: %s does not exist\n", dir)
			return nil
		}
		return fmt.Errorf("reading %s: %w", dir, err)
	}
	if dryRun {
		fmt.Printf("Would delete %s\n", dir)
		return nil
	}
	ensureElevated(true)
	if err := os.RemoveAll(dir); err != nil {
		return fmt.Errorf("deleting %s: %w", dir, err)
	}
	fmt.Printf("Deleted %s\n", dir)
	fmt.Println(resetReloginHint)
	fmt.Println(resetDLSSHint)
	return nil
}
