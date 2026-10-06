package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFindFingerprintDB(t *testing.T) {
	// LOCALAPPDATA not set → error
	t.Setenv("LOCALAPPDATA", "")
	if _, err := findFingerprintDB(); err == nil {
		t.Error("findFingerprintDB() should fail when LOCALAPPDATA is not set")
	}

	// LOCALAPPDATA set but no fingerprint.db → error
	tmpDir := t.TempDir()
	t.Setenv("LOCALAPPDATA", tmpDir)
	if _, err := findFingerprintDB(); err == nil {
		t.Error("findFingerprintDB() should fail when fingerprint.db does not exist")
	}

	// Full structure → its path is returned
	ontologyPath := filepath.Join(tmpDir, "NVIDIA Corporation", "NVIDIA App",
		"NvBackend", "ApplicationOntology", "data", "fingerprint.db")
	if err := os.MkdirAll(filepath.Dir(ontologyPath), 0o755); err != nil {
		t.Fatalf("MkdirAll ontology: %v", err)
	}
	if err := os.WriteFile(ontologyPath, []byte("work"), 0o644); err != nil {
		t.Fatalf("writing working fingerprint.db: %v", err)
	}
	got, err := findFingerprintDB()
	if err != nil {
		t.Fatalf("findFingerprintDB() error: %v", err)
	}
	if got != ontologyPath {
		t.Errorf("findFingerprintDB() = %q, want %q", got, ontologyPath)
	}
}

func TestFindDAOFingerprintDB(t *testing.T) {
	daoRel := filepath.Join("NVIDIA Corporation", "NVIDIA App", "NvBackend", "DAO")

	tests := []struct {
		name  string
		setup func(t *testing.T, localAppData string)
		want  string // path relative to localAppData; "" means failure expected
	}{
		{
			name:  "no DAO directory",
			setup: func(t *testing.T, localAppData string) {},
		},
		{
			name: "subdirectory without fingerprint.db is not a candidate",
			setup: func(t *testing.T, localAppData string) {
				if err := os.MkdirAll(filepath.Join(localAppData, daoRel, "abc123"), 0o755); err != nil {
					t.Fatalf("MkdirAll: %v", err)
				}
			},
		},
		{
			name: "skips files and subdirectories lacking the db",
			setup: func(t *testing.T, localAppData string) {
				if err := os.MkdirAll(filepath.Join(localAppData, daoRel, "aa"), 0o755); err != nil {
					t.Fatalf("MkdirAll: %v", err)
				}
				if err := os.WriteFile(filepath.Join(localAppData, daoRel, "loose.txt"), []byte("x"), 0o644); err != nil {
					t.Fatalf("writing loose file: %v", err)
				}
				dir := filepath.Join(localAppData, daoRel, "bb")
				if err := os.MkdirAll(dir, 0o755); err != nil {
					t.Fatalf("MkdirAll: %v", err)
				}
				if err := os.WriteFile(filepath.Join(dir, "fingerprint.db"), []byte("<xml/>"), 0o644); err != nil {
					t.Fatalf("writing fingerprint.db: %v", err)
				}
			},
			want: filepath.Join(daoRel, "bb", "fingerprint.db"),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			t.Setenv("LOCALAPPDATA", tmpDir)
			tc.setup(t, tmpDir)

			got, err := findDAOFingerprintDB()
			if tc.want == "" {
				if err == nil {
					t.Fatalf("findDAOFingerprintDB() = %q, want error", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("findDAOFingerprintDB() error: %v", err)
			}
			if want := filepath.Join(tmpDir, tc.want); got != want {
				t.Errorf("findDAOFingerprintDB() = %q, want %q", got, want)
			}
		})
	}

	// LOCALAPPDATA unset (or empty) → error, never a relative path
	t.Setenv("LOCALAPPDATA", "")
	if got, err := findDAOFingerprintDB(); err == nil {
		t.Errorf("findDAOFingerprintDB() = %q, want error when LOCALAPPDATA is empty", got)
	}
}
