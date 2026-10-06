package main

import (
	"testing"

	"github.com/fernandoenzo/nvfp/internal/db"
	"github.com/fernandoenzo/nvfp/internal/nvdr"
)

func TestHasDriverWork(t *testing.T) {
	tests := []struct {
		name  string
		games []*db.Game
		// filter is applied through the --game flag inside the test
		filter string
		want   bool
	}{
		{
			name: "game with app id",
			games: []*db.Game{
				{Fingerprint: "uwp_game", AppUserModelID: "Pkg!App", Versions: []string{"uwp"}},
			},
			want: true,
		},
		{
			name: "explicit driver_app without app id",
			games: []*db.Game{
				{Fingerprint: "exe_game", DriverApp: "Game.exe", Versions: []string{"steam"}},
			},
			want: true,
		},
		{
			name: "no game has a driver string",
			games: []*db.Game{
				{Fingerprint: "plain_game", Versions: []string{"steam"}},
			},
			want: false,
		},
		{
			name: "filter selects a game without app id",
			games: []*db.Game{
				{Fingerprint: "uwp_game", AppUserModelID: "Pkg!App", Versions: []string{"uwp"}},
				{Fingerprint: "plain_game", Versions: []string{"steam"}},
			},
			filter: "plain_game",
			want:   false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			oldFilter := gameFilter
			defer func() { gameFilter = oldFilter }()
			gameFilter = tt.filter

			got := hasDriverWork(&db.GameDB{Version: 1, Games: tt.games})
			if got != tt.want {
				t.Errorf("hasDriverWork() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestDriverRequests(t *testing.T) {
	fdb := newTestFingerprintDB(t)
	gameDB := &db.GameDB{
		Version: 1,
		Games: []*db.Game{
			{Fingerprint: "final_fantasy_vii_remake", AppUserModelID: "39EA002F.EXED1_n746a19ndrrjg!AppFINALFANTASYVIIREMAKEShipping", Versions: []string{"uwp"}},
			{Fingerprint: "already_uwp_game", DriverApp: "Override.exe", DriverProfile: "The Profile", Versions: []string{"steam"}},
			{Fingerprint: "plain_steam_game", Versions: []string{"steam"}},
			{Fingerprint: "missing_from_fingerprint_db", AppUserModelID: "Pkg!App", Versions: []string{"uwp"}},
			{Fingerprint: "skipped_driver_game", AppUserModelID: "Pkg_skip!App", SkipDriver: true, Versions: []string{"uwp"}},
		},
	}

	oldFilter := gameFilter
	defer func() { gameFilter = oldFilter }()
	gameFilter = ""

	reqs := driverRequests(gameDB, fdb)
	if len(reqs) != 3 {
		t.Fatalf("driverRequests() returned %d requests, want 3: %+v", len(reqs), reqs)
	}

	if reqs[0].App != "39EA002F.EXED1_n746a19ndrrjg" {
		t.Errorf("reqs[0].App = %q, want the package family name", reqs[0].App)
	}
	if reqs[0].Profile != "" {
		t.Errorf("reqs[0].Profile = %q, want empty (automatic resolution)", reqs[0].Profile)
	}
	want := []string{"FF7R.exe", "FF7R_Epic.exe"}
	if len(reqs[0].Candidates) != len(want) {
		t.Fatalf("reqs[0].Candidates = %v, want %v", reqs[0].Candidates, want)
	}
	for i := range want {
		if reqs[0].Candidates[i] != want[i] {
			t.Errorf("reqs[0].Candidates[%d] = %q, want %q", i, reqs[0].Candidates[i], want[i])
		}
	}

	if reqs[1].App != "Override.exe" {
		t.Errorf("reqs[1].App = %q, want Override.exe", reqs[1].App)
	}
	if reqs[1].Profile != "The Profile" {
		t.Errorf("reqs[1].Profile = %q, want The Profile", reqs[1].Profile)
	}

	// A game whose fingerprint is absent has no candidates to try.
	if reqs[2].Fingerprint != "missing_from_fingerprint_db" || len(reqs[2].Candidates) != 0 {
		t.Errorf("reqs[2] = %+v, want no candidates", reqs[2])
	}

	// skip_driver keeps a game out of the batch even when it has a UWP identity.
	for _, req := range reqs {
		if req.Fingerprint == "skipped_driver_game" {
			t.Errorf("skip_driver game is still in the batch: %+v", req)
		}
	}
	if hasDriverWork(&db.GameDB{Version: 1, Games: []*db.Game{gameDB.Games[4]}}) {
		t.Error("hasDriverWork() is true for a skip_driver game alone")
	}

	// The --game filter narrows the request list as well.
	gameFilter = "plain_steam_game"
	if got := driverRequests(gameDB, fdb); len(got) != 0 {
		t.Errorf("driverRequests() with --game plain_steam_game = %+v, want none", got)
	}
	gameFilter = "final_fantasy_vii_remake"
	if got := driverRequests(gameDB, fdb); len(got) != 1 || got[0].Fingerprint != "final_fantasy_vii_remake" {
		t.Errorf("driverRequests() with --game final_fantasy_vii_remake = %+v, want one", got)
	}
}

// db.MaxDriverString duplicates nvdr.MaxDriverString on purpose: internal/db
// validates the manifest without importing internal/nvdr, so the two must be
// kept in step by hand. nvdr derives its copy from the buffer the binding hands
// to the driver, which is the one that actually matters.
func TestDriverStringLimitMatchesNVAPI(t *testing.T) {
	if db.MaxDriverString != nvdr.MaxDriverString {
		t.Errorf("db.MaxDriverString = %d, nvdr.MaxDriverString = %d",
			db.MaxDriverString, nvdr.MaxDriverString)
	}
	if nvdr.MaxDriverString != 2047 {
		t.Errorf("nvdr.MaxDriverString = %d, want 2047 (NVAPI_UNICODE_STRING_MAX - 1)", nvdr.MaxDriverString)
	}
}
