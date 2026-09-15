package nvidia

import (
	"fmt"
	"reflect"
	"slices"
	"strings"

	"github.com/fernandoenzo/nvfp/internal/db"
)

// PatchStatus represents the outcome of a patch operation.
type PatchStatus string

const (
	StatusPatched         PatchStatus = "patched"
	StatusAlreadyPresent  PatchStatus = "already_present"
	StatusNotFound        PatchStatus = "not_found"
	StatusNoSource        PatchStatus = "no_source"
	StatusVersionNotFound PatchStatus = "version_not_found"
)

// PatchResult holds the result of a single game patch operation.
type PatchResult struct {
	Status  PatchStatus
	Message string
}

// versionPlan is the manifest request resolved against a fingerprint: the
// existing versions to update, whether a new uwp version must be created, and
// the requested names the fingerprint does not have.
type versionPlan struct {
	existing []*Version
	addUWP   bool
	missing  []string
}

// PatchGame ensures the requested versions of a game exist and carry the
// given overrides/removals. UWP versions are added when missing; other
// versions are only updated. Returns a PatchResult indicating what happened.
func PatchGame(fdb *FingerprintDB, game *db.Game) PatchResult {
	fp := FindFingerprint(fdb, game.Fingerprint)
	if fp == nil {
		return patchResult(StatusNotFound, "fingerprint %q not found in database", game.Fingerprint)
	}
	plan := resolveVersions(fp, game)
	var added, updated, already []string
	if plan.addUWP {
		src := FindSourceVersion(fp)
		if src == nil {
			return patchResult(StatusNoSource, "no source version found for fingerprint %q", game.Fingerprint)
		}
		fp.Versions = append(fp.Versions, AddUWPVersion(src, game.AppUserModelID, game.Overrides, game.Remove))
		added = append(added, db.UWP)
	}
	for _, version := range plan.existing {
		if applyVersion(game, version) {
			updated = append(updated, version.Name)
		} else {
			already = append(already, version.Name)
		}
	}
	return summarize(game.Fingerprint, added, updated, already, plan.missing)
}

// resolveVersions resolves the manifest request against the fingerprint: the
// existing versions to update, whether a new uwp version can be created, and
// the requested names the fingerprint does not have. A "*" request means every
// existing version, plus uwp when the game has an AppUserModelID and lacks one.
// A version that will be created is not reported as missing.
func resolveVersions(fp *Fingerprint, game *db.Game) *versionPlan {
	wanted := game.VersionKeys()
	all := wanted.Contains(db.AllVersions)
	plan := new(versionPlan)
	seen := make([]string, 0, len(fp.Versions))
	hasUWP := false
	for _, version := range fp.Versions {
		name := strings.ToLower(strings.TrimSpace(version.Name))
		seen = append(seen, name)
		hasUWP = hasUWP || name == db.UWP
		if all || wanted.Contains(name) {
			plan.existing = append(plan.existing, version)
		}
	}
	plan.addUWP = !hasUWP && (all || wanted.Contains(db.UWP)) && game.AppUserModelID != ""
	if plan.addUWP {
		seen = append(seen, db.UWP)
	}
	if !all {
		for _, name := range game.Versions {
			key := strings.ToLower(strings.TrimSpace(name))
			if !slices.Contains(seen, key) {
				plan.missing = append(plan.missing, key)
			}
		}
	}
	return plan
}

// applyVersion updates one existing version with the game's overrides and
// removals, reporting whether it changed. A version that already carries them
// is left untouched.
func applyVersion(game *db.Game, version *Version) bool {
	if len(game.Overrides) == 0 && len(game.Remove) == 0 {
		return false
	}
	updated := UpdateVersion(version, game.Overrides, game.Remove)
	if equalElements(updated, version) {
		return false
	}
	*version = *updated
	return true
}

// equalElements reports whether a rebuilt version is identical to the original.
// DeepEqual treats nil and empty slices as different, but both mean "no
// elements" — patch a version to empty, write, re-parse, patch again →
// Elements is nil and DeepEqual would say "not equal".
func equalElements(built, original *Version) bool {
	if len(built.Elements) == 0 && len(original.Elements) == 0 {
		return true
	}
	return reflect.DeepEqual(built, original)
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

// patchResult builds a PatchResult with a formatted message.
func patchResult(status PatchStatus, format string, args ...any) PatchResult {
	return PatchResult{
		Status:  status,
		Message: fmt.Sprintf(format, args...),
	}
}
