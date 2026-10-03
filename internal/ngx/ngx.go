// Package ngx maintains the NGX OTA manifest the Streamline interposer reads.
//
// The NVIDIA App's bootstrap can rewrite nvngx_config.txt and leave only the
// bundle sections, dropping the per-feature [sl_<feat>_0] and
// [sl_<feat>_override_0] ones. The interposer resolves plugins per feature, so
// both of its passes then fail and the game falls back to its bundled plugins.
// Inspect and Apply rebuild those sections from the two bundles' own package
// configs, filling a payload that exists under only one hash from its sibling
// bundle, and are idempotent: a repaired manifest is never touched again.
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

// family is one of the two Streamline bundles in the NGX cache: sl_sdk_0
// (CMSID 0, hash E658703) and sl_sdk_override_0 (CMSID 3, hash E658700). Both
// ship the same features, so a payload missing under one hash can be filled
// with the sibling's identical file.
type family struct {
	bundle string // cache directory name
	hash   string // app hash used in the manifest, e.g. app_E658703
	other  string // hash of the sibling family
}

var families = []family{
	{bundle: "sl_sdk_0", hash: "E658703", other: "E658700"},
	{bundle: "sl_sdk_override_0", hash: "E658700", other: "E658703"},
}

// Plan is what the NGX cache on disk is missing.
type Plan struct {
	Root     string    // NGX OTA cache directory
	Manifest string    // nvngx_config.txt inside Root
	Backup   string    // the first write copies the manifest here
	Sections []Section // every feature found, in bundle order
	Copies   []Copy    // payloads missing under a hash, filled from the sibling
	Missing  []string  // features with no payload in either family
	Warnings []string  // bundles that could not be examined
}

// Section is one per-feature manifest entry.
type Section struct {
	Feature string // e.g. sl_common_override_0
	Hash    string // app hash of its family
	Version string // e.g. 2.14.0
	Present bool   // the manifest already carries [Feature]
}

// Copy is one payload file the cache is missing.
type Copy struct {
	Feature string // feature whose payload is missing
	Source  string
	Dest    string
}

// Pending returns the sections the manifest is missing.
func (p *Plan) Pending() []Section {
	var pending []Section
	for _, section := range p.Sections {
		if !section.Present {
			pending = append(pending, section)
		}
	}
	return pending
}

// Changed reports whether applying the plan would touch any file.
func (p *Plan) Changed() bool {
	return len(p.Copies) > 0 || len(p.Pending()) > 0
}

// Inspect reads the NGX cache under root and reports every section and payload
// it is missing. Nothing is written.
func Inspect(root string) (*Plan, error) {
	manifest := filepath.Join(root, manifestName)
	data, err := os.ReadFile(manifest)
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", manifest, err)
	}
	plan := &Plan{
		Root:     root,
		Manifest: manifest,
		Backup:   manifest + ".bak",
	}
	text := string(data)
	queued := map[string]bool{}
	for _, fam := range families {
		features, err := familyFeatures(filepath.Join(root, fam.bundle))
		if err != nil {
			plan.Warnings = append(plan.Warnings, err.Error())
			continue
		}
		for _, feat := range features {
			plan.add(fam, feat, text, queued)
		}
	}
	return plan, nil
}

// add records one feature: a payload copy when its file is missing, and the
// manifest section it needs, unless the manifest (or this plan) already has it.
func (p *Plan) add(fam family, feat feature, manifest string, queued map[string]bool) {
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
	p.Sections = append(p.Sections, Section{
		Feature: feat.name,
		Hash:    fam.hash,
		Version: feat.version,
		Present: hasSection(manifest, feat.name),
	})
}

// Apply copies the missing payloads, backs the manifest up once and appends
// every pending section. It is idempotent: a plan with nothing to do writes
// nothing.
func Apply(plan *Plan) error {
	for _, copy := range plan.Copies {
		if err := copyPayload(copy); err != nil {
			return err
		}
	}
	pending := plan.Pending()
	if len(pending) == 0 {
		return nil
	}
	if err := backupOnce(plan.Manifest, plan.Backup); err != nil {
		return err
	}
	return appendSections(plan.Manifest, pending)
}

// copyPayload writes the sibling family's identical payload into the missing
// family's file, creating its directory.
func copyPayload(copy Copy) error {
	dir := filepath.Dir(copy.Dest)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("creating %s: %w", dir, err)
	}
	if err := copyFile(copy.Source, copy.Dest); err != nil {
		return fmt.Errorf("copying %s: %w", copy.Dest, err)
	}
	return nil
}

// backupOnce writes the .bak the first time the manifest is modified and never
// overwrites it, so it keeps the oldest copy, as the original script did.
func backupOnce(manifest, backup string) error {
	if exists(backup) {
		return nil
	}
	if err := copyFile(manifest, backup); err != nil {
		return fmt.Errorf("backing up %s: %w", manifest, err)
	}
	return nil
}

// appendSections appends the sections the way the original script wrote them:
// CRLF line endings, no trailing newline.
func appendSections(manifest string, sections []Section) error {
	var b strings.Builder
	for _, section := range sections {
		fmt.Fprintf(&b, "\r\n[%s]\r\napp_%s = %s", section.Feature, section.Hash, section.Version)
	}
	file, err := os.OpenFile(manifest, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("opening %s: %w", manifest, err)
	}
	defer file.Close()
	if _, err := file.WriteString(b.String()); err != nil {
		return fmt.Errorf("appending to %s: %w", manifest, err)
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
var featureLine = regexp.MustCompile(`^\s*(sl_[a-z0-9_]+)\s*,\s*(\d+)\.(\d+)\.(\d+)\s*,`)

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
	dir := filepath.Join(root, siblingFeature(feat.name), "versions", strconv.Itoa(feat.ota), "files")
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", false
	}
	suffix := "_" + fam.other + ".dll"
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), suffix) {
			return filepath.Join(dir, entry.Name()), true
		}
	}
	return "", false
}

// siblingFeature swaps a feature between the two bundles, e.g.
// sl_common_0 <-> sl_common_override_0.
func siblingFeature(name string) string {
	if strings.HasSuffix(name, "_override_0") {
		return strings.TrimSuffix(name, "_override_0") + "_0"
	}
	return strings.TrimSuffix(name, "_0") + "_override_0"
}

// hasSection reports whether the manifest already carries the [feature] line.
func hasSection(manifest, feature string) bool {
	header := "[" + feature + "]"
	for line := range strings.SplitSeq(manifest, "\n") {
		if strings.HasPrefix(line, header) {
			return true
		}
	}
	return false
}

// exists reports whether path is an existing file.
func exists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}
