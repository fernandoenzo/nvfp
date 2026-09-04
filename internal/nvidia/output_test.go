package nvidia

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	gamesdb "github.com/fernandoenzo/nvidia-uwp-patch/internal/db"
)

func TestPatchOutputContent(t *testing.T) {
	db, err := ParseProfileDB(filepath.Join("testdata", "fingerprint.db"))
	if err != nil {
		t.Fatalf("ParseProfileDB failed: %v", err)
	}

	result := PatchGame(db, &gamesdb.Game{
		Fingerprint:    "final_fantasy_vii_remake",
		AppUserModelID: "39EA002F.EXED1_n746a19ndrrjg!AppFINALFANTASYVIIREMAKEShipping",
		Versions:       []string{"uwp"},
		Remove:         []string{"WhisperModePopsFactor"},
		Overrides:      map[string]string{"DriverProfile": "FF7R_UWP.exe"},
	})

	if result.Status != "patched" {
		t.Fatalf("expected patched, got %s", result.Status)
	}

	// Write to temp file and read back content
	tmpDir := t.TempDir()
	tmpPath := filepath.Join(tmpDir, "fingerprint.db")
	if err := WriteProfileDB(db, tmpPath); err != nil {
		t.Fatalf("WriteProfileDB failed: %v", err)
	}

	contentBytes, err := os.ReadFile(tmpPath)
	if err != nil {
		t.Fatalf("reading output: %v", err)
	}
	content := string(contentBytes)

	// Elements that MUST be present in the UWP version
	mustPresent := []string{
		`name="uwp"`,
		`<UWPPackageFamilyName>39EA002F.EXED1_n746a19ndrrjg</UWPPackageFamilyName>`,
		`<AppUserModelId>39EA002F.EXED1_n746a19ndrrjg!AppFINALFANTASYVIIREMAKEShipping</AppUserModelId>`,
		`<Distributor>UWP</Distributor>`,
		`<DriverProfile>FF7R_UWP.exe</DriverProfile>`, // Override applied
		`<CMSID>12345</CMSID>`,                        // Preserved from source
		`<IsAutomatable>1</IsAutomatable>`,            // Preserved from source
	}

	for _, s := range mustPresent {
		if !strings.Contains(content, s) {
			t.Errorf("expected to find %q in output", s)
		}
	}

	// Now verify the UWP version specifically doesn't contain removed fields.
	// We need to check the UWP version in isolation, not the whole file
	// (since steam version legitimately has Files, Launch, etc.)
	fp := FindFingerprint(db, "final_fantasy_vii_remake")
	var uwpVersion *Version
	for i := range fp.Versions {
		if fp.Versions[i].Name == "uwp" {
			uwpVersion = &fp.Versions[i]
			break
		}
	}
	if uwpVersion == nil {
		t.Fatal("UWP version not found")
	}

	// Check that removed elements are absent from the UWP version
	removedElements := []string{"Files", "Launch", "SteamAppIds", "Directories", "InstallDirRegValues", "WhisperModePopsFactor"}
	for _, name := range removedElements {
		for _, elem := range uwpVersion.Elements {
			if strings.EqualFold(elem.ElementName(), name) {
				t.Errorf("UWP version should NOT contain %s, but found it", name)
			}
		}
	}

	// Check that UWP-specific fields are present
	elementNames := make(map[string]bool)
	for _, elem := range uwpVersion.Elements {
		elementNames[strings.ToLower(elem.ElementName())] = true
	}

	if !elementNames["uwppackagefamilyname"] {
		t.Error("UWP version should contain UWPPackageFamilyName")
	}
	if !elementNames["appusermodelid"] {
		t.Error("UWP version should contain AppUserModelId")
	}
	if !elementNames["distributor"] {
		t.Error("UWP version should contain Distributor")
	}

	// Check Distributor is UWP
	for _, elem := range uwpVersion.Elements {
		if strings.EqualFold(elem.ElementName(), "distributor") && elem.Content != "UWP" {
			t.Errorf("Distributor = %q, want UWP", elem.Content)
		}
	}

	// Check that the override replaced the source value
	for _, elem := range uwpVersion.Elements {
		if elem.ElementName() == "DriverProfile" && elem.Content != "FF7R_UWP.exe" {
			t.Errorf("DriverProfile = %q, want FF7R_UWP.exe", elem.Content)
		}
	}

	// Verify we can round-trip
	db2, err := ParseProfileDB(tmpPath)
	if err != nil {
		t.Fatalf("re-parse failed: %v", err)
	}

	fp2 := FindFingerprint(db2, "final_fantasy_vii_remake")
	if fp2 == nil {
		t.Fatal("fingerprint not found after round-trip")
	}
	if len(fp2.Versions) != 3 {
		t.Errorf("expected 3 versions after round-trip, got %d", len(fp2.Versions))
	}

	var uwp2 *Version
	for i := range fp2.Versions {
		if fp2.Versions[i].Name == "uwp" {
			uwp2 = &fp2.Versions[i]
			break
		}
	}
	if uwp2 == nil {
		t.Fatal("UWP version not found after round-trip")
	}

	// Verify round-trip preserved UWP fields
	uwp2Names := make(map[string]bool)
	for _, elem := range uwp2.Elements {
		uwp2Names[strings.ToLower(elem.ElementName())] = true
	}
	if !uwp2Names["uwppackagefamilyname"] {
		t.Error("round-trip: UWP version should contain UWPPackageFamilyName")
	}
	if !uwp2Names["appusermodelid"] {
		t.Error("round-trip: UWP version should contain AppUserModelId")
	}
}
