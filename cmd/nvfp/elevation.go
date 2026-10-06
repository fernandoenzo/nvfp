package main

import (
	"errors"
	"fmt"
	"os"
)

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
