package ngx

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The two bundles ship the same features under different names: sl_sdk_0
// (hash E658703) uses sl_<feat>_0, sl_sdk_override_0 (hash E658700) uses
// sl_<feat>_override_0.
const (
	plainConfig    = "sl_common_0, 2.14.0, .dll, sl.common.dll\nsl_reflex_0, 2.14.0, .dll, sl.reflex.dll\n"
	overrideConfig = "sl_common_override_0, 2.14.0, .dll, sl.common.dll\nsl_reflex_override_0, 2.14.0, .dll, sl.reflex.dll\n"
)

// writeFile creates a file and its directory, failing the test on error.
func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("creating %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("writing %s: %v", path, err)
	}
}

// cacheFixture builds a fake NGX cache with the given package configs and
// payloads, returning its root.
func cacheFixture(t *testing.T, manifest string, packages map[string]string, payloads []string) string {
	t.Helper()
	root := t.TempDir()
	writeFile(t, filepath.Join(root, manifestName), manifest)
	for bundle, content := range packages {
		writeFile(t, filepath.Join(root, bundle, "versions", "1", packageConfigName), content)
	}
	for _, payload := range payloads {
		writeFile(t, filepath.Join(root, payload), "payload bytes")
	}
	return root
}

// payload builds a cache-relative payload path; ota is the version directory.
func payload(feature, hash, ota string) string {
	return filepath.Join(feature, "versions", ota, "files", "1B0_"+hash+".dll")
}

func TestInspectFindsMissingSections(t *testing.T) {
	root := cacheFixture(t,
		"[sl_sdk_0]\r\napp_E658703 = 2.14.0",
		map[string]string{"sl_sdk_0": plainConfig, "sl_sdk_override_0": overrideConfig},
		// Only the override family carries payloads: the plain family's files
		// must be filled from its sibling.
		[]string{
			payload("sl_common_override_0", "E658700", "134656"),
			payload("sl_reflex_override_0", "E658700", "134656"),
		})

	plan, err := Inspect(root)
	if err != nil {
		t.Fatalf("Inspect: %v", err)
	}
	if got := len(plan.Sections); got != 4 {
		t.Fatalf("sections = %d, want 4", got)
	}
	if got := len(plan.Copies); got != 2 {
		t.Fatalf("copies = %d, want 2", len(plan.Copies))
	}
	for _, copy := range plan.Copies {
		if !strings.Contains(copy.Source, "sl_common_override_0") && !strings.Contains(copy.Source, "sl_reflex_override_0") {
			t.Errorf("copy source %s should come from the override family", copy.Source)
		}
	}
	if got := len(plan.Pending()); got != 4 {
		t.Fatalf("pending = %d, want 4", got)
	}
	if !plan.Changed() {
		t.Error("Changed() = false, want true")
	}
}

func TestInspectUsesOTAHashPath(t *testing.T) {
	root := cacheFixture(t, "", map[string]string{"sl_sdk_0": "sl_common_0, 2.14.3, .dll, sl.common.dll\n"},
		[]string{payload("sl_common_override_0", "E658700", "134659")})

	plan, err := Inspect(root)
	if err != nil {
		t.Fatalf("Inspect: %v", err)
	}
	// 2.14.3 -> 0x020E03 = 134659, not the 2.14.0 directory.
	want := filepath.Join("sl_common_0", "versions", "134659", "files", "1B0_E658703.dll")
	if len(plan.Copies) != 1 {
		t.Fatalf("copies = %v, want one", plan.Copies)
	}
	if !strings.Contains(plan.Copies[0].Dest, want) {
		t.Errorf("dest %s should contain %s", plan.Copies[0].Dest, want)
	}
}

func TestInspectSkipsSectionsAlreadyPresent(t *testing.T) {
	root := cacheFixture(t,
		"[sl_sdk_0]\r\napp_E658703 = 2.14.0\r\n\r\n[sl_common_0]\r\napp_E658703 = 2.14.0",
		map[string]string{"sl_sdk_0": "sl_common_0, 2.14.0, .dll, sl.common.dll\n"},
		[]string{payload("sl_common_0", "E658703", "134656")})

	plan, err := Inspect(root)
	if err != nil {
		t.Fatalf("Inspect: %v", err)
	}
	if len(plan.Pending()) != 0 {
		t.Errorf("pending = %v, want none", plan.Pending())
	}
	if plan.Changed() {
		t.Error("Changed() = true, want false")
	}
	if len(plan.Copies) != 0 {
		t.Errorf("copies = %v, want none: the payload already exists", plan.Copies)
	}
}

func TestInspectReportsFeatureWithoutPayload(t *testing.T) {
	root := cacheFixture(t, "", map[string]string{"sl_sdk_0": "sl_directsr_0, 2.14.0, .dll, sl.directsr.dll\n"}, nil)

	plan, err := Inspect(root)
	if err != nil {
		t.Fatalf("Inspect: %v", err)
	}
	if len(plan.Missing) != 1 || plan.Missing[0] != "sl_directsr_0" {
		t.Errorf("missing = %v, want [sl_directsr_0]", plan.Missing)
	}
	if len(plan.Sections) != 0 {
		t.Errorf("sections = %v, want none: no payload means no section", plan.Sections)
	}
}

func TestInspectWarnsOnUnreadableBundle(t *testing.T) {
	root := cacheFixture(t, "", map[string]string{"sl_sdk_override_0": overrideConfig},
		[]string{payload("sl_common_override_0", "E658700", "134656")})

	plan, err := Inspect(root)
	if err != nil {
		t.Fatalf("Inspect: %v", err)
	}
	if len(plan.Warnings) != 1 || !strings.Contains(plan.Warnings[0], "sl_sdk_0") {
		t.Errorf("warnings = %v, want one about sl_sdk_0", plan.Warnings)
	}
}

func TestInspectErrorsWithoutManifest(t *testing.T) {
	if _, err := Inspect(t.TempDir()); err == nil {
		t.Fatal("Inspect with no manifest should fail")
	}
}

func TestApplyAppendsSectionsAndBacksUp(t *testing.T) {
	initial := "[sl_sdk_0]\r\napp_E658703 = 2.14.0"
	root := cacheFixture(t, initial,
		map[string]string{"sl_sdk_0": "sl_common_0, 2.14.0, .dll, sl.common.dll\n"},
		[]string{payload("sl_common_0", "E658703", "134656")})

	plan, err := Inspect(root)
	if err != nil {
		t.Fatalf("Inspect: %v", err)
	}
	if err := Apply(plan); err != nil {
		t.Fatalf("Apply: %v", err)
	}

	backup, err := os.ReadFile(plan.Backup)
	if err != nil {
		t.Fatalf("reading backup: %v", err)
	}
	if string(backup) != initial {
		t.Errorf("backup = %q, want the pre-write manifest %q", backup, initial)
	}
	manifest, err := os.ReadFile(plan.Manifest)
	if err != nil {
		t.Fatalf("reading manifest: %v", err)
	}
	if !strings.Contains(string(manifest), "\r\n[sl_common_0]\r\napp_E658703 = 2.14.0") {
		t.Errorf("manifest = %q, want the appended CRLF section", manifest)
	}
}

func TestApplyIsIdempotent(t *testing.T) {
	root := cacheFixture(t, "[sl_sdk_override_0]\r\napp_E658700 = 2.14.0",
		map[string]string{"sl_sdk_override_0": overrideConfig},
		[]string{payload("sl_common_override_0", "E658700", "134656")})

	first, err := Inspect(root)
	if err != nil {
		t.Fatalf("Inspect: %v", err)
	}
	if err := Apply(first); err != nil {
		t.Fatalf("first Apply: %v", err)
	}
	afterFirst, err := os.ReadFile(first.Manifest)
	if err != nil {
		t.Fatalf("reading manifest: %v", err)
	}

	second, err := Inspect(root)
	if err != nil {
		t.Fatalf("second Inspect: %v", err)
	}
	if second.Changed() {
		t.Errorf("second plan still has work: %d copies, %d pending", len(second.Copies), len(second.Pending()))
	}
	if err := Apply(second); err != nil {
		t.Fatalf("second Apply: %v", err)
	}
	afterSecond, err := os.ReadFile(second.Manifest)
	if err != nil {
		t.Fatalf("reading manifest: %v", err)
	}
	if string(afterFirst) != string(afterSecond) {
		t.Errorf("manifest changed on the second run:\n%q\n%q", afterFirst, afterSecond)
	}
}

func TestApplyCopiesPayloadFromSibling(t *testing.T) {
	root := cacheFixture(t, "",
		map[string]string{"sl_sdk_0": "sl_common_0, 2.14.0, .dll, sl.common.dll\n", "sl_sdk_override_0": overrideConfig},
		[]string{payload("sl_common_override_0", "E658700", "134656")})

	plan, err := Inspect(root)
	if err != nil {
		t.Fatalf("Inspect: %v", err)
	}
	if len(plan.Copies) != 1 {
		t.Fatalf("copies = %v, want one", plan.Copies)
	}
	if err := Apply(plan); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	copied, err := os.ReadFile(plan.Copies[0].Dest)
	if err != nil {
		t.Fatalf("reading copied payload: %v", err)
	}
	if string(copied) != "payload bytes" {
		t.Errorf("copied payload = %q, want the sibling's bytes", copied)
	}
}

func TestApplyDoesNotOverwriteExistingBackup(t *testing.T) {
	root := cacheFixture(t, "[sl_sdk_0]",
		map[string]string{"sl_sdk_0": "sl_common_0, 2.14.0, .dll, sl.common.dll\n"},
		[]string{payload("sl_common_0", "E658703", "134656")})
	writeFile(t, filepath.Join(root, manifestName+".bak"), "older backup")

	plan, err := Inspect(root)
	if err != nil {
		t.Fatalf("Inspect: %v", err)
	}
	if err := Apply(plan); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	backup, err := os.ReadFile(plan.Backup)
	if err != nil {
		t.Fatalf("reading backup: %v", err)
	}
	if string(backup) != "older backup" {
		t.Errorf("backup = %q, want the oldest copy preserved", backup)
	}
}

func TestApplySkipsEverythingWhenNothingToDo(t *testing.T) {
	initial := "[sl_sdk_0]\r\n\r\n[sl_common_0]\r\napp_E658703 = 2.14.0"
	root := cacheFixture(t, initial,
		map[string]string{"sl_sdk_0": "sl_common_0, 2.14.0, .dll, sl.common.dll\n"},
		[]string{payload("sl_common_0", "E658703", "134656")})

	plan, err := Inspect(root)
	if err != nil {
		t.Fatalf("Inspect: %v", err)
	}
	if err := Apply(plan); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if _, err := os.Stat(plan.Backup); !os.IsNotExist(err) {
		t.Errorf("a backup appeared for an unchanged manifest: %v", err)
	}
	manifest, err := os.ReadFile(plan.Manifest)
	if err != nil {
		t.Fatalf("reading manifest: %v", err)
	}
	if string(manifest) != initial {
		t.Errorf("manifest = %q, want it untouched", manifest)
	}
}

func TestParseFeatureIgnoresNonFeatureLines(t *testing.T) {
	for _, line := range []string{
		"",
		"# comment",
		"dlss_g, 310.4.0, .dll, nvngx_dlssg.dll",
		"sl_common_0, 2.14, .dll, sl.common.dll",
	} {
		if feat, ok := parseFeature(line); ok {
			t.Errorf("parseFeature(%q) = %+v, want no match", line, feat)
		}
	}
	feat, ok := parseFeature("sl_common_override_0, 2.14.3, .dll, sl.common.dll")
	if !ok {
		t.Fatal("parseFeature should match a well-formed row")
	}
	if feat.name != "sl_common_override_0" || feat.version != "2.14.3" || feat.ota != 134659 {
		t.Errorf("feat = %+v, want sl_common_override_0 2.14.3 / 134659", feat)
	}
}

func TestSiblingFeatureSwapsFamilies(t *testing.T) {
	cases := map[string]string{
		"sl_common_0":          "sl_common_override_0",
		"sl_common_override_0": "sl_common_0",
		"sl_dlss_g_0":          "sl_dlss_g_override_0",
	}
	for in, want := range cases {
		if got := siblingFeature(in); got != want {
			t.Errorf("siblingFeature(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestNewestPackageConfigPicksHighestPath(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "versions", "1", packageConfigName), "")
	writeFile(t, filepath.Join(root, "versions", "2", packageConfigName), "")

	got, err := newestPackageConfig(root)
	if err != nil {
		t.Fatalf("newestPackageConfig: %v", err)
	}
	if !strings.Contains(got, filepath.Join("versions", "2")) {
		t.Errorf("newestPackageConfig = %s, want the versions/2 copy", got)
	}
}
