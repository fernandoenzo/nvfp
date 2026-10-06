package main

import (
	"fmt"
	"maps"
	"slices"

	"github.com/fernandoenzo/nvfp/internal/db"
)

func listGames(gameDB *db.GameDB) {
	fmt.Printf("Games database version: %d\n", gameDB.Version)
	fmt.Printf("Total games: %d\n\n", len(gameDB.Games))
	for _, game := range gameDB.Games {
		printGameEntry(game)
	}
}

// printGameEntry prints one manifest game and its identity fields.
func printGameEntry(game *db.Game) {
	fmt.Printf("  %s\n", game.Fingerprint)
	fmt.Printf("    AppUserModelId: %s\n", game.AppUserModelID)
	fmt.Printf("    UWPPackageFamilyName: %s\n", game.UWPPackageFamilyName())
	if game.SkipDriver {
		fmt.Println("    SkipDriver: true")
	} else if app := game.DriverAppString(); app != "" {
		fmt.Printf("    DriverApp: %s\n", app)
	}
	if game.DriverProfile != "" {
		fmt.Printf("    DriverProfile: %s\n", game.DriverProfile)
	}
	printGameOverrides(game)
}

// printGameOverrides prints the overrides sorted by key and the removal list,
// the two fields that change what the patch writes.
func printGameOverrides(game *db.Game) {
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
