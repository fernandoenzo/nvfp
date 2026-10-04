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
	App         string   // application string to register (driver_app override or package family name)
	Profile     string   // exact profile name; empty means automatic resolution
	Candidates  []string // <DriverProfile> values tried during automatic resolution
}

// Result reports the outcome for one request.
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

// Patched reports whether the request added the application string.
func (r Result) Patched() bool { return r.Status == StatusPatched }

// newResult builds the result for a request, formatting the message for the
// resolved profile. errDetail is only used by StatusFailed.
func newResult(status Status, req Request, profile string, errDetail ...string) Result {
	message := resultMessage(status, req, profile, errDetail...)
	return Result{
		Status:      status,
		Fingerprint: req.Fingerprint,
		Profile:     profile,
		App:         req.App,
		Message:     message,
	}
}

// resultMessage words the outcome for the user, naming the candidate strings
// when the profile could not be resolved.
func resultMessage(status Status, req Request, profile string, errDetail ...string) string {
	switch status {
	case StatusPatched:
		return fmt.Sprintf("added %s to driver profile %q", req.App, profile)
	case StatusAlreadyPresent:
		return fmt.Sprintf("%s already in driver profile %q", req.App, profile)
	case StatusProfileNotFound:
		return fmt.Sprintf("driver profile %q not found", profile)
	case StatusConflict:
		return fmt.Sprintf("%s is already assigned to driver profile %q", req.App, profile)
	case StatusProfileUnresolved:
		return unresolvedMessage(req)
	case StatusFailed:
		return failedMessage(req, profile, errDetail...)
	}
	return ""
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

// failedMessage reports the driver's own reason, defaulting to a placeholder
// when the caller passed none.
func failedMessage(req Request, profile string, errDetail ...string) string {
	detail := "unknown error"
	if len(errDetail) > 0 {
		detail = errDetail[0]
	}
	return fmt.Sprintf("adding %s to driver profile %q failed: %s", req.App, profile, detail)
}
