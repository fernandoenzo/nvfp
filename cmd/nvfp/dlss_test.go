package main

import (
	"errors"
	"runtime"
	"strings"
	"testing"
)

// newFakeDLSSRegistry builds an injected registry that records writes and
// returns the given current state.
func newFakeDLSSRegistry(current uint32, found bool, readErr, writeErr error) (dlssRegistry, *[]uint32) {
	var writes []uint32
	return dlssRegistry{
		ReadValue:  func() (uint32, bool, error) { return current, found, readErr },
		WriteValue: func(value uint32) error { writes = append(writes, value); return writeErr },
	}, &writes
}

// TestSetDLSSDebug drives the whole indicator flow: an absent key or value is
// created, an already matching value is a no-op, any other value is
// overwritten, --dry-run writes nothing, and both registry errors are reported.
func TestSetDLSSDebug(t *testing.T) {
	tests := []struct {
		name        string
		enabled     bool
		current     uint32
		found       bool
		readErr     error
		writeErr    error
		dryRun      bool
		wantWrites  []uint32
		wantOutput  []string
		absentInOut []string
		wantErr     string
	}{
		{
			name:       "missing key and value turns the indicator on",
			enabled:    true,
			wantWrites: []uint32{dlssDebugOn},
			wantOutput: []string{"Set ShowDlssIndicator = 1024 (DLSS debug indicator on)", "restart any running game"},
		},
		{
			name:        "missing key and value turns the indicator off without the restart hint",
			enabled:     false,
			wantWrites:  []uint32{dlssDebugOff},
			wantOutput:  []string{"Set ShowDlssIndicator = 0 (DLSS debug indicator off)"},
			absentInOut: []string{"restart any running game"},
		},
		{
			name:        "already on is a no-op",
			enabled:     true,
			current:     dlssDebugOn,
			found:       true,
			wantOutput:  []string{"Nothing to do: ShowDlssIndicator is already 1024"},
			absentInOut: []string{"Note:"},
		},
		{
			name:       "already off is a no-op",
			enabled:    false,
			current:    dlssDebugOff,
			found:      true,
			wantOutput: []string{"Nothing to do: ShowDlssIndicator is already 0"},
		},
		{
			name:       "a different value is overwritten",
			enabled:    true,
			current:    1,
			found:      true,
			wantWrites: []uint32{dlssDebugOn},
			wantOutput: []string{"Set ShowDlssIndicator = 1024"},
		},
		{
			name:       "dry run never writes the missing key",
			enabled:    true,
			dryRun:     true,
			wantOutput: []string{"Would set ShowDlssIndicator = 1024 (DLSS debug indicator on)"},
		},
		{
			name:    "read failure is reported and nothing is written",
			enabled: true,
			readErr: errors.New("opening HKLM: access denied"),
			wantErr: "opening HKLM",
		},
		{
			name:       "write failure is wrapped",
			enabled:    true,
			writeErr:   errors.New("access denied"),
			wantErr:    "writing ShowDlssIndicator",
			wantWrites: []uint32{dlssDebugOn},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			oldDry, oldElevated := dryRun, elevatedFlag
			t.Cleanup(func() { dryRun, elevatedFlag = oldDry, oldElevated })
			dryRun, elevatedFlag = tc.dryRun, true
			reg, writes := newFakeDLSSRegistry(tc.current, tc.found, tc.readErr, tc.writeErr)

			var err error
			output := captureStdout(t, func() { err = setDLSSDebug(tc.enabled, reg) })

			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("setDLSSDebug() error = %v, want it to contain %q", err, tc.wantErr)
				}
			} else if err != nil {
				t.Fatalf("setDLSSDebug() error: %v", err)
			}
			if len(*writes) != len(tc.wantWrites) {
				t.Fatalf("writes = %v, want %v", *writes, tc.wantWrites)
			}
			for i, want := range tc.wantWrites {
				if (*writes)[i] != want {
					t.Errorf("write %d = %d, want %d", i, (*writes)[i], want)
				}
			}
			for _, want := range tc.wantOutput {
				if !strings.Contains(output, want) {
					t.Errorf("output misses %q:\n%s", want, output)
				}
			}
			for _, unwanted := range tc.absentInOut {
				if strings.Contains(output, unwanted) {
					t.Errorf("output unexpectedly contains %q:\n%s", unwanted, output)
				}
			}
		})
	}
}

// TestDLSSDebugDispatch checks the flag wiring: the mode must reach the
// registry step before run() resolves the games manifest, and both flags are
// mutually exclusive with each other and with the manifest modes.
func TestDLSSDebugDispatch(t *testing.T) {
	t.Run("dispatches before resolving the manifest", func(t *testing.T) {
		if runtime.GOOS == "windows" {
			t.Skip("the real registry access would request elevation on Windows")
		}
		oldFlag, oldPath, oldElevated := dlssDebugFlag, gamesJSONPath, elevatedFlag
		t.Cleanup(func() { dlssDebugFlag, gamesJSONPath, elevatedFlag = oldFlag, oldPath, oldElevated })
		dlssDebugFlag, elevatedFlag = true, true
		gamesJSONPath = t.TempDir() + "/does-not-exist.json"

		err := run(nil, nil)
		if err == nil {
			t.Fatal("run() with --dlss-debug returned nil without a reachable registry")
		}
		if !strings.Contains(err.Error(), "only available on Windows") {
			t.Errorf("run() error = %v, want the DLSS registry error", err)
		}
		if strings.Contains(err.Error(), "loading games database") {
			t.Errorf("run() resolved the games manifest before the DLSS step: %v", err)
		}
	})

	t.Run("rejects combined flags", func(t *testing.T) {
		snapshotFlagGlobals(t)
		for _, args := range [][]string{
			{"--dlss-debug", "--no-dlss-debug"},
			{"--dlss-debug", "--list"},
			{"--no-dlss-debug", "--reset-models"},
			{"--dlss-debug", "--version"},
		} {
			t.Run(strings.Join(args, " "), func(t *testing.T) {
				cmd := newRootCmd()
				cmd.SetArgs(args)
				cmd.SetOut(nopWriter{})
				cmd.SetErr(nopWriter{})
				err := cmd.Execute()
				if err == nil || !strings.Contains(err.Error(), "if any flags in the group") {
					t.Fatalf("Execute(%v) error = %v, want a mutually exclusive flag error", args, err)
				}
			})
		}
	})
}

// snapshotFlagGlobals restores every flag-bound global when the test ends:
// cobra assigns them while parsing, even when Execute fails on validation.
func snapshotFlagGlobals(t *testing.T) {
	t.Helper()
	oldDry, oldList, oldRestore := dryRun, listOnly, restoreFlag
	oldVersion, oldNoDriver, oldElevated := versionFlag, noDriverFlag, elevatedFlag
	oldDoctor, oldResetModels := doctorFlag, resetModelsFlag
	oldDLSS, oldNoDLSS := dlssDebugFlag, noDLSSDebugFlag
	oldGame, oldGamesJSON := gameFilter, gamesJSONPath
	t.Cleanup(func() {
		dryRun, listOnly, restoreFlag = oldDry, oldList, oldRestore
		versionFlag, noDriverFlag, elevatedFlag = oldVersion, oldNoDriver, oldElevated
		doctorFlag, resetModelsFlag = oldDoctor, oldResetModels
		dlssDebugFlag, noDLSSDebugFlag = oldDLSS, oldNoDLSS
		gameFilter, gamesJSONPath = oldGame, oldGamesJSON
	})
}

// nopWriter discards cobra's own usage output during flag-validation tests.
type nopWriter struct{}

func (nopWriter) Write(p []byte) (int, error) { return len(p), nil }
