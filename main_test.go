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

// helper: create a test GameDB with known games
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

// helper: create a test FingerprintDB from the testdata file
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

	// Write initial DB
	if err := nvidia.WriteFingerprintDB(fdb, dbPath); err != nil {
		t.Fatalf("initial WriteFingerprintDB failed: %v", err)
	}

	// Apply a patch
	nvidia.PatchGame(fdb, &db.Game{Fingerprint: "final_fantasy_vii_remake", AppUserModelID: "39EA002F.EXED1_n746a19ndrrjg!AppFINALFANTASYVIIREMAKEShipping", Versions: []string{"uwp"}})

	// Write the patch
	if err := writePatch(fdb, dbPath); err != nil {
		t.Fatalf("writePatch() error: %v", err)
	}

	// Written file should be parseable
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

	// Re-parse
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

	modified, err := patchDB(gameDB, dbPath)
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

	modified, err := patchDB(gameDB, dbPath)
	if err != nil {
		t.Fatalf("patchDB error: %v", err)
	}
	if !modified {
		t.Error("patchDB should report modified=true")
	}

	// Re-parse and verify
	db2, err := nvidia.ParseFingerprintDB(dbPath)
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
			uwpVer = fp.Versions[i]
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

func TestResolveGamesCustomFile(t *testing.T) {
	original := gamesJSONPath
	defer func() { gamesJSONPath = original }()

	custom := `{"version":1,"games":[{"fingerprint":"custom_game","app_user_model_id":"Pkg_custom!App","versions":["uwp"]}]}`
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
