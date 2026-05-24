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

// UWPPackageFamilyName derives the package family name from the app_id
// by taking everything before the first '!'.
func (g Game) UWPPackageFamilyName() string {
	idx := strings.Index(g.AppID, "!")
	if idx < 0 {
		return g.AppID
	}
	return g.AppID[:idx]
}

// LoadFromBytes loads the games database from raw JSON bytes.
func LoadFromBytes(data []byte) (*GameDB, error) {
	var db GameDB
	if err := json.Unmarshal(data, &db); err != nil {
		return nil, fmt.Errorf("parsing games.json: %w", err)
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
			_ = SaveToPath(db, cachePath)
			return db, nil
		}
	}

	// Try cache
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
