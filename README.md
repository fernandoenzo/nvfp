<p align="center">
  <img src="nvfp.svg" alt="nvfp" width="128">
</p>
<h1 align="center">nvfp</h1>

<p align="center">
  <img src="https://img.shields.io/github/go-mod/go-version/fernandoenzo/nvfp?color=00ADD8&logo=go&logoColor=white" alt="Go version">
  <img src="https://img.shields.io/badge/Windows-11-blue" alt="Windows 11">
  <img src="https://img.shields.io/badge/License-GPLv3+-red" alt="License: GPLv3+">
  <img src="https://img.shields.io/github/v/release/fernandoenzo/nvfp" alt="GitHub Release">
</p>

Patches the NVIDIA App profile database (`fingerprint.db`) so it recognizes **UWP / Microsoft Store** games that NVIDIA doesn't detect natively — registers them in the **NVIDIA driver's own profile database** so ReBAR/DLSS overrides actually apply — and tweaks existing game entries through a simple JSON manifest.

## The problem

NVIDIA App keeps an XML database (`fingerprint.db`) that maps games to their platform (Steam, Epic, GOG…). Many UWP games (Microsoft Store / Xbox PC) are missing from it, so NVIDIA App never applies graphics profiles to them, doesn't list them, and won't optimize them.

Fixing `fingerprint.db` alone is not enough. The driver keeps a separate profile database (`%ProgramData%\NVIDIA Corporation\Drs\nvdrsdb0.bin` / `nvdrsdb1.bin`) where every profile lists the executables that activate it. UWP profiles do not contain the package family name, so when the game launches the driver doesn't recognize it and injects nothing (no ReBAR, no DLSS overrides…). Tools like NvidiaProfileInspectorRevamped fix this by hand through NVAPI; this tool does it automatically, in the same pass as the `fingerprint.db` patch.

This tool locates both databases, patches `fingerprint.db` with the missing entries (or updates existing ones), registers the package family name in the matching driver profile through NVAPI, and can restore the pristine `fingerprint.db` afterwards.

## Requirements

- Windows 11
- **NVIDIA App** installed (the modern one, not GeForce Experience)
- **Administrator privileges** for the driver-profile step — the program asks for them through the UAC prompt. Decline it (or pass `--no-driver`) and only `fingerprint.db` is patched.
- **Windows Terminal** or **PowerShell 7** — do not use CMD. The program prints Unicode symbols (✓ ⊘ ✗) that CMD can't render.

## Usage

### Patch everything (default mode)

```powershell
.\nvfp.exe
```

```
Processing: C:\Users\You\AppData\Local\NVIDIA Corporation\NVIDIA App\NvBackend\ApplicationOntology\data\fingerprint.db
  ✓ added uwp version(s) of "final_fantasy_vii_remake"
  ⊘ fingerprint "starfield" already has uwp version(s)
  ✗ fingerprint "nonexistent_game" not found in database
Driver profiles:
  ✓ added 39EA002F.EXED1_n746a19ndrrjg to driver profile "Final Fantasy VII Remake"
  ⊘ F1Manager.exe already in driver profile "F1 Manager"
  ✗ adding SomePkg_abc to driver profile "Some Game" failed: administrator privileges required
```

A UAC prompt appears once before the driver profiles are written. The password/consent dialog is the only interruption: after it, both databases are patched in the same run.

What each symbol means:
- **✓** — patched (version added or updated; application registered in the driver profile)
- **⊘** — already correct, nothing to do
- **✗** — failed (fingerprint not found, no source version to build UWP from, driver profile missing or conflicting)

### Preview changes without writing anything

```powershell
.\nvfp.exe --dry-run
```

Same output, but nothing is written to disk. Useful to verify before running for real.

For the driver step, `--dry-run` prints the plan instead of the result and does not request elevation or touch the driver database:

```
Driver profiles:
  → would register 39EA002F.EXED1_n746a19ndrrjg in driver profile for "final_fantasy_vii_remake" (auto)
  → would register Custom.exe in driver profile "Named Profile"
```

`(auto)` means the profile is resolved from the fingerprint's `<DriverProfile>` executables at run time; nothing is consulted in dry-run mode.

### Diagnose the driver profiles

```powershell
.\nvfp.exe --doctor
```

Read-only and privilege-free: it asks the driver how each game's profile would resolve and prints every candidate it tried with the driver's own answer, then exits without writing anything. Use it whenever the driver step seems to do nothing — see [Diagnosing profile resolution](#diagnosing-profile-resolution-without-writing-anything).

### Skip the driver step

```powershell
.\nvfp.exe --no-driver
```

Patches `fingerprint.db` only. No UAC prompt, no NVAPI call: use it when you don't want to register anything in the driver profiles or when you lack administrator privileges.

### List games in the manifest

```powershell
.\nvfp.exe --list
```

```
Games database version: 1
Total games: 3

  final_fantasy_vii_remake
    AppUserModelId: 39EA002F.EXED1_n746a19ndrrjg!AppFINALFANTASYVIIREMAKEShipping
    UWPPackageFamilyName: 39EA002F.EXED1_n746a19ndrrjg
    DriverApp: 39EA002F.EXED1_n746a19ndrrjg
  epic_only_game
    AppUserModelId: 
    UWPPackageFamilyName: 
    DriverApp: Custom.exe
    DriverProfile: Named Profile
```

`DriverApp` and `DriverProfile` are only printed when set (or, for `DriverApp`, when the game has an `app_user_model_id` to derive it from).

### Patch a single game

```powershell
.\nvfp.exe --game final_fantasy_vii_remake
```

If the fingerprint doesn't exist in the manifest, the program exits with an error (non-zero exit code).

### Use your own local manifest

```powershell
.\nvfp.exe --games-json .\my-list.json
```

Ignores the remote manifest and the cache — uses your file exclusively. If the file is invalid, it fails loudly.

### Restore the original database

```powershell
.\nvfp.exe --restore
```

```
Restored C:\Users\You\AppData\Local\NVIDIA Corporation\NVIDIA App\NvBackend\ApplicationOntology\data\fingerprint.db
  from C:\Users\You\AppData\Local\NVIDIA Corporation\NVIDIA App\NvBackend\DAO\5a1f2b3c\fingerprint.db
```

Copies NVIDIA App's own pristine copy (kept under `NvBackend\DAO\<hash>\`) over the working database, undoing every patch. The working copy is recreated if it is missing.

### A note on `--restore`

`--restore` only restores `fingerprint.db`: the entries added to the driver profile database are **not** removed. They are additive, and NVIDIA may legitimately publish the same entries again through an OTA profile update. To remove a driver-profile entry, use NVIDIA Control Panel (Manage 3D settings) or NvidiaProfileInspectorRevamped, which expose the full DRS editing UI.

`--restore` is a purely local file operation: it ignores the manifest, the cache and the network, so it also works offline. It cannot be combined with `--list`, `--game` or `--games-json`. Combine it with `--dry-run` to see which copy would be restored without writing anything.

### Print the version

```powershell
.\nvfp.exe --version
```

```
nvfp 1.3.0 (2026 Oct 1)
Copyright © 2026 Fernando Enzo Guarini
License GPLv3+: GNU GPL version 3 or later <https://gnu.org/licenses/gpl.html>.
This is free software: you are free to change and redistribute it.
There is NO WARRANTY, to the extent permitted by law.

Written by Fernando Enzo Guarini.
```

`-v` is accepted as a shorthand.

## The manifest (`games.json`)

Defines which games to patch and how. The program downloads it automatically from this repo (with embedded copy and local cache as fallbacks).

### Format

```json
{
  "version": 1,
  "games": [
    {
      "fingerprint": "final_fantasy_vii_remake",
      "app_user_model_id": "39EA002F.EXED1_n746a19ndrrjg!AppFINALFANTASYVIIREMAKEShipping",
      "versions": ["uwp"]
    }
  ]
}
```

### Fields

| Field | Type | Description |
|---|---|---|
| `fingerprint` | string | Exact entry name in `fingerprint.db` (lowercase, underscores) |
| `app_user_model_id` | string | The UWP app's AppUserModelID: `PackageFamilyName!AppId`. Only needed for `uwp` versions |
| `driver_profile` | string | Exact profile name in the NVIDIA driver database. Omit to resolve the profile automatically from the fingerprint's `<DriverProfile>` executables |
| `driver_app` | string | String to register in the driver profile. Defaults to the package family name derived from `app_user_model_id`. Set it to the game's `.exe` when the game launches as a plain executable — see [`app_user_model_id` vs `driver_app`](#app_user_model_id-vs-driver_app-why-both-exist) |
| `versions` | []string | Versions to ensure: `"uwp"` (created if missing) and/or `"steam"`, `"epic"`, etc. (updated if present). `"*"` alone means every version the game already has, plus `uwp` if it can be created |
| `overrides` | map | XML fields to overwrite or add in the version |
| `remove` | []string | XML fields to delete from the version |

`driver_app` and `driver_profile` are at most 2047 UTF-16 units (the NVAPI limit, measured in UTF-16 code units, not runes); the manifest is rejected otherwise.

### Wildcard: patch every version at once

```json
{
  "fingerprint": "final_fantasy_vii_remake",
  "app_user_model_id": "39EA002F.EXED1_n746a19ndrrjg!AppFINALFANTASYVIIREMAKEShipping",
  "versions": ["*"],
  "overrides": {
    "DriverProfile": "FF7R.exe"
  }
}
```

This updates **every** version the fingerprint has (steam, epic…) with the same
overrides/removals, and creates a `uwp` version when `app_user_model_id` is set
and the fingerprint doesn't have one yet. `"*"` must be the only entry in
`versions`; the manifest is rejected otherwise.

### Example with overrides and removals

```json
{
  "fingerprint": "some_game",
  "app_user_model_id": "SomePkg_abc123!AppGame",
  "versions": ["uwp", "steam"],
  "overrides": {
    "DriverProfile": "SomeGame_UWP.exe"
  },
  "remove": ["WhisperModePopsFactor"]
}
```

This:
1. If `uwp` doesn't exist → creates it from the Steam version (or the first non-UWP one), with overrides and removals applied
2. If `steam` exists → updates it with the same overrides and removals
3. If `uwp` already exists and there are no overrides/removals → does nothing (idempotent)

### Example with a pinned driver profile

```json
{
  "fingerprint": "some_uwp_game",
  "app_user_model_id": "Pkg_abc123!AppGame",
  "versions": ["uwp"],
  "driver_profile": "Some Game",
  "driver_app": "Custom.exe"
}
```

This registers `Custom.exe` in the driver profile named `Some Game` instead of the package family name derived from `app_user_model_id`. Both fields are optional; see [Driver profiles](#driver-profiles).

### How do I find the `fingerprint` and the `app_user_model_id`?

**Fingerprint:** open `fingerprint.db` with a text editor and search for the game you want to patch. It's the `name` attribute of the `<Fingerprint>` element.

**AppUserModelID:** in PowerShell:

```powershell
Get-StartApps | Where-Object { $_.Name -like "*game name*" }
```

You'll get something like `39EA002F.EXED1_n746a19ndrrjg!AppFINALFANTASYVIIREMAKEShipping` — that's the full ID.

**Driver profile name** (only needed when automatic resolution fails): open NVIDIA Control Panel → Manage 3D settings → Program Settings, select the game and copy the exact profile name shown there. NvidiaProfileInspectorRevamped shows the same name in its profile list.

## What it does exactly

For each game with `versions: ["uwp"]`:

1. Finds the fingerprint in `fingerprint.db`
2. Finds the best source version (priority: Steam > first non-UWP)
3. Creates a new `uwp` version:
   - Removes store-specific fields (SteamAppIds, EpicAppId, Files, Launch…)
   - Adds `Distributor: UWP`, `UWPPackageFamilyName`, `AppUserModelId`
   - Applies your overrides and removals
4. Writes the patched database
5. Registers the UWP string (package family name by default) in the game's driver profile through NVAPI, unless `--no-driver`

Nothing is backed up next to it: the NVIDIA App keeps its own pristine copy
under `NvBackend\DAO\<hash>\fingerprint.db`, which this tool never touches.

## Driver profiles

Registering the UWP game in `fingerprint.db` makes NVIDIA App see it. Registering the package family name in the driver profile makes the **driver** recognize the running game, which is what actually applies ReBAR, DLSS overrides and the rest.

For each game with `app_user_model_id` (or an explicit `driver_app`), the program:

1. Resolves the driver profile:
   - `driver_profile` set → looked up by that exact name
   - otherwise → looks up each `<DriverProfile>` executable of the fingerprint (`FF7R.exe`, `FF7R_Epic.exe`…) until one matches a profile. The driver stores such names lowercased, so a lowercase attempt follows each miss.
2. Enumerates the profile's applications and skips the game if the string is already there (case-insensitive).
3. Otherwise adds the string (the package family name, e.g. `39EA002F.EXED1_n746a19ndrrjg`) with `NvAPI_DRS_CreateApplication`, leaving every other field at zero exactly like NvidiaProfileInspectorRevamped does.
4. Saves the database once per run, only if something was added.

No profile is ever **created**: when no candidate matches, the game is reported as unresolved and the output asks you to set `driver_profile` in the manifest with the exact name shown by NVIDIA Control Panel or NvidiaProfileInspectorRevamped. Creating ad-hoc profiles would pollute the driver database and could shadow NVIDIA's own profile for the game.

The step is idempotent: running the tool again reports `⊘ already in driver profile` and writes nothing. It requires administrator privileges, hence the UAC prompt.

### `app_user_model_id` vs `driver_app`: why both exist

This trips people up, so it is worth stating plainly: **a driver profile matches by the name of the process that starts, and nothing else.** `NVDRS_APPLICATION.appName` is documented as "String name of the Application" — the driver compares it against the process image name at launch. There is no notion of package identity in the match.

That matters because the same packaged game can start in one of two ways, and each exposes a different process name:

| How the game runs | Name the driver sees | What to register |
|---|---|---|
| Hosted Store app (`ApplicationFrameHost`/`WWAHost` keeps the package identity) | the **package family name** | the default: omit `driver_app` |
| Plain executable, even if bought in the Store | the **`.exe`** name | set `driver_app` to that `.exe` |

So the default (package family name, derived from `app_user_model_id`) is right for genuinely hosted apps, and wrong for games that ship as classic executables. Two entries of the same saga can land on opposite sides:

```json
{ "fingerprint": "final_fantasy_vii_remake",
  "app_user_model_id": "39EA002F.EXED1_n746a19ndrrjg!AppFINALFANTASYVIIREMAKEShipping",
  "versions": ["uwp"] }

{ "fingerprint": "final_fantasy_vii_rebirth",
  "app_user_model_id": "39EA002F.EXED2_n746a19ndrrjg!AppFINALFANTASYVIIREBIRTHShipping",
  "driver_app": "ff7rebirth.exe",
  "versions": ["*"] }
```

Remake applies its profile with the package family name; Rebirth only applies it with `ff7rebirth.exe`, and silently does nothing with the UWP ID. Both are correct — the games just launch differently. `app_user_model_id` is still needed on both: it is what patches `fingerprint.db` for NVIDIA App.

There is no way to tell from the manifest which case a game is, so it has to be checked at runtime (see below). The `isMetro` flag in `NVDRS_APPLICATION_V4`, which sounds like it should declare "this is a Store app", is **ignored by the driver**: it reads back as 0 whatever you pass. That is why `nvfp` leaves it at 0 and why the string is the only lever available.

### Checking which name a game actually needs

The reliable way is to look at the running process while the game is at its main menu, from an **unprivileged** PowerShell:

```powershell
# Every process whose name or package identity mentions the game.
Get-Process | Where-Object { $_.Path -like '*ff7rebirth*' -or $_.Name -like '*ff7*' } |
    Select-Object Id, Name, Path, @{n='Package';e={(Get-AppxPackage -ErrorAction SilentlyContinue |
        Where-Object { $_.InstallLocation -and $_.InstallLocation -like "*$($_.Name)*" }).PackageFamilyName}}

# Simpler and usually enough: what the driver will compare against is just Name.
Get-Process | Where-Object Name -like 'ff7*' | Format-Table Id, Name, Path -Auto
```

Read the `Name` column:

- It is the game's `.exe` (`ff7rebirth.exe`) → register that with `driver_app`.
- It is an app host (`ApplicationFrameHost`, `WWAHost`, `GameBar`) → the game is hosted and the **package family name** is the right string; leave `driver_app` unset.

You can confirm what the driver sees by listing the profile's registered strings with NvidiaProfileInspectorRevamped: the entry that makes the profile apply is exactly the one matching that process name. If the profile contains the package family name but the game launches as an `.exe`, the profile will be there and still not apply — which is the whole symptom.

### Diagnosing profile resolution without writing anything

`--doctor` reports exactly what the driver answers for every candidate, and **never writes and never asks for elevation**:

```
> nvfp.exe --doctor

Driver profile resolution (read-only, nothing is written):

final_fantasy_vii_rebirth
  application string : ff7rebirth.exe
  candidates         :
      ✓ end/binaries/win64/ff7rebirth_.exe          matched profile "FINAL FANTASY VII REBIRTH"
```

When nothing matches, every attempt is listed with the driver's own status, so the fix is visible instead of guessed:

```
      ✗ End\Binaries\Win64\ff7rebirth_.exe          NVAPI_EXECUTABLE_NOT_FOUND
      ✗ end\binaries\win64\ff7rebirth_.exe          NVAPI_EXECUTABLE_NOT_FOUND
      ✗ end/binaries/win64/ff7rebirth_.exe          NVAPI_EXECUTABLE_NOT_FOUND
      ✗ ff7rebirth_.exe                             NVAPI_EXECUTABLE_NOT_FOUND
  => unresolved: set "driver_profile" in games.json with the exact name from NVIDIA Control Panel
```

Use it first whenever the driver step appears to do nothing: it needs no privileges and produces output in the current console, so nothing can be missed.

### When the profile cannot be resolved

```
Driver profiles:
  ✗ no driver profile found for "some_uwp_game" (tried: SomeGame.exe, SomeGame_UWP.exe); set "driver_profile" in games.json
```

The fingerprint's executables matched no profile in the driver database. Open NVIDIA Control Panel (Manage 3D settings) or NvidiaProfileInspectorRevamped, find the game's profile and copy its exact name into the manifest:

```json
{
  "fingerprint": "some_uwp_game",
  "app_user_model_id": "Pkg_abc123!AppGame",
  "versions": ["uwp"],
  "driver_profile": "Some Game"
}
```

Run the tool again and the profile is found by name. If the game genuinely has no driver profile at all, that is NVIDIA's job to publish (it arrives with OTA profile updates); `nvfp` deliberately does not invent one.

### Elevation

The program checks its own token before touching anything. Unelevated, and with driver work pending, it relaunches itself through `ShellExecuteExW`/`runas` with the same arguments plus an internal `--elevated` flag, waits for it and propagates its exit code. The child does the whole job — `fingerprint.db` included — so the prompt appears once, before any file is written.

The token query used to pass a null `ReturnLength` to `GetTokenInformation`. That call is documented as requiring a valid pointer there, and it fails with `ERROR_INVALID_PARAMETER` — so **every** process, the elevated child included, reported itself as unelevated and the driver step was skipped with *"administrator privileges required"* even right after accepting the UAC prompt. It now passes a real length and checks the result, mirroring `x/sys/windows.Token.IsElevated`. A failure to read the token is reported as an error instead of being silently read as "not an administrator".

The elevated child runs in its own console window, which Windows closes the moment the process exits. Because the work takes milliseconds, that window used to flash by unread. The child now holds it open with a `Press Enter to close this window...` prompt when its output is a real console; when the output is piped or redirected there is no window to lose, so no pause is added and scripts keep working.

If you decline the prompt, or the relaunch fails, the driver step is skipped with an explicit warning and `fingerprint.db` is still patched (exit code 0).

### Why automatic resolution can miss

The fingerprint and the driver store paths differently. `fingerprint.db` routinely writes `<DriverProfile>` with Windows separators —

```
<DriverProfile>End\Binaries\Win64\ff7rebirth_.exe</DriverProfile>
```

— while the driver database holds the same entry with forward slashes (`end/binaries/win64/ff7rebirth_.exe`). A lookup that only tried the literal string, then its lowercase form, therefore failed even though the profile existed with the right executable in it.

Each candidate is now tried in four spellings — as written, forward-slashed, and each of those lowercased — plus the bare file name, because some entries are registered without their directory. This is why the failure looked like "the driver step does nothing": the lookup missed, the game was reported unresolved, and nothing was ever written. Use `--doctor` above to see the outcome for every spelling.

## Manifest resolution

The program looks for `games.json` in this order:

1. **Remote** (GitHub) — if online, downloads the latest version and caches it
2. **Local cache** (`%LOCALAPPDATA%\nvfp\games.json`) — if offline but a previous download exists
3. **Embedded** in the .exe — final fallback, always available

If the cache exists but is corrupt, it warns you and falls back to the embedded copy.

## Build

```bash
make build
```

Produces `nvfp.exe` for Windows amd64, with the application icon embedded as a
Windows PE resource.

### Windows icon resources

The icon lives in `nvfp.ico` and is declared in the resource script `nvfp.rc`:

```
1 ICON "nvfp.ico"
```

`x86_64-w64-mingw32-windres` compiles that script into
`nvfp_res_windows_amd64.syso`, a COFF object holding the `.rsrc` section that the
Go linker merges into the executable. Because the name ends in
`_windows_amd64`, the Go toolchain picks it up only when targeting Windows amd64 —
Linux builds and `go test ./...` are unaffected.

The `.syso` is committed, so `make build` needs no MinGW toolchain. Regenerate it
only after changing `nvfp.rc` or `nvfp.ico`:

```bash
make resources   # requires x86_64-w64-mingw32-windres (apt: binutils-mingw-w64-x86-64)
```

`make build` aborts with a clear message if the `.syso` is missing, so an
icon-less binary is never produced by accident.

### NVAPI binding

The driver-profile step talks to NVAPI through hard-coded function IDs, status
codes and struct layouts, so those values have to match NVIDIA's headers exactly.
They are **not written by hand**: `internal/nvdr/nvapi_gen.go` is generated by
`tools/nvapi-gen` from the [NVIDIA/nvapi](https://github.com/NVIDIA/nvapi)
headers at the commit pinned by `NVAPI_COMMIT` in the `Makefile`.

Every struct size, field offset and version word is verified by compiling and
running a small C probe against the struct text extracted from `nvapi.h`. The
measured numbers are pinned a second time in `internal/nvdr/nvapi_layout.go`
(compile-time asserts against the Go structs), and
`internal/nvdr/nvapi_provenance.md` records the commit plus the SHA-256 of each
header used.

The generated file is committed, so `make build`, `go test ./...` and `go vet`
need neither network access nor a C compiler. To move to a newer header version:

```bash
make nvapi-latest                        # print the newest upstream commit
make update-nvapi NVAPI_COMMIT=<sha>     # needs network and cc
git diff                                 # review, then commit
```

Regenerating against an unchanged commit is byte-identical and prints
`unchanged: all values match`. If NVIDIA moved something, the new value appears
in the `git diff` and `internal/nvdr/nvapi_pinned_test.go` fails until it is
reviewed and pinned deliberately.

### Reproducible builds

The build is fully reproducible: the same source code always produces the same binary, regardless of the machine, OS, or filesystem layout. This is achieved with:

- `-trimpath` — strips filesystem paths from the binary
- `-buildvcs=false` — excludes VCS metadata from build info
- `-ldflags="-s -w -buildid="` — strips debug info and build ID
- `CGO_ENABLED=0` — pure Go, no host C toolchain dependency
- `windres` writes no timestamps for icon resources, so the committed `.syso` — and therefore the `.exe` — is reproducible too
- the generated NVAPI binding is committed, so `make build` never reaches the network or the C compiler used to measure the layout

Two people building the same commit on different machines will get bit-for-bit identical binaries.

## License

This project is licensed under the [GNU General Public License v3 or later (GPLv3+)](https://choosealicense.com/licenses/gpl-3.0/).

**Icon Attribution:** Application icon sourced from [icon-icons.com](https://icon-icons.com/icon/nvidia/103939) by Jeremiah. License: [CC BY 4.0](https://creativecommons.org/licenses/by/4.0/).
