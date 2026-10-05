package main

import (
	_ "embed"
	"errors"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"

	"github.com/fernandoenzo/nvfp/internal/db"
	"github.com/fernandoenzo/nvfp/internal/fsutil"
	"github.com/fernandoenzo/nvfp/internal/ngx"
	"github.com/fernandoenzo/nvfp/internal/nvdr"
	"github.com/fernandoenzo/nvfp/internal/nvidia"
	"github.com/fernandoenzo/nvfp/internal/update"
	"github.com/spf13/cobra"
)

const (
	// version is the release this binary was built from.
	version = "1.3.0-rc.6"
	// versionDate is the release date shown by --version.
	versionDate = "2026 Oct 2"
)

const versionMessage = "nvfp " + version + " (" + versionDate + ")\n" +
	"Copyright © 2026 Fernando Enzo Guarini\n" +
	"License GPLv3+: GNU GPL version 3 or later <https://gnu.org/licenses/gpl.html>.\n" +
	"This is free software: you are free to change and redistribute it.\n" +
	"There is NO WARRANTY, to the extent permitted by law.\n" +
	"\n" +
	"Written by Fernando Enzo Guarini.\n"

//go:embed games.json
var bundledGames []byte

var (
	dryRun         bool
	listOnly       bool
	restoreFlag    bool
	versionFlag    bool
	noDriverFlag   bool
	elevatedFlag   bool
	doctorFlag     bool
	slOverrideFlag bool
	gameFilter     string
	gamesJSONPath  string
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
	rootCmd.Flags().BoolVar(&slOverrideFlag, "sl-override", false, "Repair Streamline payloads and per-feature entries in the NGX cache")
}

// markExclusive declares the flag combinations that cannot be combined.
func markExclusive(rootCmd *cobra.Command) {
	rootCmd.MarkFlagsMutuallyExclusive("restore", "list")
	rootCmd.MarkFlagsMutuallyExclusive("restore", "game")
	rootCmd.MarkFlagsMutuallyExclusive("restore", "games-json")
	rootCmd.MarkFlagsMutuallyExclusive("doctor", "restore")
	rootCmd.MarkFlagsMutuallyExclusive("doctor", "no-driver")
	// The NGX repair reads the driver bundles, never the games manifest: it is
	// incompatible with every manifest-related flag.
	for _, other := range []string{"restore", "list", "doctor", "game", "games-json", "no-driver"} {
		rootCmd.MarkFlagsMutuallyExclusive("sl-override", other)
	}
}

func main() {
	if err := newRootCmd().Execute(); err != nil {
		os.Exit(1)
	}
}

func run(cmd *cobra.Command, args []string) error {
	// Version, restore and the NGX repair are local operations: they must not
	// depend on the manifest, the cache, or the network.
	switch {
	case versionFlag:
		fmt.Fprint(cmd.OutOrStdout(), versionMessage)
		return nil
	case restoreFlag:
		return restoreDB()
	case slOverrideFlag:
		return runSLOverride(ngx.Root)
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

// runSLOverride repairs the NGX cache, holding the elevated child's console
// open when there is a real window to read. resolveRoot is ngx.Root in
// production; tests inject a fixture directory instead.
func runSLOverride(resolveRoot func() (string, error)) error {
	if elevatedFlag && !stdoutIsPiped() {
		defer pauseBeforeExit()
	}
	return applySLOverride(resolveRoot)
}

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
	fdb, _, err := patchDB(gameDB, dbPath)
	if err != nil {
		return err
	}
	if noDriverFlag {
		return nil
	}
	return applyDriverStep(gameDB, fdb)
}

// restoreDB overwrites the working fingerprint.db with the pristine copy kept
// under the DAO directory, undoing every patch applied by this tool.
func restoreDB() error {
	src, err := findDAOFingerprintDB()
	if err != nil {
		return fmt.Errorf("finding the original fingerprint.db: %w", err)
	}
	dst, err := getFingerprintDBPath()
	if err != nil {
		return err
	}
	if dryRun {
		fmt.Printf("Would restore %s\n  from %s\n", dst, src)
		return nil
	}
	// The working copy may be gone entirely; recreate its directory.
	if err := fsutil.MkdirAllSync(filepath.Dir(dst), 0o755); err != nil {
		return fmt.Errorf("creating %s: %w", filepath.Dir(dst), err)
	}
	if err := fsutil.CopyFile(src, dst); err != nil {
		return fmt.Errorf("restoring %s: %w", dst, err)
	}
	fmt.Printf("Restored %s\n  from %s\n", dst, src)
	return nil
}

// ensureElevated relaunches the program through UAC when wanted is true and
// the process is not elevated yet. A failure to request elevation is a
// warning: the caller reports what it had to skip. It never returns when the
// elevated child succeeds, because that child replaces this process.
func ensureElevated(wanted bool) {
	if !wanted || elevatedFlag {
		return
	}
	elevated, err := isElevated()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: could not read the process elevation: %v\n", err)
	}
	if elevated {
		return
	}
	fmt.Fprintln(os.Stderr, "Administrator privileges required: relaunching elevated (accept the UAC prompt)")
	code, err := relaunchElevated()
	switch {
	case err == nil:
		os.Exit(code)
	case errors.Is(err, errElevationCancelled):
		// Still unelevated: the caller reports that its step was skipped.
	default:
		fmt.Fprintf(os.Stderr, "Warning: could not request administrator privileges: %v\n", err)
	}
}

// applySLOverride repairs the NGX cache: it copies compatible sibling payloads,
// adds missing per-feature sections and corrects stale versions. It backs up
// the manifest before changing it and requests UAC only when work is pending.
func applySLOverride(resolveRoot func() (string, error)) error {
	root, err := resolveRoot()
	if err != nil {
		return err
	}
	plan, err := ngx.Inspect(root)
	if err != nil {
		return err
	}
	for _, warning := range plan.Warnings {
		fmt.Fprintf(os.Stderr, "Warning: %s\n", warning)
	}
	if dryRun {
		printSLPlan(plan)
		return nil
	}
	ensureElevated(plan.Changed())
	if err := ngx.Apply(plan); err != nil {
		return fmt.Errorf("updating the NGX manifest: %w", err)
	}
	printSLResult(plan)
	return nil
}

// printSLPlan shows what the NGX repair would do, naming the files.
func printSLPlan(plan *ngx.Plan) {
	fmt.Printf("NGX OTA cache: %s\n  manifest: %s\n", plan.Root, plan.Manifest)
	for _, cp := range plan.Copies {
		fmt.Printf("  → would copy %s\n        to %s\n", cp.Source, cp.Dest)
	}
	for _, section := range plan.Additions {
		fmt.Printf("  → would add [%s]  app_%s = %s\n", section.Feature, section.Hash, section.Version)
	}
	for _, section := range plan.Updates {
		fmt.Printf("  → would update [%s]  app_%s: %s → %s\n", section.Feature, section.Hash, section.Current, section.Version)
	}
	printSLNoOp(plan)
	warnMissingFeatures(plan)
}

// printSLResult reports the repair the way the patch steps do: one line per
// change, using the same symbols.
func printSLResult(plan *ngx.Plan) {
	fmt.Printf("NGX OTA manifest: %s\n", plan.Manifest)
	for _, cp := range plan.Copies {
		fmt.Printf("  ✓ %s payload restored from the sibling bundle\n", cp.Feature)
	}
	for _, section := range plan.Additions {
		fmt.Printf("  ✓ added [%s]  app_%s = %s\n", section.Feature, section.Hash, section.Version)
	}
	for _, section := range plan.Updates {
		fmt.Printf("  ✓ updated [%s]  app_%s: %s → %s\n", section.Feature, section.Hash, section.Current, section.Version)
	}
	printSLNoOp(plan)
	if len(plan.Additions) > 0 || len(plan.Updates) > 0 {
		// The .bak is written only when the manifest itself changes.
		fmt.Printf("  backup: %s\n", plan.Backup)
	}
	warnMissingFeatures(plan)
}

func printSLNoOp(plan *ngx.Plan) {
	if plan.Changed() {
		return
	}
	if len(plan.Missing) > 0 {
		fmt.Println("  ⊘ no repairable changes: no matching-architecture payload is available for some features")
		return
	}
	if len(plan.Warnings) > 0 {
		fmt.Println("  ⊘ no repairable changes found; review the warnings for cache details")
		return
	}
	fmt.Println("  ⊘ nothing to do: all discovered per-feature sections are present and current")
}

// warnMissingFeatures reports features with no payload for their discovered
// architecture in either sibling variant.
func warnMissingFeatures(plan *ngx.Plan) {
	if len(plan.Missing) == 0 {
		return
	}
	fmt.Fprintf(os.Stderr, "Warning: no compatible-architecture payload for: %v\n", plan.Missing)
	fmt.Fprintln(os.Stderr, "These features are skipped; payloads for another GPU architecture cannot be substituted.")
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
	// On Windows: %LOCALAPPDATA%\nvfp (cached data stays on the machine)
	localAppData := os.Getenv("LOCALAPPDATA")
	if localAppData != "" {
		return filepath.Join(localAppData, "nvfp"), nil
	}
	// Fallback for non-Windows (testing)
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".cache", "nvfp"), nil
}

// getNvidiaAppDir returns the NVIDIA App backend directory.
func getNvidiaAppDir() (string, error) {
	localAppData := os.Getenv("LOCALAPPDATA")
	if localAppData == "" {
		return "", fmt.Errorf("LOCALAPPDATA not set")
	}
	dir := filepath.Join(localAppData, "NVIDIA Corporation", "NVIDIA App", "NvBackend")
	if _, err := os.Stat(dir); err != nil {
		return "", fmt.Errorf("NVIDIA App dir not found (is NVIDIA App installed?): %w", err)
	}
	return dir, nil
}

// getFingerprintDBPath returns the path of the working fingerprint.db without
// requiring the file to exist, so a restore can recreate it.
func getFingerprintDBPath() (string, error) {
	nvidiaAppDir, err := getNvidiaAppDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(nvidiaAppDir,
		"ApplicationOntology", "data", "fingerprint.db"), nil
}

// findFingerprintDB returns the path of the working fingerprint.db used by the
// NVIDIA App ontology engine.
func findFingerprintDB() (string, error) {
	path, err := getFingerprintDBPath()
	if err != nil {
		return "", err
	}
	if _, err := os.Stat(path); err != nil {
		return "", fmt.Errorf("fingerprint.db not found (is NVIDIA App installed?): %w", err)
	}
	return path, nil
}

// findDAOFingerprintDB returns the path of the pristine fingerprint.db stored
// under the NVIDIA App DAO directory.
func findDAOFingerprintDB() (string, error) {
	nvidiaAppDir, err := getNvidiaAppDir()
	if err != nil {
		return "", err
	}
	daoDir := filepath.Join(nvidiaAppDir, "DAO")
	entries, err := os.ReadDir(daoDir)
	if err != nil {
		return "", fmt.Errorf("reading DAO directory (is NVIDIA App installed?): %w", err)
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		dbPath := filepath.Join(daoDir, entry.Name(), "fingerprint.db")
		if _, err := os.Stat(dbPath); err == nil {
			return dbPath, nil
		}
	}
	return "", fmt.Errorf("fingerprint.db not found under %s", daoDir)
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

// hasDriverWork reports whether any selected game needs a driver-profile entry.
// The filter error is ignored on purpose: this only decides whether to request
// elevation, and the real error is reported by the patching step.
func hasDriverWork(gameDB *db.GameDB) bool {
	games, err := filterGames(gameDB)
	if err != nil {
		return false
	}
	for _, game := range games {
		if game.DriverAppString() != "" {
			return true
		}
	}
	return false
}

// driverRequests builds one request per selected game that has a string to
// register, with the fingerprint's executables as automatic-resolution hints.
func driverRequests(gameDB *db.GameDB, fdb *nvidia.FingerprintDB) []nvdr.Request {
	games, err := filterGames(gameDB)
	if err != nil {
		return nil
	}
	var reqs []nvdr.Request
	for _, game := range games {
		app := game.DriverAppString()
		if app == "" {
			continue
		}
		req := nvdr.Request{
			App:         app,
			Profile:     game.DriverProfile,
			Fingerprint: game.Fingerprint,
		}
		if fp := nvidia.FindFingerprint(fdb, game.Fingerprint); fp != nil {
			req.Candidates = nvidia.DriverProfileCandidates(fp)
		}
		reqs = append(reqs, req)
	}
	return reqs
}

// applyDriverStep registers every selected game's application string in its
// NVIDIA driver profile. Failures here never mask a successful fingerprint.db
// patch: they are reported as warnings.
func applyDriverStep(gameDB *db.GameDB, fdb *nvidia.FingerprintDB) error {
	reqs := driverRequests(gameDB, fdb)
	if len(reqs) == 0 {
		return nil
	}
	if dryRun {
		printDriverPlan(reqs)
		return nil
	}
	return runDriverStep(reqs)
}

// runDriverStep performs the privileged registration, skipping with a warning
// instead of failing the run.
func runDriverStep(reqs []nvdr.Request) error {
	elevated, err := isElevated()
	if err == nil && !elevated {
		err = errors.New("administrator privileges required (run as administrator)")
	}
	if err == nil {
		var results []nvdr.Result
		if results, err = nvdr.Apply(reqs); err == nil {
			printDriverResults(results)
		}
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: driver profiles skipped: %v\n", err)
	}
	return nil
}

// doctor reports how every request's profile would resolve, without writing
// anything and without requesting elevation. It exists because the elevated
// child's window closes as soon as the work is done, which makes a failing
// resolution impossible to diagnose from the console.
func doctor(gameDB *db.GameDB) error {
	reqs := driverRequests(gameDB, loadFingerprintForDoctor())
	if len(reqs) == 0 {
		fmt.Println("Nothing to do: no selected game carries a driver application string.")
		return nil
	}
	report, err := nvdr.Diagnose(reqs)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Could not read the driver profiles: %v\n", err)
		fmt.Println("Candidate resolution can only be checked on Windows, where the driver database lives.")
		return nil
	}
	fmt.Println("Driver profile resolution (read-only, nothing is written):")
	for i, req := range reqs {
		reportRequest(req, report[i])
	}
	return nil
}

// loadFingerprintForDoctor parses fingerprint.db when it can, so the report
// can name the executables; a missing database only costs the candidates.
func loadFingerprintForDoctor() *nvidia.FingerprintDB {
	dbPath, err := findFingerprintDB()
	if err != nil {
		fmt.Fprintf(os.Stderr, "fingerprint.db not found, showing manifest candidates only: %v\n", err)
		return nil
	}
	parsed, err := nvidia.ParseFingerprintDB(dbPath)
	if err != nil {
		return nil
	}
	return parsed
}

// reportRequest prints one game's resolution: its identity, every candidate
// the driver answered for, and a hint when nothing matched.
func reportRequest(req nvdr.Request, attempts []nvdr.LookupAttempt) {
	fmt.Printf("\n%s\n", req.Fingerprint)
	fmt.Printf("  application string : %s\n", req.App)
	if req.Profile != "" {
		fmt.Printf("  pinned profile     : %s\n", req.Profile)
	}
	if len(req.Candidates) == 0 {
		fmt.Println("  candidates         : none (fingerprint not found, or it has no <DriverProfile>)")
		if req.Profile == "" {
			printUnresolvedHint()
		}
		return
	}
	fmt.Println("  candidates         :")
	matched := false
	for _, attempt := range attempts {
		printAttempt(attempt)
		matched = matched || attempt.Status == 0
	}
	if !matched {
		printUnresolvedHint()
	}
}

// printAttempt renders one candidate lookup with its NVAPI status, or the
// profile it matched.
func printAttempt(attempt nvdr.LookupAttempt) {
	mark, detail := "✗", nvdr.StatusName(attempt.Status)
	if attempt.Status == 0 {
		mark, detail = "✓", fmt.Sprintf("matched profile %q", attempt.Profile)
	}
	fmt.Printf("      %s %-48s %s\n", mark, attempt.Attempt, detail)
}

func printUnresolvedHint() {
	fmt.Println("  => unresolved: set \"driver_profile\" in games.json with the exact name from NVIDIA Control Panel")
}

// printDriverPlan shows what the driver step would do, without touching the
// driver database (and therefore without requesting elevation).
func printDriverPlan(reqs []nvdr.Request) {
	fmt.Println("Driver profiles:")
	for _, req := range reqs {
		if req.Profile == "" {
			fmt.Printf("  → would register %s in driver profile for %q (auto)\n", req.App, req.Fingerprint)
			continue
		}
		fmt.Printf("  → would register %s in driver profile %q\n", req.App, req.Profile)
	}
}

// printDriverResults reports one line per request, with the same symbols the
// fingerprint patch uses.
func printDriverResults(results []nvdr.Result) {
	fmt.Println("Driver profiles:")
	for _, res := range results {
		switch res.Status {
		case nvdr.StatusPatched:
			fmt.Printf("  ✓ %s\n", res.Message)
		case nvdr.StatusAlreadyPresent:
			fmt.Printf("  ⊘ %s\n", res.Message)
		default:
			fmt.Printf("  ✗ %s\n", res.Message)
		}
	}
}

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
