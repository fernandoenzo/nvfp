package main

import (
	_ "embed"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"

	"github.com/fernandoenzo/nvidia-uwp-patch/internal/db"
	"github.com/fernandoenzo/nvidia-uwp-patch/internal/nvidia"
	"github.com/fernandoenzo/nvidia-uwp-patch/internal/update"
	"github.com/spf13/cobra"
)

//go:embed games.json
var bundledGames []byte

var (
	dryRun        bool
	listOnly      bool
	gameFilter    string
	gamesJSONPath string
)

func main() {
	rootCmd := &cobra.Command{
		Use:   "nvidia-uwp-patch",
		Short: "Patch NVIDIA App fingerprint.db to add UWP game profiles",
		Args:  cobra.NoArgs,
		RunE:  run,
	}

	rootCmd.Flags().BoolVar(&dryRun, "dry-run", false, "Show changes without writing files")
	rootCmd.Flags().BoolVar(&listOnly, "list", false, "List games in the database")
	rootCmd.Flags().StringVar(&gameFilter, "game", "", "Patch only a specific game (by fingerprint)")
	rootCmd.Flags().StringVar(&gamesJSONPath, "games-json", "", "Use a local games.json instead of the remote manifest")

	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func run(cmd *cobra.Command, args []string) error {
	gameDB, err := resolveGames()
	if err != nil {
		return fmt.Errorf("loading games database: %w", err)
	}
	if listOnly {
		listGames(gameDB)
		return nil
	}

	dbPath, err := findFingerprintDB()
	if err != nil {
		return err
	}

	_, err = patchDB(gameDB, dbPath)
	return err
}

func resolveGames() (*db.GameDB, error) {
	if gamesJSONPath != "" {
		// Explicit user request takes priority over remote and cache.
		// Fail loudly instead of silently falling back to the remote list.
		gameDB, err := db.LoadFromPath(gamesJSONPath)
		if err != nil {
			return nil, fmt.Errorf("loading custom games.json: %w", err)
		}
		return gameDB, nil
	}

	// The remote fetch must not depend on the cache dir being available.
	cacheDir, err := getCacheDir()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: cache unavailable, continuing without it: %v\n", err)
		cacheDir = ""
	}

	data, err := update.FetchGamesJSON()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: could not download remote games.json: %v\n", err)
	}

	return db.ResolveGames(cacheDir, bundledGames, data)
}

func getCacheDir() (string, error) {
	// On Windows: %LOCALAPPDATA%\nvidia-uwp-patch (cached data stays on the machine)
	localAppData := os.Getenv("LOCALAPPDATA")
	if localAppData != "" {
		return filepath.Join(localAppData, "nvidia-uwp-patch"), nil
	}
	// Fallback for non-Windows (testing)
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".cache", "nvidia-uwp-patch"), nil
}

// findFingerprintDB returns the path of the working fingerprint.db used by the
// NVIDIA App ontology engine.
func findFingerprintDB() (string, error) {
	localAppData := os.Getenv("LOCALAPPDATA")
	if localAppData == "" {
		return "", fmt.Errorf("LOCALAPPDATA not set")
	}
	path := filepath.Join(localAppData, "NVIDIA Corporation", "NVIDIA App",
		"NvBackend", "ApplicationOntology", "data", "fingerprint.db")
	if _, err := os.Stat(path); err != nil {
		return "", fmt.Errorf("fingerprint.db not found (is NVIDIA App installed?): %w", err)
	}
	return path, nil
}

func patchDB(gameDB *db.GameDB, dbPath string) (bool, error) {
	fmt.Printf("Processing: %s\n", dbPath)

	fdb, err := nvidia.ParseFingerprintDB(dbPath)
	if err != nil {
		return false, fmt.Errorf("parsing %s: %w", dbPath, err)
	}

	games, err := filterGames(gameDB)
	if err != nil {
		return false, err
	}
	modified := applyPatches(fdb, games)

	if dryRun {
		if modified {
			fmt.Println("  (dry-run: no changes written)")
		}
		return modified, nil
	}
	if !modified {
		return false, nil
	}

	return true, writePatch(fdb, dbPath)
}

// filterGames returns the games list, optionally filtered by --game flag.
// A non-empty filter that matches nothing is an error, not a silent success.
func filterGames(gameDB *db.GameDB) ([]db.Game, error) {
	if gameFilter == "" {
		return gameDB.Games, nil
	}
	for _, g := range gameDB.Games {
		if g.Fingerprint == gameFilter {
			return []db.Game{g}, nil
		}
	}
	return nil, fmt.Errorf("game %q not found in games database", gameFilter)
}

// applyPatches patches all games and returns whether any were modified.
func applyPatches(fdb *nvidia.FingerprintDB, games []db.Game) bool {
	modified := false
	for i := range games {
		result := nvidia.PatchGame(fdb, &games[i])
		switch result.Status {
		case nvidia.StatusPatched:
			modified = true
			fmt.Printf("  ✓ %s\n", result.Message)
		case nvidia.StatusAlreadyPresent:
			fmt.Printf("  ⊘ %s\n", result.Message)
		case nvidia.StatusNotFound, nvidia.StatusNoSource, nvidia.StatusVersionNotFound:
			fmt.Printf("  ✗ %s\n", result.Message)
		}
	}
	return modified
}

// writePatch backs up and writes the patched database.
func writePatch(fdb *nvidia.FingerprintDB, dbPath string) error {
	if err := nvidia.BackupFile(dbPath); err != nil {
		return fmt.Errorf("backing up %s: %w", dbPath, err)
	}
	if err := nvidia.WriteFingerprintDB(fdb, dbPath); err != nil {
		return fmt.Errorf("writing %s: %w", dbPath, err)
	}
	return nil
}

func listGames(gameDB *db.GameDB) {
	fmt.Printf("Games database version: %d\n", gameDB.Version)
	fmt.Printf("Total games: %d\n\n", len(gameDB.Games))

	for _, game := range gameDB.Games {
		fmt.Printf("  %s\n", game.Fingerprint)
		fmt.Printf("    AppUserModelId: %s\n", game.AppUserModelID)
		fmt.Printf("    UWPPackageFamilyName: %s\n", game.UWPPackageFamilyName())
		if len(game.Overrides) > 0 {
			fmt.Println("    Overrides:")
			for _, k := range slices.Sorted(maps.Keys(game.Overrides)) {
				fmt.Printf("      %s: %s\n", k, game.Overrides[k])
			}
		}
		if len(game.Remove) > 0 {
			fmt.Printf("    Remove: %v\n", slices.Sorted(slices.Values(game.Remove)))
		}
	}
}
