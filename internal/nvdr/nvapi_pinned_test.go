package nvdr

import "testing"

// The NVAPI values in drs_windows.go are transcribed from NVIDIA's headers, so
// they are pinned here as literals: a typo in an ID or a status code compiles
// and would only fail against a real driver, which no test can reach. These
// cases turn such a mistake into a failing test instead.

func TestPinnedFunctionIDs(t *testing.T) {
	tests := []struct {
		name string
		got  uint32
		want uint32
	}{
		{"idInitialize", idInitialize, 0x0150E828},
		{"idGetErrorMessage", idGetErrorMessage, 0x6C2D048C},
		{"idCreateSession", idCreateSession, 0x0694D52E},
		{"idDestroySession", idDestroySession, 0xDAD9CFF8},
		{"idLoadSettings", idLoadSettings, 0x375DBD6B},
		{"idSaveSettings", idSaveSettings, 0xFCBC7E14},
		{"idGetProfileInfo", idGetProfileInfo, 0x61CD6FD6},
		{"idFindProfileByName", idFindProfileByName, 0x7E4A9A0B},
		{"idCreateApplication", idCreateApplication, 0x4347A9DE},
		{"idFindApplicationByName", idFindApplicationByName, 0xEEE566B2},
		{"idEnumApplications", idEnumApplications, 0x7FA2173A},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.got != tt.want {
				t.Errorf("%s = 0x%08X, want 0x%08X", tt.name, tt.got, tt.want)
			}
		})
	}
}

func TestPinnedStatuses(t *testing.T) {
	tests := []struct {
		name string
		got  int32
		want int32
	}{
		{"statusOK", statusOK, 0},
		{"statusInvalidArgument", statusInvalidArgument, -5},
		{"statusEndEnumeration", statusEndEnumeration, -7},
		{"statusInvalidUserPrivilege", statusInvalidUserPrivilege, -137},
		{"statusProfileNotFound", statusProfileNotFound, -163},
		{"statusExecutableNotFound", statusExecutableNotFound, -166},
		{"statusExecutableAlreadyInUse", statusExecutableAlreadyInUse, -167},
		{"statusAccessDenied", statusAccessDenied, -175},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.got != tt.want {
				t.Errorf("%s = %d, want %d", tt.name, tt.got, tt.want)
			}
		})
	}
}
