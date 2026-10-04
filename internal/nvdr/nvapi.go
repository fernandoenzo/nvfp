// Package nvdr declares the NVAPI values the driver-profile step relies on.
// Everything here is platform-independent on purpose: the values are pinned by
// tests that must run on any machine, not only on Windows where they are used.
package nvdr

import "unsafe"

// Function IDs from NVIDIA/nvapi's nvapi_interface.h. Each ID is resolved
// through nvapi_QueryInterface at load time; the driver is free to move
// implementations, only these IDs are stable. A typo here compiles and would
// only fail against a real driver, so TestPinnedFunctionIDs locks them down.
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

// applicationV4 mirrors NVDRS_APPLICATION_V4: a version word, a predefined flag,
// five 2048-unit UTF-16 strings and a flags word before commandLine.
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

// MaxDriverString is the longest string an NVAPI unicode field can hold,
// excluding the terminating NUL, derived from the buffer the binding actually
// passes to the driver. internal/db keeps its own copy to validate the manifest
// without importing this package; TestDriverStringLimitMatchesNVAPI in
// main_test.go keeps the two in step.
const MaxDriverString = len(applicationV4{}.appName) - 1
