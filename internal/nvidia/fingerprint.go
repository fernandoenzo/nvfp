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

// IsDefaultRemoveField checks if an element name is in the default remove list.
func IsDefaultRemoveField(name string) bool {
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
		// Backup already exists
		return nil
	}

	src, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("opening %s for backup: %w", path, err)
	}
	defer src.Close()

	if err := os.MkdirAll(filepath.Dir(bakPath), 0o755); err != nil {
		return fmt.Errorf("creating backup directory: %w", err)
	}

	dst, err := os.Create(bakPath)
	if err != nil {
		return fmt.Errorf("creating %s: %w", bakPath, err)
	}
	defer dst.Close()

	if _, err := io.Copy(dst, src); err != nil {
		return fmt.Errorf("copying to backup: %w", err)
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
	// Deep copy elements
	clone := Version{
		Name:     "uwp",
		Elements: make([]XmlElement, 0, len(src.Elements)),
	}

	// Build remove set
	removeSet := make(map[string]bool)
	for _, f := range defaultRemoveFields {
		removeSet[strings.ToLower(f)] = true
	}
	for _, f := range remove {
		removeSet[strings.ToLower(f)] = true
	}

	// Build override set
	overrideSet := make(map[string]bool)
	for k := range overrides {
		overrideSet[strings.ToLower(k)] = true
	}

	// Copy elements, skipping removed fields and fields that will be overridden
	hasDistributor := false
	for _, elem := range src.Elements {
		nameLower := strings.ToLower(elem.ElementName())

		if removeSet[nameLower] {
			continue
		}

		if overrideSet[nameLower] {
			// Will be replaced by overrides below
			continue
		}

		if strings.ToLower(elem.ElementName()) == "distributor" {
			hasDistributor = true
			clone.Elements = append(clone.Elements, XmlElement{
				XMLName: xml.Name{Local: "Distributor"},
				Content: "UWP",
			})
			continue
		}

		clone.Elements = append(clone.Elements, deepCopyElement(elem))
	}

	// Add Distributor if it wasn't in source
	if !hasDistributor {
		clone.Elements = append(clone.Elements, XmlElement{
			XMLName: xml.Name{Local: "Distributor"},
			Content: "UWP",
		})
	}

	// Apply overrides
	for k, v := range overrides {
		clone.Elements = append(clone.Elements, XmlElement{
			XMLName: xml.Name{Local: k},
			Content: v,
		})
	}

	// Add UWP-specific fields
	pkgFamily := appID
	if idx := strings.Index(appID, "!"); idx >= 0 {
		pkgFamily = appID[:idx]
	}

	clone.Elements = append(clone.Elements, XmlElement{
		XMLName: xml.Name{Local: "UWPPackageFamilyName"},
		Content: pkgFamily,
	})
	clone.Elements = append(clone.Elements, XmlElement{
		XMLName: xml.Name{Local: "AppUserModelId"},
		Content: appID,
	})

	return clone
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
		return PatchResult{
			Fingerprint: fingerprint,
			Status:       "not_found",
			Message:      fmt.Sprintf("fingerprint %q not found in database", fingerprint),
		}
	}

	if HasUWPVersion(fp) {
		return PatchResult{
			Fingerprint: fingerprint,
			Status:       "already_uwp",
			Message:      fmt.Sprintf("fingerprint %q already has a UWP version (NVIDIA includes it)", fingerprint),
		}
	}

	src := FindSourceVersion(fp)
	if src == nil {
		return PatchResult{
			Fingerprint: fingerprint,
			Status:       "no_source",
			Message:      fmt.Sprintf("no source version found for fingerprint %q", fingerprint),
		}
	}

	clone := CloneVersion(src, appID, overrides, remove)
	fp.Versions = append(fp.Versions, clone)

	return PatchResult{
		Fingerprint: fingerprint,
		Status:       "patched",
		Message:      fmt.Sprintf("patched %q (cloned from %q)", fingerprint, src.Name),
	}
}