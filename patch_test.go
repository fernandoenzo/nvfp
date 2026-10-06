package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/fernandoenzo/nvfp/internal/db"
	"github.com/fernandoenzo/nvfp/internal/nvidia"
)

func TestFilterGames_All(t *testing.T) {
	gameDB := newTestGameDB()
	original := gameFilter
	gameFilter = ""
	defer func() { gameFilter = original }()

	games, err := filterGames(gameDB)
	if err != nil {
		t.Fatalf("filterGames() error: %v", err)
	}
	if len(games) != len(gameDB.Games) {
		t.Errorf("filterGames() returned %d games, want %d", len(games), len(gameDB.Games))
	}
}

func TestFilterGames_ByName(t *testing.T) {
	gameDB := newTestGameDB()
	original := gameFilter
	gameFilter = "final_fantasy_vii_remake"
	defer func() { gameFilter = original }()

	games, err := filterGames(gameDB)
	if err != nil {
		t.Fatalf("filterGames() error: %v", err)
	}
	if len(games) != 1 {
		t.Fatalf("filterGames() returned %d games, want 1", len(games))
	}
	if games[0].Fingerprint != "final_fantasy_vii_remake" {
		t.Errorf("filterGames() returned fingerprint %q, want final_fantasy_vii_remake", games[0].Fingerprint)
	}
}

func TestFilterGames_NotFound(t *testing.T) {
	gameDB := newTestGameDB()
	original := gameFilter
	gameFilter = "no_such_game"
	defer func() { gameFilter = original }()

	_, err := filterGames(gameDB)
	if err == nil {
		t.Error("filterGames() expected error for nonexistent game, got nil")
	}
}

func TestApplyPatches_PatchesGame(t *testing.T) {
	fdb := newTestFingerprintDB(t)
	gameDB := newTestGameDB()
	games := []*db.Game{gameDB.Games[0]} // final_fantasy_vii_remake

	modified := applyPatches(fdb, games)
	if !modified {
		t.Error("applyPatches() returned false, want true (game should be patched)")
	}

	fp := nvidia.FindFingerprint(fdb, "final_fantasy_vii_remake")
	if fp == nil {
		t.Fatal("fingerprint not found after patching")
	}
	if !hasUWPVersion(fp) {
		t.Error("fingerprint should have UWP version after patching")
	}
}

func TestApplyPatches_AlreadyUWP(t *testing.T) {
	fdb := newTestFingerprintDB(t)
	gameDB := &db.GameDB{
		Version: 1,
		Games: []*db.Game{
			{Fingerprint: "already_uwp_game", AppUserModelID: "Pkg!App", Versions: []string{"uwp"}},
		},
	}

	modified := applyPatches(fdb, gameDB.Games)
	if modified {
		t.Error("applyPatches() returned true for already-UWP game, want false")
	}
}

func TestApplyPatches_NotFound(t *testing.T) {
	fdb := newTestFingerprintDB(t)
	gameDB := &db.GameDB{
		Version: 1,
		Games: []*db.Game{
			{Fingerprint: "no_such_game", AppUserModelID: "Pkg!App", Versions: []string{"uwp"}},
		},
	}

	modified := applyPatches(fdb, gameDB.Games)
	if modified {
		t.Error("applyPatches() returned true for nonexistent game, want false")
	}
}

func TestWritePatch_WritesPatchedDatabase(t *testing.T) {
	fdb := newTestFingerprintDB(t)

	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "fingerprint.db")

	if err := nvidia.WriteFingerprintDB(fdb, dbPath); err != nil {
		t.Fatalf("initial WriteFingerprintDB failed: %v", err)
	}

	nvidia.PatchGame(fdb, &db.Game{Fingerprint: "final_fantasy_vii_remake", AppUserModelID: "39EA002F.EXED1_n746a19ndrrjg!AppFINALFANTASYVIIREMAKEShipping", Versions: []string{"uwp"}})

	if err := writePatch(fdb, dbPath); err != nil {
		t.Fatalf("writePatch() error: %v", err)
	}

	db2, err := nvidia.ParseFingerprintDB(dbPath)
	if err != nil {
		t.Fatalf("re-parse of written file failed: %v", err)
	}
	fp := nvidia.FindFingerprint(db2, "final_fantasy_vii_remake")
	if fp == nil {
		t.Fatal("fingerprint not found after writePatch round-trip")
	}
	if !hasUWPVersion(fp) {
		t.Error("fingerprint should have UWP version after patching")
	}
}

func TestEndToEnd_ParsePatchWriteReparse(t *testing.T) {
	fdb := newTestFingerprintDB(t)
	gameDB := newTestGameDB()

	games, err := filterGames(gameDB)
	if err != nil {
		t.Fatalf("filterGames() error: %v", err)
	}
	applyPatches(fdb, games)

	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "fingerprint.db")

	if err := nvidia.WriteFingerprintDB(fdb, dbPath); err != nil {
		t.Fatalf("WriteFingerprintDB failed: %v", err)
	}

	db2, err := nvidia.ParseFingerprintDB(dbPath)
	if err != nil {
		t.Fatalf("re-parse failed: %v", err)
	}

	fp := nvidia.FindFingerprint(db2, "final_fantasy_vii_remake")
	if fp == nil {
		t.Fatal("fingerprint not found after round-trip")
	}
	if !hasUWPVersion(fp) {
		t.Error("UWP version not found after round-trip")
	}

	var hasSteam bool
	for _, v := range fp.Versions {
		if v.Name == "steam" {
			hasSteam = true
			break
		}
	}
	if !hasSteam {
		t.Error("steam version lost after round-trip")
	}
}

func TestDryRun(t *testing.T) {
	fdb := newTestFingerprintDB(t)
	gameDB := &db.GameDB{
		Version: 1,
		Games: []*db.Game{
			{Fingerprint: "final_fantasy_vii_remake", AppUserModelID: "39EA002F.EXED1_n746a19ndrrjg!AppFINALFANTASYVIIREMAKEShipping", Versions: []string{"uwp"}},
		},
	}

	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "fingerprint.db")
	if err := nvidia.WriteFingerprintDB(fdb, dbPath); err != nil {
		t.Fatalf("WriteFingerprintDB failed: %v", err)
	}

	originalContent, err := os.ReadFile(dbPath)
	if err != nil {
		t.Fatalf("reading original file: %v", err)
	}

	originalDryRun := dryRun
	dryRun = true
	defer func() { dryRun = originalDryRun }()

	_, modified, err := patchDB(gameDB, dbPath)
	if err != nil {
		t.Fatalf("patchDB error: %v", err)
	}
	if !modified {
		t.Error("patchDB should report modified=true for dry-run of new patch")
	}

	currentContent, err := os.ReadFile(dbPath)
	if err != nil {
		t.Fatalf("reading current file: %v", err)
	}
	if string(currentContent) != string(originalContent) {
		t.Error("dry-run should not modify the file, but content changed")
	}
}

func TestPatchDB_NoChanges(t *testing.T) {
	fdb := newTestFingerprintDB(t)
	gameDB := &db.GameDB{
		Version: 1,
		Games: []*db.Game{
			{Fingerprint: "already_uwp_game", AppUserModelID: "Pkg!App", Versions: []string{"uwp"}},
		},
	}

	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "fingerprint.db")
	if err := nvidia.WriteFingerprintDB(fdb, dbPath); err != nil {
		t.Fatalf("WriteFingerprintDB failed: %v", err)
	}

	_, modified, err := patchDB(gameDB, dbPath)
	if err != nil {
		t.Fatalf("patchDB error: %v", err)
	}
	if modified {
		t.Error("patchDB should report modified=false for already-UWP game")
	}
}

func TestPatchDB_WithOverridesAndRemove(t *testing.T) {
	fdb := newTestFingerprintDB(t)
	gameDB := &db.GameDB{
		Version: 1,
		Games: []*db.Game{
			{
				Fingerprint:    "final_fantasy_vii_remake",
				AppUserModelID: "Pkg_abc!AppX",
				Versions:       []string{"uwp"},
				Overrides:      map[string]string{"DriverProfile": "custom.exe"},
				Remove:         []string{"WhisperModePopsFactor"},
			},
		},
	}

	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "fingerprint.db")
	if err := nvidia.WriteFingerprintDB(fdb, dbPath); err != nil {
		t.Fatalf("WriteFingerprintDB failed: %v", err)
	}

	_, modified, err := patchDB(gameDB, dbPath)
	if err != nil {
		t.Fatalf("patchDB error: %v", err)
	}
	if !modified {
		t.Error("patchDB should report modified=true")
	}

	db2, err := nvidia.ParseFingerprintDB(dbPath)
	if err != nil {
		t.Fatalf("re-parse failed: %v", err)
	}

	fp := nvidia.FindFingerprint(db2, "final_fantasy_vii_remake")
	if fp == nil {
		t.Fatal("fingerprint not found")
	}

	var uwpVer *nvidia.Version
	for i := range fp.Versions {
		if fp.Versions[i].Name == "uwp" {
			uwpVer = fp.Versions[i]
			break
		}
	}
	if uwpVer == nil {
		t.Fatal("UWP version not found")
	}

	foundCustomDriver := false
	for _, e := range uwpVer.Elements {
		if e.ElementName() == "DriverProfile" && e.Content == "custom.exe" {
			foundCustomDriver = true
		}
	}
	if !foundCustomDriver {
		t.Error("override DriverProfile not applied")
	}

	for _, e := range uwpVer.Elements {
		if strings.EqualFold(e.ElementName(), "WhisperModePopsFactor") {
			t.Error("WhisperModePopsFactor should have been removed")
		}
	}
}

func reloginFixture(t *testing.T) string {
	t.Helper()
	localAppData := t.TempDir()
	t.Setenv("LOCALAPPDATA", localAppData)
	ontology := filepath.Join(localAppData, "NVIDIA Corporation", "NVIDIA App",
		"NvBackend", "ApplicationOntology", "data", "fingerprint.db")
	data, err := os.ReadFile(filepath.Join("internal", "nvidia", "testdata", "fingerprint.db"))
	if err != nil {
		t.Fatalf("reading testdata fingerprint.db: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(ontology), 0o755); err != nil {
		t.Fatalf("creating ontology dir: %v", err)
	}
	if err := os.WriteFile(ontology, data, 0o644); err != nil {
		t.Fatalf("writing working fingerprint.db: %v", err)
	}
	manifest := filepath.Join(localAppData, "games.json")
	content := `{"version":1,"games":[{"fingerprint":"final_fantasy_vii_remake","app_user_model_id":"39EA002F.EXED1_n746a19ndrrjg!AppFINALFANTASYVIIREMAKEShipping","versions":["uwp"]}]}`
	if err := os.WriteFile(manifest, []byte(content), 0o644); err != nil {
		t.Fatalf("writing games.json: %v", err)
	}

	oldJSON, oldNoDriver, oldDry := gamesJSONPath, noDriverFlag, dryRun
	t.Cleanup(func() { gamesJSONPath, noDriverFlag, dryRun = oldJSON, oldNoDriver, oldDry })
	gamesJSONPath, noDriverFlag = manifest, true
	return ontology
}

// TestReloginHintAfterPatch pins when the logoff/logon reminder is printed:
// after a real fingerprint.db modification, never on a no-op or dry run.
func TestReloginHintAfterPatch(t *testing.T) {
	t.Run("prints after a real patch, not on the second run", func(t *testing.T) {
		reloginFixture(t)
		dryRun = false

		first := captureStdout(t, func() {
			if err := run(nil, nil); err != nil {
				t.Fatalf("run() error: %v", err)
			}
		})
		if !strings.Contains(first, "log out of Windows and sign back in") {
			t.Errorf("first run did not print the relogin hint:\n%s", first)
		}

		second := captureStdout(t, func() {
			if err := run(nil, nil); err != nil {
				t.Fatalf("second run() error: %v", err)
			}
		})
		if strings.Contains(second, "log out of Windows and sign back in") {
			t.Errorf("second run printed the hint without patching:\n%s", second)
		}
	})

	t.Run("dry-run prints no hint and writes nothing", func(t *testing.T) {
		working := reloginFixture(t)
		dryRun = true
		before, err := os.ReadFile(working)
		if err != nil {
			t.Fatalf("reading working fingerprint.db: %v", err)
		}

		output := captureStdout(t, func() {
			if err := run(nil, nil); err != nil {
				t.Fatalf("run() with --dry-run error: %v", err)
			}
		})
		if strings.Contains(output, "log out of Windows and sign back in") {
			t.Errorf("--dry-run printed the relogin hint:\n%s", output)
		}
		after, err := os.ReadFile(working)
		if err != nil {
			t.Fatalf("re-reading working fingerprint.db: %v", err)
		}
		if string(before) != string(after) {
			t.Error("--dry-run modified fingerprint.db")
		}
	})
}
