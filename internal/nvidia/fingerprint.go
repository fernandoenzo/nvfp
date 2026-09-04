package nvidia

import (
	"encoding/xml"
	"fmt"
	"io"
	"maps"
	"os"
	"slices"
	"strings"

	"github.com/fernandoenzo/nvidia-uwp-patch/internal/db"
)

// ---- XML model for fingerprint.db ----

// ProfileDB is the root element of fingerprint.db.
type ProfileDB struct {
	XMLName      xml.Name      `xml:"FingerprintDB"`
	Fingerprints []Fingerprint `xml:"Fingerprint"`
}

// Fingerprint represents a single game fingerprint entry.
type Fingerprint struct {
	Name     string       `xml:"name,attr"`
	Elements []XmlElement `xml:",any"`
	Versions []Version    `xml:"Version"`
}

// Version represents a version element within a fingerprint.
type Version struct {
	Name     string       `xml:"name,attr"`
	Elements []XmlElement `xml:",any"`
}

// XmlElement represents a generic XML element, preserving unknown children.
type XmlElement struct {
	XMLName  xml.Name
	Attr     []xml.Attr   `xml:",any,attr"`
	Content  string       `xml:",chardata"`
	Children []XmlElement `xml:",any"`
}

// ElementName returns the local XML element name (without namespace).
func (e *XmlElement) ElementName() string {
	return e.XMLName.Local
}

// ---- Fields removed by default during UWP addition ----

var defaultRemoveFields = []string{
	"Files",
	"Launch",
	"Directories",
	"InstallDirRegValues",
	// Platform AppIds
	"SteamAppIds",
	"EpicAppId",
	"GogAppId",
	"TencentAppId",
	"BattleNetAppId",
	"GarenaAppIds",
	"OriginAppIds",
	"OculusAppId",
}

// ---- Core operations ----

// ParseProfileDB reads and parses a fingerprint.db file.
func ParseProfileDB(path string) (*ProfileDB, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("opening %s: %w", path, err)
	}
	defer f.Close()

	var pdb ProfileDB
	if err := xml.NewDecoder(f).Decode(&pdb); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", path, err)
	}

	return &pdb, nil
}

// WriteProfileDB writes the ProfileDB to a file with XML header.
func WriteProfileDB(db *ProfileDB, path string) error {
	output, err := xml.MarshalIndent(db, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling XML: %w", err)
	}

	content := xml.Header + string(output) + "\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return fmt.Errorf("writing %s: %w", path, err)
	}

	return nil
}

// BackupFile creates a .bak copy of the file if the backup doesn't already exist.
func BackupFile(path string) error {
	bakPath := path + ".bak"
	if _, err := os.Stat(bakPath); err == nil {
		return nil
	}

	src, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("opening %s: %w", path, err)
	}
	defer src.Close()

	dst, err := os.Create(bakPath)
	if err != nil {
		return fmt.Errorf("creating %s: %w", bakPath, err)
	}
	defer dst.Close()

	if _, err := io.Copy(dst, src); err != nil {
		return fmt.Errorf("copying %s to %s: %w", path, bakPath, err)
	}
	if err := dst.Sync(); err != nil {
		return fmt.Errorf("syncing %s: %w", bakPath, err)
	}
	return nil
}

// FindFingerprint finds a fingerprint by exact name.
func FindFingerprint(db *ProfileDB, name string) *Fingerprint {
	for i := range db.Fingerprints {
		if db.Fingerprints[i].Name == name {
			return &db.Fingerprints[i]
		}
	}
	return nil
}

// FindSourceVersion finds the best source version to build a UWP version from.
// Priority: steam > first non-uwp version.
func FindSourceVersion(fp *Fingerprint) *Version {
	var firstNonUWP *Version
	for i := range fp.Versions {
		v := &fp.Versions[i]
		if strings.EqualFold(v.Name, "steam") {
			return v
		}
		if firstNonUWP == nil && !strings.EqualFold(v.Name, "uwp") {
			firstNonUWP = v
		}
	}
	return firstNonUWP
}

// AddUWPVersion builds a new UWP version from a source version.
// It removes default fields, applies overrides, adds UWP-specific fields,
// forces Distributor to UWP, and sets the version name.
func AddUWPVersion(src *Version, appID string, overrides map[string]string, remove []string) Version {
	return buildVersion(src, appID, overrides, remove, true)
}

// UpdateVersion rebuilds an existing version with overrides and removals.
// Unlike AddUWPVersion, it preserves the version name and existing forced
// fields, and only removes the explicitly listed elements.
func UpdateVersion(src *Version, overrides map[string]string, remove []string) Version {
	return buildVersion(src, "", overrides, remove, false)
}

// buildVersion builds a Version from a source, either as a new UWP addition
// (addUWP=true: default removals and forced fields from appID) or as an update
// of an existing version (addUWP=false: only explicit removals, no forced fields).
func buildVersion(src *Version, appID string, overrides map[string]string, remove []string, addUWP bool) Version {
	removeSet := buildRemoveSet(remove, addUWP)
	var forcedFields map[string]string
	if addUWP {
		forcedFields = buildUWPForcedFields(appID)
	}
	overrideSet := buildOverrideSet(overrides, forcedFields)

	name := "uwp"
	if !addUWP {
		name = src.Name
	}
	built := Version{
		Name:     name,
		Elements: make([]XmlElement, 0, len(src.Elements)+len(overrideSet)),
	}
	copyPreservedElements(&built, src, removeSet, overrideSet)
	applyOverrides(&built, overrideSet)
	return built
}

// buildRemoveSet creates a set of element names to remove when building a version.
// Default removals only apply when adding a new version.
func buildRemoveSet(extra []string, includeDefaults bool) map[string]bool {
	removeSet := make(map[string]bool)
	if includeDefaults {
		for _, f := range defaultRemoveFields {
			removeSet[strings.ToLower(f)] = true
		}
	}
	for _, f := range extra {
		removeSet[strings.ToLower(f)] = true
	}
	return removeSet
}

// buildUWPForcedFields returns the map of forced field names to their default values.
// User overrides take priority over these defaults (see buildOverrideSet).
func buildUWPForcedFields(appID string) map[string]string {
	pkgFamily := db.PackageFamilyName(appID)
	return map[string]string{
		"Distributor":          "UWP",
		"UWPPackageFamilyName": pkgFamily,
		"AppUserModelId":       appID,
	}
}

// buildOverrideSet builds the unified override lookup: every override key
// (lowercased) mapped to its original casing and value. Forced fields provide
// defaults that user overrides take priority over.
func buildOverrideSet(overrides map[string]string, forcedFields map[string]string) map[string][2]string {
	overrideSet := make(map[string][2]string, len(overrides)+len(forcedFields))
	// forcedFields first so user overrides take priority.
	for _, fields := range []map[string]string{forcedFields, overrides} {
		for k, v := range fields {
			overrideSet[strings.ToLower(k)] = [2]string{k, v}
		}
	}
	return overrideSet
}

// copyPreservedElements copies source elements that survive filtering.
func copyPreservedElements(dst *Version, src *Version, removeSet map[string]bool, overrideSet map[string][2]string) {
	for _, elem := range src.Elements {
		nameLower := strings.ToLower(elem.ElementName())
		_, overrides := overrideSet[nameLower]
		if removeSet[nameLower] || overrides {
			continue
		}
		dst.Elements = append(dst.Elements, elem)
	}
}

// applyOverrides appends override elements, sorted by key for deterministic output.
func applyOverrides(dst *Version, overrideSet map[string][2]string) {
	for _, k := range slices.Sorted(maps.Keys(overrideSet)) {
		dst.Elements = append(dst.Elements, XmlElement{
			XMLName: xml.Name{Local: overrideSet[k][0]},
			Content: overrideSet[k][1],
		})
	}
}
