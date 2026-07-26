# Agent Instructions for genv

## Project Overview

**genv** is a Go CLI tool that tracks, syncs, and reproduces software environments across Linux, macOS, native Windows, and WSL2. It sits as a thin layer on top of existing package managers (`pacman`, `paru`, `yay`, `snap`, `brew`, `linuxbrew`, `bun`, `uv`, `winget`, `scoop`, `choco`) and uses a declarative model: edit `genv.json`, run `genv apply`, and the tool makes reality match the spec.

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

The CLI is dispatched by a manual `switch` on `args[0]` in `main.go:81-127`, not by Cobra. Each command is implemented by a function named `<command>Cmd` in `main.go` that creates its own `flag.FlagSet`, parses arguments, and returns a structured exit code.

Current top-level commands (see `main.go:81-127`):

- `add` — add a package to the spec and install it
- `remove` / `rm` — remove a package from the spec and uninstall it
- `adopt` — verify a package is already installed, then track it without reinstalling
- `disown` — stop tracking a package without uninstalling it
- `list` / `ls` — list packages installed by genv (reads lock file)
- `apply` — reconcile the system against the spec
- `edit` — open `genv.json` in `$EDITOR`
- `clean` — clean adapter caches
- `scan` — bulk-adopt currently installed packages into the spec
- `status` — diff between spec, lock, and live system
- `completion` — print shell completion scripts
- `validate` — validate `genv.json` without installing anything
- `upgrade` — re-resolve and upgrade pinned packages
- `pull` — fetch the spec from the git repository declared in `repo.url`
- `migrate` — convert legacy host predicates to schemaVersion 8 target buckets
- `export` — build a single-target schemaVersion 8 snapshot plus report
- `map` — print assist-only manager mapping suggestions for a target
- `init` — interactive wizard to create a new `genv.json`
- `env` — manage global environment variables (`set`, `unset`, `list`)
- `shell` — manage shell config (`alias set/unset`, `status`, `edit`)
- `service` — manage user-space services
- `version` / `--version` — print version
- `help` / `--help` / `-h` — print usage

### Adding a New Subcommand

1. Add a new `case "<name>":` to the dispatch switch in `main.go:81-127` that calls `<name>Cmd(args[1:])`.
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

Adapters are registered in priority order in `internal/adapter/adapter.go:76-82` (`var All`). `adapter.ByName` looks up an adapter by its `Name()`.

### Adding a New Package Manager

1. Create `internal/adapter/<manager>.go` implementing `Adapter`.
2. Add the new type to the `All` slice in `internal/adapter/adapter.go:76-82`.
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
| `schema/v1/genv.json` | JSON Schema for `genv.json` validation |
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

- Prefer `genv upgrade` to plan only packages with detected updates by default, with `--all` as the escape hatch for the old brute-force “touch everything” path.
- Keep successful upgrade output visible; filtering should drop non-outdated plan noise, not hide upgrades that actually run.
- When choosing implementation scope, often asks for a recommendation first; if wanting maximum coverage, prefers the most thorough option that still keeps outdated detection honest.
- Prefers subagent-driven execution for multi-step implementation plans when that option is offered.
- Prefers agent guidance as a global baseline under `~/.config/genv/` (and Cursor User Rules when configured); add project-specific rules only when explicitly requested.
- For cross-platform shell/env work (including PowerShell), do not assume PowerShell exists on every host; gate apply/write on availability.
- Prefer `genv add` to persist into `genv.json` only after a successful install/verification (fail nonzero and leave the spec unchanged on unresolved/install failure).
- Want richer tab completions with package-manager repo autofill for `add`, not only tracked package IDs and detected managers.
- Treat multi-machine / cross-OS config portability (export-to-target / migration without sharing locks) as a first-class product goal.
- For public Linux support, focus on major distros whose packaging channels do not require separate human review gates known to reject AI-assisted tooling.

## Learned Workspace Facts

- `genv upgrade` and `genv updates check` share the upgrade planner; default planning uses outdated detection (`Filters.All` false / `OutdatedLister`), and managers without a lister (or on lister error) keep packages rather than silently skipping them.
- `OutdatedLister` coverage includes brew/linuxbrew, mas, bun, npm/pnpm/yarn, uv/pipx, cargo, winget/scoop/choco, pacman/paru/yay, and snap; mas also implements `BatchUpgrader` so multiple App Store upgrades batch into one `mas upgrade` invocation.
- Design specs and plans live under `docs/superpowers/`; completed items (e.g. outdated-aware upgrade, scheduler handoff) are marked COMPLETED/RESOLVED in-place in those docs.
- Runtime user config, lock files, and a separate global `AGENTS.md`/agent baseline live under `~/.config/genv/`.
- Project agent guidance is primarily this `AGENTS.md`; `.cursor/` is gitignored and there is no project `.cursor/rules` pack yet.
- `make ci` and GitHub CI enforce statement coverage via `cover-gate` (`COVER_MIN` default 80) and cold-start via `bench-gate`.
- Schema versions `"1"`–`"8"` are accepted. v7 adds `"shell": "powershell"` targeting for aliases/functions. v8 adds portable `defaults` plus `targets.*` bundles; top-level desired-state blocks and per-record `host` are invalid in v8. Known targets are `macos`, `windows`, `arch`, `ubuntu`, `wsl-arch`, and optional catch-all `linux`; `genv apply` selects `--target`, then `GENV_TARGET`, then host classification.
- Schema v8 target overlays support `null` tombstones for inherited `env`, `shell.aliases`, `shell.functions`, and `services` map entries. Tombstones are valid only under `targets.*`, not `defaults`.
- `genv migrate` converts v1-v7 host-scoped specs to v8 target buckets; `genv export --target --out` writes a single-target snapshot plus report and bundled relative file assets while omitting locks and sensitive env values; `genv map --target` is print-only guidance for manager mapping gaps.
- On Windows, `genv apply` uses a profile-backend abstraction (`POSIXBackend` + `PowerShellBackend`): prefer `pwsh`, else `powershell.exe`; omit PowerShell writes on non-Windows. Shared `env` maps emit both `env.sh` and `env.ps1` when the corresponding backend runs.
- Publishing the `genv` binary to winget/scoop/choco remains deferred; adapters already manage packages through those managers when present.
- Host classification currently recognizes `macos`, native `windows`, native `arch`, `ubuntu`, and `wsl-arch`. WSL2 does not inherit native `arch`: Ubuntu-like WSL2 classifies as `ubuntu`, Arch-like WSL2 classifies as `wsl-arch`, and other WSL distros require `GENV_TARGET`/`--target`.
- Public Linux channels remain Arch-first plus `snap` and `linuxbrew`; `apt`/`dnf`/`apk` adapters are deferred.
- Lock files are machine-local and must not travel with the spec. Schema v8 locks record target/GOOS/manager metadata; foreign locks are refused unless `genv apply --force-new-lock` backs them up and starts a new local lock.
- Today `genv add` can still write the spec before install succeeds and exit 0 on failure; fixing that ordering is a known v4 correctness goal.
