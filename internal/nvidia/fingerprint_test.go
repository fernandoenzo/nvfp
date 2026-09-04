package nvidia

import (
	"encoding/xml"
	"os"
	"path/filepath"
	"strings"
	"testing"

	gamesdb "github.com/fernandoenzo/nvidia-uwp-patch/internal/db"
)

func TestParseProfileDB(t *testing.T) {
	db, err := ParseProfileDB(filepath.Join("testdata", "fingerprint.db"))
	if err != nil {
		t.Fatalf("ParseProfileDB failed: %v", err)
	}

	if len(db.Fingerprints) != 5 {
		t.Fatalf("expected 5 fingerprints, got %d", len(db.Fingerprints))
	}

	// Check first fingerprint
	fp := FindFingerprint(db, "final_fantasy_vii_remake")
	if fp == nil {
		t.Fatal("expected to find final_fantasy_vii_remake")
	}
	if len(fp.Versions) != 2 {
		t.Fatalf("expected 2 versions, got %d", len(fp.Versions))
	}

	// Check steam version has expected elements
	steam := FindSourceVersion(fp)
	if steam == nil {
		t.Fatal("expected to find source version")
	}
	if steam.Name != "steam" {
		t.Fatalf("expected steam as source, got %s", steam.Name)
	}
}

func TestFindFingerprint(t *testing.T) {
	db, _ := ParseProfileDB(filepath.Join("testdata", "fingerprint.db"))

	tests := []struct {
		name     string
		found    bool
		versions int
	}{
		{"final_fantasy_vii_remake", true, 2},
		{"already_uwp_game", true, 2},
		{"no_source_game", true, 1},
		{"nonexistent", false, 0},
	}

	for _, tt := range tests {
		fp := FindFingerprint(db, tt.name)
		if tt.found && fp == nil {
			t.Errorf("FindFingerprint(%q) expected found", tt.name)
		}
		if !tt.found && fp != nil {
			t.Errorf("FindFingerprint(%q) expected not found", tt.name)
		}
		if fp != nil && len(fp.Versions) != tt.versions {
			t.Errorf("FindFingerprint(%q) expected %d versions, got %d", tt.name, tt.versions, len(fp.Versions))
		}
	}
}

func TestFindVersion(t *testing.T) {
	db, _ := ParseProfileDB(filepath.Join("testdata", "fingerprint.db"))

	tests := []struct {
		name       string
		version    string
		hasVersion bool
	}{
		{"final_fantasy_vii_remake", "uwp", false},
		{"already_uwp_game", "uwp", true},
		{"no_source_game", "uwp", true},
		{"final_fantasy_vii_remake", "steam", true},
		{"final_fantasy_vii_remake", "gog", false},
	}

	for _, tt := range tests {
		fp := FindFingerprint(db, tt.name)
		if fp == nil {
			t.Fatalf("fingerprint %q not found", tt.name)
		}
		if (findVersion(fp, tt.version) != nil) != tt.hasVersion {
			t.Errorf("findVersion(%q, %q) presence = %v, want %v", tt.name, tt.version, findVersion(fp, tt.version) != nil, tt.hasVersion)
		}
	}
}

func TestAddUWPVersion(t *testing.T) {
	db, _ := ParseProfileDB(filepath.Join("testdata", "fingerprint.db"))
	fp := FindFingerprint(db, "final_fantasy_vii_remake")
	src := FindSourceVersion(fp)

	appID := "39EA002F.EXED1_n746a19ndrrjg!AppFINALFANTASYVIIREMAKEShipping"
	got := AddUWPVersion(src, appID, nil, nil)

	// Check name
	if got.Name != "uwp" {
		t.Errorf("got name = %q, want uwp", got.Name)
	}

	// Check that removed fields are gone
	elementNames := make(map[string]bool)
	for _, e := range got.Elements {
		elementNames[strings.ToLower(e.ElementName())] = true
	}

	for _, removed := range []string{"Files", "Launch", "SteamAppIds", "Directories", "InstallDirRegValues"} {
		if elementNames[strings.ToLower(removed)] {
			t.Errorf("got should not contain %s", removed)
		}
	}

	// Check that UWP-specific fields are present
	if !elementNames["uwppackagefamilyname"] {
		t.Error("got should contain UWPPackageFamilyName")
	}
	if !elementNames["appusermodelid"] {
		t.Error("got should contain AppUserModelId")
	}

	// Check Distributor is UWP
	for _, e := range got.Elements {
		if strings.ToLower(e.ElementName()) == "distributor" {
			if e.Content != "UWP" {
				t.Errorf("Distributor = %q, want UWP", e.Content)
			}
		}
	}

	// Check preserved fields
	if !elementNames["cmsid"] {
		t.Error("got should contain CMSID")
	}
	if !elementNames["driverprofile"] {
		t.Error("got should contain DriverProfile")
	}

	// Check UWPPackageFamilyName derivation
	for _, e := range got.Elements {
		if e.ElementName() == "UWPPackageFamilyName" {
			if e.Content != "39EA002F.EXED1_n746a19ndrrjg" {
				t.Errorf("UWPPackageFamilyName = %q, want 39EA002F.EXED1_n746a19ndrrjg", e.Content)
			}
		}
		if e.ElementName() == "AppUserModelId" {
			if e.Content != appID {
				t.Errorf("AppUserModelId = %q, want %s", e.Content, appID)
			}
		}
	}
}

func TestAddUWPWithOverrides(t *testing.T) {
	db, _ := ParseProfileDB(filepath.Join("testdata", "fingerprint.db"))
	fp := FindFingerprint(db, "final_fantasy_vii_remake")
	src := FindSourceVersion(fp)

	overrides := map[string]string{
		"DriverProfile": "custom_exe.exe",
		"SpecialFlag":   "1",
	}

	got := AddUWPVersion(src, "Pkg_abc!AppX", overrides, nil)

	elementMap := make(map[string]string)
	for _, e := range got.Elements {
		elementMap[e.ElementName()] = e.Content
	}

	if elementMap["DriverProfile"] != "custom_exe.exe" {
		t.Errorf("DriverProfile override = %q, want custom_exe.exe", elementMap["DriverProfile"])
	}
	if elementMap["SpecialFlag"] != "1" {
		t.Errorf("SpecialFlag = %q, want 1", elementMap["SpecialFlag"])
	}
}

func TestAddUWPWithRemove(t *testing.T) {
	db, _ := ParseProfileDB(filepath.Join("testdata", "fingerprint.db"))
	fp := FindFingerprint(db, "final_fantasy_vii_remake")
	src := FindSourceVersion(fp)

	got := AddUWPVersion(src, "Pkg_abc!AppX", nil, []string{"WhisperModePopsFactor"})

	for _, e := range got.Elements {
		if e.ElementName() == "WhisperModePopsFactor" {
			t.Error("WhisperModePopsFactor should have been removed")
		}
	}
}

func TestPatchGame(t *testing.T) {
	db, _ := ParseProfileDB(filepath.Join("testdata", "fingerprint.db"))

	tests := []struct {
		fingerprint string
		appID       string
		versions    []string
		wantStatus  PatchStatus
	}{
		{"final_fantasy_vii_remake", "39EA002F.EXED1_n746a19ndrrjg!AppFINALFANTASYVIIREMAKEShipping", []string{"uwp"}, StatusPatched},
		{"already_uwp_game", "Pkg!App", []string{"uwp"}, StatusAlreadyPresent},
		{"nonexistent_game", "Pkg!App", []string{"uwp"}, StatusNotFound},
		{"no_source_game", "Pkg!App", []string{"uwp"}, StatusAlreadyPresent},
		{"empty_game", "Pkg!App", []string{"uwp"}, StatusNoSource},
	}

	for _, tt := range tests {
		result := PatchGame(db, gamesdb.Game{Fingerprint: tt.fingerprint, AppUserModelID: tt.appID, Versions: tt.versions})
		if result.Status != tt.wantStatus {
			t.Errorf("PatchGame(%q) status = %q, want %q", tt.fingerprint, result.Status, tt.wantStatus)
		}
	}
}

func TestPatchGame_UpdateExistingUWP(t *testing.T) {
	db, _ := ParseProfileDB(filepath.Join("testdata", "fingerprint.db"))

	overrides := map[string]string{
		"DriverProfile": "game_uwp.exe",
		"NewField":      "new-value",
	}
	remove := []string{"CMSID"}

	result := PatchGame(db, gamesdb.Game{Fingerprint: "already_uwp_game", AppUserModelID: "Pkg!App", Versions: []string{"uwp"}, Remove: remove, Overrides: overrides})
	if result.Status != StatusPatched {
		t.Fatalf("status = %q, want %q", result.Status, StatusPatched)
	}

	uwp := findVersion(FindFingerprint(db, "already_uwp_game"), "uwp")
	if uwp == nil {
		t.Fatal("UWP version not found")
	}

	seen := make(map[string]string)
	for _, e := range uwp.Elements {
		seen[strings.ToLower(e.ElementName())] = e.Content
	}

	// Override replaced the existing element content
	if seen["driverprofile"] != "game_uwp.exe" {
		t.Errorf("DriverProfile = %q, want game_uwp.exe", seen["driverprofile"])
	}
	// New override was appended
	if seen["newfield"] != "new-value" {
		t.Errorf("NewField = %q, want new-value", seen["newfield"])
	}
	// Removed element is gone
	if _, ok := seen["cmsid"]; ok {
		t.Error("CMSID should have been removed")
	}
	// Forced fields of the existing version are untouched
	if seen["distributor"] != "UWP" {
		t.Errorf("Distributor = %q, want UWP", seen["distributor"])
	}
	if seen["uwppackagefamilyname"] != "SomePkg_abc123" {
		t.Errorf("UWPPackageFamilyName = %q, want SomePkg_abc123", seen["uwppackagefamilyname"])
	}
	if seen["appusermodelid"] != "SomePkg_abc123!AppGame" {
		t.Errorf("AppUserModelId = %q, want SomePkg_abc123!AppGame", seen["appusermodelid"])
	}
}

func TestPatchGame_UpdateExistingUWP_NoChanges(t *testing.T) {
	db, _ := ParseProfileDB(filepath.Join("testdata", "fingerprint.db"))

	result := PatchGame(db, gamesdb.Game{Fingerprint: "already_uwp_game", AppUserModelID: "Pkg!App", Versions: []string{"uwp"}})
	if result.Status != StatusAlreadyPresent {
		t.Errorf("status = %q, want %q", result.Status, StatusAlreadyPresent)
	}
}

func TestPatchGame_UpdateExistingUWP_Idempotent(t *testing.T) {
	db, _ := ParseProfileDB(filepath.Join("testdata", "fingerprint.db"))

	game := gamesdb.Game{
		Fingerprint:    "already_uwp_game",
		AppUserModelID: "Pkg!App",
		Versions:       []string{"uwp"},
		Overrides:      map[string]string{"DriverProfile": "game_uwp.exe"},
	}

	first := PatchGame(db, game)
	if first.Status != StatusPatched {
		t.Fatalf("first status = %q, want %q", first.Status, StatusPatched)
	}

	// Same overrides again: nothing changes, so the result is "already present".
	second := PatchGame(db, game)
	if second.Status != StatusAlreadyPresent {
		t.Errorf("second status = %q, want %q", second.Status, StatusAlreadyPresent)
	}
}

func TestPatchGame_UpdateExistingUWP_KeepsDefaultRemoveFields(t *testing.T) {
	db, _ := ParseProfileDB(filepath.Join("testdata", "fingerprint.db"))
	fp := FindFingerprint(db, "already_uwp_game")
	uwp := findVersion(fp, "uwp")
	// Inject a field that AddUWPVersion would remove by default
	uwp.Elements = append(uwp.Elements, XmlElement{
		XMLName: xml.Name{Local: "Files"},
		Content: "some_files",
	})

	overrides := map[string]string{"DriverProfile": "game_uwp.exe"}
	result := PatchGame(db, gamesdb.Game{Fingerprint: "already_uwp_game", AppUserModelID: "Pkg!App", Versions: []string{"uwp"}, Overrides: overrides})
	if result.Status != StatusPatched {
		t.Fatalf("status = %q, want %q", result.Status, StatusPatched)
	}

	// Update mode must not apply default removals: Files survives
	found := false
	for _, e := range findVersion(fp, "uwp").Elements {
		if e.ElementName() == "Files" {
			found = true
			if e.Content != "some_files" {
				t.Errorf("Files = %q, want some_files", e.Content)
			}
		}
	}
	if !found {
		t.Error("Files should be preserved in update mode (default removals only apply to new additions)")
	}
}

func TestPatchGame_EnsureVersions_AddAndUpdate(t *testing.T) {
	db, _ := ParseProfileDB(filepath.Join("testdata", "fingerprint.db"))

	// final_fantasy_vii_remake has steam + epic, no uwp
	overrides := map[string]string{"DriverProfile": "custom.exe"}
	result := PatchGame(db, gamesdb.Game{Fingerprint: "final_fantasy_vii_remake", AppUserModelID: "TestPkg!App", Versions: []string{"uwp", "steam"}, Overrides: overrides})
	if result.Status != StatusPatched {
		t.Fatalf("status = %q, want %q (%s)", result.Status, StatusPatched, result.Message)
	}

	fp := FindFingerprint(db, "final_fantasy_vii_remake")
	// UWP was added
	uwp := findVersion(fp, "uwp")
	if uwp == nil {
		t.Fatal("UWP version should have been added")
	}
	// Steam was updated with the override
	steam := findVersion(fp, "steam")
	if steam == nil {
		t.Fatal("steam version not found")
	}
	found := false
	for _, e := range steam.Elements {
		if e.ElementName() == "DriverProfile" && e.Content == "custom.exe" {
			found = true
		}
	}
	if !found {
		t.Error("steam version should have DriverProfile=custom.exe")
	}
	// Epic untouched
	epic := findVersion(fp, "epic")
	if epic == nil {
		t.Fatal("epic version not found")
	}
	for _, e := range epic.Elements {
		if e.ElementName() == "DriverProfile" && e.Content == "custom.exe" {
			t.Error("epic version should not have been modified")
		}
	}
}

func TestPatchGame_EnsureVersions_MultiUpdate(t *testing.T) {
	db, _ := ParseProfileDB(filepath.Join("testdata", "fingerprint.db"))

	// already_uwp_game has steam + uwp
	overrides := map[string]string{"DriverProfile": "game_uwp.exe"}
	result := PatchGame(db, gamesdb.Game{Fingerprint: "already_uwp_game", AppUserModelID: "Pkg!App", Versions: []string{"steam", "uwp"}, Overrides: overrides})
	if result.Status != StatusPatched {
		t.Fatalf("status = %q, want %q (%s)", result.Status, StatusPatched, result.Message)
	}

	fp := FindFingerprint(db, "already_uwp_game")
	for _, name := range []string{"steam", "uwp"} {
		v := findVersion(fp, name)
		if v == nil {
			t.Fatalf("%s version not found", name)
		}
		found := false
		for _, e := range v.Elements {
			if e.ElementName() == "DriverProfile" && e.Content == "game_uwp.exe" {
				found = true
			}
		}
		if !found {
			t.Errorf("%s version should have DriverProfile=game_uwp.exe", name)
		}
	}
}

func TestPatchGame_EnsureVersions_Missing(t *testing.T) {
	db, _ := ParseProfileDB(filepath.Join("testdata", "fingerprint.db"))

	result := PatchGame(db, gamesdb.Game{Fingerprint: "already_uwp_game", AppUserModelID: "Pkg!App", Versions: []string{"gog", "origin"}})
	if result.Status != StatusVersionNotFound {
		t.Errorf("status = %q, want %q", result.Status, StatusVersionNotFound)
	}
}

func TestPatchGame_EnsureVersions_Mixed(t *testing.T) {
	db, _ := ParseProfileDB(filepath.Join("testdata", "fingerprint.db"))

	// uwp added, gog missing
	overrides := map[string]string{"DriverProfile": "custom.exe"}
	result := PatchGame(db, gamesdb.Game{Fingerprint: "final_fantasy_vii_remake", AppUserModelID: "TestPkg!App", Versions: []string{"uwp", "gog"}, Overrides: overrides})
	if result.Status != StatusPatched {
		t.Fatalf("status = %q, want %q (%s)", result.Status, StatusPatched, result.Message)
	}
	if !strings.Contains(result.Message, "gog not found") {
		t.Errorf("message should mention missing gog, got %q", result.Message)
	}
	if findVersion(FindFingerprint(db, "final_fantasy_vii_remake"), "uwp") == nil {
		t.Error("UWP version should have been added")
	}
}

func TestPatchGame_EnsureVersions_CaseInsensitive(t *testing.T) {
	db, _ := ParseProfileDB(filepath.Join("testdata", "fingerprint.db"))

	overrides := map[string]string{"DriverProfile": "custom.exe"}
	result := PatchGame(db, gamesdb.Game{Fingerprint: "already_uwp_game", AppUserModelID: "Pkg!App", Versions: []string{"Steam"}, Overrides: overrides})
	if result.Status != StatusPatched {
		t.Fatalf("status = %q, want %q (%s)", result.Status, StatusPatched, result.Message)
	}
	steam := findVersion(FindFingerprint(db, "already_uwp_game"), "steam")
	if steam == nil {
		t.Fatal("steam version not found")
	}
	found := false
	for _, e := range steam.Elements {
		if e.ElementName() == "DriverProfile" && e.Content == "custom.exe" {
			found = true
		}
	}
	if !found {
		t.Error("steam version should have DriverProfile=custom.exe")
	}
}

func TestPatchGame_EnsureVersions_NoChanges(t *testing.T) {
	db, _ := ParseProfileDB(filepath.Join("testdata", "fingerprint.db"))

	result := PatchGame(db, gamesdb.Game{Fingerprint: "already_uwp_game", AppUserModelID: "Pkg!App", Versions: []string{"steam", "uwp"}})
	if result.Status != StatusAlreadyPresent {
		t.Errorf("status = %q, want %q", result.Status, StatusAlreadyPresent)
	}
}

func TestWriteAndReadRoundTrip(t *testing.T) {
	// Parse, modify, write, re-parse
	db, _ := ParseProfileDB(filepath.Join("testdata", "fingerprint.db"))

	PatchGame(db, gamesdb.Game{Fingerprint: "final_fantasy_vii_remake", AppUserModelID: "39EA002F.EXED1_n746a19ndrrjg!AppFINALFANTASYVIIREMAKEShipping", Versions: []string{"uwp"}})

	// Write to temp file
	tmpDir := t.TempDir()
	tmpPath := filepath.Join(tmpDir, "fingerprint.db")

	if err := WriteProfileDB(db, tmpPath); err != nil {
		t.Fatalf("WriteProfileDB failed: %v", err)
	}

	// Re-parse
	db2, err := ParseProfileDB(tmpPath)
	if err != nil {
		t.Fatalf("re-parse failed: %v", err)
	}

	fp := FindFingerprint(db2, "final_fantasy_vii_remake")
	if fp == nil {
		t.Fatal("fingerprint not found after round-trip")
	}

	// Should now have 3 versions (steam, epic, uwp)
	if len(fp.Versions) != 3 {
		t.Errorf("expected 3 versions after patch, got %d", len(fp.Versions))
	}

	// Find the UWP version
	var uwpFound bool
	for _, v := range fp.Versions {
		if v.Name == "uwp" {
			uwpFound = true
			break
		}
	}
	if !uwpFound {
		t.Error("UWP version not found after round-trip")
	}
}

func TestBackupFile(t *testing.T) {
	tmpDir := t.TempDir()
	srcPath := filepath.Join(tmpDir, "test.db")
	bakPath := srcPath + ".bak"

	// Create source file
	os.WriteFile(srcPath, []byte("test content"), 0o644)

	// First backup should succeed
	if err := BackupFile(srcPath); err != nil {
		t.Fatalf("BackupFile failed: %v", err)
	}

	// Check backup exists
	if _, err := os.Stat(bakPath); err != nil {
		t.Fatalf("backup file not created: %v", err)
	}

	// Second backup should not overwrite
	original, _ := os.ReadFile(bakPath)
	os.WriteFile(srcPath, []byte("modified content"), 0o644)
	BackupFile(srcPath)
	after, _ := os.ReadFile(bakPath)
	if string(after) != string(original) {
		t.Error("BackupFile should not overwrite existing backup")
	}
}

func TestXmlUnmarshalMarshal(t *testing.T) {
	// Test that our XML model can parse and re-serialize the test file
	data, err := os.ReadFile(filepath.Join("testdata", "fingerprint.db"))
	if err != nil {
		t.Fatal(err)
	}

	var db ProfileDB
	if err := xml.Unmarshal(data, &db); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}

	// Re-marshal
	output, err := xml.MarshalIndent(db, "", "  ")
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}

	result := string(output)

	// Basic checks
	if !strings.Contains(result, "final_fantasy_vii_remake") {
		t.Error("output missing fingerprint name")
	}
	if !strings.Contains(result, `name="steam"`) {
		t.Error("output missing steam version")
	}
	if !strings.Contains(result, "SteamAppIds") {
		t.Error("output missing SteamAppIds")
	}
}

func TestFindSourceVersion_EpicFallback(t *testing.T) {
	db, _ := ParseProfileDB(filepath.Join("testdata", "fingerprint.db"))
	fp := FindFingerprint(db, "epic_only_game")
	if fp == nil {
		t.Fatal("epic_only_game fingerprint not found")
	}

	src := FindSourceVersion(fp)
	if src == nil {
		t.Fatal("expected a source version, got nil")
	}
	if !strings.EqualFold(src.Name, "epic") {
		t.Errorf("expected epic source version, got %q", src.Name)
	}
}

func TestAddUWPVersion_ForcedFieldOverride(t *testing.T) {
	db, _ := ParseProfileDB(filepath.Join("testdata", "fingerprint.db"))
	fp := FindFingerprint(db, "final_fantasy_vii_remake")
	src := FindSourceVersion(fp)

	appID := "TestPkg_abc!TestApp"

	// Override a forced field (Distributor) and a non-forced field.
	// User overrides take priority over forced field defaults.
	overrides := map[string]string{
		"Distributor":   "CustomDist",
		"DriverProfile": "custom.exe",
	}

	got := AddUWPVersion(src, appID, overrides, nil)

	// Build a map of element names (lowercased) to count and content
	seen := make(map[string][]string)
	for _, e := range got.Elements {
		lower := strings.ToLower(e.ElementName())
		seen[lower] = append(seen[lower], e.Content)
	}

	// Distributor must appear exactly once with the override value
	if count := len(seen["distributor"]); count != 1 {
		t.Errorf("Distributor appeared %d times, want 1", count)
	}
	if seen["distributor"][0] != "CustomDist" {
		t.Errorf("Distributor = %q, want CustomDist (override wins over forced default)", seen["distributor"][0])
	}

	// UWPPackageFamilyName must appear exactly once
	if count := len(seen["uwppackagefamilyname"]); count != 1 {
		t.Errorf("UWPPackageFamilyName appeared %d times, want 1", count)
	}
	if seen["uwppackagefamilyname"][0] != "TestPkg_abc" {
		t.Errorf("UWPPackageFamilyName = %q, want TestPkg_abc", seen["uwppackagefamilyname"][0])
	}

	// AppUserModelId must appear exactly once
	if count := len(seen["appusermodelid"]); count != 1 {
		t.Errorf("AppUserModelId appeared %d times, want 1", count)
	}
	if seen["appusermodelid"][0] != appID {
		t.Errorf("AppUserModelId = %q, want %s", seen["appusermodelid"][0], appID)
	}

	// Non-forced override should also work
	if count := len(seen["driverprofile"]); count != 1 {
		t.Errorf("DriverProfile appeared %d times, want 1", count)
	}
	if seen["driverprofile"][0] != "custom.exe" {
		t.Errorf("DriverProfile = %q, want custom.exe", seen["driverprofile"][0])
	}
}

func TestAddUWPVersion_ForcedFieldNoOverride(t *testing.T) {
	db, _ := ParseProfileDB(filepath.Join("testdata", "fingerprint.db"))
	fp := FindFingerprint(db, "final_fantasy_vii_remake")
	src := FindSourceVersion(fp)

	appID := "TestPkg_abc!TestApp"
	got := AddUWPVersion(src, appID, nil, nil)

	seen := make(map[string][]string)
	for _, e := range got.Elements {
		lower := strings.ToLower(e.ElementName())
		seen[lower] = append(seen[lower], e.Content)
	}

	// Without overrides, Distributor defaults to UWP
	if count := len(seen["distributor"]); count != 1 {
		t.Errorf("Distributor appeared %d times, want 1", count)
	}
	if seen["distributor"][0] != "UWP" {
		t.Errorf("Distributor = %q, want UWP", seen["distributor"][0])
	}

	// UWPPackageFamilyName and AppUserModelId must also appear exactly once
	if count := len(seen["uwppackagefamilyname"]); count != 1 {
		t.Errorf("UWPPackageFamilyName appeared %d times, want 1", count)
	}
	if count := len(seen["appusermodelid"]); count != 1 {
		t.Errorf("AppUserModelId appeared %d times, want 1", count)
	}
}

func TestAddUWPVersion_ForcedFieldOrder(t *testing.T) {
	db, _ := ParseProfileDB(filepath.Join("testdata", "fingerprint.db"))
	fp := FindFingerprint(db, "final_fantasy_vii_remake")
	src := FindSourceVersion(fp)

	appID := "TestPkg_abc!TestApp"
	got := AddUWPVersion(src, appID, nil, nil)

	// Collect the order of forced fields as they appear in the got
	var order []string
	for _, e := range got.Elements {
		lower := strings.ToLower(e.ElementName())
		if lower == "distributor" || lower == "uwppackagefamilyname" || lower == "appusermodelid" {
			order = append(order, lower)
		}
	}

	// applyOverrides emits elements sorted by lowercased key
	want := []string{"appusermodelid", "distributor", "uwppackagefamilyname"}
	if len(order) != len(want) {
		t.Fatalf("forced field count = %d, want %d (got %v)", len(order), len(want), order)
	}
	for i, got := range order {
		if got != want[i] {
			t.Errorf("forced field at position %d = %q, want %q", i, got, want[i])
		}
	}
}

func TestAddUWPVersion_NonForcedOverrideOrder(t *testing.T) {
	db, _ := ParseProfileDB(filepath.Join("testdata", "fingerprint.db"))
	fp := FindFingerprint(db, "final_fantasy_vii_remake")
	src := FindSourceVersion(fp)

	// Use multiple non-forced overrides to test determinism
	overrides := map[string]string{
		"ZebraField":    "z",
		"AlphaField":    "a",
		"MiddleField":   "m",
		"DriverProfile": "custom.exe", // override of existing field
	}

	got := AddUWPVersion(src, "TestPkg_abc!App", overrides, nil)

	// Collect the non-forced override elements by their appearance order
	var overrideNames []string
	for _, e := range got.Elements {
		name := e.ElementName()
		if name == "AlphaField" || name == "MiddleField" || name == "ZebraField" {
			overrideNames = append(overrideNames, name)
		}
	}

	// Non-forced overrides must appear in sorted order by key
	if len(overrideNames) != 3 {
		t.Fatalf("expected 3 non-forced override elements, got %d", len(overrideNames))
	}
	want := []string{"AlphaField", "MiddleField", "ZebraField"}
	for i, got := range overrideNames {
		if got != want[i] {
			t.Errorf("non-forced override at position %d = %q, want %q", i, got, want[i])
		}
	}
}

func TestAddUWPVersion_OverrideOverridesDefaultRemove(t *testing.T) {
	// When an override key matches a field in defaultRemoveFields (e.g., EpicAppId),
	// the override should win: the source's EpicAppId element is skipped in
	// copyPreservedElements (because it's in the remove set), but the override
	// value is then added by applyOverrides.
	db, _ := ParseProfileDB(filepath.Join("testdata", "fingerprint.db"))
	fp := FindFingerprint(db, "final_fantasy_vii_remake")
	src := FindSourceVersion(fp)

	overrides := map[string]string{
		"EpicAppId": "my-custom-epic-id",
	}

	got := AddUWPVersion(src, "TestPkg!App", overrides, nil)

	// The got should have the override value for EpicAppId
	found := false
	for _, e := range got.Elements {
		if e.ElementName() == "EpicAppId" {
			found = true
			if e.Content != "my-custom-epic-id" {
				t.Errorf("EpicAppId = %q, want my-custom-epic-id", e.Content)
			}
		}
	}
	if !found {
		t.Error("expected override EpicAppId to be present in got")
	}

	// SteamAppIds (also in defaultRemoveFields, but not in overrides) should be absent
	for _, e := range got.Elements {
		if strings.EqualFold(e.ElementName(), "SteamAppIds") {
			t.Error("SteamAppIds should be removed (it's in defaultRemoveFields with no override)")
		}
	}
}

func TestFingerprintLevelElementsPreserved(t *testing.T) {
	// Test that DisplayName, ChromaAppID, and other Fingerprint-level
	// elements are preserved through parse/marshal round-trip.
	data, err := os.ReadFile(filepath.Join("testdata", "fingerprint_metadata.db"))
	if err != nil {
		t.Fatalf("reading testdata: %v", err)
	}

	var db ProfileDB
	if err := xml.Unmarshal(data, &db); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}

	// Verify elements were parsed
	fp := FindFingerprint(&db, "with_metadata")
	if fp == nil {
		t.Fatal("fingerprint with_metadata not found")
	}

	// Collect Fingerprint-level elements (not Version children)
	elemNames := make(map[string]string)
	for _, e := range fp.Elements {
		elemNames[strings.ToLower(e.ElementName())] = e.Content
	}

	if elemNames["displayname"] != "Game With Metadata" {
		t.Errorf("DisplayName = %q, want %q", elemNames["displayname"], "Game With Metadata")
	}
	if elemNames["chromaappid"] != "81810b31-1b34-4921-8ab3-c6c3485fe4ce" {
		t.Errorf("ChromaAppID = %q, want %q", elemNames["chromaappid"], "81810b31-1b34-4921-8ab3-c6c3485fe4ce")
	}
	if elemNames["iscreativeapplication"] != "1" {
		t.Errorf("IsCreativeApplication = %q, want %q", elemNames["iscreativeapplication"], "1")
	}

	// Verify Versions still present
	if len(fp.Versions) != 1 {
		t.Fatalf("expected 1 version, got %d", len(fp.Versions))
	}
	if fp.Versions[0].Name != "steam" {
		t.Errorf("version name = %q, want steam", fp.Versions[0].Name)
	}

	// Round-trip: marshal and re-parse
	output, err := xml.MarshalIndent(&db, "", "  ")
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}
	content := xml.Header + string(output) + "\n"

	var db2 ProfileDB
	if err := xml.Unmarshal([]byte(content), &db2); err != nil {
		t.Fatalf("re-unmarshal failed: %v", err)
	}

	fp2 := FindFingerprint(&db2, "with_metadata")
	if fp2 == nil {
		t.Fatal("fingerprint with_metadata not found after round-trip")
	}

	// Verify elements preserved after round-trip
	elemNames2 := make(map[string]string)
	for _, e := range fp2.Elements {
		elemNames2[strings.ToLower(e.ElementName())] = e.Content
	}
	if elemNames2["displayname"] != "Game With Metadata" {
		t.Errorf("round-trip DisplayName = %q, want %q", elemNames2["displayname"], "Game With Metadata")
	}
	if elemNames2["chromaappid"] != "81810b31-1b34-4921-8ab3-c6c3485fe4ce" {
		t.Errorf("round-trip ChromaAppID = %q, want %q", elemNames2["chromaappid"], "81810b31-1b34-4921-8ab3-c6c3485fe4ce")
	}
	if elemNames2["iscreativeapplication"] != "1" {
		t.Errorf("round-trip IsCreativeApplication = %q, want %q", elemNames2["iscreativeapplication"], "1")
	}

	// Verify versions still intact after round-trip
	if len(fp2.Versions) != 1 {
		t.Fatalf("round-trip: expected 1 version, got %d", len(fp2.Versions))
	}
}

func TestFingerprintLevelElementsNotLostOnPatch(t *testing.T) {
	// When patching a fingerprint, its Fingerprint-level elements
	// (DisplayName, ChromaAppID, etc.) must be preserved in the output.
	db, err := ParseProfileDB(filepath.Join("testdata", "fingerprint_metadata.db"))
	if err != nil {
		t.Fatalf("ParseProfileDB failed: %v", err)
	}

	result := PatchGame(db, gamesdb.Game{Fingerprint: "with_metadata", AppUserModelID: "TestPkg!App", Versions: []string{"uwp"}})
	if result.Status != "patched" {
		t.Fatalf("expected patched, got %s: %s", result.Status, result.Message)
	}

	// Write to temp and re-read
	tmpDir := t.TempDir()
	tmpPath := filepath.Join(tmpDir, "fingerprint.db")
	if err := WriteProfileDB(db, tmpPath); err != nil {
		t.Fatalf("WriteProfileDB failed: %v", err)
	}

	db2, err := ParseProfileDB(tmpPath)
	if err != nil {
		t.Fatalf("re-parse failed: %v", err)
	}

	fp := FindFingerprint(db2, "with_metadata")
	if fp == nil {
		t.Fatal("fingerprint not found after round-trip")
	}

	elemNames := make(map[string]string)
	for _, e := range fp.Elements {
		elemNames[strings.ToLower(e.ElementName())] = e.Content
	}
	if elemNames["displayname"] != "Game With Metadata" {
		t.Errorf("DisplayName = %q after patch round-trip, want %q", elemNames["displayname"], "Game With Metadata")
	}
	if elemNames["chromaappid"] != "81810b31-1b34-4921-8ab3-c6c3485fe4ce" {
		t.Errorf("ChromaAppID = %q after patch round-trip, want %q", elemNames["chromaappid"], "81810b31-1b34-4921-8ab3-c6c3485fe4ce")
	}
}

func TestParseProfileDB_FingerprintDBRoot(t *testing.T) {
	// Verify that the root element <FingerprintDB> is correctly parsed.
	// This is the real-world root element name (BUG-0 fix).
	db, err := ParseProfileDB(filepath.Join("testdata", "fingerprint.db"))
	if err != nil {
		t.Fatalf("ParseProfileDB failed: %v", err)
	}
	if len(db.Fingerprints) == 0 {
		t.Error("expected at least one fingerprint")
	}
	if db.XMLName.Local != "FingerprintDB" {
		t.Errorf("root element = %q, want FingerprintDB", db.XMLName.Local)
	}
}
