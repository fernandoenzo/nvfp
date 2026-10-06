package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/fernandoenzo/nvfp/internal/db"
	"github.com/fernandoenzo/nvfp/internal/nvidia"
)

func newTestGameDB() *db.GameDB {
	return &db.GameDB{
		Version: 1,
		Games: []*db.Game{
			{Fingerprint: "final_fantasy_vii_remake", AppUserModelID: "39EA002F.EXED1_n746a19ndrrjg!AppFINALFANTASYVIIREMAKEShipping", Versions: []string{"uwp"}},
			{Fingerprint: "epic_only_game", AppUserModelID: "EpicPkg!AppEpic", Versions: []string{"uwp"}, Overrides: map[string]string{"DriverProfile": "EpicGame.exe"}},
			{Fingerprint: "nonexistent_in_db", AppUserModelID: "Pkg!App", Versions: []string{"uwp"}},
		},
	}
}

func newTestFingerprintDB(t *testing.T) *nvidia.FingerprintDB {
	t.Helper()
	db, err := nvidia.ParseFingerprintDB(filepath.Join("internal", "nvidia", "testdata", "fingerprint.db"))
	if err != nil {
		t.Fatalf("ParseFingerprintDB failed: %v", err)
	}
	return db
}

// hasUWPVersion reports whether a fingerprint has a UWP version.
func hasUWPVersion(fp *nvidia.Fingerprint) bool {
	for _, v := range fp.Versions {
		if strings.EqualFold(v.Name, "uwp") {
			return true
		}
	}
	return false
}

func captureStdout(t *testing.T, fn func()) string {
	stdout, _ := captureOutput(t, fn)
	return stdout
}

func captureOutput(t *testing.T, fn func()) (string, string) {
	t.Helper()
	oldStdout, oldStderr := os.Stdout, os.Stderr
	outR, outW, err := os.Pipe()
	if err != nil {
		t.Fatalf("creating stdout pipe: %v", err)
	}
	errR, errW, err := os.Pipe()
	if err != nil {
		outR.Close()
		outW.Close()
		t.Fatalf("creating stderr pipe: %v", err)
	}
	os.Stdout, os.Stderr = outW, errW
	defer func() {
		os.Stdout, os.Stderr = oldStdout, oldStderr
		outW.Close()
		errW.Close()
		outR.Close()
		errR.Close()
	}()

	fn()
	if err := outW.Close(); err != nil {
		t.Fatalf("closing stdout pipe: %v", err)
	}
	if err := errW.Close(); err != nil {
		t.Fatalf("closing stderr pipe: %v", err)
	}
	os.Stdout, os.Stderr = oldStdout, oldStderr
	var stdout, stderr bytes.Buffer
	if _, err := stdout.ReadFrom(outR); err != nil {
		t.Fatalf("reading stdout pipe: %v", err)
	}
	if _, err := stderr.ReadFrom(errR); err != nil {
		t.Fatalf("reading stderr pipe: %v", err)
	}
	return stdout.String(), stderr.String()
}
