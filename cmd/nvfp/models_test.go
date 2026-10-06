package main

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// TestResetModels drives the --reset-models step: the folder is deleted with
// the logoff/logon reminder, a misresolved path is refused, --dry-run writes
// nothing and a missing folder is a no-op.
func TestResetModels(t *testing.T) {
	fixture := func(t *testing.T) string {
		t.Helper()
		dir := filepath.Join(t.TempDir(), "models")
		if err := os.MkdirAll(filepath.Join(dir, "sl_sdk_0"), 0o755); err != nil {
			t.Fatalf("creating fixture: %v", err)
		}
		if err := os.WriteFile(filepath.Join(dir, "sl_sdk_0", "payload.dll"), []byte("bytes"), 0o644); err != nil {
			t.Fatalf("writing fixture file: %v", err)
		}
		return dir
	}
	flow := func(t *testing.T, dir string, dry bool) func() (string, error) {
		t.Helper()
		oldDry := dryRun
		t.Cleanup(func() { dryRun = oldDry })
		dryRun = dry
		return func() (string, error) { return dir, nil }
	}

	t.Run("deletes the folder and prints the reminder", func(t *testing.T) {
		dir := fixture(t)
		output := captureStdout(t, func() {
			if err := resetModels(flow(t, dir, false)); err != nil {
				t.Fatalf("resetModels() error: %v", err)
			}
		})
		if _, err := os.Stat(dir); !os.IsNotExist(err) {
			t.Errorf("models folder still exists: %v", err)
		}
		if !strings.Contains(output, "Deleted ") || !strings.Contains(output, "log out of Windows") {
			t.Errorf("output misses the deletion or the reminder:\n%s", output)
		}
		if !strings.Contains(output, "open any DLSS game once") {
			t.Errorf("output misses the DLSS warm-up note:\n%s", output)
		}
	})

	t.Run("refuses a path that is not a models directory", func(t *testing.T) {
		dir := t.TempDir()
		err := resetModels(flow(t, dir, false))
		if err == nil || !strings.Contains(err.Error(), "refusing to delete") {
			t.Errorf("resetModels() error = %v, want a refusal", err)
		}
		if _, statErr := os.Stat(dir); statErr != nil {
			t.Errorf("refused directory was touched: %v", statErr)
		}
	})

	t.Run("dry run deletes nothing", func(t *testing.T) {
		dir := fixture(t)
		output := captureStdout(t, func() {
			if err := resetModels(flow(t, dir, true)); err != nil {
				t.Fatalf("resetModels() with --dry-run error: %v", err)
			}
		})
		if _, err := os.Stat(dir); err != nil {
			t.Errorf("--dry-run deleted the folder: %v", err)
		}
		if !strings.Contains(output, "Would delete ") {
			t.Errorf("dry-run output does not describe the deletion:\n%s", output)
		}
	})

	t.Run("missing folder reports nothing to do", func(t *testing.T) {
		dir := filepath.Join(t.TempDir(), "models")
		output := captureStdout(t, func() {
			if err := resetModels(flow(t, dir, false)); err != nil {
				t.Fatalf("resetModels() error: %v", err)
			}
		})
		if !strings.Contains(output, "Nothing to do") {
			t.Errorf("missing folder not reported as nothing to do:\n%s", output)
		}
	})

	// Dispatch: --reset-models must reach the models reset before run()
	// resolves the games manifest. Off Windows the real folder is unreachable
	// by design, which makes the dispatch observable through the error; on
	// Windows the reset would touch the real models folder, so the assertion
	// cannot run.
	t.Run("dispatches before resolving the manifest", func(t *testing.T) {
		if runtime.GOOS == "windows" {
			t.Skip("nvidiaModelsDir reads the real models folder on Windows")
		}
		oldFlag, oldPath := resetModelsFlag, gamesJSONPath
		t.Cleanup(func() { resetModelsFlag, gamesJSONPath = oldFlag, oldPath })
		resetModelsFlag = true
		gamesJSONPath = filepath.Join(t.TempDir(), "does-not-exist.json")

		err := run(nil, nil)
		if err == nil {
			t.Fatal("run() with --reset-models returned nil without a reachable models folder")
		}
		if strings.Contains(err.Error(), "loading games database") {
			t.Errorf("run() resolved the games manifest before the models reset: %v", err)
		}
	})
}
