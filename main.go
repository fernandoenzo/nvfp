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
	dryRun     bool
	listOnly   bool
	gameFilter string
)

func main() {
	rootCmd := &cobra.Command{
		Use:   "nvidia-uwp-patch",
		Short: "Patch NVIDIA App fingerprint.db to add UWP game profiles",
		RunE:  run,
	}

	rootCmd.Flags().BoolVar(&dryRun, "dry-run", false, "Show changes without writing files")
	rootCmd.Flags().BoolVar(&listOnly, "list", false, "List games in the database")
	rootCmd.Flags().StringVar(&gameFilter, "game", "", "Patch only a specific game (by fingerprint)")

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
		return listGames(gameDB)
	}

	nvidiaDir, err := findNvidiaDir()
	if err != nil {
		return fmt.Errorf("finding NVIDIA directory: %w", err)
	}
	dbPaths := findFingerprintDBs(nvidiaDir)
	if len(dbPaths) == 0 {
		return fmt.Errorf("no fingerprint.db files found under %s", nvidiaDir)
	}

	return patchAllDBs(gameDB, dbPaths)
}

// patchAllDBs patches each fingerprint.db path and reports results.
// Returns an error if every DB failed; partial failures are reported to stderr.
func patchAllDBs(gameDB *db.GameDB, dbPaths []string) error {
	var errs []string
	anyModified := false
	for _, dbPath := range dbPaths {
		modified, err := patchDB(gameDB, dbPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error processing %s: %v\n", dbPath, err)
			errs = append(errs, dbPath)
			continue
		}
		if modified {
			anyModified = true
		}
	}
	if !anyModified {
		fmt.Println("No changes made.")
	}
	if len(errs) == len(dbPaths) && len(dbPaths) > 0 {
		return fmt.Errorf("all %d fingerprint databases failed", len(dbPaths))
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

func findNvidiaDir() (string, error) {
	envVars := []string{"PROGRAMDATA", "PROGRAMFILES", "PROGRAMFILES(X86)"}
	for _, envVar := range envVars {
		val := os.Getenv(envVar)
		if val == "" {
			continue
		}
		candidate := filepath.Join(val, "NVIDIA Corporation", "NvBackend")
		if found := checkNvidiaDir(candidate); found != "" {
			return found, nil
		}
	}
	return "", fmt.Errorf("NVIDIA App directory not found")
}

// checkNvidiaDir returns the directory path if it exists and is a directory, otherwise "".
func checkNvidiaDir(candidate string) string {
	info, err := os.Stat(candidate)
	if err != nil {
		return ""
	}
	if info.IsDir() {
		return candidate
	}
	return ""
}

// findDAOFingerprintDBs discovers fingerprint.db files under the DAO directory.
func findDAOFingerprintDBs(nvidiaDir string) []string {
	daoDir := filepath.Join(nvidiaDir, "DAO")
	entries, err := os.ReadDir(daoDir)
	if err != nil {
		if !os.IsNotExist(err) {
			fmt.Fprintf(os.Stderr, "  Warning: could not read DAO directory: %v\n", err)
		}
		return nil
	}
	var results []string
	for _, entry := range entries {
		if entry.IsDir() {
			dbPath := filepath.Join(daoDir, entry.Name(), "fingerprint.db")
			if _, err := os.Stat(dbPath); err == nil {
				results = append(results, dbPath)
			}
		}
	}
	return results
}

func findFingerprintDBs(nvidiaDir string) []string {
	var results []string

	// Ontology path
	ontologyPath := filepath.Join(nvidiaDir, "ApplicationOntology", "data", "fingerprint.db")
	if _, err := os.Stat(ontologyPath); err == nil {
		results = append(results, ontologyPath)
	}

	// DAO paths
	results = append(results, findDAOFingerprintDBs(nvidiaDir)...)

	return results
}

func patchDB(gameDB *db.GameDB, dbPath string) (bool, error) {
	fmt.Printf("Processing: %s\n", dbPath)

	profileDB, err := nvidia.ParseProfileDB(dbPath)
	if err != nil {
		return false, fmt.Errorf("parsing %s: %w", dbPath, err)
	}

	games := filterGames(gameDB)
	modified := applyPatches(profileDB, games)

	if !modified || dryRun {
		if dryRun && modified {
			fmt.Println("  (dry-run: no changes written)")
		}
		return modified, nil
	}

	return writePatch(profileDB, dbPath)
}

// filterGames returns the games list, optionally filtered by --game flag.
func filterGames(gameDB *db.GameDB) []db.Game {
	if gameFilter == "" {
		return gameDB.Games
	}
	for _, g := range gameDB.Games {
		if g.Fingerprint == gameFilter {
			return []db.Game{g}
		}
	}
	fmt.Printf("  Game %q not found in games database\n", gameFilter)
	return nil
}

// applyPatches patches all games and returns whether any were modified.
func applyPatches(profileDB *nvidia.ProfileDB, games []db.Game) bool {
	modified := false
	for _, game := range games {
		result := nvidia.PatchGame(profileDB, game.Fingerprint, game.AppID, game.Overrides, game.Remove)
		switch result.Status {
		case nvidia.StatusPatched:
			modified = true
			fmt.Printf("  ✓ %s\n", result.Message)
		case nvidia.StatusAlreadyUWP:
			fmt.Printf("  ⊘ %s\n", result.Message)
		case nvidia.StatusNotFound, nvidia.StatusNoSource:
			fmt.Printf("  ✗ %s\n", result.Message)
		default:
			fmt.Printf("  ? %s (%s)\n", result.Message, result.Status)
		}
	}
	return modified
}

// writePatch backs up, writes the patched database, and updates metadata.
func writePatch(profileDB *nvidia.ProfileDB, dbPath string) (bool, error) {
	if err := nvidia.BackupFile(dbPath); err != nil {
		return false, fmt.Errorf("backing up %s: %w", dbPath, err)
	}
	if err := nvidia.WriteProfileDB(profileDB, dbPath); err != nil {
		return false, fmt.Errorf("writing %s: %w", dbPath, err)
	}

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
