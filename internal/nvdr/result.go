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

// statusNames maps the NvAPI_Status codes this package handles to their header
// names, so a diagnostic can report something readable instead of a raw number.
var statusNames = map[int32]string{
	0:    "NVAPI_OK",
	-5:   "NVAPI_INVALID_ARGUMENT",
	-7:   "NVAPI_END_ENUMERATION",
	-137: "NVAPI_INVALID_USER_PRIVILEGE",
	-163: "NVAPI_PROFILE_NOT_FOUND",
	-166: "NVAPI_EXECUTABLE_NOT_FOUND",
	-167: "NVAPI_EXECUTABLE_ALREADY_IN_USE",
	-175: "NVAPI_ACCESS_DENIED",
}

// StatusName renders a raw NVAPI status for a diagnostic message.
func StatusName(status int32) string {
	if name, ok := statusNames[status]; ok {
		return name
	}
	return fmt.Sprintf("NvAPI status %d", status)
}

// Request is one game's driver-profile work.
type Request struct {
	Fingerprint string   // for messages only
	Apps        []string // application strings to register, in order (package family name first)
	Profile     string   // exact profile name; empty means automatic resolution
	Candidates  []string // <DriverProfile> values tried during automatic resolution
}

// Result reports the outcome for one request: one entry per application string
// that reached registration, or a single entry naming the fingerprint when the
// request failed before any string could be registered.
type Result struct {
	Status      Status
	Fingerprint string
	Profile     string
	App         string
	Message     string
}

// LookupAttempt reports how one candidate string fared when looking for the
// profile that owns it. It exists so the user can see what the driver answers
// without an elevated relaunch, which is otherwise invisible.
type LookupAttempt struct {
	Attempt string // the exact string handed to FindApplicationByName
	Status  int32  // the NVAPI status it returned
	Profile string // the profile it resolved to, when it matched
}

// Patched reports whether the result added an application string.
func (r Result) Patched() bool { return r.Status == StatusPatched }

// newAppResult builds the result of registering one application string in a
// resolved profile. errDetail is only used by StatusFailed.
func newAppResult(status Status, req Request, app, profile string, errDetail ...string) Result {
	return Result{
		Status:      status,
		Fingerprint: req.Fingerprint,
		Profile:     profile,
		App:         app,
		Message:     appMessage(status, app, profile, errDetail...),
	}
}

// newRequestResult builds the result of a request that never reached an
// application string — the profile could not be found or resolved — so a single
// line stands for the whole request instead of repeating per application.
func newRequestResult(status Status, req Request, profile string, errDetail ...string) Result {
	return Result{
		Status:      status,
		Fingerprint: req.Fingerprint,
		Profile:     profile,
		Message:     requestMessage(status, req, profile, errDetail...),
	}
}

// appMessage words the outcome of registering one application string.
func appMessage(status Status, app, profile string, errDetail ...string) string {
	switch status {
	case StatusPatched:
		return fmt.Sprintf("added %s to driver profile %q", app, profile)
	case StatusAlreadyPresent:
		return fmt.Sprintf("%s already in driver profile %q", app, profile)
	case StatusConflict:
		return fmt.Sprintf("%s is already assigned to driver profile %q", app, profile)
	case StatusFailed:
		return fmt.Sprintf("adding %s to driver profile %q failed: %s", app, profile, detailOf(errDetail))
	}
	return ""
}

// requestMessage words a request that failed before any application string was
// registered, naming the candidate strings when the profile could not be resolved.
func requestMessage(status Status, req Request, profile string, errDetail ...string) string {
	switch status {
	case StatusProfileNotFound:
		return fmt.Sprintf("driver profile %q not found", profile)
	case StatusProfileUnresolved:
		return unresolvedMessage(req)
	case StatusFailed:
		return fmt.Sprintf("driver profile %q lookup failed: %s", profile, detailOf(errDetail))
	}
	return ""
}

// detailOf returns the driver's own reason, defaulting to a placeholder when the
// caller passed none.
func detailOf(errDetail []string) string {
	if len(errDetail) > 0 {
		return errDetail[0]
	}
	return "unknown error"
}

// unresolvedMessage explains how to pin the profile in the manifest.
func unresolvedMessage(req Request) string {
	if len(req.Candidates) == 0 {
		return fmt.Sprintf("no driver profile found for %q; set %q in games.json",
			req.Fingerprint, "driver_profile")
	}
	return fmt.Sprintf("no driver profile found for %q (tried: %s); set %q in games.json",
		req.Fingerprint, strings.Join(req.Candidates, ", "), "driver_profile")
}
