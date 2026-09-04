package nvidia

import (
	"encoding/xml"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
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

// AttrValue returns the value of an attribute by name, or empty string.
func (e *XmlElement) AttrValue(name string) string {
	for _, a := range e.Attr {
		if a.Name.Local == name {
			return a.Value
		}
	}
	return ""
}

// ElementName returns the local XML element name (without namespace).
func (e *XmlElement) ElementName() string {
	return e.XMLName.Local
}

// ---- Fields removed by default during UWP clone ----

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

	data, err := io.ReadAll(f)
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", path, err)
	}

	var db ProfileDB
	if err := xml.Unmarshal(data, &db); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", path, err)
	}

	return &db, nil
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
	return copyFile(path, bakPath)
}

// copyFile copies src to dst, creating the destination directory if needed.
func copyFile(srcPath, dstPath string) error {
	src, err := os.Open(srcPath)
	if err != nil {
		return fmt.Errorf("opening %s: %w", srcPath, err)
	}
	defer src.Close()

	if err := os.MkdirAll(filepath.Dir(dstPath), 0o755); err != nil {
		return fmt.Errorf("creating directory: %w", err)
	}
	dst, err := os.Create(dstPath)
	if err != nil {
		return fmt.Errorf("creating %s: %w", dstPath, err)
	}
	defer dst.Close()

	if _, err := io.Copy(dst, src); err != nil {
		return fmt.Errorf("copying file: %w", err)
	}
	if err := dst.Sync(); err != nil {
		return fmt.Errorf("syncing %s: %w", dstPath, err)
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

// HasUWPVersion checks if a fingerprint already has a UWP version.
// Uses case-insensitive comparison for robustness.
func HasUWPVersion(fp *Fingerprint) bool {
	for _, v := range fp.Versions {
		if strings.EqualFold(v.Name, "uwp") {
			return true
		}
	}
	return false
}

// FindSourceVersion finds the best source version to clone from.
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

// CloneVersion creates a copy of a Version for UWP patching.
// It removes default fields, applies overrides, adds UWP-specific fields,
// forces Distributor to UWP, and sets the version name.
func CloneVersion(src *Version, appID string, overrides map[string]string, remove []string) Version {
	removeSet := buildRemoveSet(remove)
	forcedFields := buildForcedFields(appID)
	overrideSet := buildOverrideSet(overrides, forcedFields)

	clone := Version{
		Name:     "uwp",
		Elements: make([]XmlElement, 0, len(src.Elements)),
	}
	copyPreservedElements(&clone, src, removeSet, overrideSet)
	applyOverrides(&clone, overrideSet)
	return clone
}

// buildRemoveSet creates a set of element names to remove during cloning.
func buildRemoveSet(extra []string) map[string]bool {
	removeSet := make(map[string]bool)
	for _, f := range defaultRemoveFields {
		removeSet[strings.ToLower(f)] = true
	}
	for _, f := range extra {
		removeSet[strings.ToLower(f)] = true
	}
	return removeSet
}

// buildForcedFields returns the map of forced field names to their default values.
// Forced fields always win over user overrides (see buildOverrideSet).
func buildForcedFields(appID string) map[string]string {
	pkgFamily := db.PackageFamilyName(appID)
	return map[string]string{
		"Distributor":          "UWP",
		"UWPPackageFamilyName": pkgFamily,
		"AppUserModelId":       appID,
	}
}

// buildOverrideSet builds the unified override lookup: every override key
// (lowercased) mapped to its original casing and value, with forced fields
// taking priority over user overrides.
func buildOverrideSet(overrides map[string]string, forcedFields map[string]string) map[string][2]string {
	overrideSet := make(map[string][2]string, len(overrides)+len(forcedFields))
	for k, v := range overrides {
		nameLower := strings.ToLower(k)
		overrideSet[nameLower] = [2]string{k, v}
	}
	for k, v := range forcedFields {
		nameLower := strings.ToLower(k)
		overrideSet[nameLower] = [2]string{k, v}
	}
	return overrideSet
}

// copyPreservedElements copies source elements that survive filtering.
func copyPreservedElements(clone *Version, src *Version, removeSet map[string]bool, overrideSet map[string][2]string) {
	for _, elem := range src.Elements {
		nameLower := strings.ToLower(elem.ElementName())
		_, overrides := overrideSet[nameLower]
		if removeSet[nameLower] || overrides {
			continue
		}
		clone.Elements = append(clone.Elements, elem)
	}
}

func applyOverrides(clone *Version, overrideSet map[string][2]string) {
	keys := make([]string, 0, len(overrideSet))
	for k := range overrideSet {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		clone.Elements = append(clone.Elements, XmlElement{
			XMLName: xml.Name{Local: overrideSet[k][0]},
			Content: overrideSet[k][1],
		})
	}
}

// PatchStatus represents the outcome of a patch operation.
type PatchStatus string

const (
	StatusPatched    PatchStatus = "patched"
	StatusAlreadyUWP PatchStatus = "already_uwp"
	StatusNotFound   PatchStatus = "not_found"
	StatusNoSource   PatchStatus = "no_source"
)

// PatchResult holds the result of a single game patch operation.
type PatchResult struct {
	Fingerprint string
	Status      PatchStatus
	Message     string
}

// PatchGame applies the UWP profile patch for a single game.
// Returns a PatchResult indicating what happened.
func PatchGame(db *ProfileDB, fingerprint, appID string, overrides map[string]string, remove []string) PatchResult {
	fp := FindFingerprint(db, fingerprint)
	if fp == nil {
		return patchResult(fingerprint, StatusNotFound, "fingerprint %q not found in database", fingerprint)
	}
	if HasUWPVersion(fp) {
		return patchResult(fingerprint, StatusAlreadyUWP, "fingerprint %q already has a UWP version (NVIDIA includes it)", fingerprint)
	}
	src := FindSourceVersion(fp)
	if src == nil {
		return patchResult(fingerprint, StatusNoSource, "no source version found for fingerprint %q", fingerprint)
	}
	clone := CloneVersion(src, appID, overrides, remove)
	fp.Versions = append(fp.Versions, clone)
	return patchResult(fingerprint, StatusPatched, "patched %q (cloned from %q)", fingerprint, src.Name)
}

// patchResult builds a PatchResult with a formatted message.
func patchResult(fingerprint string, status PatchStatus, format string, args ...any) PatchResult {
	return PatchResult{
		Fingerprint: fingerprint,
		Status:      status,
		Message:     fmt.Sprintf(format, args...),
	}
}
