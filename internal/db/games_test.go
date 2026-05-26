package db

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadFromBytes(t *testing.T) {
	data := []byte(`{
		"version": 1,
		"games": [
			{
				"fingerprint": "test_game",
				"app_id": "Pkg_abc!AppX"
			},
			{
				"fingerprint": "weird_game",
				"app_id": "SomePkg_abc123!AppCustom",
				"overrides": {
					"DriverProfile": "custom_exe.exe"
				},
				"remove": ["WhisperModePopsFactor"]
			}
		]
	}`)

	db, err := LoadFromBytes(data)
	if err != nil {
		t.Fatalf("LoadFromBytes failed: %v", err)
	}

	if db.Version != 1 {
		t.Errorf("expected version 1, got %d", db.Version)
	}
	if len(db.Games) != 2 {
		t.Fatalf("expected 2 games, got %d", len(db.Games))
	}
	if db.Games[0].Fingerprint != "test_game" {
		t.Errorf("expected fingerprint test_game, got %s", db.Games[0].Fingerprint)
	}
	if db.Games[0].AppID != "Pkg_abc!AppX" {
		t.Errorf("expected app_id Pkg_abc!AppX, got %s", db.Games[0].AppID)
	}
	if db.Games[1].Overrides["DriverProfile"] != "custom_exe.exe" {
		t.Errorf("expected override DriverProfile=custom_exe.exe")
	}
	if len(db.Games[1].Remove) != 1 || db.Games[1].Remove[0] != "WhisperModePopsFactor" {
		t.Errorf("expected remove [WhisperModePopsFactor]")
	}
}

func TestUWPPackageFamilyName(t *testing.T) {
	tests := []struct {
		appID    string
		expected string
	}{
		{"39EA002F.EXED1_n746a19ndrrjg!AppFINALFANTASYVIIREMAKEShipping", "39EA002F.EXED1_n746a19ndrrjg"},
		{"Pkg_abc!AppX", "Pkg_abc"},
		{"NoBangHere", "NoBangHere"},
	}

	for _, tt := range tests {
		g := Game{AppID: tt.appID}
		result := g.UWPPackageFamilyName()
		if result != tt.expected {
			t.Errorf("UWPPackageFamilyName(%q) = %q, want %q", tt.appID, result, tt.expected)
		}
	}
}

func TestPackageFamilyName(t *testing.T) {
	tests := []struct {
		appID    string
		expected string
	}{
		{"39EA002F.EXED1_n746a19ndrrjg!AppFINALFANTASYVIIREMAKEShipping", "39EA002F.EXED1_n746a19ndrrjg"},
		{"Pkg_abc!AppX", "Pkg_abc"},
		{"NoBangHere", "NoBangHere"},
		{"", ""},
	}
	for _, tt := range tests {
		result := PackageFamilyName(tt.appID)
		if result != tt.expected {
			t.Errorf("PackageFamilyName(%q) = %q, want %q", tt.appID, result, tt.expected)
		}
	}
}

func TestLoadFromBytesValidation(t *testing.T) {
	t.Run("zero version rejected", func(t *testing.T) {
		data := []byte(`{"version":0,"games":[{"fingerprint":"x","app_id":"P!A"}]}`)
		_, err := LoadFromBytes(data)
		if err == nil {
			t.Fatal("expected error for version 0")
		}
	})
	t.Run("empty games rejected", func(t *testing.T) {
		data := []byte(`{"version":1,"games":[]}`)
		_, err := LoadFromBytes(data)
		if err == nil {
			t.Fatal("expected error for empty games")
		}
	})
	t.Run("negative version rejected", func(t *testing.T) {
		data := []byte(`{"version":-1,"games":[{"fingerprint":"x","app_id":"P!A"}]}`)
		_, err := LoadFromBytes(data)
		if err == nil {
			t.Fatal("expected error for negative version")
		}
	})
}

func TestSaveAndLoad(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "games.json")

	db := &GameDB{
		Version: 1,
		Games: []Game{
			{Fingerprint: "test", AppID: "Pkg!App"},
		},
	}

	if err := SaveToPath(db, path); err != nil {
		t.Fatalf("SaveToPath failed: %v", err)
	}

	loaded, err := LoadFromPath(path)
	if err != nil {
		t.Fatalf("LoadFromPath failed: %v", err)
	}

	if loaded.Version != db.Version {
		t.Errorf("version mismatch: got %d, want %d", loaded.Version, db.Version)
	}
	if len(loaded.Games) != len(db.Games) {
		t.Errorf("games count mismatch: got %d, want %d", len(loaded.Games), len(db.Games))
	}
}

func TestResolveGamesBundled(t *testing.T) {
	data := []byte(`{"version":1,"games":[{"fingerprint":"test","app_id":"Pkg!App"}]}`)

	db, err := ResolveGames(t.TempDir(), data, nil)
	if err != nil {
		t.Fatalf("ResolveGames failed: %v", err)
	}
	if len(db.Games) != 1 {
		t.Errorf("expected 1 game, got %d", len(db.Games))
	}
}

func TestResolveGamesRemote(t *testing.T) {
	bundled := []byte(`{"version":1,"games":[{"fingerprint":"bundled","app_id":"Pkg!App"}]}`)
	remote := []byte(`{"version":2,"games":[{"fingerprint":"remote","app_id":"Pkg!App2"}]}`)

	cacheDir := t.TempDir()
	db, err := ResolveGames(cacheDir, bundled, remote)
	if err != nil {
		t.Fatalf("ResolveGames failed: %v", err)
	}
	if db.Version != 2 {
		t.Errorf("expected remote version 2, got %d", db.Version)
	}
	if db.Games[0].Fingerprint != "remote" {
		t.Errorf("expected remote game, got %s", db.Games[0].Fingerprint)
	}

	// Check cache was written
	cachePath := filepath.Join(cacheDir, "games.json")
	if _, err := os.Stat(cachePath); err != nil {
		t.Errorf("cache file not created: %v", err)
	}
}

func TestResolveGamesFallbackToBundled(t *testing.T) {
	bundled := []byte(`{"version":1,"games":[{"fingerprint":"bundled","app_id":"Pkg!App"}]}`)
	cacheDir := t.TempDir()

	// No remote data, no cache → should use bundled
	db, err := ResolveGames(cacheDir, bundled, nil)
	if err != nil {
		t.Fatalf("ResolveGames failed: %v", err)
	}
	if db.Games[0].Fingerprint != "bundled" {
		t.Errorf("expected bundled game, got %s", db.Games[0].Fingerprint)
	}
}

func TestResolveGamesRemoteParseError(t *testing.T) {
	bundled := []byte(`{"version":1,"games":[{"fingerprint":"bundled","app_id":"Pkg!App"}]}`)
	remote := []byte(`not json`)
	cacheDir := t.TempDir()
	db, err := ResolveGames(cacheDir, bundled, remote)
	if err != nil {
		t.Fatalf("ResolveGames should fall back to bundled on remote parse error: %v", err)
	}
	if db.Games[0].Fingerprint != "bundled" {
		t.Errorf("expected bundled fallback, got %s", db.Games[0].Fingerprint)
	}
}