# Agent Instructions for genv

## Project Overview

**genv** is a Go CLI tool that tracks, syncs, and reproduces software environments across Linux, macOS, and WSL2. It sits as a thin layer on top of existing package managers (brew, paru, yay, snap, linuxbrew) and uses a declarative model: edit `genv.json`, run `genv apply`, and the tool makes reality match the spec.

## Architecture

### Core Packages

- **`internal/adapter`** — Adapter interface and implementations for each package manager. Each adapter defines `Name()`, `Available()`, `NormalizeID()`, `PlanInstall()`, `PlanUninstall()`, `PlanUpgrade()`, and optional `Searchable`.
- **`internal/resolver`** — Detects available managers on the host and resolves packages to concrete install/uninstall actions. Entry points: `Detect()`, `ResolveOne()`, `Plan()`.
- **`internal/commands`** — CLI command implementations (`add`, `remove`, `adopt`, `disown`, `scan`, `status`, `apply`, `upgrade`, `env`, `shell`, `service`, etc.). Each command is a pure function that operates on `schema.GenvFile`.
- **`internal/schema`** — `GenvFile` struct, JSON schema, validation logic, and `KnownManagers` registry.
- **`internal/genvfile`** — File I/O for `genv.json` and `genv.lock.json`, including `Read`, `Write`, `ReadLock`, `WriteLock`, `DefaultDir`.
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
3. `resolver.Plan()` computes delta: install new, uninstall removed
4. `adapter` commands execute the plan
5. Lock file is updated on success

## Key Files

| File | Purpose |
|------|---------|
| `main.go` | CLI entry point, Cobra command tree setup |
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

## Common Agent Tasks

### Adding a New Package Manager
1. Create `internal/adapter/<manager>.go` implementing `Adapter`
2. Add to `internal/adapter/adapter.go` `All` slice
3. Add to `schema.KnownManagers` in `internal/schema/schema.go`
4. Write unit tests in `internal/adapter/<manager>_test.go`
5. Update `README.md` supported platforms table
6. Add CI integration test in `.github/workflows/integration.yml`

### Adding a New Command
1. Create `internal/commands/<cmd>.go` with command function
2. Wire into `main.go` Cobra tree
3. Write tests in `internal/commands/<cmd>_test.go`
4. Update `README.md` CLI reference table

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
