//go:build windows

package nvdr

import (
	"fmt"
	"runtime"
	"strings"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

// Function IDs from NVIDIA/nvapi's nvapi_interface.h. Each ID is resolved
// through nvapi_QueryInterface at load time; the driver is free to move
// implementations, only these IDs are stable.
const (
	idInitialize            = 0x0150E828
	idGetErrorMessage       = 0x6C2D048C
	idCreateSession         = 0x0694D52E
	idDestroySession        = 0xDAD9CFF8
	idLoadSettings          = 0x375DBD6B
	idSaveSettings          = 0xFCBC7E14
	idGetProfileInfo        = 0x61CD6FD6
	idFindProfileByName     = 0x7E4A9A0B
	idCreateApplication     = 0x4347A9DE
	idFindApplicationByName = 0xEEE566B2
	idEnumApplications      = 0x7FA2173A
)

// NvAPI_Status values the driver-profile step branches on, from
// nvapi_lite_common.h.
const (
	statusOK                     = 0
	statusInvalidArgument        = -5
	statusEndEnumeration         = -7
	statusInvalidUserPrivilege   = -137
	statusProfileNotFound        = -163
	statusExecutableNotFound     = -166
	statusExecutableAlreadyInUse = -167
	statusAccessDenied           = -175
)

// applicationVersion is NVDRS_APPLICATION_VER_V4 (low word: struct size, high
// word: version number).
const applicationVersion = uint32(unsafe.Sizeof(applicationV4{})) | (4 << 16)

// profileVersion is NVDRS_PROFILE_VER_V1.
const profileVersion = uint32(unsafe.Sizeof(profileV1{})) | (1 << 16)

var (
	_ [applicationVersion - 282636]struct{} = [0]struct{}{}
	_ [282636 - applicationVersion]struct{} = [0]struct{}{}
	_ [profileVersion - 69652]struct{}      = [0]struct{}{}
	_ [69652 - profileVersion]struct{}      = [0]struct{}{}
)

// applicationV4 mirrors NVDRS_APPLICATION_V4: a version word, a predefined flag
// and six 2048-unit unicode strings with a flags word before the command line.
// Layout for amd64; see the asserts below.
type applicationV4 struct {
	version          uint32
	isPredefined     uint32
	appName          [2048]uint16
	userFriendlyName [2048]uint16
	launcher         [2048]uint16
	fileInFolder     [2048]uint16
	flags            uint32 // bit0 isMetro, bit1 isCommandLine: left at 0
	commandLine      [2048]uint16
}

// profileV1 mirrors NVDRS_PROFILE_V1, the read-only profile description.
type profileV1 struct {
	version       uint32
	profileName   [2048]uint16
	gpuSupport    uint32
	isPredefined  uint32
	numOfApps     uint32
	numOfSettings uint32
}

// The layouts are hand-written. The asserts below pin the Go structs to the
// sizes and offsets measured once against NVIDIA's nvapi.h for amd64, so an
// edit to the structs breaks the build. They cannot detect a change in the
// header itself: the expected values are literals, just like the structs.
var (
	_ [unsafe.Sizeof(applicationV4{}) - 20492]struct{}               = [0]struct{}{}
	_ [unsafe.Offsetof(applicationV4{}.appName) - 8]struct{}         = [0]struct{}{}
	_ [unsafe.Offsetof(applicationV4{}.commandLine) - 16396]struct{} = [0]struct{}{}
	_ [unsafe.Sizeof(profileV1{}) - 4116]struct{}                    = [0]struct{}{}
	_ [unsafe.Offsetof(profileV1{}.profileName) - 4]struct{}         = [0]struct{}{}
	_ [unsafe.Offsetof(profileV1{}.numOfApps) - 4108]struct{}        = [0]struct{}{}
)

// enumBatch is how many applications are requested per enumeration call.
const enumBatch = 32

var (
	// nvapi64.dll lives in Windows\System32, so the system-only variant is
	// used: it restricts the search to that directory and removes the DLL
	// preloading risk the stdlib's NewLazyDLL carries (x/sys documents the
	// hazard, the stdlib does not).
	modNVAPI           = windows.NewLazySystemDLL("nvapi64.dll")
	procQueryInterface = modNVAPI.NewProc("nvapi_QueryInterface")
)

// api holds the resolved addresses of the NVAPI functions in use.
type api struct {
	fnInitialize            uintptr
	fnGetErrorMessage       uintptr
	fnCreateSession         uintptr
	fnDestroySession        uintptr
	fnLoadSettings          uintptr
	fnSaveSettings          uintptr
	fnGetProfileInfo        uintptr
	fnFindProfileByName     uintptr
	fnCreateApplication     uintptr
	fnFindApplicationByName uintptr
	fnEnumApplications      uintptr
}

// openAPI resolves every NVAPI function needed. nvapi64.dll ships with the
// NVIDIA driver, so a load failure means no usable driver is installed.
func openAPI() (*api, error) {
	if err := procQueryInterface.Find(); err != nil {
		return nil, fmt.Errorf("loading nvapi64.dll (is the NVIDIA driver installed?): %w", err)
	}
	query := procQueryInterface.Addr()
	resolve := func(id uint32) (uintptr, error) {
		ptr, _, _ := syscall.SyscallN(query, uintptr(id))
		if ptr == 0 {
			return 0, fmt.Errorf("nvapi64.dll does not export function 0x%08X", id)
		}
		return ptr, nil
	}
	a := &api{}
	for _, target := range []struct {
		id uint32
		fn *uintptr
	}{
		{idInitialize, &a.fnInitialize},
		{idGetErrorMessage, &a.fnGetErrorMessage},
		{idCreateSession, &a.fnCreateSession},
		{idDestroySession, &a.fnDestroySession},
		{idLoadSettings, &a.fnLoadSettings},
		{idSaveSettings, &a.fnSaveSettings},
		{idGetProfileInfo, &a.fnGetProfileInfo},
		{idFindProfileByName, &a.fnFindProfileByName},
		{idCreateApplication, &a.fnCreateApplication},
		{idFindApplicationByName, &a.fnFindApplicationByName},
		{idEnumApplications, &a.fnEnumApplications},
	} {
		ptr, err := resolve(target.id)
		if err != nil {
			return nil, err
		}
		*target.fn = ptr
	}
	return a, nil
}

func (a *api) initialize() int32 {
	r, _, _ := syscall.SyscallN(a.fnInitialize)
	return int32(r)
}

func (a *api) createSession() (uintptr, int32) {
	var handle uintptr
	r, _, _ := syscall.SyscallN(a.fnCreateSession, uintptr(unsafe.Pointer(&handle)))
	return handle, int32(r)
}

func (a *api) destroySession(session uintptr) int32 {
	r, _, _ := syscall.SyscallN(a.fnDestroySession, session)
	return int32(r)
}

func (a *api) loadSettings(session uintptr) int32 {
	r, _, _ := syscall.SyscallN(a.fnLoadSettings, session)
	return int32(r)
}

func (a *api) saveSettings(session uintptr) int32 {
	r, _, _ := syscall.SyscallN(a.fnSaveSettings, session)
	return int32(r)
}

func (a *api) getProfileInfo(session, profile uintptr) (profileV1, int32) {
	info := profileV1{version: profileVersion}
	r, _, _ := syscall.SyscallN(a.fnGetProfileInfo, session, profile, uintptr(unsafe.Pointer(&info)))
	runtime.KeepAlive(&info)
	return info, int32(r)
}

func (a *api) findProfileByName(session uintptr, name string) (uintptr, int32) {
	var buf [2048]uint16
	if err := toUTF16(buf[:], name); err != nil {
		return 0, statusInvalidArgument
	}
	var handle uintptr
	r, _, _ := syscall.SyscallN(a.fnFindProfileByName,
		session, uintptr(unsafe.Pointer(&buf[0])), uintptr(unsafe.Pointer(&handle)))
	runtime.KeepAlive(&buf)
	return handle, int32(r)
}

// Diagnose resolves every request the way Apply would, but only reports what it
// found: it never writes and never needs a session that can save. The result is
// one slice of attempts per candidate, in the order they were tried.
func Diagnose(reqs []Request) ([][]LookupAttempt, error) {
	a, err := openAPI()
	if err != nil {
		return nil, err
	}
	session, status := a.createSession()
	if status != statusOK {
		return nil, fmt.Errorf("NvAPI_DRS_CreateSession failed: %s", a.errorDetail(status))
	}
	defer a.destroySession(session)
	if status := a.loadSettings(session); status != statusOK {
		return nil, fmt.Errorf("NvAPI_DRS_LoadSettings failed: %s", a.errorDetail(status))
	}

	report := make([][]LookupAttempt, 0, len(reqs))
	for _, req := range reqs {
		var attempts []LookupAttempt
		for _, candidate := range req.Candidates {
			for _, attempt := range []string{candidate, strings.ToLower(candidate)} {
				entry := LookupAttempt{Attempt: attempt}
				handle, stat := a.findApplicationByName(session, attempt)
				entry.Status = stat
				if stat == statusOK {
					if info, infoStat := a.getProfileInfo(session, handle); infoStat == statusOK {
						entry.Profile = fromUTF16(info.profileName[:])
					}
				}
				attempts = append(attempts, entry)
			}
		}
		report = append(report, attempts)
	}
	return report, nil
}

// findApplicationByName looks the application up and returns the profile handle
// the driver associates with it. The 20492-byte application struct is in/out:
// it must carry a valid version word or the driver rejects it.
func (a *api) findApplicationByName(session uintptr, name string) (uintptr, int32) {
	var app applicationV4
	app.version = applicationVersion
	if err := toUTF16(app.appName[:], name); err != nil {
		return 0, statusInvalidArgument
	}
	var handle uintptr
	r, _, _ := syscall.SyscallN(a.fnFindApplicationByName,
		session,
		uintptr(unsafe.Pointer(&app.appName[0])),
		uintptr(unsafe.Pointer(&handle)),
		uintptr(unsafe.Pointer(&app)))
	runtime.KeepAlive(&app)
	return handle, int32(r)
}

func (a *api) createApplication(session, profile uintptr, app *applicationV4) int32 {
	r, _, _ := syscall.SyscallN(a.fnCreateApplication,
		session, profile, uintptr(unsafe.Pointer(app)))
	runtime.KeepAlive(app)
	return int32(r)
}

func (a *api) enumApplications(session, profile uintptr, start uint32, count *uint32, buf *applicationV4) int32 {
	r, _, _ := syscall.SyscallN(a.fnEnumApplications,
		session, profile, uintptr(start), uintptr(unsafe.Pointer(count)), uintptr(unsafe.Pointer(buf)))
	return int32(r)
}

// getErrorMessage converts a status into the driver's own description.
func (a *api) getErrorMessage(status int32) string {
	var buf [64]byte
	syscall.SyscallN(a.fnGetErrorMessage, uintptr(status), uintptr(unsafe.Pointer(&buf[0])))
	runtime.KeepAlive(&buf)
	return cString(buf[:])
}

// errorDetail renders a status for user-facing messages, explaining the
// privilege errors the DRS API returns to a non-elevated process.
func (a *api) errorDetail(status int32) string {
	if status == statusInvalidUserPrivilege || status == statusAccessDenied {
		return "administrator privileges required"
	}
	if msg := a.getErrorMessage(status); msg != "" {
		return msg
	}
	return fmt.Sprintf("NvAPI status %d", status)
}

// applications lists every application name registered in the profile,
// enumerating in batches until the driver reports the end of the list.
func (a *api) applications(session, profile uintptr) ([]string, error) {
	buf := make([]applicationV4, enumBatch)
	var names []string
	var start uint32
	for {
		for i := range buf {
			buf[i] = applicationV4{version: applicationVersion}
		}
		count := uint32(len(buf))
		switch status := a.enumApplications(session, profile, start, &count, &buf[0]); status {
		case statusEndEnumeration:
			return names, nil
		case statusOK:
			if count == 0 {
				return names, nil
			}
			for i := range count {
				names = append(names, fromUTF16(buf[i].appName[:]))
			}
			start += count
		default:
			return nil, fmt.Errorf("%s", a.errorDetail(status))
		}
	}
}

// buildApplication returns an application struct ready for CreateApplication:
// zeroed except for the version word and the executable name, like
// NvidiaProfileInspectorRevamped does.
func buildApplication(name string) (*applicationV4, error) {
	app := &applicationV4{version: applicationVersion}
	if err := toUTF16(app.appName[:], name); err != nil {
		return nil, err
	}
	return app, nil
}

// Apply registers each request's application string in its driver profile.
// Nothing is persisted unless at least one application was added; a save
// failure therefore aborts the whole batch and reports an error.
func Apply(reqs []Request) ([]Result, error) {
	a, err := openAPI()
	if err != nil {
		return nil, err
	}
	if status := a.initialize(); status != statusOK {
		return nil, fmt.Errorf("NvAPI_Initialize failed: %s", a.errorDetail(status))
	}
	session, status := a.createSession()
	if status != statusOK {
		return nil, fmt.Errorf("NvAPI_DRS_CreateSession failed: %s", a.errorDetail(status))
	}
	defer a.destroySession(session)
	if status := a.loadSettings(session); status != statusOK {
		return nil, fmt.Errorf("NvAPI_DRS_LoadSettings failed: %s", a.errorDetail(status))
	}

	results := make([]Result, 0, len(reqs))
	created := false
	for _, req := range reqs {
		result := processRequest(a, session, req)
		created = created || result.Patched()
		results = append(results, result)
	}
	if created {
		if status := a.saveSettings(session); status != statusOK {
			return nil, fmt.Errorf("NvAPI_DRS_SaveSettings failed: %s", a.errorDetail(status))
		}
	}
	return results, nil
}

// processRequest resolves the request's profile, checks whether the application
// is already registered and, when it is not, adds it to the profile.
func processRequest(a *api, session uintptr, req Request) Result {
	if req.App == "" {
		return newResult(StatusFailed, req, req.Profile, "empty application string")
	}
	handle, profile, res, ok := resolveProfile(a, session, req)
	if !ok {
		return res
	}
	present, err := a.appRegistered(session, handle, req.App)
	if err != nil {
		return newResult(StatusFailed, req, profile, err.Error())
	}
	if present {
		return newResult(StatusAlreadyPresent, req, profile)
	}
	app, err := buildApplication(req.App)
	if err != nil {
		return newResult(StatusFailed, req, profile, err.Error())
	}
	switch status := a.createApplication(session, handle, app); status {
	case statusOK:
		return newResult(StatusPatched, req, profile)
	case statusExecutableAlreadyInUse:
		return newResult(StatusConflict, req, profile)
	default:
		return newResult(StatusFailed, req, profile, a.errorDetail(status))
	}
}

// appRegistered reports whether the profile already contains the application,
// comparing case-insensitively: the driver stores names in its own casing.
func (a *api) appRegistered(session, profile uintptr, app string) (bool, error) {
	names, err := a.applications(session, profile)
	if err != nil {
		return false, err
	}
	for _, name := range names {
		if strings.EqualFold(name, app) {
			return true, nil
		}
	}
	return false, nil
}

// resolveProfile returns the profile handle and name for a request: the exact
// name when the manifest pins one, otherwise the profile owning any of the
// fingerprint's executables. No profile is ever created.
func resolveProfile(a *api, session uintptr, req Request) (uintptr, string, Result, bool) {
	if req.Profile != "" {
		handle, status := a.findProfileByName(session, req.Profile)
		switch status {
		case statusOK:
			return handle, req.Profile, Result{}, true
		case statusProfileNotFound:
			return 0, req.Profile, newResult(StatusProfileNotFound, req, req.Profile), false
		default:
			return 0, req.Profile, newResult(StatusFailed, req, req.Profile, a.errorDetail(status)), false
		}
	}
	for _, candidate := range req.Candidates {
		handle, status := a.findApplicationByName(session, candidate)
		if status == statusExecutableNotFound {
			handle, status = a.findApplicationByName(session, strings.ToLower(candidate))
		}
		if status != statusOK {
			continue
		}
		info, infoStatus := a.getProfileInfo(session, handle)
		if infoStatus != statusOK {
			return 0, candidate, newResult(StatusFailed, req, candidate, a.errorDetail(infoStatus)), false
		}
		return handle, fromUTF16(info.profileName[:]), Result{}, true
	}
	return 0, "", newResult(StatusProfileUnresolved, req, ""), false
}
