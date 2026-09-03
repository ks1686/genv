# Agent Instructions for genv

## Project Overview

**genv** is a Go CLI tool that tracks, syncs, and reproduces software environments across Linux, macOS, native Windows, and WSL2. It sits as a thin layer on top of existing package managers (`pacman`, `paru`, `yay`, `apt`, `dnf`, `apk`, `snap`, `brew`, `linuxbrew`, `bun`, `uv`, `winget`, `scoop`, `choco`) and uses a declarative model: edit `genv.json`, run `genv apply`, and the tool makes reality match the spec.

## Architecture

### Core Packages

- **`internal/adapter`** — Adapter interface and implementations for each package manager. Each adapter defines `Name()`, `Available()`, `NormalizeID()`, `PlanInstall()`, `PlanUninstall()`, `PlanUpgrade()`, `PlanClean()`, `Query()`, `ListInstalled()`, and `QueryVersion()`. Optional `Searchable` extension for repository search and optional `VersionLister` extension for managers whose list command reports installed versions.
- **`internal/resolver`** — Detects available managers on the host and resolves packages to concrete install/uninstall actions. Entry points: `Detect()`, `ResolveOne()`, `Reconcile()`, `ExecuteApply()`.
- **`internal/commands`** — Pure helper functions that mutate `schema.GenvFile` in memory (`Add`, `Remove`, `EnvSet`, `EnvUnset`, `ShellAliasSet`, `ShellAliasUnset`, `ServiceAdd`, etc.). These are library functions, not CLI command entry points.
- **`internal/schema`** — `GenvFile` struct, JSON schema, validation logic, and `KnownManagers` registry.
- **`internal/genvfile`** — File I/O for `genv.json` and `genv.lock.json`, including `Read`, `Write`, `ReadLock`, `WriteLock`, `DefaultSpecPath`, `LockPathFrom`, and `DefaultDir`.
- **`internal/search`** — Package search across all available managers.
- **`internal/service`** — User-space service lifecycle management (systemd/launchd unit file generation).
- **`internal/shellcfg`** — Shell alias and environment variable management.
- **`internal/env`** — Global environment variable reconciliation.
- **`internal/output`** — JSON and human-readable output formatting.
- **`internal/logging`** — Structured logging setup.
- **`internal/version`** — Version info and build metadata.

### Data Flow

1. User edits `genv.json` (desired state)
2. `genv apply` reads `genv.json` + `genv.lock.json` (last applied)
3. `resolver.Reconcile()` computes delta: install new, uninstall removed
4. `resolver.ExecuteApply()` runs the adapter commands
5. Lock file is updated on success

## CLI Dispatch

The CLI is dispatched by a manual `switch` on `args[0]` in `main.go` (see the `switch args[0]` block near the top of `main()`), not by Cobra. Each command is implemented by a function named `<command>Cmd` that creates its own `flag.FlagSet`, parses arguments, and returns a structured exit code.

Current top-level commands:

- `add` / `remove` (`rm`) / `adopt` / `disown` / `scan` / `list` (`ls`)
- `apply` / `validate` / `status` / `upgrade` / `updates` / `profile`
- `pull` / `migrate` / `export` / `map` / `init` / `edit` / `clean`
- `env` / `shell` / `service` / `completion`
- `version` / `--version` / `help` / `--help` / `-h`

### Adding a New Subcommand

1. Add a new `case "<name>":` to the dispatch switch in `main.go` that calls `<name>Cmd(args[1:])`.
2. Implement `<name>Cmd(args []string) int` in `main.go` (or a new `main_<name>.go` file). Use `flag.NewFlagSet` for flags and return the appropriate exit code.
3. If the command mutates the spec, add a pure helper in `internal/commands/<area>.go` and call it from the command function.
4. Add unit tests in `main_test.go` or `internal/commands/<area>_test.go`.
5. Update `README.md` CLI reference and shell completion scripts under `completions/`.

## The Adapter Interface

Every package manager adapter lives in `internal/adapter/` and implements the `Adapter` interface defined in `internal/adapter/adapter.go`:

```go
type Adapter interface {
    Name() string
    Available() bool
    NormalizeID(id string, managers map[string]string) (name string, explicit bool)
    PlanInstall(pkgName string) []string
    PlanUninstall(pkgName string) []string
    PlanUpgrade(pkgName string) []string
    PlanClean() [][]string
    Query(pkgName string) (bool, error)
    ListInstalled() ([]string, error)
    QueryVersion(pkgName string) (string, error)
}
```

Optional `Searchable`, `VersionLister`, and `BatchUpgrader` extensions are defined in `internal/adapter/adapter.go`.

Adapters are registered in priority order in `internal/adapter/adapter.go` (`var All`). `adapter.ByName` looks up an adapter by its `Name()`.

### Adding a New Package Manager

1. Create `internal/adapter/<manager>.go` implementing `Adapter`.
2. Add the new type to the `All` slice in `internal/adapter/adapter.go`.
3. Add the manager name to `schema.KnownManagers` in `internal/schema/schema.go`.
4. Write unit tests in `internal/adapter/<manager>_test.go`.
5. Update `README.md` supported platforms table.
6. Add CI integration test in `.github/workflows/integration.yml` if applicable.

## Key Files

| File | Purpose |
|------|---------|
| `main.go` | CLI entry point and manual command dispatch (`main.go:81-127`) |
| `go.mod` | Module: `github.com/ks1686/genv`, Go 1.24.3 |
| `Makefile` | Build, test, CI, lint, benchmark targets |
| `schema/v8/genv.json` | JSON Schema mirror for v8 (Go `ParseAndValidate` is authoritative) |
| `.goreleaser.yml` | Release build configuration |
| `e2e/e2e_test.go` | End-to-end integration tests |

## Build & Test

```bash
# Build binary
make build                    # or: go build -o genv .

# Run all tests
make test                     # or: go test ./...

# Run CI suite (format check + race tests + coverage)
make ci

# Run benchmarks
make bench
make bench-gate               # enforces <200ms cold-start budget

# Lint
make lint                     # requires golangci-lint

# Format
make fmt                      # gofmt -w .
```

## Coding Conventions

- **Go version**: 1.24.3 (see `go.mod`)
- **Error handling**: Return `error` values; wrap with context using `fmt.Errorf("...: %w", err)`
- **Logging**: Use `log/slog` for structured logging; never `fmt.Println` in library code
- **Tests**: Table-driven tests with `t.Run`. Use `testing/fstest` for filesystem mocks where possible.
- **Comments**: Explain WHY, not WHAT. Doc comments on exported symbols.
- **Package managers**: Each new manager needs an `Adapter` implementation in `internal/adapter/` + registration in `adapter.All`.
- **Security**: All user input goes through `schema.Validate()` before touching the filesystem or subprocesses.

### Fixing a Bug

1. Write a failing test first
2. Fix minimally in the relevant package
3. Run `make ci` before considering done
4. If touching `resolver` or `adapter`, run `make bench` to ensure no cold-start regression

## Release Process

Tag-driven GitHub releases via `.github/workflows/release.yml`. See `RELEASING.md` for full process.

## CodeGraph (optional)

This repo does **not** ship a `.codegraph/` index today — treat CodeGraph as an
optional local aid, not part of the checked-in tooling. If you choose to build an
index locally, you can then explore code with it before falling back to grep/find:

```bash
codegraph explore "resolver Detect"     # symbols + call paths
codegraph node resolver.go              # full file with dependents
codegraph impact "Adapter.Available"    # blast radius of a change
```

If you wire up a post-commit hook to auto-sync the index, that too is a local,
opt-in convenience and is not configured in this repository.

## Learned User Preferences

- Prefer `genv upgrade` to plan only packages with detected updates by default, with `--all` as the escape hatch for the old brute-force “touch everything” path. `genv upgrade` also runs OS vendor updates (and firmware when available) for the active target; more extra-tool steps land in follow-ups.
- Keep successful upgrade output visible; filtering should drop non-outdated plan noise, not hide upgrades that actually run.
- Prefer git worktrees under `/Users/ks1686/Documents/Worktrees/genv/<branch-or-slice>` for isolated feature work instead of switching branches in the main checkout.
- Prefers subagent-driven execution for multi-step implementation plans when that option is offered.
- Prefers agent guidance as a global baseline under `~/.config/genv/` (and Cursor User Rules when configured); add project-specific rules only when explicitly requested.
- For cross-platform shell/env work (including PowerShell), do not assume PowerShell exists on every host; gate apply/write on availability.
- Prefer cross-platform aliases/env (e.g. EDITOR, general shell aliases) in v8 `defaults`, with OS-specific overlays only under `targets.*`.
- Prefer `genv add` to persist into `genv.json` only after a successful install/verification (fail nonzero and leave the spec unchanged on unresolved/install failure).
- Prefer repo-name tab completions for new package ids on `add`/`adopt` (tracked-ID commands stay as today): ambiguous deduped bare names across managers, then the existing add picker sets `prefer`; hybrid model matching Homebrew — full local name dumps with no min-prefix gate, live `Search` only as fallback; v1 covers all `Searchable` managers plus easy high-value listers (e.g. mas, npm/bun) where practical.
- Treat multi-machine / cross-OS config portability (export-to-target / migration without sharing locks) as a first-class product goal.
- For public Linux support, focus on major distros whose packaging channels do not require separate human review gates known to reject AI-assisted tooling.
- Prefer `--dry-run` and confirmation (unless `--yes`) for bulk-mutating discovery commands like `scan` before writing the spec or lock.

## Learned Workspace Facts

- `genv upgrade` and `genv updates check` share the tracked-package upgrade planner; default planning uses outdated detection (`Filters.All` false / `OutdatedLister`). Managers without a lister (or on lister error) keep packages rather than silently skipping them. `OutdatedLister` covers brew/linuxbrew, mas, bun, npm/pnpm/yarn, uv/pipx, pip-user, volta, cargo, winget/scoop/choco, pacman/paru/yay, apt/dnf/apk, snap, and vscode (newest stable; pre-release gallery versions are ignored because `code --install-extension --force` cannot install them); mas also implements `BatchUpgrader`. `genv upgrade` then runs named system/firmware steps (`internal/upgrade` runner). `genv updates check` / the timer / `updates.autoApply` stay tracked-packages-only and must not grow those OS/firmware steps. Follow-up slices add rustup, editors, containers, and other extra tools to `upgrade` only.
- Darwin GitHub Release / Homebrew binaries use Developer ID Application signing and App Store Connect notarization (`APPLE_API_KEY_ID` / `APPLE_API_ISSUER_ID` / `APPLE_API_KEY_PATH`); local codesign on this Mac uses Apple Development only—do not use Development for Gatekeeper / Homebrew / notarized distribution. Never commit `.p8` / `.p12` / private keys; human-readable identity notes live in `~/.appstoreconnect/IDENTITY.md`. Treat GitHub Release + Homebrew as release success; AUR SSH publish may fail—do not re-run the full Release workflow after GoReleaser publishes (`already_exists`); use aur-only repair instead.
- `make ci` and GitHub CI enforce statement coverage via `cover-gate` (`COVER_MIN` default 80) and cold-start via `bench-gate`. Integration workflow also runs `scripts/docker-v8-command-matrix.sh` (`make integration-v8`) — an Arch Docker job that builds genv and exercises every CLI command against a schemaVersion 8 pacman-backed spec.
- Schema versions `"1"`–`"8"` are accepted. v7 adds `"shell": "powershell"` targeting. v8 uses portable `defaults` plus `targets.*` (known: `macos`, `windows`, `arch`, `ubuntu`, `wsl-arch`, optional `linux`); top-level desired-state blocks and per-record `host` are invalid in v8. Target overlays support `null` tombstones for inherited `env` / `shell.aliases` / `shell.functions` / `services` (targets only, not defaults). Target selection is `--target`, then `GENV_TARGET`, then host classification.
- Any command that reads tracked packages or lifecycle hooks on schemaVersion 8 must materialize via `resolveEffectiveSpec` / `materializeSpecForCommand` / `materializedHooks` (`target.Resolve` + `schema.MergeTarget`). `host.FilterForHost` alone drops v8 `targets.*` / `defaults` and leaves top-level `packages`/`hooks` empty — the 4.0.0 bug in `status` / `upgrade` / `updates check` (fixed by sharing apply’s materialize path and `--target`).
- `genv migrate` converts v1–v7 host-scoped specs to v8 target buckets; `genv export --target --out` writes a single-target snapshot plus report and bundled relative file assets while omitting locks and sensitive env values; `genv map --target` is print-only guidance for manager mapping gaps.
- On Windows, `genv apply` uses a profile-backend abstraction (`POSIXBackend` + `PowerShellBackend`): prefer `pwsh`, else `powershell.exe`; omit PowerShell writes on non-Windows. Shared `env` maps emit both `env.sh` and `env.ps1` when the corresponding backend runs.
- Host classification recognizes `macos`, native `windows`, native `arch`, `ubuntu`, and `wsl-arch`. WSL2 does not inherit native `arch`: Ubuntu-like WSL2 → `ubuntu`, Arch-like WSL2 → `wsl-arch`; other WSL distros need `GENV_TARGET`/`--target`.
- Lock files are machine-local and must not travel with the spec. Schema v8 locks record target/GOOS/manager metadata; foreign locks are refused unless `genv apply --force-new-lock` backs them up and starts a new local lock.
- `updates start` registers the hourly checker via `internal/selfpath.PreferStable`: prefer same-inode PATH hits, else derive Homebrew `<prefix>/bin/<name>` from Caskroom/Cellar versioned paths even when the brew symlink is missing/dangling mid-upgrade. Scheduled PATH strips Homebrew shims; on Windows it keeps scoop/winget shims and appends user shim dirs. `__run-once` has a wall-clock deadline (launchd `TimeOut` / systemd `TimeoutStartSec` / Task Scheduler `ExecutionTimeLimit`) so TLS/keychain hangs cannot wedge StartInterval. Windows uses a per-user `schtasks` task (logon + interval) started through the scheduler so OpenSSH job objects cannot kill the checker. Heal with `genv updates start`; `updates status` / `validate` warn on dangling agent binaries.
- Unresolved `genv apply` file mismatches skip post-apply hooks with an explicit message; human-readable output names mismatch paths and includes the files plan. `--force` / `--backup` overwrite mismatched managed files. Lifecycle hooks inherit `os.Stdin` (like package actions) and receive `GENV_YES=true|false` when `--yes` is set; hooks must still opt into manager noninteractive flags (e.g. pacman `--noconfirm`) themselves — genv does not rewrite hook argv.
- `genv add` soft-locks the chosen manager via persisted `prefer` (honored first when available, then `managers` map / defaults). `add`/`adopt` Tab completion uses `adapter.NameLister` / `CompletionNamer` dumps (cached under `~/.config/genv/cache/completions/`, 14-day TTL) plus live `Search` fallback via `genv __complete repo-packages`. `add` persists the spec only after a successful install and exits `exitLogic` (4) on unresolved or install failure, leaving the spec unchanged. New empty specs are schemaVersion 8; named profiles are refused on v8.
