// Package ngx maintains the NGX OTA manifest the Streamline interposer reads.
//
// The NVIDIA App's bootstrap can rewrite nvngx_config.txt and leave only the
// bundle sections, dropping the per-feature [sl_<feat>_0] and
// [sl_<feat>_override_0] ones. The interposer resolves plugins per feature, so
// once those sections are gone both of its passes fail and the game falls back
// to its bundled plugins. Inspect and Apply rebuild them from the bundles' own
// package configs, correct a section that pins a version the cache no longer
// has, fill a payload that exists under only one hash from its sibling bundle,
// and are idempotent: a repaired manifest is never touched again.
//
// Nothing about the bundles is hard-coded: a Streamline bundle is any
// <dir>/versions/<ota>/files/<arch>_<hash>/nvngx_package_config.txt under the
// root whose rows name sl_ features, and its arch and app hash come from that
// directory name. A bundle holding several configs contributes the one under
// the highest numeric OTA directory. A feature name is taken once, from the
// first bundle (in bundle-name order) that names it.
//
// The manifest is parsed, mutated in memory and written back whole, so the
// format itself lives in one place: manifest.go.
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
	Root      string    // NGX OTA cache directory
	Manifest  string    // nvngx_config.txt inside Root
	Backup    string    // the first write copies the manifest here
	Additions []Section // sections the manifest lacks
	Updates   []Section // sections pinning a version the bundle no longer ships
	Copies    []Copy    // payloads missing under a hash, filled from the sibling
	Missing   []string  // features with no payload in either family
	Warnings  []string  // bundles that could not be examined

	doc *manifest // parsed manifest, mutated by Apply
}

// Section is one per-feature manifest entry and what the manifest says about it.
type Section struct {
	Feature string // e.g. sl_common_override_0
	Hash    string // app hash of its bundle, e.g. E658700
	Version string // version the bundle ships now
	Current string // version the manifest declares for Updates, empty when absent
}

// Copy is one payload file the cache is missing.
type Copy struct {
	Feature string // feature whose payload is missing
	Source  string
	Dest    string
}

// Changed reports whether applying the plan would touch any file.
func (p *Plan) Changed() bool {
	return len(p.Copies) > 0 || len(p.Additions) > 0 || len(p.Updates) > 0
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
		plan.Warnings = append(plan.Warnings, fmt.Sprintf(
			"%s: no Streamline (sl_) bundle found; open the NVIDIA App once to populate its cache", root))
	}
	for _, feat := range features {
		plan.add(root, feat)
	}
	return plan, nil
}

// add records one feature: a payload copy when its file is missing, and the
// manifest entry it needs: an appended section, a corrected version, or
// nothing when the manifest is already current.
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
	section := Section{Feature: feat.name, Hash: feat.hash, Version: feat.version}
	b := p.doc.section(feat.name)
	if b == nil {
		p.Additions = append(p.Additions, section)
		return
	}
	section.Current, _ = b.get("app_" + feat.hash)
	if section.Current != section.Version {
		p.Updates = append(p.Updates, section)
	}
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
	if len(plan.Additions) == 0 && len(plan.Updates) == 0 {
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
	for _, section := range plan.Updates {
		if b := plan.doc.section(section.Feature); b != nil {
			b.set("app_"+section.Hash, section.Version)
		}
	}
	for _, section := range plan.Additions {
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

// config is one bundle's package config in the cache layout, with the arch and
// app hash taken from its directory name.
type config struct {
	bundle string // cache directory, e.g. .../sl_sdk_0
	ota    int    // numeric OTA directory, -1 when it does not parse
	arch   string // e.g. 1B0
	hash   string // e.g. E658703
	path   string
}

// discover finds every Streamline bundle in the cache and returns its features,
// one per feature name, in bundle-name order, plus a warning per selected
// config whose directory name or content could not be read.
func discover(root string) ([]feature, []string) {
	var (
		newest   = map[string]config{} // bundle dir -> its config under the highest OTA
		warnings []string
	)
	_ = filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() || entry.Name() != packageConfigName {
			return nil
		}
		cfg, ok := configFrom(path)
		if !ok {
			return nil
		}
		if current, seen := newest[cfg.bundle]; !seen || cfg.ota > current.ota {
			newest[cfg.bundle] = cfg
		}
		return nil
	})
	var (
		features []feature
		seen     = set.New[string](len(newest))
	)
	for _, cfg := range slices.SortedFunc(maps.Values(newest), func(a, b config) int {
		return strings.Compare(a.bundle, b.bundle)
	}) {
		if cfg.arch == "" || cfg.hash == "" {
			warnings = append(warnings, fmt.Sprintf("%s: no <arch>_<hash> in its directory name", cfg.path))
			continue
		}
		feats, err := parseConfig(cfg.path, cfg.arch, cfg.hash)
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

// configFrom parses a package config path in the cache layout
// <bundle>/versions/<ota>/files/<arch>_<hash>/<name>. A non-numeric <ota>
// sorts lowest; ok is false when the path is not in that layout.
func configFrom(path string) (config, bool) {
	parts := strings.Split(filepath.ToSlash(path), "/")
	if len(parts) < 5 || parts[len(parts)-5] != versionsDir {
		return config{}, false
	}
	ota, err := strconv.Atoi(parts[len(parts)-4])
	if err != nil {
		ota = -1
	}
	arch, hash := splitArchHash(parts[len(parts)-2])
	return config{
		bundle: filepath.FromSlash(strings.Join(parts[:len(parts)-5], "/")),
		ota:    ota,
		arch:   arch,
		hash:   hash,
		path:   path,
	}, true
}

// splitArchHash splits a payload directory name into its arch and hash:
// "1B0_E658703" -> "1B0", "E658703".
func splitArchHash(name string) (arch, hash string) {
	i := strings.IndexByte(name, '_')
	if i <= 0 || i == len(name)-1 {
		return "", ""
	}
	return name[:i], name[i+1:]
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
// sibling payload is matched by feature name and extension, never by a
// hard-coded hash; the first match in sorted order wins.
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

// exists reports whether path is an existing file.
func exists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}
