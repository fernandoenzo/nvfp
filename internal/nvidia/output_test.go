package nvidia

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	gamesdb "github.com/fernandoenzo/nvfp/internal/db"
	"github.com/fernandoenzo/set"
)

func TestPatchOutputContent(t *testing.T) {
	db, err := ParseFingerprintDB(filepath.Join("testdata", "fingerprint.db"))
	if err != nil {
		t.Fatalf("ParseFingerprintDB failed: %v", err)
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

	tmpDir := t.TempDir()
	tmpPath := filepath.Join(tmpDir, "fingerprint.db")
	if err := WriteFingerprintDB(db, tmpPath); err != nil {
		t.Fatalf("WriteFingerprintDB failed: %v", err)
	}

	contentBytes, err := os.ReadFile(tmpPath)
	if err != nil {
		t.Fatalf("reading output: %v", err)
	}
	content := string(contentBytes)

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
			uwpVersion = fp.Versions[i]
			break
		}
	}
	if uwpVersion == nil {
		t.Fatal("UWP version not found")
	}

	removedElements := []string{"Files", "Launch", "SteamAppIds", "Directories", "InstallDirRegValues", "WhisperModePopsFactor"}
	for _, name := range removedElements {
		for _, elem := range uwpVersion.Elements {
			if strings.EqualFold(elem.ElementName(), name) {
				t.Errorf("UWP version should NOT contain %s, but found it", name)
			}
		}
	}

	elementNames := set.New[string](len(uwpVersion.Elements))
	for _, elem := range uwpVersion.Elements {
		elementNames.Add(strings.ToLower(elem.ElementName()))
	}

	if !elementNames.Contains("uwppackagefamilyname") {
		t.Error("UWP version should contain UWPPackageFamilyName")
	}
	if !elementNames.Contains("appusermodelid") {
		t.Error("UWP version should contain AppUserModelId")
	}
	if !elementNames.Contains("distributor") {
		t.Error("UWP version should contain Distributor")
	}

	for _, elem := range uwpVersion.Elements {
		if strings.EqualFold(elem.ElementName(), "distributor") && elem.Content != "UWP" {
			t.Errorf("Distributor = %q, want UWP", elem.Content)
		}
	}

	for _, elem := range uwpVersion.Elements {
		if elem.ElementName() == "DriverProfile" && elem.Content != "FF7R_UWP.exe" {
			t.Errorf("DriverProfile = %q, want FF7R_UWP.exe", elem.Content)
		}
	}

	// Whitespace-only chardata (indentation between child elements) must not
	// survive the round-trip as &#xA; entities.
	if strings.Contains(content, "&#xA;") {
		t.Error("output contains &#xA; entities from indentation whitespace")
	}

	db2, err := ParseFingerprintDB(tmpPath)
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
			uwp2 = fp2.Versions[i]
			break
		}
	}
	if uwp2 == nil {
		t.Fatal("UWP version not found after round-trip")
	}

	uwp2Names := set.New[string](len(uwp2.Elements))
	for _, elem := range uwp2.Elements {
		uwp2Names.Add(strings.ToLower(elem.ElementName()))
	}
	if !uwp2Names.Contains("uwppackagefamilyname") {
		t.Error("round-trip: UWP version should contain UWPPackageFamilyName")
	}
	if !uwp2Names.Contains("appusermodelid") {
		t.Error("round-trip: UWP version should contain AppUserModelId")
	}
}
