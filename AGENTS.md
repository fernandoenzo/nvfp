# Repository Guidelines

## Project Overview

CLI tool that patches the NVIDIA App fingerprint database to add UWP (Microsoft Store) game entries. It locates the working fingerprint.db (ApplicationOntology\data) on Windows, patches it with game metadata from a bundled or remotely-fetched JSON manifest, and backs up the original before writing.

## Architecture & Data Flow

```
games.json (bundled/embedded) ──┐
                                 ▼
                           resolveGames ──► db.ResolveGames
                                 │
findFingerprintDB ──► dbPath
                                 │
                                 ▼
                            patchDB
                                 │
                                 ▼
          ParseFingerprintDB ──► applyPatches ──► writePatch
                    │                              │
                    ▼                              ▼
            PatchGame per game          BackupFile → WriteFingerprintDB
                    │
                    ▼
          FindFingerprint → resolveVersions
          → FindSourceVersion → AddUWPVersion / UpdateVersion
```

Four-layer architecture:
1. **CLI layer** (`main.go`): Cobra commands, flags (`--dry-run`, `--list`, `--game`, `--games-json`), orchestration
2. **Data layer** (`internal/db`): Game manifest model, I/O, resolve fallback chain
3. **Core logic layer** (`internal/nvidia`): XML fingerprint parsing/patching
4. **Network layer** (`internal/update`): Remote games.json fetch

## Key Directories

| Directory | Purpose |
|---|---|
| `.` | Entry point (`main.go`), embedded `games.json`, Makefile |
| `internal/db/` | Game manifest model, JSON I/O, resolve logic |
| `internal/nvidia/` | Fingerprint XML parsing/patching, metadata handling |
| `internal/nvidia/testdata/` | XML fixture files for tests |
| `internal/update/` | Remote games.json fetcher |

## Development Commands

```bash
# Build (cross-compiles for Windows amd64, reproducible)
make build
# Equivalent:
# CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -trimpath -buildvcs=false -ldflags="-s -w -buildid=" -o nvfp.exe .

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
- **UWP version modes**: `AddUWPVersion` (new version: default removals + forced fields from appUserModelID) vs `UpdateVersion` (existing version: only explicit removals, forced fields preserved).
- **Version requests**: `versions: ["*"]` means every version the fingerprint already has, plus a new `uwp` when the game has an `app_user_model_id` and lacks one. `"*"` is rejected unless it is the only entry. A requested `uwp` that gets created is never reported as missing. Lookup is `Game.VersionKeys()`, which lowercases and trims the names.
- **Version ordering**: versions are reported in fingerprint document order, never in set iteration order; per-version results go through `applyVersion`.
- **Deterministic output**: override elements are emitted sorted by lowercased key.
- **Source version priority**: Steam > first non-UWP version found.
- **Embedded resources**: `games.json` embedded via `//go:embed` and used as fallback.
- **HTTP safeguards**: 10s timeout, 5MB `io.LimitReader`, custom `User-Agent` header.

## Important Files

| File | Role |
|---|---|
| `main.go` | CLI entry point, Cobra setup, orchestration functions |
| `games.json` | Bundled game manifest (embedded at build time) |
| `internal/db/games.go` | `GameDB`, `Game`, `PackageFamilyName`, `ResolveGames`, `LoadFromBytes`, `LoadFromPath`, `SaveToPath` |
| `internal/nvidia/fingerprint.go` | `FingerprintDB`, `Fingerprint`, `Version`, `XmlElement`, `ParseFingerprintDB`, `WriteFingerprintDB`, `BackupFile`, `FindFingerprint`, `FindSourceVersion`, `AddUWPVersion`, `UpdateVersion` |
| `internal/nvidia/patch.go` | `PatchGame`, `PatchResult`, `PatchStatus`, `resolveVersions`, `applyVersion`, `summarize` |
| `internal/update/updater.go` | `GamesURL`, `FetchGamesJSON` |
| `internal/nvidia/testdata/fingerprint.db` | Primary XML fixture (5 fingerprints) |
| `internal/nvidia/testdata/fingerprint_metadata.db` | Fixture with Fingerprint-level metadata (2 fingerprints) |

## Runtime/Tooling Preferences

- **Language**: Go 1.27, latest patch. The `go` directive in go.mod pins the newest available patch (currently `1.27.1`); bump it when a new patch ships.
- **Target**: Windows amd64 only (`GOOS=windows GOARCH=amd64`)
- **Direct dependencies**: `github.com/fernandoenzo/set` v1.1.0 (version-name sets), `github.com/spf13/cobra` v1.10.2 (CLI framework)
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
