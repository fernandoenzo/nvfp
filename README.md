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

`nvfp` adds missing UWP game entries to NVIDIA App's `fingerprint.db`, registers the process string in the NVIDIA driver profile, and supports manifest overrides, restore and NGX cache reset.

## The problem

NVIDIA App's XML database often omits UWP games. Adding an entry makes the app recognize them; registering the process string in the separate DRS database lets the driver apply the profile.

## Requirements

- Windows 11
- **NVIDIA App** installed (the modern one, not GeForce Experience)
- **Administrator privileges** for the driver-profile step — the program asks for them through the UAC prompt. Decline it (or pass `--no-driver`) and only `fingerprint.db` is patched. The `--reset-models` step also needs them, because `ProgramData\NVIDIA` is not writable by a normal user.
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

The patch only takes effect after you **log out of Windows and sign back in** — no reboot needed. Without that, relaunching NVIDIA App and refreshing the games list still shows nothing: the ontology is rebuilt at session start. The program prints the same reminder whenever it actually modifies `fingerprint.db`.

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

### Reset the NVIDIA NGX models folder

```powershell
.\nvfp.exe --reset-models
```

```
Deleted C:\ProgramData\NVIDIA\NGX\models
Note: log out of Windows and sign back in (no reboot needed); NVIDIA rebuilds the folder at the next session.
Note: after signing back in, open any DLSS game once (until its main menu) and close it before playing: NVIDIA creates the dlss payloads in models only on that first run.
```

The NVIDIA NGX OTA cache (the `models` folder) can end up incomplete: the NVIDIA
App's bootstrap rewrites `nvngx_config.txt` and drops the per-feature sections,
and the games silently fall back to their bundled plugins. Deleting the whole
folder is enough to fix it — at the next session NVIDIA rebuilds it completely
and with the optimal configuration, so there is no need to repair the manifest
by hand.

`--reset-models` resolves the folder from `OTACachePath` in
`HKLM\SOFTWARE\NVIDIA Corporation\Global\NGXCore`, falling back to
`C:\ProgramData\NVIDIA\NGX\models`, and refuses to delete anything whose last
path component is not `models`. It requests administrator privileges through
the UAC prompt (declining it means nothing is deleted and the error is
reported), reports `Nothing to do` when the folder does not exist, and
supports `--dry-run` (`Would delete <path>`, nothing touched).

The command is local and cannot be combined with `--list`, `--game`,
`--games-json`, `--doctor`, `--no-driver` or `--restore`. Run it after the
NVIDIA App has started and before launching the game; repeat it after a driver
or app update.

**After the session restart, warm the cache up once:** open any DLSS game,
wait until it reaches the main menu and close it; after that, play whatever you
want. NVIDIA App rebuilds `models` and every `sl_` bundle at session start, but
the `dlss`-family folders inside `models` are created only when a DLSS game
runs for the first time after the reset — until then, DLSS titles still find no
payloads.

### Skip the driver step

```powershell
.\nvfp.exe --no-driver
```

Patches `fingerprint.db` only. No UAC prompt, no NVAPI call: use it when you don't want to register anything in the driver profiles or when you lack administrator privileges.

### List games in the manifest

```powershell
.\nvfp.exe --list
```

The list starts with the database version and count, then prints each fingerprint and any available app/driver identity fields. The entries depend on the resolved `games.json`.

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

`--restore` is a local file operation: it ignores the manifest, cache and network, so it works offline. It cannot be combined with `--list`, `--game`, `--games-json`, `--doctor` or `--reset-models`. Combine it with `--dry-run` to preview the copy.

### Print the version

```powershell
.\nvfp.exe --version
```

`--version` prints the release, date and GPLv3+ notice. `-v` is a shorthand.

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
| `skip_driver` | bool | Leave the driver database untouched for this game. The `fingerprint.db` patch still runs; `driver_app` and `driver_profile` are ignored |
| `versions` | []string | Versions to ensure: `"uwp"` (created if missing) and/or `"steam"`, `"epic"`, etc. (updated if present). `"*"` alone means every version the game already has, plus `uwp` if it can be created |
| `overrides` | map | XML fields to overwrite or add in the version |
| `remove` | []string | XML fields to delete from the version |

`driver_app`, `driver_profile` and `app_user_model_id` are limited to 2047 UTF-16 units (the NVAPI limit, measured in code units, not runes); the manifest is rejected otherwise.

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

### Example with the driver step disabled for one game

Some games already work without touching the driver, so there is no reason to add an entry for them:

```json
{
  "fingerprint": "your_uwp_game",
  "app_user_model_id": "Pkg_abc123!AppGame",
  "skip_driver": true,
  "versions": ["uwp"]
}
```

`fingerprint.db` is still patched — that is what makes NVIDIA App see the game — but the driver profile is left alone. It is the per-game equivalent of `--no-driver`, and it also keeps the game out of the "driver work pending" count, so no UAC prompt is requested on its behalf.

Note: `"driver_app": ""` does **not** do this. An empty value is indistinguishable from an omitted one, so the package family name would be registered instead — the opposite of what you want. Use `skip_driver`.

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

With `--reset-models` instead, nothing above runs: it only deletes the NGX models folder, as described in [Reset the NVIDIA NGX models folder](#reset-the-nvidia-ngx-models-folder).

The patched games become visible only after a Windows logoff/logon (no reboot needed): the ontology is rebuilt at session start, and until then NVIDIA App keeps showing its cached list no matter how many times you refresh it. The program prints that reminder whenever it modifies the database.

No backup is created beside `fingerprint.db`: NVIDIA App keeps its pristine copy
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

**A driver profile matches by the name of the process that starts, and nothing else** — `NVDRS_APPLICATION.appName` is compared against the process image name; there is no package identity in the match. A packaged game can start in one of two ways, each exposing a different process name:

| How the game runs | Name the driver sees | What to register |
|---|---|---|
| Hosted Store app (`ApplicationFrameHost`/`WWAHost` keeps the package identity) | the **package family name** | the default: omit `driver_app` |
| Plain executable, even if bought in the Store | the **`.exe`** name | set `driver_app` to that `.exe` |

Two entries of the same saga can land on opposite sides: Remake applies its profile with the package family name (`"versions": ["uwp"]`, no `driver_app`), while Rebirth only applies it with `"driver_app": "ff7rebirth.exe"` and silently does nothing with the UWP ID. Both are correct — the games just launch differently. `app_user_model_id` is still needed on both: it is what patches `fingerprint.db`.

The `isMetro` flag, which sounds like it should declare "this is a Store app", is **ignored by the driver**: it reads back as 0 whatever you pass, so `nvfp` leaves it at 0 and the string is the only lever available.

### Checking which name a game actually needs

Look at the running process while the game is at its main menu, from an unprivileged PowerShell:

```powershell
Get-Process | Where-Object Name -like 'ff7*' | Format-Table Id, Name, Path -Auto
```

Read the `Name` column: the game's `.exe` (e.g. `ff7rebirth.exe`) → set `driver_app` to it; an app host (`ApplicationFrameHost`, `WWAHost`, `GameBar`) → the game is hosted and the package family name is right, leave `driver_app` unset. NvidiaProfileInspectorRevamped shows which registered string actually makes the profile apply: the one matching that process name.

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

The program checks its own token before touching anything. Unelevated, and with driver work pending (`--reset-models` requests it too, but only when the folder exists), it relaunches itself through `ShellExecuteExW`/`runas` with the same arguments plus an internal `--elevated` flag, waits for it and propagates its exit code. The child does the whole job — `fingerprint.db` included — so the prompt appears once, before any file is written. `shell32.dll` is resolved from `System32` only (`windows.NewLazySystemDLL`), so the search order cannot be hijacked by a DLL planted next to the executable.

The token is opened explicitly with `OpenProcessToken(CurrentProcess(), TOKEN_QUERY, ...)` and the elevation comes from `x/sys/windows.Token.IsElevated`: a failure to read it is reported as an error instead of being silently read as "not an administrator".

The elevated child runs in its own console window, which Windows closes the moment the process exits. Because the work takes milliseconds, the child holds it open with a `Press Enter to close this window...` prompt when its output is a real console; when the output is piped or redirected there is no window to lose, so no pause is added and scripts keep working.

If you decline the prompt, or the relaunch fails, the driver step is skipped with an explicit warning and `fingerprint.db` is still patched (exit code 0).

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

The `.syso` icon resource is committed, so `make build` needs no MinGW toolchain. After changing `nvfp.rc` or `nvfp.ico`, run `make resources` (requires `x86_64-w64-mingw32-windres`).

The build targets Windows amd64 with CGO disabled. Reproducible output requires the same Go toolchain, module versions and build flags; a cold module cache may need network access to download dependencies.

## License

This project is licensed under the [GNU General Public License v3 or later (GPLv3+)](https://choosealicense.com/licenses/gpl-3.0/).

**Icon Attribution:** Application icon sourced from [icon-icons.com](https://icon-icons.com/icon/nvidia/103939) by Jeremiah. License: [CC BY 4.0](https://creativecommons.org/licenses/by/4.0/).
