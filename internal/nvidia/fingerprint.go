package nvidia

import (
	"encoding/xml"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// ---- XML model for fingerprint.db ----

// ProfileDB is the root element of fingerprint.db.
type ProfileDB struct {
	XMLName     xml.Name      `xml:"ProfileDB"`
	Fingerprints []Fingerprint `xml:"Fingerprint"`
}

// Fingerprint represents a single game fingerprint entry.
type Fingerprint struct {
	Name     string    `xml:"name,attr"`
	Versions []Version `xml:"Version"`
}

// Version represents a version element within a fingerprint.
type Version struct {
	Name     string      `xml:"name,attr"`
	Elements []XmlElement `xml:",any"`
}

// XmlElement represents a generic XML element, preserving unknown children.
type XmlElement struct {
	XMLName  xml.Name
	Attr     []xml.Attr `xml:",any,attr"`
	Content  string     `xml:",chardata"`
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

// isDefaultRemoveField checks if an element name is in the default remove list.
func isDefaultRemoveField(name string) bool {
	for _, f := range defaultRemoveFields {
		if strings.EqualFold(name, f) {
			return true
		}
	}
	return false
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

// CloneVersion creates a deep copy of a Version for UWP patching.
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
	copyPreservedElements(&clone, src, removeSet, forcedFields, overrideSet)
	applyNonForcedOverrides(&clone, overrides, forcedFields)
	appendForcedElements(&clone, forcedFields, overrides)
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
// Override values take priority and are resolved later in appendForcedElements.
func buildForcedFields(appID string) map[string]string {
	pkgFamily := appID
	if idx := strings.Index(appID, "!"); idx >= 0 {
		pkgFamily = appID[:idx]
	}
	return map[string]string{
		"distributor":          "UWP",
		"uwppackagefamilyname": pkgFamily,
		"appusermodelid":       appID,
	}
}

// buildOverrideSet collects non-forced override keys.
func buildOverrideSet(overrides map[string]string, forcedFields map[string]string) map[string]bool {
	overrideSet := make(map[string]bool)
	for k := range overrides {
		nameLower := strings.ToLower(k)
		if _, forced := forcedFields[nameLower]; !forced {
			overrideSet[nameLower] = true
		}
	}
	return overrideSet
}

// copyPreservedElements deep-copies source elements that survive filtering.
func copyPreservedElements(clone *Version, src *Version, removeSet map[string]bool, forcedFields map[string]string, overrideSet map[string]bool) {
	for _, elem := range src.Elements {
		nameLower := strings.ToLower(elem.ElementName())
		if removeSet[nameLower] || overrideSet[nameLower] {
			continue
		}
		if _, forced := forcedFields[nameLower]; forced {
			continue
		}
		clone.Elements = append(clone.Elements, deepCopyElement(elem))
	}
}
// applyNonForcedOverrides appends override elements that are not forced fields.
func applyNonForcedOverrides(clone *Version, overrides map[string]string, forcedFields map[string]string) {
	for k, v := range overrides {
		if _, forced := forcedFields[strings.ToLower(k)]; forced {
			continue
		}
		clone.Elements = append(clone.Elements, XmlElement{
			XMLName: xml.Name{Local: k},
			Content: v,
		})
	}
}

// appendForcedElements emits forced fields exactly once, with override values taking priority.
func appendForcedElements(clone *Version, forcedFields map[string]string, overrides map[string]string) {
	for name, defaultVal := range forcedFields {
		val := defaultVal
		elemName := canonicalFieldName(name)
		if overrides != nil {
			for k, v := range overrides {
				if strings.ToLower(k) == name {
					val = v
					elemName = k
					break
				}
			}
		}
		clone.Elements = append(clone.Elements, XmlElement{
			XMLName: xml.Name{Local: elemName},
			Content: val,
		})
	}
}

// canonicalFieldName maps a lowercased forced field name back to its canonical casing.
func canonicalFieldName(lower string) string {
	switch lower {
	case "distributor":
		return "Distributor"
	case "uwppackagefamilyname":
		return "UWPPackageFamilyName"
	case "appusermodelid":
		return "AppUserModelId"
	default:
		return lower
	}
}

// deepCopyElement creates a deep copy of an XmlElement.
func deepCopyElement(elem XmlElement) XmlElement {
	cp := XmlElement{
		XMLName: elem.XMLName,
		Content: elem.Content,
	}
	cp.Attr = make([]xml.Attr, len(elem.Attr))
	copy(cp.Attr, elem.Attr)
	if len(elem.Children) > 0 {
		cp.Children = make([]XmlElement, len(elem.Children))
		for i, child := range elem.Children {
			cp.Children[i] = deepCopyElement(child)
		}
	}
	return cp
}

// PatchResult holds the result of a single game patch operation.
type PatchResult struct {
	Fingerprint string
	Status       string // "patched", "already_uwp", "not_found", "no_source"
	Message      string
}

// PatchGame applies the UWP profile patch for a single game.
// Returns a PatchResult indicating what happened.
func PatchGame(db *ProfileDB, fingerprint, appID string, overrides map[string]string, remove []string) PatchResult {
	fp := FindFingerprint(db, fingerprint)
	if fp == nil {
		return patchResult(fingerprint, "not_found", "fingerprint %q not found in database", fingerprint)
	}
	if HasUWPVersion(fp) {
		return patchResult(fingerprint, "already_uwp", "fingerprint %q already has a UWP version (NVIDIA includes it)", fingerprint)
	}
	src := FindSourceVersion(fp)
	if src == nil {
		return patchResult(fingerprint, "no_source", "no source version found for fingerprint %q", fingerprint)
	}
	clone := CloneVersion(src, appID, overrides, remove)
	fp.Versions = append(fp.Versions, clone)
	return patchResult(fingerprint, "patched", "patched %q (cloned from %q)", fingerprint, src.Name)
}

// patchResult builds a PatchResult with a formatted message.
func patchResult(fingerprint, status, format string, args ...any) PatchResult {
	return PatchResult{
		Fingerprint: fingerprint,
		Status:       status,
		Message:      fmt.Sprintf(format, args...),
	}
}