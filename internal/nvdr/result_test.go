package nvdr

import "testing"

func TestNewResultMessages(t *testing.T) {
	req := Request{
		Fingerprint: "final_fantasy_vii_remake",
		App:         "39EA002F.EXED1_n746a19ndrrjg",
		Profile:     "FF7R Profile",
		Candidates:  []string{"FF7R.exe", "FF7R_Epic.exe"},
	}

	tests := []struct {
		name    string
		status  Status
		profile string
		detail  string
		want    string
	}{
		{
			name:    "patched",
			status:  StatusPatched,
			profile: "FF7R Profile",
			want:    `added 39EA002F.EXED1_n746a19ndrrjg to driver profile "FF7R Profile"`,
		},
		{
			name:    "already present",
			status:  StatusAlreadyPresent,
			profile: "FF7R Profile",
			want:    `39EA002F.EXED1_n746a19ndrrjg already in driver profile "FF7R Profile"`,
		},
		{
			name:    "profile not found",
			status:  StatusProfileNotFound,
			profile: "FF7R Profile",
			want:    `driver profile "FF7R Profile" not found`,
		},
		{
			name:   "profile unresolved lists candidates",
			status: StatusProfileUnresolved,
			want:   `no driver profile found for "final_fantasy_vii_remake" (tried: FF7R.exe, FF7R_Epic.exe); set "driver_profile" in games.json`,
		},
		{
			name:    "conflict",
			status:  StatusConflict,
			profile: "Other Profile",
			want:    `39EA002F.EXED1_n746a19ndrrjg is already assigned to driver profile "Other Profile"`,
		},
		{
			name:    "failed with detail",
			status:  StatusFailed,
			profile: "FF7R Profile",
			detail:  "administrator privileges required",
			want:    `adding 39EA002F.EXED1_n746a19ndrrjg to driver profile "FF7R Profile" failed: administrator privileges required`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := newResult(tt.status, req, tt.profile, tt.detail)
			if got.Message != tt.want {
				t.Errorf("message = %q, want %q", got.Message, tt.want)
			}
			if got.Fingerprint != req.Fingerprint || got.App != req.App {
				t.Errorf("result lost request identity: %+v", got)
			}
			if got.Profile != tt.profile {
				t.Errorf("profile = %q, want %q", got.Profile, tt.profile)
			}
			if got.Patched() != (tt.status == StatusPatched) {
				t.Errorf("Patched() = %v for status %q", got.Patched(), tt.status)
			}
		})
	}

	t.Run("unresolved without candidates omits the tried list", func(t *testing.T) {
		got := newResult(StatusProfileUnresolved, Request{Fingerprint: "lonely_game"}, "")
		want := `no driver profile found for "lonely_game"; set "driver_profile" in games.json`
		if got.Message != want {
			t.Errorf("message = %q, want %q", got.Message, want)
		}
	})
}
