package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/fernandoenzo/nvfp/internal/db"
	"github.com/fernandoenzo/nvfp/internal/nvdr"
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

func TestListGames(t *testing.T) {
	gameDB := newTestGameDB()

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

func TestRootCmd_VersionFlag(t *testing.T) {
	for _, arg := range []string{"--version", "-v"} {
		t.Run(arg, func(t *testing.T) {
			defer func() { versionFlag = false }()

			cmd := newRootCmd()
			var buf bytes.Buffer
			cmd.SetOut(&buf)
			cmd.SetArgs([]string{arg})

			if err := cmd.Execute(); err != nil {
				t.Fatalf("Execute(%s) error: %v", arg, err)
			}
			if got := buf.String(); got != versionMessage {
				t.Errorf("%s output = %q, want %q", arg, got, versionMessage)
			}
		})
	}
}

func TestHasDriverWork(t *testing.T) {
	tests := []struct {
		name  string
		games []*db.Game
		// filter is applied through the --game flag inside the test
		filter string
		want   bool
	}{
		{
			name: "game with app id",
			games: []*db.Game{
				{Fingerprint: "uwp_game", AppUserModelID: "Pkg!App", Versions: []string{"uwp"}},
			},
			want: true,
		},
		{
			name: "explicit driver_app without app id",
			games: []*db.Game{
				{Fingerprint: "exe_game", DriverApp: "Game.exe", Versions: []string{"steam"}},
			},
			want: true,
		},
		{
			name: "no game has a driver string",
			games: []*db.Game{
				{Fingerprint: "plain_game", Versions: []string{"steam"}},
			},
			want: false,
		},
		{
			name: "filter selects a game without app id",
			games: []*db.Game{
				{Fingerprint: "uwp_game", AppUserModelID: "Pkg!App", Versions: []string{"uwp"}},
				{Fingerprint: "plain_game", Versions: []string{"steam"}},
			},
			filter: "plain_game",
			want:   false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			oldFilter := gameFilter
			defer func() { gameFilter = oldFilter }()
			gameFilter = tt.filter

			got := hasDriverWork(&db.GameDB{Version: 1, Games: tt.games})
			if got != tt.want {
				t.Errorf("hasDriverWork() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestDriverRequests(t *testing.T) {
	fdb := newTestFingerprintDB(t)
	gameDB := &db.GameDB{
		Version: 1,
		Games: []*db.Game{
			{Fingerprint: "final_fantasy_vii_remake", AppUserModelID: "39EA002F.EXED1_n746a19ndrrjg!AppFINALFANTASYVIIREMAKEShipping", Versions: []string{"uwp"}},
			{Fingerprint: "already_uwp_game", DriverApp: "Override.exe", DriverProfile: "The Profile", Versions: []string{"steam"}},
			{Fingerprint: "plain_steam_game", Versions: []string{"steam"}},
			{Fingerprint: "missing_from_fingerprint_db", AppUserModelID: "Pkg!App", Versions: []string{"uwp"}},
			{Fingerprint: "skipped_driver_game", AppUserModelID: "Pkg_skip!App", SkipDriver: true, Versions: []string{"uwp"}},
		},
	}

	oldFilter := gameFilter
	defer func() { gameFilter = oldFilter }()
	gameFilter = ""

	reqs := driverRequests(gameDB, fdb)
	if len(reqs) != 3 {
		t.Fatalf("driverRequests() returned %d requests, want 3: %+v", len(reqs), reqs)
	}

	if reqs[0].App != "39EA002F.EXED1_n746a19ndrrjg" {
		t.Errorf("reqs[0].App = %q, want the package family name", reqs[0].App)
	}
	if reqs[0].Profile != "" {
		t.Errorf("reqs[0].Profile = %q, want empty (automatic resolution)", reqs[0].Profile)
	}
	want := []string{"FF7R.exe", "FF7R_Epic.exe"}
	if len(reqs[0].Candidates) != len(want) {
		t.Fatalf("reqs[0].Candidates = %v, want %v", reqs[0].Candidates, want)
	}
	for i := range want {
		if reqs[0].Candidates[i] != want[i] {
			t.Errorf("reqs[0].Candidates[%d] = %q, want %q", i, reqs[0].Candidates[i], want[i])
		}
	}

	if reqs[1].App != "Override.exe" {
		t.Errorf("reqs[1].App = %q, want Override.exe", reqs[1].App)
	}
	if reqs[1].Profile != "The Profile" {
		t.Errorf("reqs[1].Profile = %q, want The Profile", reqs[1].Profile)
	}

	// A game whose fingerprint is absent has no candidates to try.
	if reqs[2].Fingerprint != "missing_from_fingerprint_db" || len(reqs[2].Candidates) != 0 {
		t.Errorf("reqs[2] = %+v, want no candidates", reqs[2])
	}

	// skip_driver keeps a game out of the batch even when it has a UWP identity.
	for _, req := range reqs {
		if req.Fingerprint == "skipped_driver_game" {
			t.Errorf("skip_driver game is still in the batch: %+v", req)
		}
	}
	if hasDriverWork(&db.GameDB{Version: 1, Games: []*db.Game{gameDB.Games[4]}}) {
		t.Error("hasDriverWork() is true for a skip_driver game alone")
	}

	// The --game filter narrows the request list as well.
	gameFilter = "plain_steam_game"
	if got := driverRequests(gameDB, fdb); len(got) != 0 {
		t.Errorf("driverRequests() with --game plain_steam_game = %+v, want none", got)
	}
	gameFilter = "final_fantasy_vii_remake"
	if got := driverRequests(gameDB, fdb); len(got) != 1 || got[0].Fingerprint != "final_fantasy_vii_remake" {
		t.Errorf("driverRequests() with --game final_fantasy_vii_remake = %+v, want one", got)
	}
}

// db.MaxDriverString duplicates nvdr.MaxDriverString on purpose: internal/db
// validates the manifest without importing internal/nvdr, so the two must be
// kept in step by hand. nvdr derives its copy from the buffer the binding hands
// to the driver, which is the one that actually matters.
func TestDriverStringLimitMatchesNVAPI(t *testing.T) {
	if db.MaxDriverString != nvdr.MaxDriverString {
		t.Errorf("db.MaxDriverString = %d, nvdr.MaxDriverString = %d",
			db.MaxDriverString, nvdr.MaxDriverString)
	}
	if nvdr.MaxDriverString != 2047 {
		t.Errorf("nvdr.MaxDriverString = %d, want 2047 (NVAPI_UNICODE_STRING_MAX - 1)", nvdr.MaxDriverString)
	}
}

// slFixture builds the NGX cache layout the --sl-override flow repairs: two
// bundles, a manifest holding only the bundle sections, and payloads under the
// override hash only.
func slFixture(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	files := map[string]string{
		"nvngx_config.txt": "[sl_sdk_0]\r\napp_E658703 = 2.14.0\r\n\r\n[sl_sdk_override_0]\r\napp_E658700 = 2.14.0",
		filepath.Join("sl_sdk_0", "versions", "134656", "files", "1B0_E658703", "nvngx_package_config.txt"):          "sl_common_0, 2.14.0, .dll, sl.common.dll\n",
		filepath.Join("sl_sdk_override_0", "versions", "134656", "files", "1B0_E658700", "nvngx_package_config.txt"): "sl_common_override_0, 2.14.0, .dll, sl.common.dll\n",
		filepath.Join("sl_common_override_0", "versions", "134656", "files", "1B0_E658700.dll"):                      "payload bytes",
	}
	for name, content := range files {
		writeFixtureFile(t, root, name, content)
	}
	return root
}

// writeFixtureFile writes a fixture file under root, creating its parents.
func writeFixtureFile(t *testing.T, root, rel, content string) {
	t.Helper()
	path := filepath.Join(root, rel)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("creating %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("writing %s: %v", path, err)
	}
}

// withNGXRoot points the --sl-override flow at a fixture directory, turns the
// flag on and restores every global afterwards.
func withNGXRoot(t *testing.T, root string, dry bool) {
	t.Helper()
	oldRoot, oldFlag, oldDry, oldJSON := ngxRoot, slOverrideFlag, dryRun, gamesJSONPath
	t.Cleanup(func() { ngxRoot, slOverrideFlag, dryRun, gamesJSONPath = oldRoot, oldFlag, oldDry, oldJSON })
	ngxRoot = func() (string, error) { return root, nil }
	slOverrideFlag, dryRun = true, dry
}

// TestSLOverride drives the whole --sl-override flow: skipping the games
// manifest, --dry-run, idempotence, and the stale/missing repair end to end.
// Each subtest builds its own cache fixture and its own global seam.
func TestSLOverride(t *testing.T) {
	t.Run("skips manifest resolution", func(t *testing.T) {
		// --sl-override is a pure local file operation: with an invalid
		// --games-json it still repairs, proving run() never resolves the manifest.
		root := slFixture(t)
		withNGXRoot(t, root, false)
		gamesJSONPath = filepath.Join(root, "does-not-exist.json")

		if err := run(nil, nil); err != nil {
			t.Fatalf("run() with --sl-override error: %v", err)
		}

		manifest, err := os.ReadFile(filepath.Join(root, "nvngx_config.txt"))
		if err != nil {
			t.Fatalf("reading manifest: %v", err)
		}
		for _, want := range []string{"[sl_common_0]", "app_E658703 = 2.14.0", "[sl_common_override_0]", "app_E658700 = 2.14.0"} {
			if !strings.Contains(string(manifest), want) {
				t.Errorf("manifest missing %q:\n%s", want, manifest)
			}
		}
		if _, err := os.Stat(filepath.Join(root, "sl_common_0", "versions", "134656", "files", "1B0_E658703.dll")); err != nil {
			t.Errorf("the plain bundle payload was not filled from the sibling: %v", err)
		}
		if _, err := os.Stat(filepath.Join(root, "nvngx_config.txt.bak")); err != nil {
			t.Errorf("no backup was written: %v", err)
		}
	})

	t.Run("dry run writes nothing", func(t *testing.T) {
		root := slFixture(t)
		withNGXRoot(t, root, true)

		before, err := os.ReadFile(filepath.Join(root, "nvngx_config.txt"))
		if err != nil {
			t.Fatalf("reading manifest: %v", err)
		}
		output := captureStdout(t, func() {
			if err := run(nil, nil); err != nil {
				t.Fatalf("run() with --sl-override --dry-run error: %v", err)
			}
		})
		after, err := os.ReadFile(filepath.Join(root, "nvngx_config.txt"))
		if err != nil {
			t.Fatalf("reading manifest: %v", err)
		}
		if string(before) != string(after) {
			t.Error("--dry-run modified the manifest")
		}
		if _, err := os.Stat(filepath.Join(root, "nvngx_config.txt.bak")); !os.IsNotExist(err) {
			t.Errorf("--dry-run wrote a backup: %v", err)
		}
		if !strings.Contains(output, "would add [sl_common_0]") {
			t.Errorf("dry-run output does not describe the pending section:\n%s", output)
		}
	})

	t.Run("idempotent", func(t *testing.T) {
		root := slFixture(t)
		withNGXRoot(t, root, false)

		if err := run(nil, nil); err != nil {
			t.Fatalf("first run() error: %v", err)
		}
		first, err := os.ReadFile(filepath.Join(root, "nvngx_config.txt"))
		if err != nil {
			t.Fatalf("reading manifest: %v", err)
		}
		output := captureStdout(t, func() {
			if err := run(nil, nil); err != nil {
				t.Fatalf("second run() error: %v", err)
			}
		})
		second, err := os.ReadFile(filepath.Join(root, "nvngx_config.txt"))
		if err != nil {
			t.Fatalf("reading manifest: %v", err)
		}
		if string(first) != string(second) {
			t.Errorf("the second run changed the manifest:\n%q\n%q", first, second)
		}
		if !strings.Contains(output, "nothing to do") {
			t.Errorf("the second run did not report that there was nothing to do:\n%s", output)
		}
	})

	// A payload copy is real work: the report must never claim there was
	// nothing to do while it filled a missing file.
	t.Run("copy only is not nothing to do", func(t *testing.T) {
		root := t.TempDir()
		writeFixtureFile(t, root, "nvngx_config.txt",
			"[sl_sdk_0]\r\napp_E658703 = 2.14.0\r\n\r\n[sl_common_0]\r\napp_E658703 = 2.14.0")
		writeFixtureFile(t, root, "sl_sdk_0/versions/134656/files/1B0_E658703/nvngx_package_config.txt",
			"sl_common_0, 2.14.0, .dll, sl.common.dll\n")
		writeFixtureFile(t, root, "sl_common_override_0/versions/134656/files/1B0_E658700.dll", "payload bytes")
		withNGXRoot(t, root, false)

		output := captureStdout(t, func() {
			if err := run(nil, nil); err != nil {
				t.Fatalf("run: %v", err)
			}
		})
		if _, err := os.Stat(filepath.Join(root, "sl_common_0", "versions", "134656", "files", "1B0_E658703.dll")); err != nil {
			t.Fatalf("payload not copied from the sibling: %v", err)
		}
		if strings.Contains(output, "nothing to do") {
			t.Errorf("the report claims nothing to do after copying a payload:\n%s", output)
		}
		if !strings.Contains(output, "payload restored from the sibling bundle") {
			t.Errorf("the report does not name the payload copy:\n%s", output)
		}
		if strings.Contains(output, "backup:") {
			t.Errorf("copy-only repair reported a manifest backup:\n%s", output)
		}
	})

	t.Run("missing other-architecture payload is reported accurately", func(t *testing.T) {
		root := t.TempDir()
		writeFixtureFile(t, root, "nvngx_config.txt", "[sl_sdk_0]\r\n")
		writeFixtureFile(t, root, "sl_sdk_0/versions/134656/files/1B0_E658703/nvngx_package_config.txt",
			"sl_common_0, 2.14.0, .dll, sl.common.dll\n")
		writeFixtureFile(t, root, "sl_common_override_0/versions/134656/files/160_E658700.dll", "other-arch")
		withNGXRoot(t, root, false)

		stdout, stderr := captureOutput(t, func() {
			if err := run(nil, nil); err != nil {
				t.Fatalf("run: %v", err)
			}
		})
		if !strings.Contains(stdout, "no repairable changes") || strings.Contains(stdout, "present and current") {
			t.Errorf("result does not explain that repair was skipped:\n%s", stdout)
		}
		if !strings.Contains(stderr, "no compatible-architecture payload") || strings.Contains(stderr, "not been published") {
			t.Errorf("warning misstates the other-architecture payload:\n%s", stderr)
		}
	})

	t.Run("no Streamline bundle does not claim every section is current", func(t *testing.T) {
		root := t.TempDir()
		writeFixtureFile(t, root, "nvngx_config.txt", "[sl_sdk_0]\r\napp_E658703 = 2.14.0")
		withNGXRoot(t, root, false)

		stdout, stderr := captureOutput(t, func() {
			if err := run(nil, nil); err != nil {
				t.Fatalf("run: %v", err)
			}
		})
		if !strings.Contains(stderr, "no Streamline") {
			t.Errorf("missing-cache warning not reported: %s", stderr)
		}
		if !strings.Contains(stdout, "review the warnings") || strings.Contains(stdout, "present and current") {
			t.Errorf("result implies the uninspected cache is current:\n%s", stdout)
		}
	})

	// The whole repair in one run: a comment, a per-feature section pinning an
	// outdated version, sections missing entirely, and payloads missing under
	// one hash. Regression test for the stale-version gap inherited from the
	// PowerShell.
	t.Run("stale and missing", func(t *testing.T) {
		root := t.TempDir()
		writeFixtureFile(t, root, "nvngx_config.txt",
			"; NVIDIA NGX OTA cache\r\n"+
				"[sl_sdk_0]\r\napp_E658703 = 2.14.0\r\n\r\n"+
				"[sl_common_0]\r\napp_E658703=2.14.0\r\n")
		writeFixtureFile(t, root, "sl_sdk_0/versions/134659/files/1B0_E658703/nvngx_package_config.txt",
			"sl_common_0, 2.14.3, .dll, sl.common.dll\nsl_reflex_0, 2.14.3, .dll, sl.reflex.dll\n")
		writeFixtureFile(t, root, "sl_sdk_override_0/versions/134659/files/1B0_E658700/nvngx_package_config.txt",
			"sl_common_override_0, 2.14.3, .dll, sl.common.dll\nsl_reflex_override_0, 2.14.3, .dll, sl.reflex.dll\n")
		writeFixtureFile(t, root, "sl_common_override_0/versions/134659/files/1B0_E658700.dll", "common")
		writeFixtureFile(t, root, "sl_reflex_override_0/versions/134659/files/1B0_E658700.dll", "reflex")
		withNGXRoot(t, root, false)

		output := captureStdout(t, func() {
			if err := run(nil, nil); err != nil {
				t.Fatalf("run: %v", err)
			}
		})
		got, err := os.ReadFile(filepath.Join(root, "nvngx_config.txt"))
		if err != nil {
			t.Fatalf("reading manifest: %v", err)
		}
		// The comment survives, the stale section is corrected keeping its `=`
		// spacing, and the missing sections are appended blank-line separated.
		want := "; NVIDIA NGX OTA cache\r\n" +
			"[sl_sdk_0]\r\napp_E658703 = 2.14.0\r\n\r\n" +
			"[sl_common_0]\r\napp_E658703=2.14.3\r\n\r\n" +
			"[sl_reflex_0]\r\napp_E658703 = 2.14.3\r\n\r\n" +
			"[sl_common_override_0]\r\napp_E658700 = 2.14.3\r\n\r\n" +
			"[sl_reflex_override_0]\r\napp_E658700 = 2.14.3\r\n"
		if string(got) != want {
			t.Errorf("manifest mismatch\n got %q\nwant %q", got, want)
		}
		if !strings.Contains(output, "updated [sl_common_0]  app_E658703: 2.14.0 → 2.14.3") {
			t.Errorf("output does not report the version correction:\n%s", output)
		}
		for _, payload := range []string{
			"sl_common_0/versions/134659/files/1B0_E658703.dll",
			"sl_reflex_0/versions/134659/files/1B0_E658703.dll",
		} {
			if _, err := os.Stat(filepath.Join(root, payload)); err != nil {
				t.Errorf("payload not filled from the sibling: %s: %v", payload, err)
			}
		}
	})
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
