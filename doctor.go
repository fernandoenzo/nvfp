package main

import (
	"fmt"
	"os"

	"github.com/fernandoenzo/nvfp/internal/db"
	"github.com/fernandoenzo/nvfp/internal/nvdr"
	"github.com/fernandoenzo/nvfp/internal/nvidia"
)

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
