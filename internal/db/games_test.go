package db

import (
	"encoding/xml"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestLoadFromBytes(t *testing.T) {
	data := []byte(`{
		"version": 1,
		"games": [
			{
				"fingerprint": "test_game",
				"app_user_model_id": "Pkg_abc!AppX",
				"versions": ["uwp"]
			},
			{
				"fingerprint": "weird_game",
				"app_user_model_id": "SomePkg_abc123!AppCustom",
				"versions": ["steam", "uwp"],
				"overrides": {
					"DriverProfile": "custom_exe.exe"
				},
				"remove": ["WhisperModePopsFactor"]
			}
		]
	}`)

	db, err := LoadFromBytes(data)
	if err != nil {
		t.Fatalf("LoadFromBytes failed: %v", err)
	}

	if db.Version != 1 {
		t.Errorf("expected version 1, got %d", db.Version)
	}
	if len(db.Games) != 2 {
		t.Fatalf("expected 2 games, got %d", len(db.Games))
	}
	if db.Games[0].Fingerprint != "test_game" {
		t.Errorf("expected fingerprint test_game, got %s", db.Games[0].Fingerprint)
	}
	if db.Games[0].AppUserModelID != "Pkg_abc!AppX" {
		t.Errorf("expected app_user_model_id Pkg_abc!AppX, got %s", db.Games[0].AppUserModelID)
	}
	if db.Games[1].Overrides["DriverProfile"] != "custom_exe.exe" {
		t.Errorf("expected override DriverProfile=custom_exe.exe")
	}
	if len(db.Games[1].Remove) != 1 || db.Games[1].Remove[0] != "WhisperModePopsFactor" {
		t.Errorf("expected remove [WhisperModePopsFactor]")
	}
}

func TestValidElementName(t *testing.T) {
	tests := []struct {
		name string
		ok   bool
	}{
		{"DriverProfile", true},
		{"DLSS_Override_No_OPS", true},
		{"ok-name_1.x", true},
		{"_private", true},
		{"a", true},
		{"中文", false},
		{"café", false},
		{"ȫ", false},
		{"ﬁ", false},
		{"", false},
		{"1abc", false},
		{"-lead", false},
		{".dot", false},
		{"ns:foo", false},
		{"with space", false},
		{"<evil>", false},
		{"a:b", false},
		{"a\tb", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ValidElementName(tt.name); got != tt.ok {
				t.Errorf("ValidElementName(%q) = %v, want %v", tt.name, got, tt.ok)
			}
		})
	}
}

// TestValidElementNameMatchesParser pins the rule to what encoding/xml accepts:
// the patched database must re-parse in the tool itself.
func TestValidElementNameMatchesParser(t *testing.T) {
	candidates := []string{"DriverProfile", "ok-name_1.x", "_private", "中文", "café", "ȫ", "ﬁ", "ǅ", "1abc", "ns:foo", "with space", "<evil>", ""}
	for _, name := range candidates {
		if !ValidElementName(name) {
			continue
		}
		out, err := xml.Marshal(struct {
			XMLName xml.Name
			Value   string `xml:",chardata"`
		}{XMLName: xml.Name{Local: name}, Value: "v"})
		if err != nil {
			t.Errorf("ValidElementName(%q) accepted, marshal failed: %v", name, err)
			continue
		}
		var back struct {
			XMLName xml.Name
			Value   string `xml:",chardata"`
		}
		if err := xml.Unmarshal(out, &back); err != nil {
			t.Errorf("ValidElementName(%q) accepted but %q does not re-parse: %v", name, out, err)
		}
	}
}

func TestUWPPackageFamilyName(t *testing.T) {
	tests := []struct {
		appID    string
		expected string
	}{
		{"39EA002F.EXED1_n746a19ndrrjg!AppFINALFANTASYVIIREMAKEShipping", "39EA002F.EXED1_n746a19ndrrjg"},
		{"Pkg_abc!AppX", "Pkg_abc"},
		{"NoBangHere", "NoBangHere"},
	}

	for _, tt := range tests {
		g := Game{AppUserModelID: tt.appID}
		result := g.UWPPackageFamilyName()
		if result != tt.expected {
			t.Errorf("UWPPackageFamilyName(%q) = %q, want %q", tt.appID, result, tt.expected)
		}
	}
}

func TestPackageFamilyName(t *testing.T) {
	tests := []struct {
		appID    string
		expected string
	}{
		{"39EA002F.EXED1_n746a19ndrrjg!AppFINALFANTASYVIIREMAKEShipping", "39EA002F.EXED1_n746a19ndrrjg"},
		{"Pkg_abc!AppX", "Pkg_abc"},
		{"NoBangHere", "NoBangHere"},
		{"", ""},
	}
	for _, tt := range tests {
		result := PackageFamilyName(tt.appID)
		if result != tt.expected {
			t.Errorf("PackageFamilyName(%q) = %q, want %q", tt.appID, result, tt.expected)
		}
	}
}

func TestLoadFromBytesValidation(t *testing.T) {
	t.Run("zero version rejected", func(t *testing.T) {
		data := []byte(`{"version":0,"games":[{"fingerprint":"x","app_user_model_id":"P!A","versions":["uwp"]}]}`)
		_, err := LoadFromBytes(data)
		if err == nil {
			t.Fatal("expected error for version 0")
		}
	})
	t.Run("empty games rejected", func(t *testing.T) {
		data := []byte(`{"version":1,"games":[]}`)
		_, err := LoadFromBytes(data)
		if err == nil {
			t.Fatal("expected error for empty games")
		}
	})
	t.Run("negative version rejected", func(t *testing.T) {
		data := []byte(`{"version":-1,"games":[{"fingerprint":"x","app_user_model_id":"P!A","versions":["uwp"]}]}`)
		_, err := LoadFromBytes(data)
		if err == nil {
			t.Fatal("expected error for negative version")
		}
	})
	t.Run("game without versions rejected", func(t *testing.T) {
		data := []byte(`{"version":1,"games":[{"fingerprint":"x","app_user_model_id":"P!A"}]}`)
		_, err := LoadFromBytes(data)
		if err == nil {
			t.Fatal("expected error for game without versions")
		}
	})
	t.Run("null game entry rejected instead of panicking", func(t *testing.T) {
		data := []byte(`{"version":1,"games":[null]}`)
		_, err := LoadFromBytes(data)
		if err == nil {
			t.Fatal("expected error for a null game entry")
		}
		if !strings.Contains(err.Error(), "game 0 is null") {
			t.Errorf("error = %v, want it to name the null entry", err)
		}
	})
	t.Run("null entry among valid games rejected", func(t *testing.T) {
		data := []byte(`{"version":1,"games":[{"fingerprint":"x","app_user_model_id":"P!A","versions":["uwp"]},null]}`)
		_, err := LoadFromBytes(data)
		if err == nil {
			t.Fatal("expected error for a null game entry")
		}
		if !strings.Contains(err.Error(), "game 1 is null") {
			t.Errorf("error = %v, want it to name the null entry", err)
		}
	})
	t.Run("invalid overrides key rejected", func(t *testing.T) {
		data := []byte(`{"version":1,"games":[{"fingerprint":"x","app_user_model_id":"P!A","versions":["uwp"],"overrides":{"<evil>":"1"}}]}`)
		_, err := LoadFromBytes(data)
		if err == nil {
			t.Fatal("expected error for an invalid overrides key")
		}
		if !strings.Contains(err.Error(), "overrides key") {
			t.Errorf("error = %v, want it to name the overrides key", err)
		}
	})
	t.Run("invalid remove entry rejected", func(t *testing.T) {
		data := []byte(`{"version":1,"games":[{"fingerprint":"x","app_user_model_id":"P!A","versions":["uwp"],"remove":["ns:foo"]}]}`)
		_, err := LoadFromBytes(data)
		if err == nil {
			t.Fatal("expected error for an invalid remove entry")
		}
		if !strings.Contains(err.Error(), "remove entry") {
			t.Errorf("error = %v, want it to name the remove entry", err)
		}
	})
}

func TestSaveAndLoad(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "games.json")

	db := &GameDB{
		Version: 1,
		Games: []*Game{
			{Fingerprint: "test", AppUserModelID: "Pkg!App", Versions: []string{"uwp"}},
		},
	}

	if err := SaveToPath(db, path); err != nil {
		t.Fatalf("SaveToPath failed: %v", err)
	}

	loaded, err := LoadFromPath(path)
	if err != nil {
		t.Fatalf("LoadFromPath failed: %v", err)
	}

	if loaded.Version != db.Version {
		t.Errorf("version mismatch: got %d, want %d", loaded.Version, db.Version)
	}
	if len(loaded.Games) != len(db.Games) {
		t.Errorf("games count mismatch: got %d, want %d", len(loaded.Games), len(db.Games))
	}
}

func TestResolveGamesBundled(t *testing.T) {
	data := []byte(`{"version":1,"games":[{"fingerprint":"test","app_user_model_id":"Pkg!App","versions":["uwp"]}]}`)

	db, err := ResolveGames(t.TempDir(), data, nil, true)
	if err != nil {
		t.Fatalf("ResolveGames failed: %v", err)
	}
	if len(db.Games) != 1 {
		t.Errorf("expected 1 game, got %d", len(db.Games))
	}
}

func TestResolveGamesRemote(t *testing.T) {
	bundled := []byte(`{"version":1,"games":[{"fingerprint":"bundled","app_user_model_id":"Pkg!App","versions":["uwp"]}]}`)
	remote := []byte(`{"version":2,"games":[{"fingerprint":"remote","app_user_model_id":"Pkg!App2","versions":["uwp"]}]}`)

	cacheDir := t.TempDir()
	db, err := ResolveGames(cacheDir, bundled, remote, true)
	if err != nil {
		t.Fatalf("ResolveGames failed: %v", err)
	}
	if db.Version != 2 {
		t.Errorf("expected remote version 2, got %d", db.Version)
	}
	if db.Games[0].Fingerprint != "remote" {
		t.Errorf("expected remote game, got %s", db.Games[0].Fingerprint)
	}

	cachePath := filepath.Join(cacheDir, "games.json")
	if _, err := os.Stat(cachePath); err != nil {
		t.Errorf("cache file not created: %v", err)
	}
}

func TestResolveGamesNoCacheDir(t *testing.T) {
	bundled := []byte(`{"version":1,"games":[{"fingerprint":"bundled","app_user_model_id":"Pkg!App","versions":["uwp"]}]}`)
	remote := []byte(`{"version":2,"games":[{"fingerprint":"remote","app_user_model_id":"Pkg!App2","versions":["uwp"]}]}`)

	// Empty cacheDir: remote is used but nothing is written to disk.
	db, err := ResolveGames("", bundled, remote, true)
	if err != nil {
		t.Fatalf("ResolveGames failed: %v", err)
	}
	if db.Games[0].Fingerprint != "remote" {
		t.Errorf("expected remote game, got %s", db.Games[0].Fingerprint)
	}

	// Empty cacheDir with no remote: bundled fallback, no cache read.
	db, err = ResolveGames("", bundled, nil, true)
	if err != nil {
		t.Fatalf("ResolveGames failed: %v", err)
	}
	if db.Games[0].Fingerprint != "bundled" {
		t.Errorf("expected bundled game, got %s", db.Games[0].Fingerprint)
	}
}

func TestResolveGamesNoCacheWrite(t *testing.T) {
	bundled := []byte(`{"version":1,"games":[{"fingerprint":"bundled","app_user_model_id":"Pkg!App","versions":["uwp"]}]}`)
	remote := []byte(`{"version":2,"games":[{"fingerprint":"remote","app_user_model_id":"Pkg!App2","versions":["uwp"]}]}`)

	cacheDir := t.TempDir()
	cached := []byte(`{"version":3,"games":[{"fingerprint":"cached","app_user_model_id":"Pkg!App3","versions":["uwp"]}]}`)
	cachePath := filepath.Join(cacheDir, "games.json")
	if err := os.WriteFile(cachePath, cached, 0o644); err != nil {
		t.Fatalf("writing cache: %v", err)
	}

	db, err := ResolveGames(cacheDir, bundled, remote, false)
	if err != nil {
		t.Fatalf("ResolveGames failed: %v", err)
	}
	if db.Games[0].Fingerprint != "remote" {
		t.Errorf("expected remote game, got %s", db.Games[0].Fingerprint)
	}
	got, err := os.ReadFile(cachePath)
	if err != nil {
		t.Fatalf("reading cache: %v", err)
	}
	if string(got) != string(cached) {
		t.Errorf("cache was rewritten in read-only mode: %s", got)
	}

	db, err = ResolveGames(cacheDir, bundled, nil, false)
	if err != nil {
		t.Fatalf("ResolveGames failed: %v", err)
	}
	if db.Games[0].Fingerprint != "cached" {
		t.Errorf("expected cached game, got %s", db.Games[0].Fingerprint)
	}
}

func TestResolveGamesFallbackToBundled(t *testing.T) {
	bundled := []byte(`{"version":1,"games":[{"fingerprint":"bundled","app_user_model_id":"Pkg!App","versions":["uwp"]}]}`)
	cacheDir := t.TempDir()

	db, err := ResolveGames(cacheDir, bundled, nil, true)
	if err != nil {
		t.Fatalf("ResolveGames failed: %v", err)
	}
	if db.Games[0].Fingerprint != "bundled" {
		t.Errorf("expected bundled game, got %s", db.Games[0].Fingerprint)
	}
}

func TestResolveGamesRemoteParseError(t *testing.T) {
	bundled := []byte(`{"version":1,"games":[{"fingerprint":"bundled","app_user_model_id":"Pkg!App","versions":["uwp"]}]}`)
	remote := []byte(`not json`)
	cacheDir := t.TempDir()
	db, err := ResolveGames(cacheDir, bundled, remote, true)
	if err != nil {
		t.Fatalf("ResolveGames should fall back to bundled on remote parse error: %v", err)
	}
	if db.Games[0].Fingerprint != "bundled" {
		t.Errorf("expected bundled fallback, got %s", db.Games[0].Fingerprint)
	}
}

func TestResolveGamesCorruptCache(t *testing.T) {
	bundled := []byte(`{"version":1,"games":[{"fingerprint":"bundled","app_user_model_id":"Pkg!App","versions":["uwp"]}]}`)

	captureStderr := func(t *testing.T, fn func()) string {
		t.Helper()
		r, w, err := os.Pipe()
		if err != nil {
			t.Fatalf("os.Pipe: %v", err)
		}
		orig := os.Stderr
		os.Stderr = w
		defer func() { os.Stderr = orig }()

		fn()

		w.Close()
		out, err := io.ReadAll(r)
		if err != nil {
			t.Fatalf("reading stderr pipe: %v", err)
		}
		return string(out)
	}

	t.Run("corrupt cache falls back to bundled with warning", func(t *testing.T) {
		cacheDir := t.TempDir()
		if err := os.WriteFile(filepath.Join(cacheDir, "games.json"), []byte(`not json`), 0o644); err != nil {
			t.Fatalf("writing corrupt cache: %v", err)
		}

		var gameDB *GameDB
		var err error
		stderr := captureStderr(t, func() {
			gameDB, err = ResolveGames(cacheDir, bundled, nil, true)
		})
		if err != nil {
			t.Fatalf("ResolveGames should fall back to bundled on corrupt cache: %v", err)
		}
		if gameDB.Games[0].Fingerprint != "bundled" {
			t.Errorf("expected bundled fallback, got %s", gameDB.Games[0].Fingerprint)
		}
		if !strings.Contains(stderr, "unusable") {
			t.Errorf("expected corrupt-cache warning on stderr, got %q", stderr)
		}
	})

	t.Run("missing cache does not warn", func(t *testing.T) {
		cacheDir := t.TempDir() // empty: cache file does not exist

		var gameDB *GameDB
		var err error
		stderr := captureStderr(t, func() {
			gameDB, err = ResolveGames(cacheDir, bundled, nil, true)
		})
		if err != nil {
			t.Fatalf("ResolveGames failed: %v", err)
		}
		if gameDB.Games[0].Fingerprint != "bundled" {
			t.Errorf("expected bundled fallback, got %s", gameDB.Games[0].Fingerprint)
		}
		if stderr != "" {
			t.Errorf("missing cache should not warn, got %q", stderr)
		}
	})
}

func TestLoadFromBytes_VersionRequests(t *testing.T) {
	tests := []struct {
		name    string
		game    string
		wantErr bool
	}{
		{"wildcard alone", `{"fingerprint":"x","versions":["*"]}`, false},
		{"wildcard padded and uppercase", `{"fingerprint":"x","versions":[" * "]}`, false},
		{"wildcard with another version", `{"fingerprint":"x","versions":["*","uwp"]}`, true},
		{"uwp without app_user_model_id", `{"fingerprint":"x","versions":["uwp"]}`, true},
		{"uwp uppercase without app_user_model_id", `{"fingerprint":"x","versions":["UWP"]}`, true},
		{"steam only without app_user_model_id", `{"fingerprint":"x","versions":["steam"]}`, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data := []byte(`{"version":1,"games":[` + tt.game + `]}`)
			_, err := LoadFromBytes(data)
			if tt.wantErr && err == nil {
				t.Error("expected an error, got nil")
			}
			if !tt.wantErr && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestGameVersionKeys(t *testing.T) {
	g := Game{Versions: []string{" Steam ", "UWP", "gog"}}
	got := g.VersionKeys()
	for _, want := range []string{"steam", "uwp", "gog"} {
		if !got.Contains(want) {
			t.Errorf("VersionKeys() missing %q", want)
		}
	}
	if got.Len() != 3 {
		t.Errorf("VersionKeys() len = %d, want 3", got.Len())
	}
	if g.VersionKeys() != got {
		t.Error("VersionKeys() should return the cached set on repeated calls")
	}
}

func TestLoadFromBytes_DriverFields(t *testing.T) {
	data := []byte(`{
		"version": 1,
		"games": [
			{
				"fingerprint": "uwp_default",
				"app_user_model_id": "Pkg_abc!AppX",
				"versions": ["uwp"]
			},
			{
				"fingerprint": "uwp_override",
				"app_user_model_id": "Pkg_abc!AppX",
				"versions": ["uwp"],
				"driver_app": ["Other.exe"],
				"driver_profile": "Some Profile"
			},
			{
				"fingerprint": "plain_steam",
				"versions": ["steam"]
			},
			{
				"fingerprint": "skipped",
				"app_user_model_id": "Pkg_skip!AppX",
				"versions": ["uwp"],
				"skip_driver": true
			},
			{
				"fingerprint": "skipped_with_override",
				"app_user_model_id": "Pkg_skip2!AppX",
				"versions": ["uwp"],
				"driver_app": ["Ignored.exe"],
				"driver_profile": "Ignored Profile",
				"skip_driver": true
			}
		]
	}`)

	got, err := LoadFromBytes(data)
	if err != nil {
		t.Fatalf("LoadFromBytes failed: %v", err)
	}
	if got.Games[0].DriverApps != nil || got.Games[0].DriverProfile != "" {
		t.Errorf("unset driver fields should stay empty, got %v/%q",
			got.Games[0].DriverApps, got.Games[0].DriverProfile)
	}
	if len(got.Games[1].DriverApps) != 1 || got.Games[1].DriverApps[0] != "Other.exe" {
		t.Errorf("driver_app = %v, want [Other.exe]", got.Games[1].DriverApps)
	}
	if got.Games[1].DriverProfile != "Some Profile" {
		t.Errorf("driver_profile = %q, want Some Profile", got.Games[1].DriverProfile)
	}
	if !got.Games[3].SkipDriver {
		t.Error("skip_driver did not survive parsing")
	}

	tests := []struct {
		name string
		game Game
		want []string
	}{
		{"package family name by default", *got.Games[0], []string{"Pkg_abc"}},
		{"override is additive after the package family name", *got.Games[1], []string{"Pkg_abc", "Other.exe"}},
		{"no UWP identity and no override", *got.Games[2], nil},
		{"override without app id", Game{DriverApps: []string{"Solo.exe"}}, []string{"Solo.exe"}},
		{"skip_driver suppresses every string", *got.Games[3], nil},
		{"skip_driver beats explicit driver_app", *got.Games[4], nil},
		{"duplicate of the package family name is dropped", Game{AppUserModelID: "Pkg!App", DriverApps: []string{"pkg", "Pkg"}}, []string{"Pkg"}},
		{"manifest order is preserved after the package family name", Game{AppUserModelID: "Pkg!App", DriverApps: []string{"b.exe", "a.exe"}}, []string{"Pkg", "b.exe", "a.exe"}},
		// strings.ToLower maps U+0130 (İ) and U+0049/U+0069 (I/i) to the same key
		// while strings.EqualFold — the comparison the driver itself uses — keeps
		// them apart. Deduplicating on the lowered form would drop an entry the
		// driver still needs registered.
		{"dotted capital I is not a duplicate of i", Game{DriverApps: []string{"\u0130.exe", "i.exe"}}, []string{"\u0130.exe", "i.exe"}},
		{"dotted capital I does not collide with the package family name", Game{AppUserModelID: "i!App", DriverApps: []string{"\u0130.exe"}}, []string{"i", "\u0130.exe"}},
		{"ASCII case duplicates are still dropped", Game{DriverApps: []string{"Game.exe", "GAME.EXE"}}, []string{"Game.exe"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.game.DriverAppStrings()
			if !slices.Equal(got, tt.want) {
				t.Errorf("DriverAppStrings() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestLoadFromBytes_DriverValidation(t *testing.T) {
	// 2048 UTF-16 units is one more than the NVAPI limit.
	tooLong := strings.Repeat("a", 2048)
	asciiOK := strings.Repeat("a", 2047)
	// Astral-plane runes cost two UTF-16 units each, so 1024 of them exceed
	// the limit even though the string is shorter than 2047 runes.
	surrogatePair := strings.Repeat("\U0001F600", 1024)
	surrogateOK := strings.Repeat("\U0001F600", 1023) + strings.Repeat("a", 1)

	tests := []struct {
		name    string
		game    string
		wantErr bool
	}{
		{"driver_app at the limit", `{"driver_app":["` + asciiOK + `"]}`, false},
		{"driver_app over the limit", `{"driver_app":["` + tooLong + `"]}`, true},
		{"driver_app surrogate pairs over", `{"driver_app":["` + surrogatePair + `"]}`, true},
		{"driver_app surrogate pairs at the limit", `{"driver_app":["` + surrogateOK + `"]}`, false},
		{"empty driver_app entry", `{"driver_app":["ok.exe",""]}`, true},
		{"driver_profile over the limit", `{"driver_profile":"` + tooLong + `"}`, true},
		{"app_user_model_id over the limit", `{"app_user_model_id":"` + tooLong + `"}`, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data := []byte(`{"version":1,"games":[{"fingerprint":"x","versions":["steam"],` + tt.game[1:] + `]}`)
			_, err := LoadFromBytes(data)
			if tt.wantErr && err == nil {
				t.Error("expected an error, got nil")
			}
			if !tt.wantErr && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}
