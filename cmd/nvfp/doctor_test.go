package main

import (
	"strings"
	"testing"

	"github.com/fernandoenzo/nvfp/internal/nvdr"
)

// TestReportRequest drives the --doctor report for the three request shapes:
// a candidate that matched, a candidate list that matched nothing, and a pinned
// profile looked up by name. It was untested until the pinned path was fixed.
func TestReportRequest(t *testing.T) {
	match := nvdr.LookupAttempt{Attempt: "ff7remake_.exe", Status: 0, Profile: "Final Fantasy VII Remake"}
	miss := nvdr.LookupAttempt{Attempt: "End/Binaries/Win64/game.exe", Status: -166}

	tests := []struct {
		name     string
		req      nvdr.Request
		attempts []nvdr.LookupAttempt
		want     []string
		absent   []string
	}{
		{
			name:     "candidate resolves and reports the matched profile",
			req:      nvdr.Request{Fingerprint: "final_fantasy_vii_remake", App: "39EA002F.EXED1_n746a19ndrrjg", Candidates: []string{"End\\Binaries\\Win64\\ff7remake_.exe"}},
			attempts: []nvdr.LookupAttempt{match},
			want:     []string{"final_fantasy_vii_remake", "39EA002F.EXED1_n746a19ndrrjg", "candidates", "✓", "Final Fantasy VII Remake"},
			absent:   []string{"unresolved"},
		},
		{
			name:     "candidates all miss and the report asks for driver_profile",
			req:      nvdr.Request{Fingerprint: "the_witcher_3", App: "CDPROJEKTSA.Witcher3GOTY_0xkrdv0gjmk28", Candidates: []string{"bin/x64/witcher3.exe"}},
			attempts: []nvdr.LookupAttempt{miss},
			want:     []string{"the_witcher_3", "✗", "NVAPI_EXECUTABLE_NOT_FOUND", "unresolved", "driver_profile"},
		},
		{
			name:     "no fingerprint entry explains why there are no candidates",
			req:      nvdr.Request{Fingerprint: "missing_game", App: "Pkg!App"},
			attempts: nil,
			want:     []string{"candidates         : none", "unresolved"},
		},
		{
			name:     "pinned profile names the lookup and the fix",
			req:      nvdr.Request{Fingerprint: "pinned_game", App: "game.exe", Profile: "Renamed Profile"},
			attempts: []nvdr.LookupAttempt{{Attempt: "Renamed Profile", Status: -163}},
			want:     []string{"pinned profile     : Renamed Profile", "✗", "NVAPI_PROFILE_NOT_FOUND", "no profile is ever created", "Renamed Profile"},
			absent:   []string{"candidates"},
		},
		{
			name:     "pinned profile that exists reports the match",
			req:      nvdr.Request{Fingerprint: "pinned_game", App: "game.exe", Profile: "Some Game"},
			attempts: []nvdr.LookupAttempt{{Attempt: "Some Game", Status: 0, Profile: "Some Game"}},
			want:     []string{"pinned profile     : Some Game", "✓", "Some Game"},
			absent:   []string{"unresolved"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out := captureStdout(t, func() { reportRequest(tt.req, tt.attempts) })
			for _, want := range tt.want {
				if !strings.Contains(out, want) {
					t.Errorf("report missing %q:\n%s", want, out)
				}
			}
			for _, absent := range tt.absent {
				if strings.Contains(out, absent) {
					t.Errorf("report should not contain %q:\n%s", absent, out)
				}
			}
		})
	}
}
