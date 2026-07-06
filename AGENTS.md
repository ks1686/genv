# Agent Instructions for genv

## Project Overview

**genv** is a Go CLI tool that tracks, syncs, and reproduces software environments across Linux, macOS, and WSL2. It sits as a thin layer on top of existing package managers (brew, paru, yay, snap, linuxbrew) and uses a declarative model: edit `genv.json`, run `genv apply`, and the tool makes reality match the spec.

## Architecture

### Core Packages

- **`internal/adapter`** — Adapter interface and implementations for each package manager. Each adapter defines `Name()`, `Available()`, `NormalizeID()`, `PlanInstall()`, `PlanUninstall()`, `PlanUpgrade()`, `PlanClean()`, `Query()`, `ListInstalled()`, and `QueryVersion()`. Optional `Searchable` extension for repository search.
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

Every package manager adapter lives in `internal/adapter/` and implements the `Adapter` interface defined in `internal/adapter/adapter.go:28-71`:

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

Optional `Searchable` is defined at `internal/adapter/adapter.go:17-23`.

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
| `go.mod` | Module: `github.com/ks1686/genv`, Go 1.26.1 |
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

- **Go version**: 1.26.1
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

## CodeGraph

This repo has a `.codegraph/` index. Use it before grep/find when exploring code:

```bash
codegraph explore "resolver Detect"     # symbols + call paths
codegraph node resolver.go              # full file with dependents
codegraph impact "Adapter.Available"    # blast radius of a change
```

The post-commit hook auto-syncs the index after each commit.
