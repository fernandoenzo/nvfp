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
          ParseProfileDB ──► applyPatches ──► writePatch
                    │                              │
                    ▼                              ▼
            PatchGame per game          BackupFile → WriteProfileDB
                    │
                    ▼
          FindFingerprint → findUWPVersion?
          → FindSourceVersion → AddUWPVersion
```

Four-layer architecture:
1. **CLI layer** (`main.go`): Cobra commands, flags (`--dry-run`, `--list`, `--game`, `--games-json`), orchestration
2. **Data layer** (`internal/db`): Game manifest model, I/O, resolve fallback chain
3. **Core logic layer** (`internal/nvidia`): XML fingerprint parsing/patching
4. **Network layer** (`internal/update`): Remote games.json fetch with rate-limit safeguards

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
# Build (cross-compiles for Windows amd64)
make build
# Equivalent: GOOS=windows GOARCH=amd64 go build -ldflags="-s -w" -o nvidia-uwp-patch.exe .

# Run tests
go test ./...

# Run tests with verbose output
go test -v ./...

# Run specific package tests
go test -v ./internal/nvidia/...
go test -v ./internal/db/...
```

No lint or coverage targets in the Makefile. Use `go vet ./...` manually.

## Code Conventions & Common Patterns

- **Function length**: Max 25 lines per function. Extract helpers early.
- **Error handling**: Wrap errors with context using `fmt.Errorf("...: %w", err)`. Report non-fatal failures to stderr; return error only when the caller should abort.
- **Naming**: Standard Go conventions. Acronyms stay cased (`AppID`, `UWP`, `SHA256`).
- **Table-driven tests**: Use `[]struct{ name string; ... }` with `t.Run(tc.name, ...)`.
- **Temp files**: Always use `t.TempDir()` for isolation; never hardcode paths.
- **XML model**: Generic `XmlElement` struct (XMLName, Attr, Content, Children) for forward compatibility — no domain-specific structs for XML nodes.
- **Patch result**: `PatchResult` with `Status` field: `patched`, `already_uwp`, `not_found`, `no_source`.
- **Game resolution fallback**: Remote → cache → bundled (in that priority).
- **Forced field defaults**: `Distributor`, `UWPPackageFamilyName`, `AppUserModelId` are derived from the appID; user overrides take priority over these defaults.
- **UWP version modes**: `AddUWPVersion` (new version: default removals + forced fields from appID) vs `UpdateVersion` (existing version: only explicit removals, forced fields preserved).
- **Deterministic output**: override elements are emitted sorted by lowercased key.
- **Source version priority**: Steam > first non-UWP version found.
- **Embedded resources**: `games.json` embedded via `//go:embed` and used as fallback.
- **HTTP safeguards**: 10s timeout, 5MB `io.LimitReader`, custom `User-Agent` header.

## Important Files

| File | Role |
|---|---|
| `main.go` | CLI entry point, Cobra setup, orchestration functions |
| `games.json` | Bundled game manifest (embedded at build time) |
| `internal/db/games.go` | `GameDB`, `Game` types, `ResolveGames`, `LoadFromBytes`, `SaveToPath` |
| `internal/nvidia/fingerprint.go` | `ProfileDB`, `XmlElement`, `AddUWPVersion`, `UpdateVersion`, `ParseProfileDB`, `WriteProfileDB`, `BackupFile` |
| `internal/nvidia/patch.go` | `PatchGame`, `PatchResult`, `PatchStatus` and status constants |
| `internal/update/updater.go` | `FetchGamesJSON` (HTTP fetch with safeguards) |
| `internal/nvidia/testdata/fingerprint.db` | Primary XML fixture (5 fingerprints) |

## Runtime/Tooling Preferences

- **Language**: Go 1.27+
- **Target**: Windows amd64 only (`GOOS=windows GOARCH=amd64`)
- **Dependencies**: `github.com/spf13/cobra` v1.10.2 (CLI framework)
- **No external test frameworks** — standard `testing` package only
- **No mocking libraries** — use `httptest.NewServer` for HTTP tests

## Git Workflow

- **Commits are signed when the local GPG key is configured** (`git commit -S` or `git commit --gpg-sign`). Recent history contains both signed and unsigned commits; do not assume a signature requirement.
- **Every plan execution must end with commit and push**: after all changes are verified, commit with a descriptive message and push to remote.
- **No conventional commit prefixes** (fix:, feat:, refactor:, etc.). Commit messages go in plain natural language.
## Testing & QA

- Run all tests: `go test ./...`
- Tests use real XML fixtures from `internal/nvidia/testdata/`, not mocks
- Integration tests in `main_test.go` cover end-to-end: parse → patch → write → re-parse → verify
- `output_test.go` validates patch output content (forced fields present, removed fields absent)
- `updater_test.go` uses `httptest` for FetchGamesJSON error/success scenarios
- When adding new patch behavior, add corresponding test cases to `fingerprint_test.go` and verify with a round-trip parse/write/re-parse