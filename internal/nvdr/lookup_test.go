package nvdr

import "testing"

// The driver stores whatever spelling an application was registered with, and
// fingerprint.db routinely spells paths with Windows backslashes. These cases
// reproduce the real data from a machine where automatic resolution silently
// failed: fingerprint.db said `End\Binaries\Win64\ff7rebirth_.exe` while the
// driver database contained `end/binaries/win64/ff7rebirth_.exe`.
func TestProfileLookupAttempts(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want []string
	}{
		{
			name: "backslashed path is also tried with forward slashes and bare",
			in:   `End\Binaries\Win64\ff7rebirth_.exe`,
			want: []string{
				`End\Binaries\Win64\ff7rebirth_.exe`,
				"end\\binaries\\win64\\ff7rebirth_.exe",
				"End/Binaries/Win64/ff7rebirth_.exe",
				"end/binaries/win64/ff7rebirth_.exe",
				"ff7rebirth_.exe",
			},
		},
		{
			name: "bare name is not duplicated",
			in:   "game.exe",
			want: []string{"game.exe"},
		},
		{
			name: "forward slashes are still lowercased",
			in:   "Dir/Game.exe",
			want: []string{"Dir/Game.exe", "dir/game.exe", "Game.exe"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := profileLookupAttempts(tt.in)
			for _, want := range tt.want {
				found := false
				for _, g := range got {
					if g == want {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("profileLookupAttempts(%q) misses %q; got %q", tt.in, want, got)
				}
			}
			seen := map[string]bool{}
			for _, g := range got {
				if seen[g] {
					t.Errorf("profileLookupAttempts(%q) repeats %q", tt.in, g)
				}
				seen[g] = true
			}
		})
	}
}
