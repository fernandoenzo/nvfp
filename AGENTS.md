# Repository Guidelines

## Project Overview

CLI tool that patches the NVIDIA App fingerprint database to add UWP (Microsoft Store) game entries and registers those games in the NVIDIA driver profile database (DRS) through NVAPI. It locates the working fingerprint.db (ApplicationOntology\data) on Windows, patches it with game metadata from a bundled or remotely-fetched JSON manifest, relaunches itself elevated when the driver step needs it, can restore the pristine copy NVIDIA App keeps under NvBackend\DAO, and repairs the NGX OTA manifest (`nvngx_config.txt`) that Streamline's interposer reads (`--sl-override`).

## Architecture & Data Flow

```
games.json (bundled/embedded) ──┐
                                 ▼
                           resolveGames ──► db.ResolveGames
                                 │
findFingerprintDB ──► dbPath     │
                                 ▼
                            patchDB ──► fdb, modified
                                 │
                                 ▼
          ParseFingerprintDB ──► applyPatches ──► writePatch
                    │                              │
                    ▼                              ▼
            PatchGame per game          WriteFingerprintDB
                    │
                    ▼
          FindFingerprint → resolveVersions
          → FindSourceVersion → AddUWPVersion / UpdateVersion

                    ┌─────────── hasDriverWork ─► relaunchElevated (UAC, --elevated)
                    │
                    ▼
 driverRequests ──► nvdr.Apply ──► nvapi_QueryInterface
   (DriverAppString,         │
    DriverProfile,            ├─ DRS_CreateSession / LoadSettings
    DriverProfileCandidates)  ├─ FindProfileByName | FindApplicationByName
                              ├─ EnumApplications (already-present check)
                              └─ CreateApplication + SaveSettings

findDAOFingerprintDB ──┐  (--restore)
                       ▼
      getFingerprintDBPath ──► restoreDB ──► CopyFile → working fingerprint.db
```

Five-layer architecture:
1. **CLI layer** (`main.go`, `elevation_*.go`): Cobra commands (`newRootCmd`), flags (`--dry-run`, `--list`, `--restore`, `--game`, `--games-json`, `--no-driver`, `--doctor`, `--sl-override`, `--elevated`, `--version`), orchestration, UAC relaunch, NGX repair flow
2. **Data layer** (`internal/db`): Game manifest model, I/O, resolve fallback chain
3. **Core logic layer** (`internal/nvidia`): XML fingerprint parsing/patching, file copy
4. **Driver layer** (`internal/nvdr`): NVAPI DRS binding (Windows) + status/message types (all platforms)
5. **NGX layer** (`internal/ngx`): Streamline OTA manifest repair (all platforms; the cache path comes from the Windows-only `Root`, which tests bypass through the `ngxRoot` seam in `main.go`)
6. **Network layer** (`internal/update`): Remote games.json fetch

## Key Directories

| Directory | Purpose |
|---|---|
| `.` | Entry point (`main.go`), elevation files, embedded `games.json`, icon resources (`nvfp.rc`, `nvfp.ico`, `.syso`), Makefile |
| `internal/db/` | Game manifest model, JSON I/O, resolve logic |
| `internal/nvidia/` | Fingerprint XML parsing/patching, metadata handling |
| `internal/nvidia/testdata/` | XML fixture files for tests |
| `internal/nvdr/` | NVIDIA driver profile (DRS) binding: NVAPI loading, profile/application registration, result messages |
| `internal/ngx/` | Streamline OTA manifest (`nvngx_config.txt`) repair: feature-list parsing, missing-payload fill, idempotent section append |
| `internal/update/` | Remote games.json fetcher |

## Development Commands

```bash
# Build (cross-compiles for Windows amd64, reproducible)
make build
# Equivalent:
# CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -trimpath -buildvcs=false -ldflags="-s -w -buildid=" -o nvfp.exe .

# Regenerate the Windows resource object (app icon) from nvfp.rc — needs MinGW
make resources

# Run tests
make test
# Equivalent: go test ./...

# Run tests with verbose output
go test -v ./...

# Run specific package tests
go test -v ./internal/nvidia/...
go test -v ./internal/db/...

# Static checks (no Makefile target)
gofmt -l .
go vet ./...
```

No lint or coverage targets in the Makefile.

### Windows icon resources

The application icon is a Windows PE resource, not a Go embed. `nvfp.rc` declares
`1 ICON "nvfp.ico"` and `x86_64-w64-mingw32-windres` compiles it into
`nvfp_res_windows_amd64.syso`, which the Go linker merges into the `.exe` as a
`.rsrc` section. The `_windows_amd64` suffix makes the Go toolchain ignore the
object on every other GOOS/GOARCH, so plain `go build`/`go vet`/`go test` keep
working on Linux.

- The `.syso` is **committed**: `make build` must work without a MinGW toolchain
  installed. It is a generated artifact, so it only changes when `nvfp.rc` or
  `nvfp.ico` changes.
- `make build` fails with an explicit message when the `.syso` is missing, rather
  than silently producing an icon-less binary.
- `windres` output is deterministic (no timestamp is written for icons), so
  regenerating it yields a byte-identical file and the reproducible-build
  guarantee in `README.md` still holds.

## Code Conventions & Common Patterns

- **Function length**: Max 25 lines of code per function (comments excluded). Extract helpers early. One known exception: `resolveVersions` in `internal/nvidia/patch.go` (27).
- **Error handling**: Wrap errors with context using `fmt.Errorf("...: %w", err)`. Report non-fatal failures to stderr; return error only when the caller should abort.
- **Naming**: Standard Go conventions. Acronyms stay cased (`AppUserModelID`, `UWP`, `HTTP`).
- **Table-driven tests**: Use `[]struct{ name string; ... }` with `t.Run(tc.name, ...)`.
- **Temp files**: Always use `t.TempDir()` for isolation; never hardcode paths.
- **XML model**: Generic `XmlElement` struct (XMLName, Attr, Content, Children) for forward compatibility — no domain-specific structs for XML nodes.
- **PatchGame signature**: Takes `*FingerprintDB` + `*db.Game`. All game fields (fingerprint, appUserModelID, versions, remove, overrides) come from the struct.
- **Patch result**: `PatchResult` with `Status` + `Message` fields, typed as `PatchStatus`: `patched`, `already_present`, `not_found`, `no_source`, `version_not_found`.
- **Game resolution fallback**: Remote → cache → bundled (in that priority). An empty cacheDir disables the cache layer entirely (no read, no write). Cache lives in `%LOCALAPPDATA%\nvidia-uwp-patch` (falls back to `~/.cache/nvidia-uwp-patch` when `LOCALAPPDATA` is unset).
- **Forced field defaults**: `Distributor`, `UWPPackageFamilyName`, `AppUserModelId` are derived from the appUserModelID; user overrides take priority over these defaults.
- **JSON handling**: `Game` fields are `[]*Game` and `[]*Version`; the manifest uses `encoding/json/v2` with `json.Deterministic(true)` on save.
- **Manifest driver fields**: `driver_profile` (exact DRS profile name) and `driver_app` (string to register, defaults to the package family name) are optional and independent of `versions`: a game with `driver_app` but no `app_user_model_id` still produces a driver request. `skip_driver` (bool) opts a game out of the driver step entirely while keeping its `fingerprint.db` patch — it is the per-game `--no-driver`, and it also excludes the game from `hasDriverWork`, so no UAC is requested on its behalf. `Game.DriverAppString()` is the single source of truth for what gets registered: it returns empty for `skip_driver`, otherwise `driver_app` when set, otherwise the package family name. An empty `driver_app` cannot express "register nothing" because `omitempty` makes it indistinguishable from an omitted field.
- **UWP version modes**: `AddUWPVersion` (new version: default removals + forced fields from appUserModelID) vs `UpdateVersion` (existing version: only explicit removals, forced fields preserved).
- **Version requests**: `versions: ["*"]` means every version the fingerprint already has, plus a new `uwp` when the game has an `app_user_model_id` and lacks one. `"*"` is rejected unless it is the only entry. A requested `uwp` that gets created is never reported as missing. Lookup is `Game.VersionKeys()`, which lowercases and trims the names.
- **Version ordering**: versions are reported in fingerprint document order, never in set iteration order; per-version results go through `applyVersion`.
- **Deterministic output**: override elements are emitted sorted by lowercased key.
- **Source version priority**: Steam > first non-UWP version found.
- **Embedded resources**: `games.json` embedded via `//go:embed` and used as fallback.
- **Version banner**: `--version`/`-v` is a plain bool flag handled at the top of `run()`. The banner is assembled in `main.go` from the `version` and `versionDate` constants; bump both on every release. It does **not** use Cobra's `Command.Version`/`SetVersionTemplate`: that path pulls `cobra.tmpl` → `text/template` (+`reflect`) into the binary (~+1.9 MB). Its shape mirrors the author's other CLIs (name, version, date, copyright, GPLv3+ notice, author).
- **HTTP safeguards**: 10s timeout, 5MB `io.LimitReader`, custom `User-Agent` header.
- **No sidecar backups**: patching writes the working fingerprint.db in place. Undo is `--restore`, which copies the pristine `NvBackend\DAO\<hash>\fingerprint.db` (first subdirectory containing the file) over the working copy via `nvidia.CopyFile` (overwrites by design). Restore is a purely local operation: it runs before `resolveGames`, so it needs no manifest, cache or network, and it recreates the destination directory when missing. `getFingerprintDBPath` returns the path without requiring the file to exist; `findFingerprintDB` adds the existence check. `--restore` is mutually exclusive with `--list`, `--game` and `--games-json`. It restores `fingerprint.db` only: driver-profile entries are additive and are not removed.
- **Driver profile step**: after `patchDB`, `applyDriverStep` registers every selected game's `DriverAppString()` in its DRS profile through `nvdr.Apply`. `patchDB` therefore returns the parsed `*nvidia.FingerprintDB` so the driver step derives candidates without reading the file twice. Failures here are warnings to stderr, never errors: a successful fingerprint.db patch must not be masked. `nvdr` never imports `internal/nvidia` (candidates are plain `[]string`).
- **Driver profile resolution**: `driver_profile` in the manifest wins; otherwise each `nvidia.DriverProfileCandidates` value (the fingerprint's `<DriverProfile>` executables, deduped case-insensitively) is looked up verbatim, retrying lowercased only when the driver answers `NVAPI_EXECUTABLE_NOT_FOUND` — it stores entries lowercased. No profile is ever created — an unresolved profile asks the user for `driver_profile` instead of inventing a name. `--doctor` runs the same lookups read-only and privilege-free, printing every attempt with its NVAPI status, because the elevated child's console window is not readable.
- **`appName` matching is by process name, so packaged games split in two**: the driver compares `NVDRS_APPLICATION.appName` against the name of the process that starts; there is no package identity in the match. A hosted Store app is seen as its **package family name**; a game shipping a plain executable is seen as its **`.exe`**, even when bought in the Store. Hence `DriverAppString()` (→ package family name) is right for the former and wrong for the latter, where the manifest pins `driver_app` to the `.exe`. `appRegistered` compares case-insensitively because the driver stores whatever strings it accumulates. The `isMetro` bit in `applicationV4.flags` is **ignored by the driver** (reads back as 0), so the string is the only lever and `nvfp` leaves `flags` at 0. It cannot be inferred from the manifest — must be checked against the running process.
- **NVAPI binding** (`internal/nvdr/`): `nvapi.go` holds the values that do not depend on the platform — the 11 function IDs from `nvapi_interface.h`, the 8 status codes from `nvapi_lite_common.h`, both `NVDRS_*` structs, their compile-time size/offset asserts and `MaxDriverString`. It carries no build tag so `nvapi_pinned_test.go` can pin every literal on any platform: a typo in an ID compiles and would only fail against a real driver, which no test can reach. `drs_windows.go` (`//go:build windows`) is the Windows half: loads `nvapi64.dll` with `windows.NewLazySystemDLL` (x/sys restricts the search to `System32`, removing the DLL-preloading risk the stdlib's `NewLazyDLL` carries), resolves function addresses with `nvapi_QueryInterface` IDs (never by name), and calls them through `syscall.SyscallN` — x/sys has no way to call a raw address, since `windows.Proc.addr` is unexported and has no constructor. `DRS_CreateApplication` is called with everything zeroed except `version` and `appName`. `text.go` holds `toUTF16`/`fromUTF16`/`cString` with no build tag so they are testable everywhere; `drs_other.go` stubs `Apply` so `go test ./...` keeps working on Linux, and `result.go` holds the platform-independent `Result`/`Status` types and message formatting.
- **Driver step idempotence**: the profile's applications are enumerated (`DRS_EnumApplications` in batches of 32, honoring `NVAPI_END_ENUMERATION`) and compared case-insensitively before adding; `DRS_SaveSettings` runs once per batch and only when something was added.
- **Elevation**: `elevation_windows.go` implements `isElevated() (bool, error)` opening the process token explicitly (`windows.OpenProcessToken(windows.CurrentProcess(), windows.TOKEN_QUERY, ...)`) and delegating to `Token.IsElevated`; a token error is surfaced instead of read as "not an administrator". `relaunchElevated` is ours (`ShellExecuteExW` with `runas` + `SEE_MASK_NOCLOSEPROCESS`, same args plus `--elevated`, `os.Getwd()` as directory, waits and returns the child's exit code) because x/sys only exposes `ShellExecute`; `shell32.dll` loads with `windows.NewLazySystemDLL` (system-only, as for `nvapi64.dll`). `run()` requests elevation through `ensureElevated(!dryRun && !noDriverFlag && hasDriverWork())`, so UAC appears once. The child's console is destroyed on exit, so `run()` installs `pauseBeforeExit` when `elevatedFlag` is set — but only when stdout is a real console (`stdoutIsPiped`), never for scripts. Declining UAC (`errElevationCancelled`) or any relaunch failure degrades to a warning and the driver step is skipped; `elevation_other.go` provides the non-Windows stubs.
- **Streamline OTA manifest** (`internal/ngx/`): `manifest.go` owns the file format and nothing else does. `parseManifest` reads the file as UTF-8 (stripping a BOM when present; NUL bytes are refused as not-text), tolerates every line ending, and splits it into blocks and lines. `bytes()` is the only writer: UTF-8, CRLF, every line terminated. An unrecognised line is kept verbatim in `raw`; a recognised one keeps the literal text before its value in `prefix`, so only the edited value changes. `set` ignores surrounding whitespace, keeps the line's trailing whitespace, and returns whether the document changed, which is what makes the whole command idempotent.
- **NGX plan and apply** (`internal/ngx/ngx.go`): `Inspect(root)` parses the manifest once, reads the two bundles' newest `nvngx_package_config.txt` (highest path sorts last), and returns a `Plan` holding the parsed document plus what is missing: `Additions()` (sections the manifest lacks), `Updates()` (sections pinning a version the bundle no longer ships — the interposer resolves a feature to `versions\<ota>`, so a stale pin points at a deleted directory), `Copies` (payloads missing under one hash, filled from the sibling bundle's identical bytes), `Missing` (features with no payload anywhere) and per-bundle `Warnings`. `Apply` copies payloads, backs the manifest up to `nvngx_config.txt.bak` on the first write (never overwritten afterwards), then rewrites it whole from the mutated model. A plan with nothing to do writes nothing. `--sl-override` returns before `resolveGames`, so it works with an invalid `--games-json`; it requests elevation through `ensureElevated` only when `plan.Changed()`, and `--dry-run` prints the plan and writes nothing. `Root()` is Windows-only (registry `OTACachePath`, else `C:\ProgramData\NVIDIA\NGX\models`); `main.go` holds it in the `ngxRoot` var so the flow is testable on Linux with a fixture.
- **Why not an INI/TOML/YAML library**: the manifest only looks like INI. TOML cannot parse it at all (bare `310.4.0` values are not TOML), YAML parses `[dlss]` as a flow sequence and silently drops the rest, and `gopkg.in/ini.v1` round-trips by rewriting the whole file in its own style — LF instead of CRLF, aligned `key    = value`, comments moved, duplicate keys collapsed, and a load error on a line without `=`. All three impose their format on a file NVIDIA owns. The model above is ~150 lines of stdlib-only code and preserves what it does not understand.
- **Elevation request is one helper**: `ensureElevated(wanted)` in `main.go` is the single place that reads the token and relaunches through UAC; the patch flow calls it for the driver step, `applySLOverride` for the NGX repair. It is a no-op when `wanted` is false or `--elevated` is already set, treats a token error as a warning, and `os.Exit`s with the child's code on success.
- **Driver string limits**: NVAPI unicode fields hold 2048 UTF-16 units including the NUL (`db.MaxDriverString = 2047`). `LoadFromBytes` rejects `driver_app`, `driver_profile` and `app_user_model_id` over that limit; `nvdr.toUTF16` fails instead of truncating and writes the NUL terminator itself, so its destination needs no prior zeroing and can be reused across calls.

## Important Files

| File | Role |
|---|---|
| `main.go` | CLI entry point, Cobra setup, orchestration functions, driver step |
| `elevation_windows.go` | `isElevated`, `relaunchElevated`, `elevatedLaunchInfo`, `runElevated` (ShellExecuteExW/runas) for Windows |
| `elevation_other.go` | Non-Windows stubs for the elevation helpers |
| `games.json` | Bundled game manifest (embedded at build time) |
| `nvfp.rc` | Windows resource script declaring the application icon |
| `nvfp.ico` | Multi-resolution application icon (7 sizes, 32–256px) |
| `nvfp.svg` | SVG source of the icon (green/turquoise circle with the NVIDIA eye) |
| `nvfp_res_windows_amd64.syso` | Compiled resources linked into the `.exe`; regenerate with `make resources` |
| `LICENSE` | GPLv3 full text |
| `internal/db/games.go` | `GameDB`, `Game`, `PackageFamilyName`, `DriverAppString`, `MaxDriverString`, `ResolveGames`, `LoadFromBytes`, `LoadFromPath`, `SaveToPath` |
| `internal/nvidia/fingerprint.go` | `FingerprintDB`, `Fingerprint`, `Version`, `XmlElement`, `ParseFingerprintDB`, `WriteFingerprintDB`, `CopyFile`, `FindFingerprint`, `FindSourceVersion`, `DriverProfileCandidates`, `AddUWPVersion`, `UpdateVersion` |
| `internal/nvidia/patch.go` | `PatchGame`, `PatchResult`, `PatchStatus`, `resolveVersions`, `applyVersion`, `summarize` |
| `internal/nvdr/result.go` | `Request`, `Result`, `Status`, message formatting (compiles everywhere) |
| `internal/nvdr/drs_windows.go` | NVAPI DRS binding: `openAPI`, `Apply`, `resolveProfile` |
| `internal/nvdr/nvapi.go` | NVAPI IDs, status codes, struct layouts and their compile-time asserts (compiles everywhere, so the pinned tests run on any platform) |
| `internal/nvdr/text.go` | `toUTF16`, `fromUTF16`, `cString` (compiles everywhere, so they are testable on any platform) |
| `internal/nvdr/drs_other.go` | `Apply` stub for non-Windows builds |
| `internal/update/updater.go` | `GamesURL`, `FetchGamesJSON` |
| `internal/ngx/manifest.go` | The manifest format, in one place: `parseManifest`, `bytes`, blocks/lines, UTF-8 decoding, canonical CRLF output |
| `internal/ngx/ngx.go` | `Inspect`, `Apply`, `Plan`, `Section`, `Copy`, feature parsing, sibling-payload fill |
| `internal/ngx/root_windows.go` | `Root`: `OTACachePath` from the registry, falling back to `ProgramData\NVIDIA\NGX\models` |
| `internal/ngx/root_other.go` | `Root` stub for non-Windows builds |
| `internal/nvidia/testdata/fingerprint.db` | Primary XML fixture (5 fingerprints) |
| `internal/nvidia/testdata/fingerprint_metadata.db` | Fixture with Fingerprint-level metadata (2 fingerprints) |

## Runtime/Tooling Preferences

- **Language**: Go 1.27, latest patch. The `go` directive in go.mod pins the newest available patch (currently `1.27.1`); bump it when a new patch ships.
- **Target**: Windows amd64 only (`GOOS=windows GOARCH=amd64`)
- **Direct dependencies**: `github.com/fernandoenzo/set` v1.2.6 (version-name sets), `github.com/spf13/cobra` v1.10.2 (CLI framework)
- **No external test frameworks** — standard `testing` package only
- **No mocking libraries** — use `httptest.NewServer` for HTTP tests

## Git Workflow

- **Every commit must be signed**: `commit.gpgsign` is set in this repo's local config, so plain `git commit` signs; in a fresh clone pass `-S` explicitly. If signing fails, stop and report; never fall back to an unsigned commit.
- **Every plan execution must end with commit and push**: after all changes are verified, commit and push to remote.
- **Commit messages are one short line**: a single concise subject, no body, no bullet points, no paragraph. Say what changed, not why.
- **No conventional commit prefixes** (fix:, feat:, refactor:, etc.). Commit messages go in plain natural language.

## Testing & QA

- Run all tests: `go test ./...`
- Tests use real XML fixtures from `internal/nvidia/testdata/`, not mocks
- Integration tests in `main_test.go` cover end-to-end: parse → patch → write → re-parse → verify
- `output_test.go` validates patch output content (forced fields present, removed fields absent)
- `updater_test.go` uses `httptest` for FetchGamesJSON error/success scenarios
- When adding new patch behavior, add corresponding test cases to `fingerprint_test.go` and verify with a round-trip parse/write/re-parse
- The NVAPI binding itself cannot run on Linux (Windows only), so `internal/nvdr/result.go` keeps the message formatting and status handling testable everywhere via `result_test.go`; `nvapi.go` and `text.go` carry no build tag, so `nvapi_pinned_test.go` pins every ID, status code and layout literal and `text_test.go` covers the UTF-16/cString helpers on any platform; the Windows-only `drs_windows.go` is covered by `GOOS=windows go build`/`go vet`.

## License

GPLv3+. See `LICENSE` for the full text.
