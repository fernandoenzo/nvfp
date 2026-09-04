package nvidia

import (
	"fmt"
	"strings"
)

// PatchStatus represents the outcome of a patch operation.
type PatchStatus string

const (
	StatusPatched         PatchStatus = "patched"
	StatusAlreadyUWP      PatchStatus = "already_uwp"
	StatusNotFound        PatchStatus = "not_found"
	StatusNoSource        PatchStatus = "no_source"
	StatusVersionNotFound PatchStatus = "version_not_found"
)

// PatchResult holds the result of a single game patch operation.
type PatchResult struct {
	Fingerprint string
	Status      PatchStatus
	Message     string
}

// PatchGame ensures the requested versions of a game exist and carry the
// given overrides/removals. UWP versions are added when missing; other
// versions are only updated. Returns a PatchResult indicating what happened.
func PatchGame(db *ProfileDB, fingerprint, appID string, versions, remove []string, overrides map[string]string) PatchResult {
	fp := FindFingerprint(db, fingerprint)
	if fp == nil {
		return patchResult(fingerprint, StatusNotFound, "fingerprint %q not found in database", fingerprint)
	}

	var added, updated, already, missing []string
	for _, name := range versions {
		if v := findVersion(fp, name); v != nil {
			if len(overrides) == 0 && len(remove) == 0 {
				already = append(already, name)
				continue
			}
			*v = UpdateVersion(v, overrides, remove)
			updated = append(updated, name)
			continue
		}
		if strings.EqualFold(name, "uwp") {
			src := FindSourceVersion(fp)
			if src == nil {
				return patchResult(fingerprint, StatusNoSource, "no source version found for fingerprint %q", fingerprint)
			}
			addedVersion := AddUWPVersion(src, appID, overrides, remove)
			fp.Versions = append(fp.Versions, addedVersion)
			added = append(added, name)
			continue
		}
		missing = append(missing, name)
	}

	switch {
	case len(added)+len(updated) > 0:
		msg := describeChanges(fingerprint, added, updated, missing)
		return patchResult(fingerprint, StatusPatched, "%s", msg)
	case len(missing) > 0:
		return patchResult(fingerprint, StatusVersionNotFound, "fingerprint %q has none of the requested versions: %s", fingerprint, strings.Join(missing, ", "))
	case len(already) > 0:
		return patchResult(fingerprint, StatusAlreadyUWP, "fingerprint %q already has %s version(s)", fingerprint, strings.Join(already, ", "))
	default:
		return patchResult(fingerprint, StatusVersionNotFound, "fingerprint %q has none of the requested versions", fingerprint)
	}
}

// describeChanges composes the patch message for a game.
func describeChanges(fingerprint string, added, updated, missing []string) string {
	var parts []string
	if len(added) > 0 {
		parts = append(parts, "added "+strings.Join(added, ", ")+" version(s)")
	}
	if len(updated) > 0 {
		parts = append(parts, "updated "+strings.Join(updated, ", ")+" version(s)")
	}
	msg := fmt.Sprintf("%s of %q", strings.Join(parts, ", "), fingerprint)
	if len(missing) > 0 {
		msg += fmt.Sprintf(" (%s not found)", strings.Join(missing, ", "))
	}
	return msg
}

// findVersion returns the version with the given name (case-insensitive), or nil.
func findVersion(fp *Fingerprint, name string) *Version {
	for i := range fp.Versions {
		if strings.EqualFold(fp.Versions[i].Name, name) {
			return &fp.Versions[i]
		}
	}
	return nil
}

// patchResult builds a PatchResult with a formatted message.
func patchResult(fingerprint string, status PatchStatus, format string, args ...any) PatchResult {
	return PatchResult{
		Fingerprint: fingerprint,
		Status:      status,
		Message:     fmt.Sprintf(format, args...),
	}
}
