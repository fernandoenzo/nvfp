package ngx

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The two Streamline bundles ship the same features under different names:
// sl_sdk_0 (hash E658703) uses sl_<feat>_0, sl_sdk_override_0 (hash E658700)
// uses sl_<feat>_override_0. Nothing about them is hard-coded in the package:
// the tests below also build bundles with other names, hashes and arches.
const (
	plainConfig    = "sl_common_0, 2.14.0, .dll, sl.common.dll\nsl_reflex_0, 2.14.0, .dll, sl.reflex.dll\n"
	overrideConfig = "sl_common_override_0, 2.14.0, .dll, sl.common.dll\nsl_reflex_override_0, 2.14.0, .dll, sl.reflex.dll\n"
)

// bundleSpec is one bundle's package config under the real cache layout:
// <bundle>/versions/<ota>/files/<arch>_<hash>/nvngx_package_config.txt
type bundleSpec struct {
	bundle  string // cache directory name, e.g. sl_sdk_0
	arch    string // e.g. 1B0
	hash    string // e.g. E658703
	ota     string // version directory, e.g. 134656
	content string
}

func plainSpec(content string) bundleSpec {
	return bundleSpec{"sl_sdk_0", "1B0", "E658703", "134656", content}
}

func overrideSpec(content string) bundleSpec {
	return bundleSpec{"sl_sdk_override_0", "1B0", "E658700", "134656", content}
}

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
func cacheFixture(t *testing.T, manifest string, bundles []bundleSpec, payloads []string) string {
	t.Helper()
	root := t.TempDir()
	writeFile(t, filepath.Join(root, manifestName), manifest)
	for _, b := range bundles {
		dir := filepath.Join(root, b.bundle, versionsDir, b.ota, filesDir, b.arch+"_"+b.hash)
		writeFile(t, filepath.Join(dir, packageConfigName), b.content)
	}
	for _, payload := range payloads {
		writeFile(t, filepath.Join(root, payload), "payload bytes")
	}
	return root
}

// payload builds a cache-relative payload path; ota is the version directory.
func payload(feature, arch, hash, ota string) string {
	return filepath.Join(feature, versionsDir, ota, filesDir, arch+"_"+hash+".dll")
}

// inspectFixture inspects a cache and fails the test on error.
func inspectFixture(t *testing.T, root string) *Plan {
	t.Helper()
	plan, err := Inspect(root)
	if err != nil {
		t.Fatalf("Inspect: %v", err)
	}
	return plan
}

func TestInspectFindsMissingSectionsAndCopies(t *testing.T) {
	root := cacheFixture(t,
		"[sl_sdk_0]\r\napp_E658703 = 2.14.0",
		[]bundleSpec{plainSpec(plainConfig), overrideSpec(overrideConfig)},
		// Only the override bundle carries payloads: the plain bundle's files
		// must be filled from its sibling.
		[]string{
			payload("sl_common_override_0", "1B0", "E658700", "134656"),
			payload("sl_reflex_override_0", "1B0", "E658700", "134656"),
		})

	plan := inspectFixture(t, root)
	if got := len(plan.Additions); got != 4 {
		t.Fatalf("additions = %d, want 4", got)
	}
	if got := len(plan.Copies); got != 2 {
		t.Fatalf("copies = %d, want 2", len(plan.Copies))
	}
	for _, copy := range plan.Copies {
		if !strings.Contains(copy.Source, "_override_0") {
			t.Errorf("copy source %s should come from the override bundle", copy.Source)
		}
	}
	if !plan.Changed() {
		t.Error("Changed() = false, want true")
	}
}

// The payload destination is keyed by the row's version in its numeric OTA
// form: 2.14.3 -> 0x020E03 = 134659, not the 2.14.0 directory.
func TestInspectUsesOTAHashPath(t *testing.T) {
	root := cacheFixture(t, "",
		[]bundleSpec{{"sl_sdk_0", "1B0", "E658703", "134659", "sl_common_0, 2.14.3, .dll, sl.common.dll\n"}},
		[]string{payload("sl_common_override_0", "1B0", "E658700", "134659")})

	plan := inspectFixture(t, root)
	want := filepath.Join("sl_common_0", versionsDir, "134659", filesDir, "1B0_E658703.dll")
	if len(plan.Copies) != 1 {
		t.Fatalf("copies = %v, want one", plan.Copies)
	}
	if !strings.Contains(plan.Copies[0].Dest, want) {
		t.Errorf("dest %s should contain %s", plan.Copies[0].Dest, want)
	}
}

func TestInspectReportsFeatureWithoutPayload(t *testing.T) {
	root := cacheFixture(t, "",
		[]bundleSpec{plainSpec("sl_directsr_0, 2.14.0, .dll, sl.directsr.dll\n")}, nil)

	plan := inspectFixture(t, root)
	if len(plan.Missing) != 1 || plan.Missing[0] != "sl_directsr_0" {
		t.Errorf("missing = %v, want [sl_directsr_0]", plan.Missing)
	}
	// No payload means no section can be repaired.
	if len(plan.Additions) != 0 || len(plan.Updates) != 0 {
		t.Errorf("additions = %v, updates = %v, want none", plan.Additions, plan.Updates)
	}
}

// A bundle whose config has no sl_ rows is not a Streamline bundle (the cache
// also ships a DLSS payload bundle): it is ignored, not warned about.
func TestInspectIgnoresNonStreamlineBundle(t *testing.T) {
	dlss := "dlss, 310.9.0, .bin, nvngx_dlss.dll\ndlssg, 310.9.0, .bin, nvngx_dlssg.dll\n"
	root := cacheFixture(t, "",
		[]bundleSpec{
			{"dlss_override", "160", "E658700", "20318464", dlss},
			plainSpec("sl_common_0, 2.14.0, .dll, sl.common.dll\n"),
		},
		[]string{payload("sl_common_0", "1B0", "E658703", "134656")})

	plan := inspectFixture(t, root)
	if len(plan.Warnings) != 0 {
		t.Errorf("warnings = %v, want none: the DLSS bundle is simply not Streamline", plan.Warnings)
	}
	if len(plan.Additions) != 1 || plan.Additions[0].Feature != "sl_common_0" {
		t.Errorf("additions = %v, want just sl_common_0", plan.Additions)
	}
}

// A cache with no Streamline bundle at all is reported, with the actionable
// hint that the NVIDIA App has to run once to populate the cache.
func TestInspectWarnsWithoutStreamlineBundle(t *testing.T) {
	dlss := "dlss, 310.9.0, .bin, nvngx_dlss.dll\n"
	root := cacheFixture(t, "",
		[]bundleSpec{{"dlss_override", "160", "E658700", "20318464", dlss}}, nil)

	plan := inspectFixture(t, root)
	if len(plan.Warnings) != 1 {
		t.Fatalf("warnings = %v, want one", plan.Warnings)
	}
	if !strings.Contains(plan.Warnings[0], "Streamline") || !strings.Contains(plan.Warnings[0], "NVIDIA App") {
		t.Errorf("warning %q should name Streamline and the NVIDIA App", plan.Warnings[0])
	}
	if len(plan.Additions) != 0 || len(plan.Updates) != 0 {
		t.Errorf("additions = %v, updates = %v, want none", plan.Additions, plan.Updates)
	}
}

// Nothing about the bundles is hard-coded: an arbitrary bundle name, hash and
// arch are discovered from disk and used as-is.
func TestInspectDiscoversArbitraryBundle(t *testing.T) {
	root := cacheFixture(t,
		"[anything]\r\napp_D00DFACE = 1.2.3",
		[]bundleSpec{{"sl_whatever_7", "2FF", "D00DFACE", "66051", "sl_newfeat_7, 1.2.3, .dll, sl.newfeat.dll\n"}},
		[]string{payload("sl_newfeat_7", "2FF", "D00DFACE", "66051")})

	plan := inspectFixture(t, root)
	if len(plan.Additions) != 1 {
		t.Fatalf("additions = %v, want one", plan.Additions)
	}
	got := plan.Additions[0]
	if got.Feature != "sl_newfeat_7" || got.Hash != "D00DFACE" || got.Version != "1.2.3" {
		t.Errorf("addition = %+v, want sl_newfeat_7 / D00DFACE / 1.2.3", got)
	}
	if !plan.Changed() {
		t.Error("Changed() = false, want true")
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
		[]bundleSpec{plainSpec("sl_common_0, 2.14.0, .dll, sl.common.dll\n")},
		[]string{payload("sl_common_0", "1B0", "E658703", "134656")})

	plan := inspectFixture(t, root)
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
	if !strings.Contains(string(manifest), "\r\n\r\n[sl_common_0]\r\napp_E658703 = 2.14.0") {
		t.Errorf("manifest = %q, want the appended section, blank-line separated", manifest)
	}
}

func TestApplyIsIdempotent(t *testing.T) {
	root := cacheFixture(t, "[sl_sdk_override_0]\r\napp_E658700 = 2.14.0",
		[]bundleSpec{overrideSpec(overrideConfig)},
		[]string{payload("sl_common_override_0", "1B0", "E658700", "134656")})

	first := inspectFixture(t, root)
	if err := Apply(first); err != nil {
		t.Fatalf("first Apply: %v", err)
	}
	afterFirst, err := os.ReadFile(first.Manifest)
	if err != nil {
		t.Fatalf("reading manifest: %v", err)
	}

	second := inspectFixture(t, root)
	if second.Changed() {
		t.Errorf("second plan still has work: %d copies, %d additions", len(second.Copies), len(second.Additions))
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
		[]bundleSpec{plainSpec("sl_common_0, 2.14.0, .dll, sl.common.dll\n"), overrideSpec(overrideConfig)},
		[]string{payload("sl_common_override_0", "1B0", "E658700", "134656")})

	plan := inspectFixture(t, root)
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

// A new GPU arch is discovered from disk, not hard-coded: the payload is filled
// under that arch's name.
func TestApplyCopiesPayloadUnderNewArch(t *testing.T) {
	root := cacheFixture(t, "",
		[]bundleSpec{
			{"sl_sdk_0", "1C0", "E658703", "134656", "sl_common_0, 2.14.0, .dll, sl.common.dll\n"},
			{"sl_sdk_override_0", "1C0", "E658700", "134656", overrideConfig},
		},
		[]string{payload("sl_common_override_0", "1C0", "E658700", "134656")})

	plan := inspectFixture(t, root)
	if len(plan.Copies) != 1 {
		t.Fatalf("copies = %v, want one", plan.Copies)
	}
	if !strings.HasSuffix(plan.Copies[0].Dest, "1C0_E658703.dll") {
		t.Errorf("dest = %s, want it to end in 1C0_E658703.dll (the discovered arch)", plan.Copies[0].Dest)
	}
	if err := Apply(plan); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if _, err := os.Stat(plan.Copies[0].Dest); err != nil {
		t.Errorf("payload not written under the discovered arch: %v", err)
	}
}

func TestApplyDoesNotOverwriteExistingBackup(t *testing.T) {
	root := cacheFixture(t, "[sl_sdk_0]",
		[]bundleSpec{plainSpec("sl_common_0, 2.14.0, .dll, sl.common.dll\n")},
		[]string{payload("sl_common_0", "1B0", "E658703", "134656")})
	writeFile(t, filepath.Join(root, manifestName+".bak"), "older backup")

	plan := inspectFixture(t, root)
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

// The sibling bundle carries payloads for more than one arch: only the one
// matching the destination's arch may fill the gap — another arch's bytes
// under an 1B0 name would silently corrupt the cache.
func TestApplySiblingCopyKeepsTheArch(t *testing.T) {
	root := cacheFixture(t, "",
		[]bundleSpec{plainSpec("sl_common_0, 2.14.0, .dll, sl.common.dll\n")},
		[]string{
			payload("sl_common_override_0", "160", "E658700", "134656"),
			payload("sl_common_override_0", "1B0", "E658700", "134656"),
		})

	plan := inspectFixture(t, root)
	if len(plan.Copies) != 1 {
		t.Fatalf("copies = %+v, want one", plan.Copies)
	}
	if !strings.Contains(plan.Copies[0].Source, "1B0_") {
		t.Errorf("copy source %s should carry the destination's 1B0 arch", plan.Copies[0].Source)
	}

	// The sibling has no payload for the destination's arch at all: the feature
	// must be reported, never filled with the wrong arch.
	other := cacheFixture(t, "",
		[]bundleSpec{{"sl_sdk_0", "1B0", "E658703", "134656", "sl_common_0, 2.14.0, .dll, sl.common.dll\n"}},
		[]string{payload("sl_common_override_0", "160", "E658700", "134656")})
	otherPlan := inspectFixture(t, other)
	if len(otherPlan.Copies) != 0 {
		t.Errorf("copies = %+v, want none: the sibling has no 1B0 payload", otherPlan.Copies)
	}
	if len(otherPlan.Missing) != 1 || otherPlan.Missing[0] != "sl_common_0" {
		t.Errorf("missing = %v, want [sl_common_0]", otherPlan.Missing)
	}
}

// Applying the same plan twice must not duplicate a section: the plan is
// re-checked against its document before every append.
func TestApplyTwiceOnTheSamePlanIsIdempotent(t *testing.T) {
	root := cacheFixture(t, "[sl_sdk_0]",
		[]bundleSpec{plainSpec("sl_common_0, 2.14.0, .dll, sl.common.dll\n")},
		[]string{payload("sl_common_0", "1B0", "E658703", "134656")})

	plan := inspectFixture(t, root)
	if err := Apply(plan); err != nil {
		t.Fatalf("first Apply: %v", err)
	}
	if err := Apply(plan); err != nil {
		t.Fatalf("second Apply: %v", err)
	}
	data, err := os.ReadFile(plan.Manifest)
	if err != nil {
		t.Fatalf("reading manifest: %v", err)
	}
	if got := strings.Count(string(data), "[sl_common_0]"); got != 1 {
		t.Errorf("the section appears %d times, want 1:\n%s", got, data)
	}
}

func TestParseFeatureIgnoresNonFeatureLines(t *testing.T) {
	for _, line := range []string{
		"",
		"# comment",
		"dlss_g, 310.4.0, .dll, nvngx_dlssg.dll",
		"sl_common_0, 2.14, .dll, sl.common.dll",
		// A version component must fit the OTA's 8 bits; 2.14.300 would
		// otherwise collide with 2.15.44.
		"sl_common_0, 2.14.300, .dll, sl.common.dll",
		"sl_common_0, 256.0.0, .dll, sl.common.dll",
	} {
		if feat, ok := parseFeature(line); ok {
			t.Errorf("parseFeature(%q) = %+v, want no match", line, feat)
		}
	}
	feat, ok := parseFeature("sl_common_override_0, 2.14.3, .dll, sl.common.dll")
	if !ok {
		t.Fatal("parseFeature should match a well-formed row")
	}
	if feat.name != "sl_common_override_0" || feat.version != "2.14.3" || feat.ext != ".dll" || feat.ota != 134659 {
		t.Errorf("feat = %+v, want sl_common_override_0 2.14.3 / .dll / 134659", feat)
	}
	// The OTA directory is numeric, so two spellings of a version must never
	// reach the manifest as two versions: leading zeros are canonicalised.
	padded, ok := parseFeature("sl_common_0, 002.14.00, .dll, sl.common.dll")
	if !ok || padded.version != "2.14.0" || padded.ota != 134656 {
		t.Errorf("padded = %+v, %v; want 2.14.0 / 134656", padded, ok)
	}
}

func TestConfigFromAndSplitArchHash(t *testing.T) {
	path := filepath.FromSlash("/cache/sl_sdk_0/versions/134656/files/1B0_E658703/nvngx_package_config.txt")
	cfg, ok := configFrom(path)
	if !ok {
		t.Fatalf("configFrom(%q) not ok", path)
	}
	if cfg.bundle != filepath.FromSlash("/cache/sl_sdk_0") || cfg.ota != 134656 || cfg.arch != "1B0" || cfg.hash != "E658703" {
		t.Errorf("config = %+v, want /cache/sl_sdk_0 / 134656 / 1B0 / E658703", cfg)
	}
	if _, ok := configFrom("nvngx_package_config.txt"); ok {
		t.Error("configFrom of a bare name should not be ok")
	}
	// The `files` component is part of the layout: a path without it is not a
	// bundle config, whatever else looks right.
	if _, ok := configFrom(filepath.FromSlash("/cache/sl_sdk_0/versions/134656/not-files/1B0_E658703/nvngx_package_config.txt")); ok {
		t.Error("configFrom accepted a path without the files component")
	}
	// A non-numeric OTA does not parse, but the config is still recognised and
	// sorts lowest.
	odd, ok := configFrom(filepath.FromSlash("/cache/sl_sdk_0/versions/nightly/files/1B0_E658703/nvngx_package_config.txt"))
	if !ok || odd.ota != -1 {
		t.Errorf("configFrom with a non-numeric ota = %+v, %v; want ok with ota -1", odd, ok)
	}
	for _, bad := range []string{"nodash", "_E658703", "1B0_", ""} {
		if arch, hash := splitArchHash(bad); arch != "" || hash != "" {
			t.Errorf("splitArchHash(%q) = %q,%q want empty", bad, arch, hash)
		}
	}
}

func TestSiblingFeatureSwapsBundles(t *testing.T) {
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

// A bundle keeps one config per update; the highest numeric OTA wins, even
// when a narrower directory name would sort higher as a string ("9" > "10").
func TestDiscoverPicksNewestConfig(t *testing.T) {
	root := cacheFixture(t, "",
		[]bundleSpec{
			{"sl_sdk_0", "1B0", "E658703", "9", "sl_common_0, 2.14.0, .dll, sl.common.dll\n"},
			{"sl_sdk_0", "1B0", "E658703", "10", "sl_common_0, 2.9.0, .dll, sl.common.dll\n"},
		}, nil)

	features, warnings := discover(root)
	if len(warnings) != 0 {
		t.Fatalf("warnings = %v, want none", warnings)
	}
	if len(features) != 1 || features[0].version != "2.9.0" {
		t.Errorf("features = %+v, want the versions/10 config", features)
	}
}

// Two payload directories under the same OTA version are a tie the walk order
// must not decide: the lexicographically first path wins, run after run.
func TestDiscoverBreaksOTATiesDeterministically(t *testing.T) {
	specs := []bundleSpec{
		{"sl_sdk_0", "160", "E658703", "134656", "sl_common_0, 2.14.0, .dll, sl.common.dll\n"},
		{"sl_sdk_0", "1B0", "E658703", "134656", "sl_common_0, 2.14.0, .dll, sl.common.dll\n"},
	}
	for range 5 {
		features, warnings := discover(cacheFixture(t, "", specs, nil))
		if len(warnings) != 0 {
			t.Fatalf("warnings = %v, want none", warnings)
		}
		if len(features) != 1 || features[0].arch != "160" {
			t.Fatalf("features = %+v, want the 160 config (first path wins)", features)
		}
	}
}

// A malformed config directory never shadows its bundle's valid configs: the
// newest usable one wins, and the malformed one is warned about.
func TestDiscoverSkipsMalformedConfigDir(t *testing.T) {
	root := cacheFixture(t, "",
		[]bundleSpec{
			{"sl_sdk_0", "1B0", "E658703", "134656", "sl_common_0, 2.14.0, .dll, sl.common.dll\n"},
			{"sl_sdk_0", "nodash", "", "134657", "sl_common_0, 2.14.1, .dll, sl.common.dll\n"},
		}, nil)

	features, warnings := discover(root)
	if len(features) != 1 || features[0].version != "2.14.0" {
		t.Errorf("features = %+v, want the valid versions/134656 config", features)
	}
	if len(warnings) != 1 || !strings.Contains(warnings[0], "no <arch>_<hash>") {
		t.Errorf("warnings = %v, want one about the malformed directory name", warnings)
	}
}

// A bundle whose config has no sl_ rows is not Streamline, so an odd directory
// name there is not the user's business — the DLSS bundles ship their own
// layouts and must not be warned about.
func TestDiscoverMalformedNonStreamlineDirIsNotWarned(t *testing.T) {
	root := cacheFixture(t, "",
		[]bundleSpec{
			{"dlss_override", "nodash", "", "20318464", "dlss, 310.9.0, .bin, nvngx_dlss.dll\n"},
			plainSpec("sl_common_0, 2.14.0, .dll, sl.common.dll\n"),
		}, nil)

	features, warnings := discover(root)
	if len(warnings) != 0 {
		t.Errorf("warnings = %v, want none: the DLSS bundle is not Streamline", warnings)
	}
	if len(features) != 1 || features[0].name != "sl_common_0" {
		t.Errorf("features = %+v, want just sl_common_0", features)
	}
}

// --- Stale versions (the manifest pinning a version the cache no longer has) ---

func TestInspectDetectsStaleVersion(t *testing.T) {
	root := cacheFixture(t,
		"[sl_sdk_0]\r\napp_E658703 = 2.14.0\r\n\r\n[sl_common_0]\r\napp_E658703 = 2.14.0",
		[]bundleSpec{{"sl_sdk_0", "1B0", "E658703", "134659", "sl_common_0, 2.14.3, .dll, sl.common.dll\n"}},
		[]string{payload("sl_common_0", "1B0", "E658703", "134659")})

	plan := inspectFixture(t, root)
	if len(plan.Updates) != 1 {
		t.Fatalf("updates = %+v, want one stale section", plan.Updates)
	}
	if plan.Updates[0].Feature != "sl_common_0" || plan.Updates[0].Current != "2.14.0" || plan.Updates[0].Version != "2.14.3" {
		t.Errorf("update = %+v, want sl_common_0 2.14.0 -> 2.14.3", plan.Updates[0])
	}
	if len(plan.Additions) != 0 {
		t.Errorf("additions = %d, want 0: the section exists", len(plan.Additions))
	}
	if !plan.Changed() {
		t.Error("Changed() = false, want true: a stale version must be corrected")
	}
}

func TestApplyCorrectsStaleVersionInPlace(t *testing.T) {
	initial := "[sl_sdk_0]\r\napp_E658703 = 2.14.0\r\n\r\n[sl_common_0]\r\napp_E658703 = 2.14.0"
	root := cacheFixture(t, initial,
		[]bundleSpec{{"sl_sdk_0", "1B0", "E658703", "134659", "sl_common_0, 2.14.3, .dll, sl.common.dll\n"}},
		[]string{payload("sl_common_0", "1B0", "E658703", "134659")})

	plan := inspectFixture(t, root)
	if err := Apply(plan); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	manifest, err := os.ReadFile(plan.Manifest)
	if err != nil {
		t.Fatalf("reading manifest: %v", err)
	}
	// The version is updated in place and every other byte is untouched.
	want := "[sl_sdk_0]\r\napp_E658703 = 2.14.0\r\n\r\n[sl_common_0]\r\napp_E658703 = 2.14.3\r\n"
	if string(manifest) != want {
		t.Errorf("manifest = %q, want %q", manifest, want)
	}
	// A second run has nothing to do.
	second := inspectFixture(t, root)
	if second.Changed() {
		t.Errorf("second run still has work: updates=%+v", second.Updates)
	}
}

func TestApplyCorrectsStaleVersionKeepingLineStyle(t *testing.T) {
	// No spaces around `=`, and an opaque comment above the section.
	initial := "; NVIDIA cache\r\n[sl_common_0]\r\napp_E658703=2.14.0\r\n"
	root := cacheFixture(t, initial,
		[]bundleSpec{{"sl_sdk_0", "1B0", "E658703", "134659", "sl_common_0, 2.14.3, .dll, sl.common.dll\n"}},
		[]string{payload("sl_common_0", "1B0", "E658703", "134659")})

	plan := inspectFixture(t, root)
	if err := Apply(plan); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	manifest, err := os.ReadFile(plan.Manifest)
	if err != nil {
		t.Fatalf("reading manifest: %v", err)
	}
	want := "; NVIDIA cache\r\n[sl_common_0]\r\napp_E658703=2.14.3\r\n"
	if string(manifest) != want {
		t.Errorf("manifest = %q, want %q (comment kept, spacing kept)", manifest, want)
	}
}

// The whole cache is already correct: Inspect finds nothing to do and Apply
// writes nothing — no backup, the manifest comes out byte-identical.
func TestCurrentVersionIsNotTouched(t *testing.T) {
	initial := "[sl_sdk_0]\r\n\r\n[sl_common_0]\r\napp_E658703 = 2.14.0"
	root := cacheFixture(t, initial,
		[]bundleSpec{plainSpec("sl_common_0, 2.14.0, .dll, sl.common.dll\n")},
		[]string{payload("sl_common_0", "1B0", "E658703", "134656")})

	plan := inspectFixture(t, root)
	if len(plan.Additions) != 0 || len(plan.Updates) != 0 || len(plan.Copies) != 0 || len(plan.Missing) != 0 {
		t.Errorf("plan = %+v, want nothing to do", plan)
	}
	if plan.Changed() {
		t.Error("Changed() = true, want false")
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
