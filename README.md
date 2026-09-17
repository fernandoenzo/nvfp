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

Patches the NVIDIA App profile database (`fingerprint.db`) so it recognizes **UWP / Microsoft Store** games that NVIDIA doesn't detect natively — and tweaks existing game entries through a simple JSON manifest.

## The problem

NVIDIA App keeps an XML database (`fingerprint.db`) that maps games to their platform (Steam, Epic, GOG…). Many UWP games (Microsoft Store / Xbox PC) are missing from it, so NVIDIA App never applies graphics profiles to them, doesn't list them, and won't optimize them.

This tool locates that database, patches it with the missing entries (or updates existing ones), and can restore the pristine copy afterwards.

## Requirements

- Windows 11
- **NVIDIA App** installed (the modern one, not GeForce Experience)
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
```

What each symbol means:
- **✓** — patched (version added or updated)
- **⊘** — already correct, nothing to do
- **✗** — failed (fingerprint not found, or no source version to build UWP from)

### Preview changes without writing anything

```powershell
.\nvfp.exe --dry-run
```

Same output, but nothing is written to disk. Useful to verify before running for real.

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
```

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

`--restore` is a purely local file operation: it ignores the manifest, the cache and the network, so it also works offline. It cannot be combined with `--list`, `--game` or `--games-json`. Combine it with `--dry-run` to see which copy would be restored without writing anything.

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
| `versions` | []string | Versions to ensure: `"uwp"` (created if missing) and/or `"steam"`, `"epic"`, etc. (updated if present). `"*"` alone means every version the game already has, plus `uwp` if it can be created |
| `overrides` | map | XML fields to overwrite or add in the version |
| `remove` | []string | XML fields to delete from the version |

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

### How do I find the `fingerprint` and the `app_user_model_id`?

**Fingerprint:** open `fingerprint.db` with a text editor and search for the game you want to patch. It's the `name` attribute of the `<Fingerprint>` element.

**AppUserModelID:** in PowerShell:

```powershell
Get-StartApps | Where-Object { $_.Name -like "*game name*" }
```

You'll get something like `39EA002F.EXED1_n746a19ndrrjg!AppFINALFANTASYVIIREMAKEShipping` — that's the full ID.

## What it does exactly

For each game with `versions: ["uwp"]`:

1. Finds the fingerprint in `fingerprint.db`
2. Finds the best source version (priority: Steam > first non-UWP)
3. Creates a new `uwp` version:
   - Removes store-specific fields (SteamAppIds, EpicAppId, Files, Launch…)
   - Adds `Distributor: UWP`, `UWPPackageFamilyName`, `AppUserModelId`
   - Applies your overrides and removals
4. Writes the patched database

Nothing is backed up next to it: the NVIDIA App keeps its own pristine copy
under `NvBackend\DAO\<hash>\fingerprint.db`, which this tool never touches.

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

### Reproducible builds

The build is fully reproducible: the same source code always produces the same binary, regardless of the machine, OS, or filesystem layout. This is achieved with:

- `-trimpath` — strips filesystem paths from the binary
- `-buildvcs=false` — excludes VCS metadata from build info
- `-ldflags="-s -w -buildid="` — strips debug info and build ID
- `CGO_ENABLED=0` — pure Go, no host C toolchain dependency
- `windres` writes no timestamps for icon resources, so the committed `.syso` — and therefore the `.exe` — is reproducible too

Two people building the same commit on different machines will get bit-for-bit identical binaries.

## License

This project is licensed under the [GNU General Public License v3 or later (GPLv3+)](https://choosealicense.com/licenses/gpl-3.0/).

**Icon Attribution:** Application icon sourced from [icon-icons.com](https://icon-icons.com/icon/nvidia/103939) by Jeremiah. License: [CC BY 4.0](https://creativecommons.org/licenses/by/4.0/).
