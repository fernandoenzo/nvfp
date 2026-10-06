package main

import (
	"fmt"

	"github.com/fernandoenzo/nvfp/internal/db"
	"github.com/fernandoenzo/nvfp/internal/nvidia"
)

// patchReloginHint follows a real fingerprint.db patch: without a new session
// the NVIDIA App keeps showing its cached game list.
const patchReloginHint = "Note: log out of Windows and sign back in (no reboot needed) for the NVIDIA App to show the patched games."

// patchEverything patches fingerprint.db and, unless the step is disabled,
// registers the selected games in the driver profiles.
func patchEverything(gameDB *db.GameDB) error {
	// The elevated relaunch does all the work, driver step included: never ask
	// for UAC twice.
	ensureElevated(!dryRun && !noDriverFlag && hasDriverWork(gameDB))
	// The elevated child gets its own console, and Windows closes it the
	// instant the process exits: the whole run would flash by unread. Hold it
	// open long enough to read the result, but never when the output is being
	// piped (then there is no window to lose and a pause would hang a script).
	if elevatedFlag && hasDriverWork(gameDB) && !stdoutIsPiped() {
		defer pauseBeforeExit()
	}
	dbPath, err := findFingerprintDB()
	if err != nil {
		return err
	}
	fdb, modified, err := patchDB(gameDB, dbPath)
	if err != nil {
		return err
	}
	if !noDriverFlag {
		if err := applyDriverStep(gameDB, fdb); err != nil {
			return err
		}
	}
	// patchDB reports a dry-run modification too, hence the extra guard.
	if modified && !dryRun {
		fmt.Println(patchReloginHint)
	}
	return nil
}

// patchDB patches fingerprint.db and returns the parsed database together with
// whether anything was modified. The parsed copy is what the driver step uses
// to derive profile candidates, so the file is never read twice.
func patchDB(gameDB *db.GameDB, dbPath string) (*nvidia.FingerprintDB, bool, error) {
	fmt.Printf("Processing: %s\n", dbPath)

	fdb, err := nvidia.ParseFingerprintDB(dbPath)
	if err != nil {
		return nil, false, fmt.Errorf("parsing %s: %w", dbPath, err)
	}

	games, err := filterGames(gameDB)
	if err != nil {
		return nil, false, err
	}
	modified := applyPatches(fdb, games)

	if dryRun {
		if modified {
			fmt.Println("  (dry-run: no changes written)")
		}
		return fdb, modified, nil
	}
	if !modified {
		return fdb, false, nil
	}

	if err := writePatch(fdb, dbPath); err != nil {
		return nil, false, err
	}
	return fdb, true, nil
}

// filterGames returns the games list, optionally filtered by --game flag.
// A non-empty filter that matches nothing is an error, not a silent success.
func filterGames(gameDB *db.GameDB) ([]*db.Game, error) {
	if gameFilter == "" {
		return gameDB.Games, nil
	}
	for _, g := range gameDB.Games {
		if g.Fingerprint == gameFilter {
			return []*db.Game{g}, nil
		}
	}
	return nil, fmt.Errorf("game %q not found in games database", gameFilter)
}

// applyPatches patches all games and returns whether any were modified.
func applyPatches(fdb *nvidia.FingerprintDB, games []*db.Game) bool {
	modified := false
	for _, game := range games {
		result := nvidia.PatchGame(fdb, game)
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

// writePatch writes the patched database.
func writePatch(fdb *nvidia.FingerprintDB, dbPath string) error {
	if err := nvidia.WriteFingerprintDB(fdb, dbPath); err != nil {
		return fmt.Errorf("writing %s: %w", dbPath, err)
	}
	return nil
}
