package fsutil

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCopyFileOverwritesDestination(t *testing.T) {
	tmpDir := t.TempDir()
	srcPath := filepath.Join(tmpDir, "source.db")
	dstPath := filepath.Join(tmpDir, "dest.db")

	if err := os.WriteFile(srcPath, []byte("pristine content"), 0o644); err != nil {
		t.Fatalf("writing source: %v", err)
	}
	// Destination already patched: restore must replace it, not preserve it.
	if err := os.WriteFile(dstPath, []byte("patched content that is longer"), 0o644); err != nil {
		t.Fatalf("writing destination: %v", err)
	}

	if err := CopyFile(srcPath, dstPath); err != nil {
		t.Fatalf("CopyFile failed: %v", err)
	}

	got, err := os.ReadFile(dstPath)
	if err != nil {
		t.Fatalf("reading destination: %v", err)
	}
	if string(got) != "pristine content" {
		t.Errorf("destination = %q, want %q", got, "pristine content")
	}

	// Source must be left untouched.
	gotSrc, err := os.ReadFile(srcPath)
	if err != nil {
		t.Fatalf("reading source: %v", err)
	}
	if string(gotSrc) != "pristine content" {
		t.Errorf("source = %q, want %q", gotSrc, "pristine content")
	}

	// A missing source must fail rather than create an empty destination.
	if err := CopyFile(filepath.Join(tmpDir, "nope.db"), filepath.Join(tmpDir, "new.db")); err == nil {
		t.Error("CopyFile with missing source should fail")
	}
	if _, err := os.Stat(filepath.Join(tmpDir, "new.db")); err == nil {
		t.Error("CopyFile created a destination despite a missing source")
	}
}

func TestWriteFileAtomicReplacesAndLeavesNoTemporary(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "nvngx_config.txt")
	if err := os.WriteFile(path, []byte("old"), 0o644); err != nil {
		t.Fatalf("writing seed file: %v", err)
	}

	if err := WriteFileAtomic(path, []byte("new content"), 0o644); err != nil {
		t.Fatalf("WriteFileAtomic: %v", err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading file: %v", err)
	}
	if string(got) != "new content" {
		t.Errorf("file = %q, want %q", got, "new content")
	}
	if _, err := os.Stat(path + ".tmp"); !os.IsNotExist(err) {
		t.Errorf("the temporary file survived the rename: %v", err)
	}

	// The rename must also create a file that did not exist yet.
	fresh := filepath.Join(dir, "sub", "fresh.txt")
	if err := os.MkdirAll(filepath.Dir(fresh), 0o755); err != nil {
		t.Fatalf("creating directory: %v", err)
	}
	if err := WriteFileAtomic(fresh, []byte("x"), 0o644); err != nil {
		t.Fatalf("WriteFileAtomic on a fresh path: %v", err)
	}
	if _, err := os.Stat(fresh); err != nil {
		t.Errorf("fresh file not created: %v", err)
	}
}

// A failed write must leave the original file alone and no temporary behind.
func TestWriteFileAtomicFailureLeavesNoTemporary(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "missing", "nvngx_config.txt")

	if err := WriteFileAtomic(path, []byte("new"), 0o644); err == nil {
		t.Fatal("WriteFileAtomic into a missing directory should fail")
	}
	if _, err := os.Stat(path + ".tmp"); !os.IsNotExist(err) {
		t.Errorf("a temporary file was left behind: %v", err)
	}
}
