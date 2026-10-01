// Package nvdr registers UWP package family names in the NVIDIA driver's
// profile database (DRS) through NVAPI, so the driver recognizes and applies
// profiles to games that are launched as Microsoft Store apps.
package nvdr

import (
	"fmt"
	"strings"
)

// Status reports the outcome of a driver-profile request.
type Status string

const (
	StatusPatched           Status = "patched"
	StatusAlreadyPresent    Status = "already_present"
	StatusProfileNotFound   Status = "profile_not_found"
	StatusProfileUnresolved Status = "profile_unresolved"
	StatusConflict          Status = "conflict"
	StatusFailed            Status = "failed"
)

// Request is one game's driver-profile work.
type Request struct {
	Fingerprint string   // for messages only
	App         string   // string to register (PackageFamilyName)
	Profile     string   // exact profile name; empty means automatic resolution
	Candidates  []string // .exe names from the fingerprint for automatic resolution
}

// Result reports the outcome for one request.
type Result struct {
	Status      Status
	Fingerprint string
	Profile     string
	App         string
	Message     string
}

// Patched reports whether the request added the application string.
func (r Result) Patched() bool { return r.Status == StatusPatched }

// newResult builds the result for a request, formatting the message for the
// resolved profile. errDetail is only used by StatusFailed.
func newResult(status Status, req Request, profile string, errDetail ...string) Result {
	res := Result{
		Status:      status,
		Fingerprint: req.Fingerprint,
		Profile:     profile,
		App:         req.App,
	}
	switch status {
	case StatusPatched:
		res.Message = fmt.Sprintf("added %s to driver profile %q", req.App, profile)
	case StatusAlreadyPresent:
		res.Message = fmt.Sprintf("%s already in driver profile %q", req.App, profile)
	case StatusProfileNotFound:
		res.Message = fmt.Sprintf("driver profile %q not found", profile)
	case StatusProfileUnresolved:
		if len(req.Candidates) == 0 {
			res.Message = fmt.Sprintf("no driver profile found for %q; set %q in games.json",
				req.Fingerprint, "driver_profile")
			break
		}
		res.Message = fmt.Sprintf("no driver profile found for %q (tried: %s); set %q in games.json",
			req.Fingerprint, strings.Join(req.Candidates, ", "), "driver_profile")
	case StatusConflict:
		res.Message = fmt.Sprintf("%s is already assigned to driver profile %q", req.App, profile)
	case StatusFailed:
		detail := "unknown error"
		if len(errDetail) > 0 {
			detail = errDetail[0]
		}
		res.Message = fmt.Sprintf("adding %s to driver profile %q failed: %s", req.App, profile, detail)
	}
	return res
}
