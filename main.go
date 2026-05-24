package main

import (
	_ "embed"
	"fmt"
	"os"
	"path/filepath"

	"github.com/fernandoenzo/nvidia-uwp-patch/internal/db"
	"github.com/fernandoenzo/nvidia-uwp-patch/internal/nvidia"
	"github.com/fernandoenzo/nvidia-uwp-patch/internal/update"
	"github.com/spf13/cobra"
)

//go:embed games.json
var bundledGames []byte

var (
	offline bool
	dryRun  bool
	listOnly bool
	gameFilter string
)

func main() {
	rootCmd := &cobra.Command{
		Use:   "nvidia-uwp-patch",
		Short: "Patch NVIDIA App fingerprint.db to add UWP game profiles",
		RunE:  run,
	}

	rootCmd.Flags().BoolVar(&offline, "offline", false, "Don't download remote games.json")
	rootCmd.Flags().BoolVar(&dryRun, "dry-run", false, "Show changes without writing files")
	rootCmd.Flags().BoolVar(&listOnly, "list", false, "List games in the database")
	rootCmd.Flags().StringVar(&gameFilter, "game", "", "Patch only a specific game (by fingerprint)")

	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func run(cmd *cobra.Command, args []string) error {
	// Resolve games database
	gameDB, err := resolveGames()
	if err != nil {
		return fmt.Errorf("loading games database: %w", err)
	}

	// --list mode
	if listOnly {
		return listGames(gameDB)
	}

	// Find NVIDIA fingerprint.db files
	nvidiaDir, err := findNvidiaDir()
	if err != nil {
		return fmt.Errorf("finding NVIDIA directory: %w", err)
	}

	dbPaths, err := findFingerprintDBs(nvidiaDir)
	if err != nil {
		return fmt.Errorf("finding fingerprint databases: %w", err)
	}

	if len(dbPaths) == 0 {
		return fmt.Errorf("no fingerprint.db files found under %s", nvidiaDir)
	}

	// Patch each fingerprint.db
	anyModified := false
	for _, dbPath := range dbPaths {
		modified, err := patchDB(gameDB, dbPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error processing %s: %v\n", dbPath, err)
			continue
		}
		if modified {
			anyModified = true
		}
	}

	if !anyModified {
		fmt.Println("No changes made.")
	}

	return nil
}

func resolveGames() (*db.GameDB, error) {
	cacheDir, err := getCacheDir()
	if err != nil {
		// Fall back to bundled only
		return db.LoadFromBytes(bundledGames)
	}

	// Try remote
	var remoteData []byte
	if !offline {
		data, err := update.FetchGamesJSON()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: could not download remote games.json: %v\n", err)
		} else {
			remoteData = data
			// Cache it
			remoteDB, err := db.LoadFromBytes(remoteData)
			if err == nil {
				_ = db.SaveToPath(remoteDB, filepath.Join(cacheDir, "games.json"))
			}
		}
	}

	return db.ResolveGames(cacheDir, offline, bundledGames, remoteData)
}

func getCacheDir() (string, error) {
	// On Windows: %APPDATA%\nvidia-uwp-patch
	appData := os.Getenv("APPDATA")
	if appData != "" {
		return filepath.Join(appData, "nvidia-uwp-patch"), nil
	}
	// Fallback for non-Windows (testing)
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".cache", "nvidia-uwp-patch"), nil
}

func findNvidiaDir() (string, error) {
	// Check common NVIDIA App installation paths
	// On Windows: C:\ProgramData\NVIDIA Corporation\NvBackend
	programData := os.Getenv("PROGRAMDATA")
	if programData != "" {
		candidate := filepath.Join(programData, "NVIDIA Corporation", "NvBackend")
		if info, err := os.Stat(candidate); err == nil && info.IsDir() {
			return candidate, nil
		}
	}

	// Check PROGRAMFILES
	progFiles := os.Getenv("PROGRAMFILES")
	if progFiles != "" {
		candidate := filepath.Join(progFiles, "NVIDIA Corporation", "NvBackend")
		if info, err := os.Stat(candidate); err == nil && info.IsDir() {
			return candidate, nil
		}
	}

	// Check PROGRAMFILES(X86)
	progFiles86 := os.Getenv("PROGRAMFILES(X86)")
	if progFiles86 != "" {
		candidate := filepath.Join(progFiles86, "NVIDIA Corporation", "NvBackend")
		if info, err := os.Stat(candidate); err == nil && info.IsDir() {
			return candidate, nil
		}
	}

	return "", fmt.Errorf("NVIDIA App directory not found")
}

func findFingerprintDBs(nvidiaDir string) ([]string, error) {
	var results []string

	// Ontology path
	ontologyPath := filepath.Join(nvidiaDir, "ApplicationOntology", "data", "fingerprint.db")
	if _, err := os.Stat(ontologyPath); err == nil {
		results = append(results, ontologyPath)
	}

	// DAO paths
	daoDir := filepath.Join(nvidiaDir, "DAO")
	entries, err := os.ReadDir(daoDir)
	if err == nil {
		for _, entry := range entries {
			if entry.IsDir() {
				dbPath := filepath.Join(daoDir, entry.Name(), "fingerprint.db")
				if _, err := os.Stat(dbPath); err == nil {
					results = append(results, dbPath)
				}
			}
		}
	}

	return results, nil
}

func patchDB(gameDB *db.GameDB, dbPath string) (bool, error) {
	fmt.Printf("Processing: %s\n", dbPath)

	// Parse the fingerprint database
	profileDB, err := nvidia.ParseProfileDB(dbPath)
	if err != nil {
		return false, fmt.Errorf("parsing %s: %w", dbPath, err)
	}

	// Track which games we patched
	modified := false

	games := gameDB.Games
	if gameFilter != "" {
		var filtered []db.Game
		for _, g := range games {
			if g.Fingerprint == gameFilter {
				filtered = append(filtered, g)
				break
			}
		}
		if len(filtered) == 0 {
			fmt.Printf("  Game %q not found in games database\n", gameFilter)
			return false, nil
		}
		games = filtered
	}

	for _, game := range games {
		result := nvidia.PatchGame(profileDB, game.Fingerprint, game.AppID, game.Overrides, game.Remove)

		switch result.Status {
		case "patched":
			modified = true
			fmt.Printf("  ✓ %s\n", result.Message)
		case "already_uwp":
			fmt.Printf("  ⊘ %s\n", result.Message)
		case "not_found":
			fmt.Printf("  ✗ %s\n", result.Message)
		case "no_source":
			fmt.Printf("  ✗ %s\n", result.Message)
		}
	}

	if !modified || dryRun {
		if dryRun && modified {
			fmt.Println("  (dry-run: no changes written)")
		}
		return modified, nil
	}

	// Backup before writing
	if err := nvidia.BackupFile(dbPath); err != nil {
		return false, fmt.Errorf("backing up %s: %w", dbPath, err)
	}

	// Write patched database
	if err := nvidia.WriteProfileDB(profileDB, dbPath); err != nil {
		return false, fmt.Errorf("writing %s: %w", dbPath, err)
	}

	// Update metadata.json SHA256 if it exists
	dir := filepath.Dir(dbPath)
	metadataPath := filepath.Join(dir, "metadata.json")
	if _, err := os.Stat(metadataPath); err == nil {
		if err := nvidia.UpdateMetadataSHA256(metadataPath, dbPath); err != nil {
			fmt.Fprintf(os.Stderr, "  Warning: failed to update metadata.json: %v\n", err)
		} else {
			fmt.Printf("  Updated metadata.json SHA256\n")
		}
	}

	return true, nil
}

func listGames(gameDB *db.GameDB) error {
	fmt.Printf("Games database version: %d\n", gameDB.Version)
	fmt.Printf("Total games: %d\n\n", len(gameDB.Games))

	for _, game := range gameDB.Games {
		pkgFamily := game.UWPPackageFamilyName()
		fmt.Printf("  %s\n", game.Fingerprint)
		fmt.Printf("    AppID: %s\n", game.AppID)
		fmt.Printf("    UWPPackageFamilyName: %s\n", pkgFamily)
		if len(game.Overrides) > 0 {
			fmt.Printf("    Overrides: %v\n", game.Overrides)
		}
		if len(game.Remove) > 0 {
			fmt.Printf("    Remove: %v\n", game.Remove)
		}
	}

	return nil
}