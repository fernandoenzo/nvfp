package db

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// GameDB represents the games.json database.
type GameDB struct {
	Version int    `json:"version"`
	Games   []Game `json:"games"`
}

// Game represents a single game entry in games.json.
type Game struct {
	Fingerprint string            `json:"fingerprint"`
	AppID       string            `json:"app_id"`
	Overrides   map[string]string `json:"overrides,omitempty"`
	Remove      []string          `json:"remove,omitempty"`
}

// PackageFamilyName extracts the package family name from a UWP app ID
// by taking everything before the first '!'. If no '!' is present,
// the full app ID is returned.
func PackageFamilyName(appID string) string {
	idx := strings.Index(appID, "!")
	if idx < 0 {
		return appID
	}
	return appID[:idx]
}

// UWPPackageFamilyName derives the package family name from the app_id
// by taking everything before the first '!'.
func (g Game) UWPPackageFamilyName() string {
	return PackageFamilyName(g.AppID)
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
	return &db, nil
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
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("creating cache directory: %w", err)
	}
	data, err := json.MarshalIndent(db, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling games.json: %w", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("writing %s: %w", path, err)
	}
	return nil
}

// ResolveGames loads the games database with priority: remote > cache > bundled.
func ResolveGames(cacheDir string, bundledData []byte, remoteData []byte) (*GameDB, error) {
	// Try remote first
	if remoteData != nil {
		db, err := LoadFromBytes(remoteData)
		if err == nil {
			cachePath := filepath.Join(cacheDir, "games.json")
			if err := SaveToPath(db, cachePath); err != nil {
				fmt.Fprintf(os.Stderr, "Warning: could not cache games database: %v\n", err)
			}
			return db, nil
		}
		fmt.Fprintf(os.Stderr, "Warning: remote games.json parse failed: %v\n", err)
	}
	cachePath := filepath.Join(cacheDir, "games.json")
	if _, err := os.Stat(cachePath); err == nil {
		db, err := LoadFromPath(cachePath)
		if err == nil {
			return db, nil
		}
	}

	// Bundled fallback
	return LoadFromBytes(bundledData)
}