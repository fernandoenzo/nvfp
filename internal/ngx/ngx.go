// Package ngx maintains the NGX OTA manifest the Streamline interposer reads.
//
// The NVIDIA App's bootstrap can rewrite nvngx_config.txt and leave only the
// bundle sections, dropping the per-feature [sl_<feat>_0] and
// [sl_<feat>_override_0] ones. The interposer resolves plugins per feature, so
// both of its passes then fail and the game falls back to its bundled plugins.
// Inspect and Apply rebuild those sections from the bundles' own package
// configs, correct a section that still points at a version the cache no longer
// has, fill a payload that exists under only one hash from its sibling bundle,
// and are idempotent: a repaired manifest is never touched again.
//
// Nothing about the bundles is hard-coded: the cache is discovered from disk.
// A bundle is any <dir> under the root holding a
// versions/<ota>/files/<arch>_<hash>/nvngx_package_config.txt whose rows name
// Streamline (sl_) features; the arch and the app hash come from that directory
// name, and a feature's payload lives at
// <root>/<feature>/versions/<ota>/files/<arch>_<hash><ext>. A bundle with no
// sl_ rows is not a Streamline bundle and is left alone.
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
	"maps"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/fernandoenzo/set"
)

const (
	// manifestName is the Streamline OTA manifest the interposer reads.
	manifestName = "nvngx_config.txt"
	// packageConfigName is the authoritative feature list inside each bundle.
	packageConfigName = "nvngx_package_config.txt"
	// versionsDir and filesDir are the two fixed path components between a
	// cache directory and a payload; the rest (arch, hash) is discovered.
	versionsDir = "versions"
	filesDir    = "files"
)

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
	Hash    string // app hash of its bundle, e.g. E658700
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
	features, warnings := discover(root)
	plan.Warnings = warnings
	if len(features) == 0 {
		plan.Warnings = append(plan.Warnings, fmt.Sprintf("%s: no Streamline (sl_) bundle found", root))
	}
	for _, feat := range features {
		plan.add(root, feat)
	}
	return plan, nil
}

// add records one feature: a payload copy when its file is missing, and the
// manifest entry it needs.
func (p *Plan) add(root string, feat feature) {
	dest := payloadPath(root, feat)
	if !exists(dest) {
		source, ok := siblingPayload(root, feat)
		if !ok {
			p.Missing = append(p.Missing, feat.name)
			return
		}
		p.Copies = append(p.Copies, Copy{Feature: feat.name, Source: source, Dest: dest})
	}
	section := Section{
		Feature: feat.name,
		Hash:    feat.hash,
		Version: feat.version,
	}
	if b := p.doc.section(feat.name); b != nil {
		section.Present = true
		section.Current, _ = b.get("app_" + feat.hash)
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

// discover finds every Streamline bundle in the cache and returns its features,
// one per feature name, in bundle-then-config order, plus a warning per bundle
// that could not be read.
func discover(root string) ([]feature, []string) {
	// newest maps each bundle dir to the package config under its highest OTA.
	newest := map[string]string{}
	_ = filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() || entry.Name() != packageConfigName {
			return nil
		}
		bundle := bundleDir(path)
		if bundle == "" {
			return nil
		}
		// The newest config is the one under the highest numeric OTA directory.
		if current, ok := newest[bundle]; !ok || ota(path) > ota(current) {
			newest[bundle] = path
		}
		return nil
	})
	var (
		features []feature
		warnings []string
		seen     = set.New[string](len(newest))
	)
	for _, bundle := range slices.Sorted(maps.Keys(newest)) {
		path := newest[bundle]
		arch, hash := dirArchHash(filepath.Dir(path))
		if arch == "" || hash == "" {
			warnings = append(warnings, fmt.Sprintf("%s: no <arch>_<hash> in its directory name", path))
			continue
		}
		feats, err := parseConfig(path, arch, hash)
		if err != nil {
			warnings = append(warnings, err.Error())
			continue
		}
		for _, feat := range feats {
			if seen.Contains(feat.name) {
				continue
			}
			seen.Add(feat.name)
			features = append(features, feat)
		}
	}
	return features, warnings
}

// feature is one feature row of a bundle's package config.
type feature struct {
	name    string // e.g. sl_common_override_0
	version string // e.g. 2.14.0
	ext     string // e.g. .dll
	ota     int    // version directory: 2.14.0 -> 134656
	arch    string // GPU arch from the config directory, e.g. 1B0
	hash    string // app hash from the config directory, e.g. E658703
}

// featureLine matches a Streamline feature row and its four fields:
//
//	sl_common_override_0, 2.14.0, .dll, sl.common.dll
var featureLine = regexp.MustCompile(`^\x{FEFF}?\s*(sl_[a-z0-9_]+)\s*,\s*(\d+)\.(\d+)\.(\d+)\s*,\s*(\.[A-Za-z0-9]+)`)

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
		ext:     m[5],
		ota:     major<<16 | minor<<8 | patch,
	}, true
}

// parseConfig reads a bundle's Streamline (sl_) feature rows, stamping each
// with the bundle's arch and hash. A bundle with no sl_ rows yields an empty
// slice.
func parseConfig(path, arch, hash string) ([]feature, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", path, err)
	}
	var features []feature
	for line := range strings.SplitSeq(string(data), "\n") {
		if feat, ok := parseFeature(line); ok {
			feat.arch = arch
			feat.hash = hash
			features = append(features, feat)
		}
	}
	return features, nil
}

// payloadPath is where the interposer looks for a feature's payload: the
// feature directory holds it under the version directory and the file is named
// after the arch and hash, e.g. 1B0_E658703.dll.
func payloadPath(root string, feat feature) string {
	file := feat.arch + "_" + feat.hash + feat.ext
	return filepath.Join(root, feat.name, versionsDir, strconv.Itoa(feat.ota), filesDir, file)
}

// siblingPayload finds the same payload under the sibling family's hash: both
// bundles ship identical bytes, so one fills the other's missing file. The
// sibling is matched by feature name and payload extension, never by a
// hard-coded hash.
func siblingPayload(root string, feat feature) (string, bool) {
	dir := filepath.Join(root, siblingFeature(feat.name), versionsDir, strconv.Itoa(feat.ota), filesDir)
	matches, _ := filepath.Glob(filepath.Join(dir, "*"+feat.ext))
	if len(matches) == 0 {
		return "", false
	}
	return matches[0], true
}

// siblingFeature swaps a feature between the two bundles, e.g.
// sl_common_0 <-> sl_common_override_0.
func siblingFeature(name string) string {
	if base, ok := strings.CutSuffix(name, "_override_0"); ok {
		return base + "_0"
	}
	base, _ := strings.CutSuffix(name, "_0")
	return base + "_override_0"
}

// ota returns the numeric OTA version directory of a package config path, or
// -1 when the path does not match the layout bundleDir recognises.
func ota(path string) int {
	parts := strings.Split(filepath.ToSlash(path), "/")
	if len(parts) < 5 {
		return -1
	}
	n, err := strconv.Atoi(parts[len(parts)-4])
	if err != nil {
		return -1
	}
	return n
}

// bundleDir returns the cache directory holding a package config, or "" when
// the path is not <bundle>/versions/<ota>/files/<arch>_<hash>/<name>.
func bundleDir(path string) string {
	parts := strings.Split(filepath.ToSlash(path), "/")
	if len(parts) < 5 || parts[len(parts)-5] != versionsDir {
		return ""
	}
	return filepath.FromSlash(strings.Join(parts[:len(parts)-5], "/"))
}

// dirArchHash splits a payload directory name into its arch and hash:
// "1B0_E658703" -> "1B0", "E658703".
func dirArchHash(dir string) (arch, hash string) {
	name := filepath.Base(dir)
	i := strings.IndexByte(name, '_')
	if i <= 0 || i == len(name)-1 {
		return "", ""
	}
	return name[:i], name[i+1:]
}

// exists reports whether path is an existing file.
func exists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}
