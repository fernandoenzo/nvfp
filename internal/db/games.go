package db

import (
	"encoding/json/jsontext"
	"encoding/json/v2"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"unicode/utf16"

	"github.com/fernandoenzo/nvfp/internal/fsutil"
	"github.com/fernandoenzo/set"
)

// GameDB represents the games.json database.
type GameDB struct {
	Version int     `json:"version"`
	Games   []*Game `json:"games"`
}

// Game represents a single game entry in games.json.
type Game struct {
	Fingerprint    string            `json:"fingerprint"`
	AppUserModelID string            `json:"app_user_model_id"`
	DriverProfile  string            `json:"driver_profile,omitempty"`
	DriverApps     []string          `json:"driver_app,omitempty"`
	SkipDriver     bool              `json:"skip_driver,omitempty"`
	Versions       []string          `json:"versions"`
	Overrides      map[string]string `json:"overrides,omitempty"`
	Remove         []string          `json:"remove,omitempty"`
	versionKeys    *set.Set[string]
}

const (
	AllVersions = "*"
	UWP         = "uwp"
)

// MaxDriverString is the maximum length of an NVAPI unicode string (2048 UTF-16
// units including the terminating NUL).
const MaxDriverString = 2047

// PackageFamilyName extracts the package family name from a UWP app ID
// by taking everything before the first '!'. If no '!' is present,
// the full app ID is returned.
func PackageFamilyName(appID string) string {
	prefix, _, _ := strings.Cut(appID, "!")
	return prefix
}

// UWPPackageFamilyName derives the package family name from the app_user_model_id
// by taking everything before the first '!'.
func (g Game) UWPPackageFamilyName() string {
	return PackageFamilyName(g.AppUserModelID)
}

// DriverAppStrings returns the application strings to register in the driver
// profile: the package family name derived from AppUserModelID first (when the
// game has a UWP identity), then the manifest's DriverApps overrides, in order
// and with duplicates removed the way the driver compares them. Nil when
// SkipDriver is set — the manifest's way to opt a game out of the driver step
// entirely without having to omit its app_user_model_id — or when there is
// nothing to register.
func (g Game) DriverAppStrings() []string {
	if g.SkipDriver {
		return nil
	}
	apps := make([]string, 0, len(g.DriverApps)+1)
	apps = appendUniqueApp(apps, g.UWPPackageFamilyName())
	for _, app := range g.DriverApps {
		apps = appendUniqueApp(apps, app)
	}
	return apps
}

// appendUniqueApp appends app unless the list already holds the same
// application. The comparison is strings.EqualFold — exactly what NvAPI_DRS
// uses — and not strings.ToLower: the two disagree on U+0130 (İ) with U+0049 (I)
// and U+0069 (i), which ToLower collapses into one key although the driver keeps
// them distinct, silently dropping an entry that has to be registered.
func appendUniqueApp(apps []string, app string) []string {
	if app == "" || slices.ContainsFunc(apps, func(existing string) bool {
		return strings.EqualFold(existing, app)
	}) {
		return apps
	}
	return append(apps, app)
}

// VersionKeys returns the requested versions as a set of lowercased, trimmed
// names, so manifest lookups ignore case and stray whitespace. The set is
// built on first use and cached; Versions must not be mutated afterwards.
func (g *Game) VersionKeys() *set.Set[string] {
	if g.versionKeys == nil {
		g.versionKeys = set.New[string](len(g.Versions))
		for _, v := range g.Versions {
			g.versionKeys.Add(strings.ToLower(strings.TrimSpace(v)))
		}
	}
	return g.versionKeys
}

// LoadFromBytes loads the games database from raw JSON bytes.
func LoadFromBytes(data []byte) (*GameDB, error) {
	var db GameDB
	if err := json.Unmarshal(data, &db); err != nil {
		return nil, fmt.Errorf("parsing games.json: %w", err)
	}
	if db.Version < 1 {
		return nil, fmt.Errorf("invalid games database version: %d", db.Version)
	}
	if len(db.Games) == 0 {
		return nil, fmt.Errorf("games database contains no games")
	}
	for i, g := range db.Games {
		// json/v2 decodes a JSON null entry into a nil pointer; every later
		// step dereferences the games, so reject it here instead of panicking.
		if g == nil {
			return nil, fmt.Errorf("game %d is null", i)
		}
		if err := validateGame(g); err != nil {
			return nil, err
		}
	}
	return &db, nil
}

// validateGame rejects the manifest entries the patch flow cannot honor: an
// empty versions list, a wildcard sharing the list, a uwp request without an
// AppUserModelID, an empty driver_app entry, and driver strings the NVAPI
// fields cannot hold.
func validateGame(g *Game) error {
	if len(g.Versions) == 0 {
		return fmt.Errorf("game %q has no versions", g.Fingerprint)
	}
	keys := g.VersionKeys()
	if keys.Contains(AllVersions) && len(g.Versions) != 1 {
		return fmt.Errorf("game %q: %q must be the only version", g.Fingerprint, AllVersions)
	}
	if keys.Contains(UWP) && g.AppUserModelID == "" {
		return fmt.Errorf("game %q has %q but doesn't have %q", g.Fingerprint, UWP, "AppUserModelID")
	}
	if err := validateDriverApps(g); err != nil {
		return err
	}
	if err := validateDriverStrings(g); err != nil {
		return err
	}
	return validateElementNames(g)
}

// validateElementNames rejects the overrides keys and remove entries XML cannot
// hold as element names.
func validateElementNames(g *Game) error {
	for _, key := range slices.Sorted(maps.Keys(g.Overrides)) {
		if !ValidElementName(key) {
			return fmt.Errorf("game %q: overrides key %q is not a valid XML element name", g.Fingerprint, key)
		}
	}
	for _, name := range g.Remove {
		if !ValidElementName(name) {
			return fmt.Errorf("game %q: remove entry %q is not a valid XML element name", g.Fingerprint, name)
		}
	}
	return nil
}

// validateDriverApps rejects an empty entry and any entry the NVAPI application
// name cannot hold.
func validateDriverApps(g *Game) error {
	for _, app := range g.DriverApps {
		if app == "" {
			return fmt.Errorf("game %q: driver_app has an empty entry", g.Fingerprint)
		}
		if len(utf16.Encode([]rune(app))) > MaxDriverString {
			return fmt.Errorf("game %q: driver_app exceeds %d characters", g.Fingerprint, MaxDriverString)
		}
	}
	return nil
}

// validateDriverStrings rejects the single-string driver fields the NVAPI
// unicode fields cannot hold.
func validateDriverStrings(g *Game) error {
	for _, f := range []struct{ name, value string }{
		{"driver_profile", g.DriverProfile},
		{"app_user_model_id", g.AppUserModelID},
	} {
		if len(utf16.Encode([]rune(f.value))) > MaxDriverString {
			return fmt.Errorf("game %q: %s exceeds %d characters", g.Fingerprint, f.name, MaxDriverString)
		}
	}
	return nil
}

var reElementName = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_.-]*\z`)

// ValidElementName reports whether name can be written as an XML element name.
func ValidElementName(name string) bool {
	return reElementName.MatchString(name)
}

// LoadFromPath loads games.json from a file path.
func LoadFromPath(path string) (*GameDB, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", path, err)
	}
	return LoadFromBytes(data)
}

// SaveToPath saves the game database to a file.
func SaveToPath(db *GameDB, path string) error {
	if err := fsutil.MkdirAllSync(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("creating cache directory: %w", err)
	}
	data, err := json.Marshal(db, jsontext.WithIndent("  "), json.Deterministic(true))
	if err != nil {
		return fmt.Errorf("marshaling games.json: %w", err)
	}
	if err := fsutil.WriteFileAtomic(path, data, 0o644); err != nil {
		return fmt.Errorf("writing %s: %w", path, err)
	}
	return nil
}

// ResolveGames loads the games database with priority: remote > cache > bundled.
// An empty cacheDir disables the cache layer (no read, no write).
func ResolveGames(cacheDir string, bundledData []byte, remoteData []byte, writeCache bool) (*GameDB, error) {
	if remoteData != nil {
		gameDB, err := LoadFromBytes(remoteData)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: remote games.json parse failed: %v\n", err)
		} else {
			if cacheDir != "" && writeCache {
				if err := SaveToPath(gameDB, filepath.Join(cacheDir, "games.json")); err != nil {
					fmt.Fprintf(os.Stderr, "Warning: could not cache games database: %v\n", err)
				}
			}
			return gameDB, nil
		}
	}
	if cacheDir != "" {
		cachePath := filepath.Join(cacheDir, "games.json")
		if gameDB, err := LoadFromPath(cachePath); err == nil {
			return gameDB, nil
		} else if !errors.Is(err, fs.ErrNotExist) {
			fmt.Fprintf(os.Stderr, "Warning: cached games.json unusable, ignoring: %v\n", err)
		}
	}
	return LoadFromBytes(bundledData)
}
