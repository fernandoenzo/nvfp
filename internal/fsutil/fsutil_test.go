package fsutil

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"sync"
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

	keepPath := filepath.Join(tmpDir, "keep.db")
	if err := os.WriteFile(keepPath, []byte("keep me"), 0o644); err != nil {
		t.Fatalf("writing keep file: %v", err)
	}
	if err := CopyFile(filepath.Join(tmpDir, "nope.db"), keepPath); err == nil {
		t.Error("CopyFile with missing source should fail")
	}
	gotKeep, err := os.ReadFile(keepPath)
	if err != nil {
		t.Fatalf("reading keep file: %v", err)
	}
	if string(gotKeep) != "keep me" {
		t.Errorf("destination after missing-source failure = %q, want %q", gotKeep, "keep me")
	}
	newPath := filepath.Join(tmpDir, "new.db")
	if err := CopyFile(filepath.Join(tmpDir, "nope.db"), newPath); err == nil {
		t.Error("CopyFile with a missing source should fail")
	}
	if _, err := os.Stat(newPath); !os.IsNotExist(err) {
		t.Errorf("CopyFile created a destination without a source: %v", err)
	}
}

func TestCopyFileWithSameSourceAndDestination(t *testing.T) {
	path := filepath.Join(t.TempDir(), "same.db")
	if err := os.WriteFile(path, []byte("unchanged"), 0o644); err != nil {
		t.Fatalf("writing source: %v", err)
	}
	if err := CopyFile(path, path); err != nil {
		t.Fatalf("CopyFile with identical paths: %v", err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading destination: %v", err)
	}
	if string(got) != "unchanged" {
		t.Errorf("file = %q, want unchanged", got)
	}
}

func TestCopyFileReadFailurePreservesDestination(t *testing.T) {
	dst := filepath.Join(t.TempDir(), "payload.dll")
	if err := os.WriteFile(dst, []byte("known-good"), 0o644); err != nil {
		t.Fatalf("writing destination: %v", err)
	}
	if err := CopyFile(t.TempDir(), dst); err == nil {
		t.Fatal("CopyFile from a directory should fail")
	}
	got, err := os.ReadFile(dst)
	if err != nil {
		t.Fatalf("reading destination: %v", err)
	}
	if string(got) != "known-good" {
		t.Errorf("destination after failed copy = %q, want known-good", got)
	}
	assertNoAtomicTemporary(t, dst)
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
	assertNoAtomicTemporary(t, path)

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

func TestWriteFileAtomicReadFailurePreservesDestination(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "nvngx_config.txt")
	if err := os.WriteFile(path, []byte("old contents"), 0o644); err != nil {
		t.Fatalf("writing original: %v", err)
	}
	if err := writeAtomic(path, partialErrorReader{}, 0o644); err == nil {
		t.Fatal("writeAtomic with a failing reader should fail")
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading original: %v", err)
	}
	if string(got) != "old contents" {
		t.Errorf("destination after failed replacement = %q, want %q", got, "old contents")
	}
	assertNoAtomicTemporary(t, path)
}

func TestWriteFileAtomicRenameFailureRemovesTemporary(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "target")
	if err := os.Mkdir(path, 0o755); err != nil {
		t.Fatalf("creating target directory: %v", err)
	}
	if err := WriteFileAtomic(path, []byte("new"), 0o644); err == nil {
		t.Fatal("replacing a directory with a file should fail")
	}
	if info, err := os.Stat(path); err != nil || !info.IsDir() {
		t.Errorf("target directory changed after failed replace: info=%v err=%v", info, err)
	}
	assertNoAtomicTemporary(t, path)
}

func TestWriteFileAtomicConcurrentWritersLeaveWholeContents(t *testing.T) {
	const writers, size = 6, 64 << 10
	path := filepath.Join(t.TempDir(), "shared.txt")
	contents := make([][]byte, writers)
	start := make(chan struct{})
	errs := make(chan error, writers)
	var wg sync.WaitGroup
	for i := range writers {
		contents[i] = bytes.Repeat([]byte{byte(i + 1)}, size)
		wg.Add(1)
		go func(data []byte) {
			defer wg.Done()
			<-start
			errs <- WriteFileAtomic(path, data, 0o644)
		}(contents[i])
	}
	close(start)
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Errorf("concurrent WriteFileAtomic: %v", err)
		}
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading final file: %v", err)
	}
	for _, want := range contents {
		if bytes.Equal(got, want) {
			return
		}
	}
	t.Errorf("final file is not any writer's complete contents (size %d)", len(got))
}

type partialErrorReader struct{}

func (partialErrorReader) Read(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	p[0] = 'x'
	return 1, io.ErrUnexpectedEOF
}

func assertNoAtomicTemporary(t *testing.T, path string) {
	t.Helper()
	matches, err := filepath.Glob(filepath.Join(filepath.Dir(path), ".nvfp-tmp-*"))
	if err != nil {
		t.Fatalf("searching for temporary files: %v", err)
	}
	if len(matches) != 0 {
		t.Errorf("temporary files survived: %v", matches)
	}
}
