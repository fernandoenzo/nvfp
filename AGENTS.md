# Repository Guidelines

## Project Overview

`nvfp` patches NVIDIA App's `fingerprint.db` for UWP games, registers the matching process string in the driver profile database, can restore the DAO copy, and can repair Streamline's NGX manifest.

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

Architecture layers:
1. **CLI layer** (`main.go`, `elevation_*.go`): Cobra commands (`newRootCmd`), flags (`--dry-run`, `--list`, `--restore`, `--game`, `--games-json`, `--no-driver`, `--doctor`, `--sl-override`, `--elevated`, `--version`), orchestration, UAC relaunch, NGX repair flow
2. **Data layer** (`internal/db`): Game manifest model, I/O, resolve fallback chain
3. **Core logic layer** (`internal/nvidia`): XML fingerprint parsing/patching
4. **Driver layer** (`internal/nvdr`): NVAPI DRS binding (Windows) + status/message types (all platforms)
5. **NGX layer** (`internal/ngx`): Streamline OTA manifest repair (all platforms; the cache path comes from the Windows-only `Root`, which `runSLOverride` receives as an argument so tests can drive the repair against a fixture)
6. **Network layer** (`internal/update`): Remote games.json fetch
7. **Shared utilities** (`internal/fsutil`): staged file copy/replacement and durable directory creation where supported

## Windows icon resources

The application icon is a Windows PE resource, not a Go embed. `nvfp.rc` declares `1 ICON "nvfp.ico"` and `x86_64-w64-mingw32-windres` compiles it into `nvfp_res_windows_amd64.syso`, a COFF object holding the `.rsrc` section the Go linker merges into the executable. The `_windows_amd64` suffix makes the Go toolchain ignore the object on every other GOOS/GOARCH, so plain `go build`/`go vet`/`go test` keep working on Linux.

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
- Wrap errors with `fmt.Errorf("...: %w", err)`. Report non-fatal failures to stderr; return an error only when the caller should abort.
- Use standard Go naming and table driven tests with `t.TempDir()`; use `httptest` for HTTP. Tests use the standard `testing` package and the `set` helper from `github.com/fernandoenzo/set` where a set is already in play — no test framework or mocking library.
- Keep `XmlElement` generic so unknown fingerprint XML survives round-trips. `PatchGame` takes `*FingerprintDB` and `*db.Game`.
- JSON types: `GameDB.Games []*db.Game`, `db.Game.Versions []string`; save deterministically. Resolve remote → cache → bundled; an empty cache path disables disk caching.
- `driver_app`, `driver_profile` and `skip_driver` are independent of versions. `DriverAppString()` is the sole registration source; `skip_driver` skips DRS but keeps the XML patch.
- UWP addition applies default removals and forced fields; updating an existing version applies only explicit changes. `versions: ["*"]` must stand alone; preserve document order and deterministic overrides.
- Resolve profiles by exact `driver_profile` or `<DriverProfile>` candidates; never invent profiles. DRS matches process name, not package identity, and ignores `isMetro`.
- NVAPI IDs and struct layouts are pinned with cross-platform tests and compile-time asserts. Driver strings are limited to 2047 UTF-16 units excluding NUL; never truncate.
- `ensureElevated` is the single UAC path. `--restore` restores only `fingerprint.db`; DRS entries are additive.
- **Version banner**: `--version`/`-v` is a plain bool flag handled at the top of `run()`. The banner is assembled in `main.go` from the `version` and `versionDate` constants; bump both on every release. It does **not** use Cobra's `Command.Version`/`SetVersionTemplate`: that path pulls `cobra.tmpl` → `text/template` (+`reflect`) into the binary (~+1.9 MB). Its shape mirrors the author's other CLIs (name, version, date, copyright, GPLv3+ notice, author).
- Cache directory and `User-Agent` both use the `nvfp` name (`%LOCALAPPDATA%\nvfp`, fallback `~/.cache/nvfp`).
- The manifest parser preserves opaque lines and value prefixes, validates UTF-8, rejects NUL, and writes canonical CRLF. `CopyFile` and `WriteFileAtomic` stage to unique sibling files, flush before replacement and sync directories where supported; OS/filesystem guarantees vary.
- NGX selects the highest numeric OTA (lexical path breaks ties), warns only for malformed Streamline configs, and copies sibling payloads only for the same architecture. `Missing` means no compatible-architecture payload was found.
- Keep the custom manifest parser; general INI/TOML/YAML libraries rewrite or misparse NVIDIA's format. HTTP fetches use a 10s timeout, 5MB limit and custom `User-Agent`.

## Runtime/Tooling Preferences

- `go.mod` is the source of truth for the minimum Go version and module versions. Release target: Windows amd64.

## Git Workflow

- **Every commit must be signed**: `commit.gpgsign` is set in this repo's local config, so plain `git commit` signs; in a fresh clone pass `-S` explicitly. If signing fails, stop and report; never fall back to an unsigned commit.
- **Every plan execution must end with commit and push**: after all changes are verified, commit and push to remote.
- **Commit messages are one short line**: a single concise subject, no body, no bullet points, no paragraph. Say what changed, not why.
- **No conventional commit prefixes** (fix:, feat:, refactor:, etc.). Commit messages go in plain natural language.

## Testing & QA

- Run `make test`, `go vet ./...`, and a Windows amd64 cross-build for Windows-only code.
- Use XML fixtures for fingerprint round-trips, temporary trees for file/NGX behavior, and `httptest` for HTTP.
- NVAPI literal/layout tests run cross-platform; actual DRS calls require Windows and an NVIDIA driver.

## License

GPLv3+. See `LICENSE` for the full text.
