package nvidia

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/fernandoenzo/nvfp/internal/db"
	"github.com/fernandoenzo/set"
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

// versionOutcome classifies what ensureVersion did with one requested version.
type versionOutcome int

const (
	outcomeAdded versionOutcome = iota
	outcomeUpdated
	outcomeAlready
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
	versions, missingVersions := resolveVersions(fp, game)
	if versions.Len() == 0 {
		return patchResult(StatusNoSource, "no source version found for fingerprint %q", game.Fingerprint)
	}
	for version := range versions.IterAll() {
		switch ensureVersion(fp, game, version) {
		case outcomeAdded:
			added = append(added, version.Name)
		case outcomeUpdated:
			updated = append(updated, version.Name)
		case outcomeAlready:
			already = append(already, version.Name)
		case outcomeNoSource:
			return patchResult(StatusNoSource, "no source version found for fingerprint %q", game.Fingerprint)
		}
	}
	for versionName := range missingVersions.IterAll() {
		missing = append(missing, versionName)
	}
	return summarize(game.Fingerprint, added, updated, already, missing)
}

// resolveVersions returns the version names to process: the manifest list,
// or for a "*" request every existing version plus "uwp" when the game has
// an app_user_model_id and no UWP version exists yet.
func resolveVersions(fp *Fingerprint, game *db.Game) (versions *set.Set[*Version], missing *set.Set[string]) {
	versions = set.New[*Version](len(fp.Versions) + 1)
	versionNames := set.New[string](len(fp.Versions) + 1)
	all := game.VersionKeys.Contains(db.AllVersions)
	hasUWP := false
	for _, v := range fp.Versions {
		if all {
			versions.Add(v)
			continue
		}
		versionName := strings.ToLower(strings.TrimSpace(v.Name))
		if strings.EqualFold(versionName, db.UWP) {
			hasUWP = true
		}
		if game.VersionKeys.Contains(versionName) {
			versions.Add(v)
			versionNames.Add(versionName)
		}
	}
	if !hasUWP && (all || game.VersionKeys.Contains(db.UWP)) && game.AppUserModelID != "" {
		versions.Add(&Version{Name: db.UWP})
		versionNames.Add(db.UWP)
	}
	if !all {
		missing = game.VersionKeys.Difference(versionNames)
	}
	return versions, missing
}

// ensureVersion makes one requested version exist and carry the game's
// overrides/removals, and classifies the outcome. Only UWP versions are
// created when missing; any other missing version is just reported.
func ensureVersion(fp *Fingerprint, game *db.Game, version *Version) versionOutcome {
	if strings.EqualFold(version.Name, db.UWP) && version.Elements == nil {
		src := FindSourceVersion(fp)
		if src == nil {
			return outcomeNoSource
		}
		fp.Versions = append(fp.Versions, AddUWPVersion(src, game.AppUserModelID, game.Overrides, game.Remove))
		return outcomeAdded
	}
	if len(game.Overrides) == 0 && len(game.Remove) == 0 {
		return outcomeAlready
	}
	updated := UpdateVersion(version, game.Overrides, game.Remove)
	// DeepEqual treats nil and empty slices as different, but both mean
	// "no elements" — patch a version to empty, write, re-parse, patch
	// again → Elements is nil and DeepEqual would say "not equal".
	if len(updated.Elements) == 0 && len(version.Elements) == 0 {
		return outcomeAlready
	}
	if reflect.DeepEqual(updated, version) {
		return outcomeAlready
	}
	*version = *updated
	return outcomeUpdated
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
