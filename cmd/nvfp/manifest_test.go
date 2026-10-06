package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveGamesCustomFile(t *testing.T) {
	original := gamesJSONPath
	defer func() { gamesJSONPath = original }()

	custom := `{"version":1,"games":[{"fingerprint":"custom_game","app_user_model_id":"Pkg_custom!App","versions":["uwp"]}]}`
	customPath := filepath.Join(t.TempDir(), "custom.json")
	if err := os.WriteFile(customPath, []byte(custom), 0o644); err != nil {
		t.Fatalf("writing custom games.json: %v", err)
	}

	gamesJSONPath = customPath
	db, err := resolveGames()
	if err != nil {
		t.Fatalf("resolveGames() with --games-json error: %v", err)
	}
	if len(db.Games) != 1 || db.Games[0].Fingerprint != "custom_game" {
		t.Errorf("resolveGames() with --games-json = %+v, want custom_game", db.Games)
	}
}

func TestResolveGamesCustomFileInvalid(t *testing.T) {
	original := gamesJSONPath
	defer func() { gamesJSONPath = original }()

	customPath := filepath.Join(t.TempDir(), "custom.json")
	if err := os.WriteFile(customPath, []byte(`not json`), 0o644); err != nil {
		t.Fatalf("writing custom games.json: %v", err)
	}

	gamesJSONPath = customPath
	if _, err := resolveGames(); err == nil {
		t.Error("resolveGames() with invalid --games-json should fail, got nil")
	}
}
