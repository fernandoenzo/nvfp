package db

import (
	"encoding/json/jsontext"
	"encoding/json/v2"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
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
	DriverApp      string            `json:"driver_app,omitempty"`
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

// DriverAppString returns the string registered in the driver profile: DriverApp
// when set, otherwise the package family name derived from AppUserModelID.
// Empty when the game has no UWP identity and no explicit override, or when
// SkipDriver is set — the manifest's way to opt a game out of the driver step
// entirely without having to omit its app_user_model_id.
func (g Game) DriverAppString() string {
	if g.SkipDriver {
		return ""
	}
	if g.DriverApp != "" {
		return g.DriverApp
	}
	return g.UWPPackageFamilyName()
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
	for _, g := range db.Games {
		if err := validateGame(g); err != nil {
			return nil, err
		}
	}
	return &db, nil
}

// validateGame rejects the manifest entries the patch flow cannot honor: an
// empty versions list, a wildcard sharing the list, a uwp request without an
// AppUserModelID, and driver strings the NVAPI fields cannot hold.
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
	for _, f := range []struct{ name, value string }{
		{"driver_app", g.DriverApp},
		{"driver_profile", g.DriverProfile},
		{"app_user_model_id", g.AppUserModelID},
	} {
		if len(utf16.Encode([]rune(f.value))) > MaxDriverString {
			return fmt.Errorf("game %q: %s exceeds %d characters", g.Fingerprint, f.name, MaxDriverString)
		}
	}
	return nil
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
func ResolveGames(cacheDir string, bundledData []byte, remoteData []byte) (*GameDB, error) {
	if remoteData != nil {
		gameDB, err := LoadFromBytes(remoteData)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: remote games.json parse failed: %v\n", err)
		} else {
			if cacheDir != "" {
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
