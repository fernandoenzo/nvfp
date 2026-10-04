// Package ngx maintains the NGX OTA manifest the Streamline interposer reads.
//
// The NVIDIA App's bootstrap can rewrite nvngx_config.txt and leave only the
// bundle sections, dropping the per-feature [sl_<feat>_0] and
// [sl_<feat>_override_0] ones. The interposer resolves plugins per feature, so
// both of its passes then fail and the game falls back to its bundled plugins.
// Inspect and Apply rebuild those sections from the two bundles' own package
// configs, correct a section that still points at a version the cache no longer
// has, fill a payload that exists under only one hash from its sibling bundle,
// and are idempotent: a repaired manifest is never touched again.
//
// The manifest is parsed, mutated in memory and written back whole, so the
// format itself (line endings, section spacing) lives in one place:
// manifest.go. Everything the parser does not recognise survives the rewrite
// byte for byte.
package ngx

import (
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
)

const (
	// manifestName is the Streamline OTA manifest the interposer reads.
	manifestName = "nvngx_config.txt"
	// packageConfigName is the authoritative feature list inside each bundle.
	packageConfigName = "nvngx_package_config.txt"
	// arch is the GPU architecture marker in payload file names: 0x1B0 (RTX 50
	// series) is the only one the cache ships.
	arch = "1B0"
)

// family is one Streamline bundle; the two ship the same features, so a payload
// missing under one hash can be filled with the sibling's identical file.
type family struct {
	bundle string // cache directory name
	hash   string // app hash used in the manifest, e.g. app_E658703
	other  string // hash of the sibling family
}

// sl_sdk_0 (CMSID 0, hash E658703) and sl_sdk_override_0 (CMSID 3, hash E658700).
var families = []family{
	{bundle: "sl_sdk_0", hash: "E658703", other: "E658700"},
	{bundle: "sl_sdk_override_0", hash: "E658700", other: "E658703"},
}

// Plan is what applying the repair to the NGX cache would do.
type Plan struct {
	Root     string    // NGX OTA cache directory
	Manifest string    // nvngx_config.txt inside Root
	Backup   string    // the first write copies the manifest here
	Sections []Section // every feature found, in bundle order
	Copies   []Copy    // payloads missing under a hash, filled from the sibling
	Missing  []string  // features with no payload in either family
	Warnings []string  // bundles that could not be examined

	doc *manifest // parsed manifest, mutated by Apply
}

// Section is one per-feature manifest entry and what the manifest says about it.
type Section struct {
	Feature string // e.g. sl_common_override_0
	Hash    string // app hash of its family
	Version string // version the bundle ships now
	Current string // version the manifest declares, empty when absent
	Present bool   // the manifest already carries [Feature]
}

// Copy is one payload file the cache is missing.
type Copy struct {
	Feature string // feature whose payload is missing
	Source  string
	Dest    string
}

// Stale reports whether the manifest pins a version other than the one the
// bundle ships, which is what makes the interposer miss the payload.
func (s Section) Stale() bool {
	return s.Present && s.Current != s.Version
}

// Additions returns the sections the manifest does not have yet.
func (p *Plan) Additions() []Section {
	var out []Section
	for _, section := range p.Sections {
		if !section.Present {
			out = append(out, section)
		}
	}
	return out
}

// Updates returns the sections whose declared version is no longer the one the
// bundle ships. The interposer resolves a feature to versions\<ota>, so a stale
// version points at a directory the cache may have deleted.
func (p *Plan) Updates() []Section {
	var out []Section
	for _, section := range p.Sections {
		if section.Stale() {
			out = append(out, section)
		}
	}
	return out
}

// Changed reports whether applying the plan would touch any file.
func (p *Plan) Changed() bool {
	return len(p.Copies) > 0 || len(p.Additions()) > 0 || len(p.Updates()) > 0
}

// Inspect reads the NGX cache under root and reports every section and payload
// it is missing or pointing at an outdated version. Nothing is written.
func Inspect(root string) (*Plan, error) {
	path := filepath.Join(root, manifestName)
	doc, err := parseFile(path)
	if err != nil {
		return nil, err
	}
	plan := &Plan{
		Root:     root,
		Manifest: path,
		Backup:   path + ".bak",
		doc:      doc,
	}
	queued := map[string]bool{}
	for _, fam := range families {
		features, err := familyFeatures(filepath.Join(root, fam.bundle))
		if err != nil {
			plan.Warnings = append(plan.Warnings, err.Error())
			continue
		}
		for _, feat := range features {
			plan.add(fam, feat, queued)
		}
	}
	return plan, nil
}

// add records one feature: a payload copy when its file is missing, and the
// manifest entry it needs unless the plan already handled that feature.
func (p *Plan) add(fam family, feat feature, queued map[string]bool) {
	if queued[feat.name] {
		return
	}
	queued[feat.name] = true
	dest := payloadPath(p.Root, feat, fam.hash)
	if !exists(dest) {
		source, ok := siblingPayload(p.Root, feat, fam)
		if !ok {
			p.Missing = append(p.Missing, feat.name)
			return
		}
		p.Copies = append(p.Copies, Copy{Feature: feat.name, Source: source, Dest: dest})
	}
	section := Section{
		Feature: feat.name,
		Hash:    fam.hash,
		Version: feat.version,
	}
	if b := p.doc.section(feat.name); b != nil {
		section.Present = true
		section.Current, _ = b.get("app_" + fam.hash)
	}
	p.Sections = append(p.Sections, section)
}

// Apply copies the missing payloads, backs the manifest up once and writes the
// corrected sections: new ones appended, outdated versions rewritten in place.
// It is idempotent: a plan with nothing to do writes nothing.
func Apply(plan *Plan) error {
	for _, copy := range plan.Copies {
		if err := os.MkdirAll(filepath.Dir(copy.Dest), 0o755); err != nil {
			return fmt.Errorf("creating %s: %w", filepath.Dir(copy.Dest), err)
		}
		if err := copyFile(copy.Source, copy.Dest); err != nil {
			return fmt.Errorf("copying %s: %w", copy.Dest, err)
		}
	}
	if len(plan.Additions()) == 0 && len(plan.Updates()) == 0 {
		return nil
	}
	// The .bak is written the first time the manifest is modified and never
	// overwritten, so it keeps the oldest copy, as the original script did.
	if !exists(plan.Backup) {
		if err := copyFile(plan.Manifest, plan.Backup); err != nil {
			return fmt.Errorf("backing up %s: %w", plan.Manifest, err)
		}
	}
	return writeManifest(plan)
}

// writeManifest applies the plan's section changes to the parsed manifest and
// writes it back whole. Only the sections the plan owns are touched: the rest
// of the file, recognised or not, keeps its bytes.
func writeManifest(plan *Plan) error {
	for _, section := range plan.Updates() {
		if b := plan.doc.section(section.Feature); b != nil {
			b.set("app_"+section.Hash, section.Version)
		}
	}
	for _, section := range plan.Additions() {
		b := plan.doc.appendSection(section.Feature)
		b.set("app_"+section.Hash, section.Version)
	}
	if err := os.WriteFile(plan.Manifest, plan.doc.bytes(), 0o644); err != nil {
		return fmt.Errorf("writing %s: %w", plan.Manifest, err)
	}
	return nil
}

// copyFile streams src into dst, reporting a write error.
func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("opening %s: %w", src, err)
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return fmt.Errorf("creating %s: %w", dst, err)
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return fmt.Errorf("copying %s to %s: %w", src, dst, err)
	}
	return out.Close()
}

// feature is one feature row of a bundle's package config.
type feature struct {
	name    string // e.g. sl_common_override_0
	version string // e.g. 2.14.0
	ota     int    // version directory: 2.14.0 -> 134656
}

// featureLine matches a bundle's feature rows:
//
//	sl_common_override_0, 2.14.0, .dll, sl.common.dll
var featureLine = regexp.MustCompile(`^\x{FEFF}?\s*(sl_[a-z0-9_]+)\s*,\s*(\d+)\.(\d+)\.(\d+)\s*,`)

// parseFeature reads one feature row; anything else yields false.
func parseFeature(line string) (feature, bool) {
	m := featureLine.FindStringSubmatch(line)
	if m == nil {
		return feature{}, false
	}
	major, err := strconv.Atoi(m[2])
	if err != nil {
		return feature{}, false
	}
	minor, err := strconv.Atoi(m[3])
	if err != nil {
		return feature{}, false
	}
	patch, err := strconv.Atoi(m[4])
	if err != nil {
		return feature{}, false
	}
	return feature{
		name:    m[1],
		version: m[2] + "." + m[3] + "." + m[4],
		ota:     major<<16 | minor<<8 | patch,
	}, true
}

// familyFeatures reads a bundle's newest package config and returns every
// feature row, in file order.
func familyFeatures(bundleDir string) ([]feature, error) {
	path, err := newestPackageConfig(bundleDir)
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", path, err)
	}
	var features []feature
	for line := range strings.SplitSeq(string(data), "\n") {
		if feat, ok := parseFeature(line); ok {
			features = append(features, feat)
		}
	}
	if len(features) == 0 {
		return nil, fmt.Errorf("%s: no feature rows found", path)
	}
	return features, nil
}

// newestPackageConfig returns a bundle's newest package config. A bundle keeps
// one per update, and the highest path sorts last: the same pick the original
// script made with Sort-Object -Descending.
func newestPackageConfig(bundleDir string) (string, error) {
	var found []string
	_ = filepath.WalkDir(bundleDir, func(path string, entry fs.DirEntry, err error) error {
		if err == nil && !entry.IsDir() && entry.Name() == packageConfigName {
			found = append(found, path)
		}
		return nil
	})
	if len(found) == 0 {
		return "", fmt.Errorf("%s: no %s found (open the NVIDIA App once)", bundleDir, packageConfigName)
	}
	slices.Sort(found)
	return found[len(found)-1], nil
}

// payloadPath is where the interposer looks for a feature's payload under one
// family's hash.
func payloadPath(root string, feat feature, hash string) string {
	file := arch + "_" + hash + ".dll"
	return filepath.Join(root, feat.name, "versions", strconv.Itoa(feat.ota), "files", file)
}

// siblingPayload finds the same payload under the sibling family's hash: both
// bundles ship identical bytes, so one fills the other's missing file.
func siblingPayload(root string, feat feature, fam family) (string, bool) {
	pattern := filepath.Join(root, siblingFeature(feat.name), "versions", strconv.Itoa(feat.ota), "files", "*_"+fam.other+".dll")
	matches, _ := filepath.Glob(pattern)
	if len(matches) == 0 {
		return "", false
	}
	return matches[0], true
}

// siblingFeature swaps a feature between the two bundles, e.g.
// sl_common_0 <-> sl_common_override_0.
func siblingFeature(name string) string {
	if strings.HasSuffix(name, "_override_0") {
		return strings.TrimSuffix(name, "_override_0") + "_0"
	}
	return strings.TrimSuffix(name, "_0") + "_override_0"
}

// exists reports whether path is an existing file.
func exists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}
