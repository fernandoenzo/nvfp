package main

import (
	"fmt"
	"os"

	assets "github.com/fernandoenzo/nvfp"
	"github.com/fernandoenzo/nvfp/internal/db"
	"github.com/fernandoenzo/nvfp/internal/update"
)

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

	return db.ResolveGames(cacheDir, assets.Games, data)
}
