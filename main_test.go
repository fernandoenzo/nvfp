package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/fernandoenzo/nvidia-uwp-patch/internal/db"
	"github.com/fernandoenzo/nvidia-uwp-patch/internal/nvidia"
)

// helper: create a test GameDB with known games
func newTestGameDB() *db.GameDB {
	return &db.GameDB{
		Version: 1,
		Games: []db.Game{
			{Fingerprint: "final_fantasy_vii_remake", AppID: "39EA002F.EXED1_n746a19ndrrjg!AppFINALFANTASYVIIREMAKEShipping", Versions: []string{"uwp"}},
			{Fingerprint: "epic_only_game", AppID: "EpicPkg!AppEpic", Versions: []string{"uwp"}, Overrides: map[string]string{"DriverProfile": "EpicGame.exe"}},
			{Fingerprint: "nonexistent_in_db", AppID: "Pkg!App", Versions: []string{"uwp"}},
		},
	}
}

// helper: create a test ProfileDB from the testdata file
func newTestProfileDB(t *testing.T) *nvidia.ProfileDB {
	t.Helper()
	db, err := nvidia.ParseProfileDB(filepath.Join("internal", "nvidia", "testdata", "fingerprint.db"))
	if err != nil {
		t.Fatalf("ParseProfileDB failed: %v", err)
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

func TestFilterGames_All(t *testing.T) {
	gameDB := newTestGameDB()
	// No filter → return all games
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
	profileDB := newTestProfileDB(t)
	gameDB := newTestGameDB()
	games := []db.Game{gameDB.Games[0]} // final_fantasy_vii_remake

	modified := applyPatches(profileDB, games)
	if !modified {
		t.Error("applyPatches() returned false, want true (game should be patched)")
	}

	fp := nvidia.FindFingerprint(profileDB, "final_fantasy_vii_remake")
	if fp == nil {
		t.Fatal("fingerprint not found after patching")
	}
	if !hasUWPVersion(fp) {
		t.Error("fingerprint should have UWP version after patching")
	}
}

func TestApplyPatches_AlreadyUWP(t *testing.T) {
	profileDB := newTestProfileDB(t)
	gameDB := &db.GameDB{
		Version: 1,
		Games: []db.Game{
			{Fingerprint: "already_uwp_game", AppID: "Pkg!App", Versions: []string{"uwp"}},
		},
	}

	modified := applyPatches(profileDB, gameDB.Games)
	if modified {
		t.Error("applyPatches() returned true for already-UWP game, want false")
	}
}

func TestApplyPatches_NotFound(t *testing.T) {
	profileDB := newTestProfileDB(t)
	gameDB := &db.GameDB{
		Version: 1,
		Games: []db.Game{
			{Fingerprint: "no_such_game", AppID: "Pkg!App", Versions: []string{"uwp"}},
		},
	}

	modified := applyPatches(profileDB, gameDB.Games)
	if modified {
		t.Error("applyPatches() returned true for nonexistent game, want false")
	}
}

func TestWritePatch_CreatesBackupAndWrites(t *testing.T) {
	profileDB := newTestProfileDB(t)

	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "fingerprint.db")

	// Write initial DB
	if err := nvidia.WriteProfileDB(profileDB, dbPath); err != nil {
		t.Fatalf("initial WriteProfileDB failed: %v", err)
	}

	// Apply a patch
	nvidia.PatchGame(profileDB, db.Game{Fingerprint: "final_fantasy_vii_remake", AppID: "39EA002F.EXED1_n746a19ndrrjg!AppFINALFANTASYVIIREMAKEShipping", Versions: []string{"uwp"}})

	// Write the patch
	modified, err := writePatch(profileDB, dbPath)
	if err != nil {
		t.Fatalf("writePatch() error: %v", err)
	}
	if !modified {
		t.Error("writePatch() returned false, want true")
	}

	// Backup should exist
	if _, err := os.Stat(dbPath + ".bak"); err != nil {
		t.Errorf("backup not created: %v", err)
	}

	// Written file should be parseable
	db2, err := nvidia.ParseProfileDB(dbPath)
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

func TestWritePatch_NoMetadata(t *testing.T) {
	profileDB := newTestProfileDB(t)
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "fingerprint.db")

	// Write initial DB
	if err := nvidia.WriteProfileDB(profileDB, dbPath); err != nil {
		t.Fatalf("WriteProfileDB failed: %v", err)
	}

	// Apply patch
	nvidia.PatchGame(profileDB, db.Game{Fingerprint: "final_fantasy_vii_remake", AppID: "39EA002F.EXED1_n746a19ndrrjg!AppFINALFANTASYVIIREMAKEShipping", Versions: []string{"uwp"}})

	// writePatch should succeed even without metadata.json
	modified, err := writePatch(profileDB, dbPath)
	if err != nil {
		t.Fatalf("writePatch() error: %v", err)
	}
	if !modified {
		t.Error("writePatch() returned false, want true")
	}
}

func TestListGames(t *testing.T) {
	gameDB := newTestGameDB()

	// Capture stdout
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	listGames(gameDB)

	w.Close()
	os.Stdout = old

	var buf bytes.Buffer
	buf.ReadFrom(r)
	output := buf.String()

	if !strings.Contains(output, "final_fantasy_vii_remake") {
		t.Error("listGames() output missing fingerprint name")
	}
	if !strings.Contains(output, "Games database version: 1") {
		t.Error("listGames() output missing version line")
	}
	if !strings.Contains(output, "Total games: 3") {
		t.Error("listGames() output missing total games count")
	}
}

func TestEndToEnd_ParsePatchWriteReparse(t *testing.T) {
	// Full E2E: parse → patch → write → re-parse → verify
	profileDB := newTestProfileDB(t)
	gameDB := newTestGameDB()

	games, err := filterGames(gameDB)
	if err != nil {
		t.Fatalf("filterGames() error: %v", err)
	}
	applyPatches(profileDB, games)

	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "fingerprint.db")

	if err := nvidia.WriteProfileDB(profileDB, dbPath); err != nil {
		t.Fatalf("WriteProfileDB failed: %v", err)
	}

	// Re-parse
	db2, err := nvidia.ParseProfileDB(dbPath)
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

	// Verify original steam version still present
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
	// Verify dryRun flag prevents file writes
	profileDB := newTestProfileDB(t)
	gameDB := &db.GameDB{
		Version: 1,
		Games: []db.Game{
			{Fingerprint: "final_fantasy_vii_remake", AppID: "39EA002F.EXED1_n746a19ndrrjg!AppFINALFANTASYVIIREMAKEShipping", Versions: []string{"uwp"}},
		},
	}

	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "fingerprint.db")
	if err := nvidia.WriteProfileDB(profileDB, dbPath); err != nil {
		t.Fatalf("WriteProfileDB failed: %v", err)
	}

	// Read original file content
	originalContent, err := os.ReadFile(dbPath)
	if err != nil {
		t.Fatalf("reading original file: %v", err)
	}

	// Set dry-run flag
	originalDryRun := dryRun
	dryRun = true
	defer func() { dryRun = originalDryRun }()

	modified, err := patchDB(gameDB, dbPath)
	if err != nil {
		t.Fatalf("patchDB error: %v", err)
	}
	if !modified {
		t.Error("patchDB should report modified=true for dry-run of new patch")
	}

	// File should be unchanged
	currentContent, err := os.ReadFile(dbPath)
	if err != nil {
		t.Fatalf("reading current file: %v", err)
	}
	if string(currentContent) != string(originalContent) {
		t.Error("dry-run should not modify the file, but content changed")
	}
}

func TestPatchDB_NoChanges(t *testing.T) {
	// Patching a game that already has UWP → no changes
	profileDB := newTestProfileDB(t)
	gameDB := &db.GameDB{
		Version: 1,
		Games: []db.Game{
			{Fingerprint: "already_uwp_game", AppID: "Pkg!App", Versions: []string{"uwp"}},
		},
	}

	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "fingerprint.db")
	if err := nvidia.WriteProfileDB(profileDB, dbPath); err != nil {
		t.Fatalf("WriteProfileDB failed: %v", err)
	}

	modified, err := patchDB(gameDB, dbPath)
	if err != nil {
		t.Fatalf("patchDB error: %v", err)
	}
	if modified {
		t.Error("patchDB should report modified=false for already-UWP game")
	}
}

func TestPatchDB_WithOverridesAndRemove(t *testing.T) {
	profileDB := newTestProfileDB(t)
	gameDB := &db.GameDB{
		Version: 1,
		Games: []db.Game{
			{
				Fingerprint: "final_fantasy_vii_remake",
				AppID:       "Pkg_abc!AppX",
				Versions:    []string{"uwp"},
				Overrides:   map[string]string{"DriverProfile": "custom.exe"},
				Remove:      []string{"WhisperModePopsFactor"},
			},
		},
	}

	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "fingerprint.db")
	if err := nvidia.WriteProfileDB(profileDB, dbPath); err != nil {
		t.Fatalf("WriteProfileDB failed: %v", err)
	}

	modified, err := patchDB(gameDB, dbPath)
	if err != nil {
		t.Fatalf("patchDB error: %v", err)
	}
	if !modified {
		t.Error("patchDB should report modified=true")
	}

	// Re-parse and verify
	db2, err := nvidia.ParseProfileDB(dbPath)
	if err != nil {
		t.Fatalf("re-parse failed: %v", err)
	}

	fp := nvidia.FindFingerprint(db2, "final_fantasy_vii_remake")
	if fp == nil {
		t.Fatal("fingerprint not found")
	}

	// Find UWP version
	var uwpVer *nvidia.Version
	for i := range fp.Versions {
		if fp.Versions[i].Name == "uwp" {
			uwpVer = &fp.Versions[i]
			break
		}
	}
	if uwpVer == nil {
		t.Fatal("UWP version not found")
	}

	// Check override applied
	foundCustomDriver := false
	for _, e := range uwpVer.Elements {
		if e.ElementName() == "DriverProfile" && e.Content == "custom.exe" {
			foundCustomDriver = true
		}
	}
	if !foundCustomDriver {
		t.Error("override DriverProfile not applied")
	}

	// Check removal applied
	for _, e := range uwpVer.Elements {
		if strings.EqualFold(e.ElementName(), "WhisperModePopsFactor") {
			t.Error("WhisperModePopsFactor should have been removed")
		}
	}
}

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

func TestResolveGamesCustomFile(t *testing.T) {
	original := gamesJSONPath
	defer func() { gamesJSONPath = original }()

	custom := `{"version":1,"games":[{"fingerprint":"custom_game","app_id":"Pkg_custom!App","versions":["uwp"]}]}`
	customPath := filepath.Join(t.TempDir(), "custom.json")
	if err := os.WriteFile(customPath, []byte(custom), 0o644); err != nil {
		t.Fatalf("writing custom games.json: %v", err)
	}

	gamesJSONPath = customPath
	db, err := resolveGames()
	if err != nil {
		t.Fatalf("resolveGames() with --games-json error: %v", err)
	}
	if len(db.Games) != 1 || db.Games[0].Fingerprint != "custom_game" {
		t.Errorf("resolveGames() with --games-json = %+v, want custom_game", db.Games)
	}
}

func TestResolveGamesCustomFileInvalid(t *testing.T) {
	original := gamesJSONPath
	defer func() { gamesJSONPath = original }()

	customPath := filepath.Join(t.TempDir(), "custom.json")
	if err := os.WriteFile(customPath, []byte(`not json`), 0o644); err != nil {
		t.Fatalf("writing custom games.json: %v", err)
	}

	gamesJSONPath = customPath
	if _, err := resolveGames(); err == nil {
		t.Error("resolveGames() with invalid --games-json should fail, got nil")
	}
}
