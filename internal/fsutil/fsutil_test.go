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
