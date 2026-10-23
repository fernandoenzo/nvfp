# Repository Guidelines

## Project Overview

`nvfp` patches NVIDIA App's `fingerprint.db` for UWP games, registers the matching process string in the driver profile database, can restore the DAO copy, and can reset the NGX models folder.

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
   (DriverAppStrings,        │
    DriverProfile,            ├─ DRS_CreateSession / LoadSettings
    DriverProfileCandidates)  ├─ FindProfileByName | FindApplicationByName
                              ├─ EnumApplications (already-present check)
                              └─ CreateApplication (one per string) + SaveSettings

findDAOFingerprintDB ──┐  (--restore)
                       ▼
      getFingerprintDBPath ──► restoreDB ──► CopyFile → working fingerprint.db

nvidiaModelsDir ──► resetModels ──► RemoveAll → NGX models folder  (--reset-models)
```

Architecture layers:
1. **CLI layer** (`cmd/nvfp/`, one file per responsibility: `main.go`, `version.go`, `manifest.go`, `patch.go`, `driver.go`, `doctor.go`, `list.go`, `paths.go`, `restore.go`, `models*.go`, `dlss*.go`, `elevation*.go`): Cobra commands (`newRootCmd`), flags (`--dry-run`, `--list`, `--restore`, `--game`, `--games-json`, `--no-driver`, `--doctor`, `--reset-models`, `--dlss-debug`, `--no-dlss-debug`, `--elevated`, `--version`), orchestration, UAC relaunch, models-reset and DLSS-indicator flows
2. **Data layer** (`internal/db`): Game manifest model, I/O, resolve fallback chain
3. **Core logic layer** (`internal/nvidia`): XML fingerprint parsing/patching
4. **Driver layer** (`internal/nvdr`): NVAPI DRS binding (Windows) + status/message types (all platforms)
5. **Network layer** (`internal/update`): Remote games.json fetch
6. **Shared utilities** (`internal/fsutil`): staged file copy/replacement and durable directory creation where supported
7. **Embedded assets** (`assets.go` at the module root, package `assets`): embeds `games.json` for `cmd/nvfp`. The shim sits in the root because `//go:embed` cannot leave its package directory and `games.json` must stay reachable at the published `raw/master/games.json` URL.

## Windows icon resources

The application icon is a Windows PE resource, not a Go embed. `nvfp.rc` declares `1 ICON "nvfp.ico"` and `x86_64-w64-mingw32-windres` compiles it into `cmd/nvfp/nvfp_res_windows_amd64.syso`, a COFF object holding the `.rsrc` section the Go linker merges into the executable. The `_windows_amd64` suffix makes the Go toolchain ignore the object on every other GOOS/GOARCH, so plain `go build`/`go vet`/`go test` keep working on Linux. The `.syso` lives in the package directory because `go/build` collects it as part of the package's file list; `nvfp.rc` and `nvfp.ico` stay at the repo root and `windres` runs from there, so the icon reference inside the object is unchanged.

- The `.syso` is committed: `make build` must work without a MinGW toolchain installed. It is a generated artifact, so it only changes when `nvfp.rc` or `nvfp.ico` changes.
- `make build` fails with an explicit message when the `.syso` is missing, rather than silently producing an icon-less binary.
- `windres` output is deterministic (no timestamp is written for icons), so regenerating it yields a byte-identical file and the reproducible-build guarantee in `README.md` still holds.

## Development Commands

```bash
make build
make resources
make test
go vet ./...
gofmt -l .
```

`make build` targets Windows amd64 and uses the committed icon resource object, so it needs no MinGW. `make resources` requires `x86_64-w64-mingw32-windres`.

## Code Conventions & Common Patterns

- **Function length**: max 25 lines of code per function (comments excluded). Extract helpers early.
- The `cmd/nvfp` package is split one file per responsibility (`main.go` CLI wiring, `version.go`, `manifest.go`, `patch.go`, `driver.go`, `doctor.go`, `list.go`, `paths.go`, `restore.go`, `models*.go`, `ngx.go`, `dlss*.go`, `elevation*.go`); put new code in the matching file. `assets.go` at the module root embeds `games.json`. Registry keys and value names live in `ngx.go` (platform neutral) so tests off Windows can pin them; `models*.go` and `dlss*.go` hold only the build-tagged accessors.
- Wrap errors with `fmt.Errorf("...: %w", err)`. Report non-fatal failures to stderr; return an error only when the caller should abort.
- Use standard Go naming and table driven tests with `t.TempDir()`; use `httptest` for HTTP. Tests use the standard `testing` package and the `set` helper from `github.com/fernandoenzo/set` where a set is already in play — no test framework or mocking library.
- Keep `XmlElement` generic so unknown fingerprint XML survives round-trips. `PatchGame` takes `*FingerprintDB` and `*db.Game`.
- JSON types: `GameDB.Games []*db.Game`, `db.Game.Versions []string`; save deterministically. Resolve remote → cache → bundled; an empty cache path disables disk caching.
- `driver_app`, `driver_profile` and `skip_driver` are independent of versions. `Game.DriverAppStrings()` is the sole registration source: it returns the package family name derived from `app_user_model_id` first, then the `driver_app` entries in order, deduplicated with `strings.EqualFold`; `skip_driver` returns none while keeping the XML patch. `nvdr.Request.Apps` carries that list and `Apply` emits one `Result` per string, so a conflict or failure on one never stops the rest.
- Deduplicate application strings with `strings.EqualFold`, never `strings.ToLower`: the driver matches with case folding (`NvAPI_DRS` compares UTF-16 names case-insensitively), and the two disagree on U+0130 (`İ`) with U+0049/U+0069 (`I`/`i`), which `ToLower` collapses although the driver keeps them distinct. The same rule applies to any set of driver-facing strings.
- UWP addition applies default removals and forced fields; updating an existing version applies only explicit changes. `versions: ["*"]` must stand alone; preserve document order and deterministic overrides.
- Resolve profiles by exact `driver_profile` or `<DriverProfile>` candidates; never invent profiles. DRS matches process name, not package identity, and ignores `isMetro`. Hand each candidate to `NvAPI_DRS_FindApplicationByName` exactly as the fingerprint spells it: no spelling variants are generated, because the driver's own lookup already copes with them. Verified against a real driver database: it stores no entry with a backslash, yet a backslashed fingerprint path resolves, an entry registered without its directory resolves from a full path, and one stored lowercased resolves from a capitalized candidate.
- NVAPI IDs and struct layouts are pinned with cross-platform tests and compile-time asserts. Driver strings are limited to 2047 UTF-16 units excluding NUL; never truncate.
- `ensureElevated` is the single UAC path. `--restore` restores only `fingerprint.db`; DRS entries are additive.
- `--reset-models` deletes the registry-resolved NGX models folder and refuses paths whose last component is not `models`. A `fingerprint.db` patch is visible only after a Windows logoff/logon; the program prints that reminder whenever it modifies the database.
- `--dlss-debug`/`--no-dlss-debug` write the `ShowDlssIndicator` DWORD (`1024`/`0`) under `ngxRegKey` through a `dlssRegistry` of injected accessors: `ngxDlssRegistry()` in `dlss_windows.go` and a failing stub in `dlss_other.go`, so the flow runs and tests off Windows. A missing value is the factory state and is created. The **leaf** `NGXCore` may be missing too (`ngxKeyName` in `ngx.go`) and is created, but only by opening `nvidiaRegGlobal` first and calling `registry.CreateKey` with the bare leaf name: handing the full path to `CreateKey` would fabricate every missing intermediate key. A missing `nvidiaRegGlobal` aborts with a diagnosis, never a repair. A value already holding the target prints `Nothing to do`; a value with a type other than `REG_DWORD` is refused instead of overwritten. `--dry-run` neither elevates nor writes. `TestNGXRegistryLayout` pins the separator-free leaf.
- **Version banner**: `--version`/`-v` is a plain bool flag handled by `runLocalOp` before the manifest is resolved. The banner is assembled in `version.go` from the `version` and `versionDate` constants; bump both on every release. It does **not** use Cobra's `Command.Version`/`SetVersionTemplate`: that path pulls `cobra.tmpl` → `text/template` (+`reflect`) into the binary (~+1.9 MB). Its shape mirrors the author's other CLIs (name, version, date, copyright, GPLv3+ notice, author).
- Cache directory and `User-Agent` both use the `nvfp` name (`%LOCALAPPDATA%\nvfp`, fallback `~/.cache/nvfp`).
- `CopyFile` and `WriteFileAtomic` stage to unique sibling files, flush before replacement and sync directories where supported; OS/filesystem guarantees vary.
- HTTP fetches use a 10s timeout, 5MB limit and custom `User-Agent`.

## Runtime/Tooling Preferences

- `go.mod` is the source of truth for the minimum Go version and module versions. Release target: Windows amd64.

## Git Workflow

- **Every commit must be signed**: `commit.gpgsign` is set in this repo's local config, so plain `git commit` signs; in a fresh clone pass `-S` explicitly. If signing fails, stop and report; never fall back to an unsigned commit.
- **Every plan execution must end with commit and push**: after all changes are verified, commit and push to remote.
- **Commit messages are one short line**: a single concise subject, no body, no bullet points, no paragraph. Say what changed, not why.
- **No conventional commit prefixes** (fix:, feat:, refactor:, etc.). Commit messages go in plain natural language.

## Testing & QA

- Run `make test`, `go vet ./...`, and a Windows amd64 cross-build for Windows-only code.
- Use XML fixtures for fingerprint round-trips, temporary trees for file behavior, and `httptest` for HTTP.
- NVAPI literal/layout tests run cross-platform; actual DRS calls require Windows and an NVIDIA driver.

## License

GPLv3+. See `LICENSE` for the full text.
