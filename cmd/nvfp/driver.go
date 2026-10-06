package main

import (
	"errors"
	"fmt"
	"os"

	"github.com/fernandoenzo/nvfp/internal/db"
	"github.com/fernandoenzo/nvfp/internal/nvdr"
	"github.com/fernandoenzo/nvfp/internal/nvidia"
)

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
