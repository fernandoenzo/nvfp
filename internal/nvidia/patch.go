package nvidia

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/fernandoenzo/nvidia-uwp-patch/internal/db"
)

// PatchStatus represents the outcome of a patch operation.
type PatchStatus string

const (
	StatusPatched         PatchStatus = "patched"
	StatusAlreadyPresent  PatchStatus = "already_uwp"
	StatusNotFound        PatchStatus = "not_found"
	StatusNoSource        PatchStatus = "no_source"
	StatusVersionNotFound PatchStatus = "version_not_found"
)

// PatchResult holds the result of a single game patch operation.
type PatchResult struct {
	Status  PatchStatus
	Message string
}

// versionOutcome classifies what ensureVersion did with one requested version.
type versionOutcome int

const (
	outcomeAdded versionOutcome = iota
	outcomeUpdated
	outcomeAlready
	outcomeMissing
	outcomeNoSource
)

// PatchGame ensures the requested versions of a game exist and carry the
// given overrides/removals. UWP versions are added when missing; other
// versions are only updated. Returns a PatchResult indicating what happened.
func PatchGame(fdb *FingerprintDB, game *db.Game) PatchResult {
	fp := FindFingerprint(fdb, game.Fingerprint)
	if fp == nil {
		return patchResult(StatusNotFound, "fingerprint %q not found in database", game.Fingerprint)
	}

	var added, updated, already, missing []string
	for _, name := range game.Versions {
		switch ensureVersion(fp, game, name) {
		case outcomeAdded:
			added = append(added, name)
		case outcomeUpdated:
			updated = append(updated, name)
		case outcomeAlready:
			already = append(already, name)
		case outcomeMissing:
			missing = append(missing, name)
		case outcomeNoSource:
			return patchResult(StatusNoSource, "no source version found for fingerprint %q", game.Fingerprint)
		}
	}
	return summarize(game.Fingerprint, added, updated, already, missing)
}

// ensureVersion makes one requested version exist and carry the game's
// overrides/removals, and classifies the outcome. Only UWP versions are
// created when missing; any other missing version is just reported.
func ensureVersion(fp *Fingerprint, game *db.Game, name string) versionOutcome {
	if v := findVersion(fp, name); v != nil {
		if len(game.Overrides) == 0 && len(game.Remove) == 0 {
			return outcomeAlready
		}
		updated := UpdateVersion(v, game.Overrides, game.Remove)
		if reflect.DeepEqual(updated, *v) {
			return outcomeAlready
		}
		*v = updated
		return outcomeUpdated
	}
	if !strings.EqualFold(name, "uwp") {
		return outcomeMissing
	}
	src := FindSourceVersion(fp)
	if src == nil {
		return outcomeNoSource
	}
	fp.Versions = append(fp.Versions, AddUWPVersion(src, game.AppUserModelID, game.Overrides, game.Remove))
	return outcomeAdded
}

// summarize composes the final PatchResult from the per-version outcomes.
func summarize(fingerprint string, added, updated, already, missing []string) PatchResult {
	if len(added)+len(updated) > 0 {
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
		return PatchResult{Status: StatusPatched, Message: msg}
	}
	switch {
	case len(missing) > 0:
		return patchResult(StatusVersionNotFound, "fingerprint %q has none of the requested versions: %s", fingerprint, strings.Join(missing, ", "))
	case len(already) > 0:
		return patchResult(StatusAlreadyPresent, "fingerprint %q already has %s version(s)", fingerprint, strings.Join(already, ", "))
	default:
		return patchResult(StatusVersionNotFound, "fingerprint %q has none of the requested versions", fingerprint)
	}
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
func patchResult(status PatchStatus, format string, args ...any) PatchResult {
	return PatchResult{
		Status:  status,
		Message: fmt.Sprintf(format, args...),
	}
}
