package nvidia

import (
	"encoding/xml"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseProfileDB(t *testing.T) {
	db, err := ParseProfileDB(filepath.Join("testdata", "fingerprint.db"))
	if err != nil {
		t.Fatalf("ParseProfileDB failed: %v", err)
	}

	if len(db.Fingerprints) != 3 {
		t.Fatalf("expected 3 fingerprints, got %d", len(db.Fingerprints))
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

func TestHasUWPVersion(t *testing.T) {
	db, _ := ParseProfileDB(filepath.Join("testdata", "fingerprint.db"))

	tests := []struct {
		name   string
		hasUWP bool
	}{
		{"final_fantasy_vii_remake", false},
		{"already_uwp_game", true},
		{"no_source_game", true},
	}

	for _, tt := range tests {
		fp := FindFingerprint(db, tt.name)
		if fp == nil {
			t.Fatalf("fingerprint %q not found", tt.name)
		}
		if HasUWPVersion(fp) != tt.hasUWP {
			t.Errorf("HasUWPVersion(%q) = %v, want %v", tt.name, !tt.hasUWP, tt.hasUWP)
		}
	}
}

func TestCloneVersion(t *testing.T) {
	db, _ := ParseProfileDB(filepath.Join("testdata", "fingerprint.db"))
	fp := FindFingerprint(db, "final_fantasy_vii_remake")
	src := FindSourceVersion(fp)

	appID := "39EA002F.EXED1_n746a19ndrrjg!AppFINALFANTASYVIIREMAKEShipping"
	clone := CloneVersion(src, appID, nil, nil)

	// Check name
	if clone.Name != "uwp" {
		t.Errorf("clone name = %q, want uwp", clone.Name)
	}

	// Check that removed fields are gone
	elementNames := make(map[string]bool)
	for _, e := range clone.Elements {
		elementNames[strings.ToLower(e.ElementName())] = true
	}

	for _, removed := range []string{"Files", "Launch", "SteamAppIds", "Directories", "InstallDirRegValues"} {
		if elementNames[strings.ToLower(removed)] {
			t.Errorf("clone should not contain %s", removed)
		}
	}

	// Check that UWP-specific fields are present
	if !elementNames["uwppackagefamilyname"] {
		t.Error("clone should contain UWPPackageFamilyName")
	}
	if !elementNames["appusermodelid"] {
		t.Error("clone should contain AppUserModelId")
	}

	// Check Distributor is UWP
	for _, e := range clone.Elements {
		if strings.ToLower(e.ElementName()) == "distributor" {
			if e.Content != "UWP" {
				t.Errorf("Distributor = %q, want UWP", e.Content)
			}
		}
	}

	// Check preserved fields
	if !elementNames["cmsid"] {
		t.Error("clone should contain CMSID")
	}
	if !elementNames["driverprofile"] {
		t.Error("clone should contain DriverProfile")
	}

	// Check UWPPackageFamilyName derivation
	for _, e := range clone.Elements {
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

func TestCloneWithOverrides(t *testing.T) {
	db, _ := ParseProfileDB(filepath.Join("testdata", "fingerprint.db"))
	fp := FindFingerprint(db, "final_fantasy_vii_remake")
	src := FindSourceVersion(fp)

	overrides := map[string]string{
		"DriverProfile": "custom_exe.exe",
		"SpecialFlag":   "1",
	}

	clone := CloneVersion(src, "Pkg_abc!AppX", overrides, nil)

	elementMap := make(map[string]string)
	for _, e := range clone.Elements {
		elementMap[e.ElementName()] = e.Content
	}

	if elementMap["DriverProfile"] != "custom_exe.exe" {
		t.Errorf("DriverProfile override = %q, want custom_exe.exe", elementMap["DriverProfile"])
	}
	if elementMap["SpecialFlag"] != "1" {
		t.Errorf("SpecialFlag = %q, want 1", elementMap["SpecialFlag"])
	}
}

func TestCloneWithRemove(t *testing.T) {
	db, _ := ParseProfileDB(filepath.Join("testdata", "fingerprint.db"))
	fp := FindFingerprint(db, "final_fantasy_vii_remake")
	src := FindSourceVersion(fp)

	clone := CloneVersion(src, "Pkg_abc!AppX", nil, []string{"WhisperModePopsFactor"})

	for _, e := range clone.Elements {
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
		wantStatus  string
	}{
		{"final_fantasy_vii_remake", "39EA002F.EXED1_n746a19ndrrjg!AppFINALFANTASYVIIREMAKEShipping", "patched"},
		{"already_uwp_game", "Pkg!App", "already_uwp"},
		{"nonexistent_game", "Pkg!App", "not_found"},
		{"no_source_game", "Pkg!App", "already_uwp"},
	}

	for _, tt := range tests {
		result := PatchGame(db, tt.fingerprint, tt.appID, nil, nil)
		if result.Status != tt.wantStatus {
			t.Errorf("PatchGame(%q) status = %q, want %q", tt.fingerprint, result.Status, tt.wantStatus)
		}
	}
}

func TestWriteAndReadRoundTrip(t *testing.T) {
	// Parse, modify, write, re-parse
	db, _ := ParseProfileDB(filepath.Join("testdata", "fingerprint.db"))

	PatchGame(db, "final_fantasy_vii_remake", "39EA002F.EXED1_n746a19ndrrjg!AppFINALFANTASYVIIREMAKEShipping", nil, nil)

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