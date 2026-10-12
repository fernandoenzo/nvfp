package nvdr

import (
	"slices"
	"testing"
)

// The driver stores whatever spelling an application was registered with, and
// fingerprint.db routinely spells paths with Windows backslashes. These cases
// reproduce the real data from a machine where automatic resolution silently
// failed: fingerprint.db said `End\Binaries\Win64\ff7rebirth_.exe` while the
// driver database contained `end/binaries/win64/ff7rebirth_.exe`.
func TestProfileLookupAttempts(t *testing.T) {
	tests := []struct {
		name      string
		candidate string
		want      []string
	}{
		{
			name:      "windows separators get a forward-slash and a basename spelling",
			candidate: `End\Binaries\Win64\ff7rebirth_.exe`,
			want: []string{
				`End\Binaries\Win64\ff7rebirth_.exe`,
				`end\binaries\win64\ff7rebirth_.exe`,
				`End/Binaries/Win64/ff7rebirth_.exe`,
				`end/binaries/win64/ff7rebirth_.exe`,
				`ff7rebirth_.exe`,
			},
		},
		{
			name:      "a bare name keeps its single spelling and the lowered one",
			candidate: `Spider-Man2.exe`,
			want:      []string{`Spider-Man2.exe`, `spider-man2.exe`},
		},
		{
			name:      "a forward-slashed candidate is not duplicated",
			candidate: `bin/x64/witcher3.exe`,
			want:      []string{`bin/x64/witcher3.exe`, `witcher3.exe`},
		},
		{
			name:      "empty candidate yields nothing",
			candidate: "",
			want:      nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := profileLookupAttempts(tt.candidate)
			slices.Sort(got)
			want := slices.Clone(tt.want)
			slices.Sort(want)
			if !slices.Equal(got, want) {
				t.Errorf("profileLookupAttempts(%q) = %v, want %v", tt.candidate, got, want)
			}
		})
	}
}

// The spelling the driver actually holds must be among the attempts, or
// automatic resolution silently falls back to "unresolved".
func TestProfileLookupAttemptsCoverDriverSpellings(t *testing.T) {
	cases := []struct {
		candidate string
		stored    string
	}{
		{`End\Binaries\Win64\ff7remake_.exe`, `ff7remake_.exe`},
		{`End\Binaries\Win64\ff7rebirth_.exe`, `end/binaries/win64/ff7rebirth_.exe`},
		{`bin\x64\witcher3.exe`, `witcher3.exe`},
		{`Spider-Man2.exe`, `spider-man2.exe`},
	}
	for _, tc := range cases {
		if got := profileLookupAttempts(tc.candidate); !slices.Contains(got, tc.stored) {
			t.Errorf("candidate %q never tries the stored spelling %q (got %v)", tc.candidate, tc.stored, got)
		}
	}
}
