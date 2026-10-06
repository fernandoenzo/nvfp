package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRestoreDB(t *testing.T) {
	relOntology := filepath.Join("NVIDIA Corporation", "NVIDIA App",
		"NvBackend", "ApplicationOntology", "data", "fingerprint.db")
	relDAO := filepath.Join("NVIDIA Corporation", "NVIDIA App",
		"NvBackend", "DAO", "abc123", "fingerprint.db")

	// setup builds the NVIDIA App tree; withPatched=false leaves the working
	// copy missing, as it is on a fresh install, to prove a restore recreates it.
	setup := func(t *testing.T, localAppData string, withPatched bool) (working, pristine string) {
		t.Helper()
		pristine = filepath.Join(localAppData, relDAO)
		if err := os.MkdirAll(filepath.Dir(pristine), 0o755); err != nil {
			t.Fatalf("MkdirAll DAO: %v", err)
		}
		if err := os.WriteFile(pristine, []byte("pristine content"), 0o644); err != nil {
			t.Fatalf("writing DAO copy: %v", err)
		}
		working = filepath.Join(localAppData, relOntology)
		if !withPatched {
			return working, pristine
		}
		if err := os.MkdirAll(filepath.Dir(working), 0o755); err != nil {
			t.Fatalf("MkdirAll ontology: %v", err)
		}
		if err := os.WriteFile(working, []byte("patched content that is longer"), 0o644); err != nil {
			t.Fatalf("writing working copy: %v", err)
		}
		return working, pristine
	}

	tests := []struct {
		name        string
		withPatched bool
	}{
		{name: "overwrites the patched working copy", withPatched: true},
		{name: "recreates a missing working copy", withPatched: false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			t.Setenv("LOCALAPPDATA", tmpDir)
			working, pristine := setup(t, tmpDir, tc.withPatched)

			if err := restoreDB(); err != nil {
				t.Fatalf("restoreDB() error: %v", err)
			}

			got, err := os.ReadFile(working)
			if err != nil {
				t.Fatalf("reading restored working copy: %v", err)
			}
			if string(got) != "pristine content" {
				t.Errorf("working copy = %q, want %q", got, "pristine content")
			}

			// The DAO copy is the restore source: it must survive untouched.
			stillPristine, err := os.ReadFile(pristine)
			if err != nil {
				t.Fatalf("reading DAO copy: %v", err)
			}
			if string(stillPristine) != "pristine content" {
				t.Errorf("DAO copy = %q, want %q", stillPristine, "pristine content")
			}
		})
	}

	// No DAO copy → error, and the working copy is left as it was.
	t.Run("fails when there is no DAO copy", func(t *testing.T) {
		tmpDir := t.TempDir()
		t.Setenv("LOCALAPPDATA", tmpDir)
		working, _ := setup(t, tmpDir, true)
		if err := os.RemoveAll(filepath.Join(tmpDir, "NVIDIA Corporation", "NVIDIA App", "NvBackend", "DAO")); err != nil {
			t.Fatalf("removing DAO: %v", err)
		}

		if err := restoreDB(); err == nil {
			t.Fatal("restoreDB() should fail without a DAO copy")
		}
		got, err := os.ReadFile(working)
		if err != nil {
			t.Fatalf("reading working copy: %v", err)
		}
		if string(got) != "patched content that is longer" {
			t.Errorf("working copy = %q, want it untouched", got)
		}
	})

	// --dry-run must not touch the working copy, matching its contract.
	t.Run("dry-run writes nothing", func(t *testing.T) {
		tmpDir := t.TempDir()
		t.Setenv("LOCALAPPDATA", tmpDir)
		working, _ := setup(t, tmpDir, true)

		oldDryRun := dryRun
		defer func() { dryRun = oldDryRun }()
		dryRun = true

		if err := restoreDB(); err != nil {
			t.Fatalf("restoreDB() error: %v", err)
		}

		got, err := os.ReadFile(working)
		if err != nil {
			t.Fatalf("reading working copy: %v", err)
		}
		if string(got) != "patched content that is longer" {
			t.Errorf("working copy = %q, want it untouched by --dry-run", got)
		}
	})

	// LOCALAPPDATA unset → error.
	t.Run("fails without LOCALAPPDATA", func(t *testing.T) {
		t.Setenv("LOCALAPPDATA", "")
		if err := restoreDB(); err == nil {
			t.Error("restoreDB() should fail when LOCALAPPDATA is not set")
		}
	})
}

func TestRestoreFlagSkipsManifestResolution(t *testing.T) {
	// --restore must be a pure local file operation: with an invalid
	// --games-json it still restores, proving run() never resolves the manifest.
	tmpDir := t.TempDir()
	t.Setenv("LOCALAPPDATA", tmpDir)

	pristine := filepath.Join(tmpDir, "NVIDIA Corporation", "NVIDIA App",
		"NvBackend", "DAO", "abc123", "fingerprint.db")
	if err := os.MkdirAll(filepath.Dir(pristine), 0o755); err != nil {
		t.Fatalf("MkdirAll DAO: %v", err)
	}
	if err := os.WriteFile(pristine, []byte("pristine content"), 0o644); err != nil {
		t.Fatalf("writing DAO copy: %v", err)
	}

	oldFlag, oldPath := restoreFlag, gamesJSONPath
	defer func() { restoreFlag, gamesJSONPath = oldFlag, oldPath }()
	restoreFlag = true
	gamesJSONPath = filepath.Join(tmpDir, "does-not-exist.json")

	if err := run(nil, nil); err != nil {
		t.Fatalf("run() with --restore error: %v", err)
	}

	working := filepath.Join(tmpDir, "NVIDIA Corporation", "NVIDIA App",
		"NvBackend", "ApplicationOntology", "data", "fingerprint.db")
	got, err := os.ReadFile(working)
	if err != nil {
		t.Fatalf("reading restored working copy: %v", err)
	}
	if string(got) != "pristine content" {
		t.Errorf("working copy = %q, want %q", got, "pristine content")
	}
}
