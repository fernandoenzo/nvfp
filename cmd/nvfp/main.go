package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var (
	dryRun          bool
	listOnly        bool
	restoreFlag     bool
	versionFlag     bool
	noDriverFlag    bool
	elevatedFlag    bool
	doctorFlag      bool
	resetModelsFlag bool
	gameFilter      string
	gamesJSONPath   string
)

// newRootCmd builds the command tree. It is separate from main so tests can
// exercise the real flag wiring.
func newRootCmd() *cobra.Command {
	rootCmd := &cobra.Command{
		Use:   "nvfp",
		Short: "Patch NVIDIA App fingerprint.db to add UWP game profiles",
		Args:  cobra.NoArgs,
		RunE:  run,
	}
	registerFlags(rootCmd)
	markExclusive(rootCmd)
	return rootCmd
}

// registerFlags binds every command-tree flag.
func registerFlags(rootCmd *cobra.Command) {
	rootCmd.Flags().BoolVar(&dryRun, "dry-run", false, "Show changes without writing files")
	rootCmd.Flags().BoolVar(&listOnly, "list", false, "List games in the database")
	rootCmd.Flags().BoolVar(&restoreFlag, "restore", false, "Restore the original fingerprint.db from the DAO copy")
	rootCmd.Flags().BoolVarP(&versionFlag, "version", "v", false, "Print version information and exit")
	rootCmd.Flags().StringVar(&gameFilter, "game", "", "Patch only a specific game (by fingerprint)")
	rootCmd.Flags().StringVar(&gamesJSONPath, "games-json", "", "Use a local games.json instead of the remote manifest")
	rootCmd.Flags().BoolVar(&noDriverFlag, "no-driver", false, "Skip patching NVIDIA driver profiles")
	rootCmd.Flags().BoolVar(&elevatedFlag, "elevated", false, "Internal: set after the UAC relaunch")
	rootCmd.Flags().MarkHidden("elevated")
	rootCmd.Flags().BoolVar(&doctorFlag, "doctor", false, "Report how each driver profile would resolve, without writing anything")
	rootCmd.Flags().BoolVar(&resetModelsFlag, "reset-models", false, "Delete the NVIDIA NGX models folder; NVIDIA rebuilds it at the next session")
}

// markExclusive declares the flag combinations that cannot be combined.
func markExclusive(rootCmd *cobra.Command) {
	rootCmd.MarkFlagsMutuallyExclusive("restore", "list")
	rootCmd.MarkFlagsMutuallyExclusive("restore", "game")
	rootCmd.MarkFlagsMutuallyExclusive("restore", "games-json")
	rootCmd.MarkFlagsMutuallyExclusive("doctor", "restore")
	rootCmd.MarkFlagsMutuallyExclusive("doctor", "no-driver")
	// The models reset works on the NGX cache, never the games manifest: it is
	// incompatible with every manifest-related flag.
	for _, other := range []string{"restore", "list", "doctor", "game", "games-json", "no-driver"} {
		rootCmd.MarkFlagsMutuallyExclusive("reset-models", other)
	}
}

func main() {
	if err := newRootCmd().Execute(); err != nil {
		os.Exit(1)
	}
}

func run(cmd *cobra.Command, args []string) error {
	// Version, restore and the models reset are local operations: they must not
	// depend on the manifest, the cache, or the network.
	switch {
	case versionFlag:
		fmt.Fprint(cmd.OutOrStdout(), versionMessage)
		return nil
	case restoreFlag:
		return restoreDB()
	case resetModelsFlag:
		return runResetModels(nvidiaModelsDir)
	}
	gameDB, err := resolveGames()
	if err != nil {
		return fmt.Errorf("loading games database: %w", err)
	}
	// List and doctor never touch the driver database.
	switch {
	case listOnly:
		listGames(gameDB)
		return nil
	case doctorFlag:
		return doctor(gameDB)
	}
	return patchEverything(gameDB)
}
