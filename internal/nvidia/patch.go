package nvidia

import "fmt"

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
