# Changelog

All notable changes to this project will be documented in this file.

## v0.2.0 - 2026-03-23

Second stable release of `gpm`. Milestones M3, M4, and M5 are complete. All five delivery milestones are now done.

### New commands

- **`gpm scan`** — discovers every package installed across all available managers and bulk-adopts them into `gpm.json` and the lock file. Deduplicates packages that appear in multiple managers (e.g. `paru` and `yay` both surface the pacman DB).
- **`gpm status`** — compares `gpm.json` against `gpm.lock.json` and reports drift, missing installs, and orphaned lock entries. Exits with code 4 when actionable drift is found, making it usable as a CI gate.

### New flags

- `gpm apply --yes` — skips the confirmation prompt; safe for CI pipelines and bootstrap scripts.
- `gpm apply --json`, `gpm status --json`, `gpm scan --json` — emits a stable JSON envelope to stdout and routes subprocess output to stderr, keeping stdout clean for `jq` and other tools.
- `gpm apply --timeout <duration>` — sets a per-subprocess deadline (e.g. `--timeout 5m`); the process is cancelled cleanly when the deadline fires.
- `gpm apply --debug`, `gpm status --debug`, `gpm scan --debug` — enables debug-level structured logging to stderr via `log/slog`, including subprocess spawn events with elapsed duration.

### Reproducibility

- Lock file now records `installedVersion` after each successful install (best-effort via per-adapter `QueryVersion`).
- `gpm apply` detects version drift: if the recorded version no longer satisfies the spec constraint, the package is queued for reinstall.
- Packages with no recorded version (old lock entries) are never treated as drifted — full backward compatibility with existing lock files.

### Release hardening

- Binaries are built with `-trimpath` for reproducible output across machines.
- `checksums.txt` is signed with [cosign](https://docs.sigstore.dev/cosign/overview/) using keyless (OIDC) signing. The `.sig` and `.pem` files are attached to every GitHub release.

### Cross-platform (M5)

- macOS `brew` and `macports` adapters validated in the `macos-latest` CI runner.
- WSL2 detection sanitises `$PATH` to strip Windows-host binary paths, preventing Windows binaries from shadowing Linux ones.
- Install guides added for [macOS](docs/macos-install.md) and [WSL2](docs/wsl2-install.md).

### Internal

- New `internal/logging` package: calls `slog.SetDefault` so all packages use the global logger without import coupling.
- New `internal/output` package: stable `Envelope`, `PlanResult`, `StatusResult`, `ScanResult`, and `ApplyResult` JSON types.
- `resolver.Execute` and `resolver.ExecuteApply` now accept `context.Context` as the first argument.
- Each adapter implements `ListInstalled() ([]string, error)` and `QueryVersion(pkgName string) (string, error)`.
- New `internal/version` package: `Satisfies(constraint, installed string) bool` with wildcard prefix support.
- New `internal/commands/status.go`: `Status(f, lf)` pure function, fully unit-tested.

---

## v0.1.0 - 2026-03-18

First stable release of `gpm`. Milestones M1 and M2 are complete and validated on Linux.

Highlights:

- core CLI commands: `add`, `remove`, `adopt`, `disown`, `list`, `apply`, `edit`, `clean`, and `version`
- `gpm.json` schema v1 with line-aware validation errors
- declarative apply flow backed by `gpm.lock.json`
- `gpm adopt` — track an already-installed package without reinstalling it
- `gpm disown` — stop tracking a package without uninstalling it
- resolver and adapter support for `apt`, `dnf`, `pacman`, `paru`, `yay`, `flatpak`, `snap`, `brew`, `macports`, and `linuxbrew`
- Docker-based integration tests validating all Linux adapters in CI
- Homebrew tap and AUR (`gpm-bin`) distribution

Notes:

- macOS (`brew`, `macports`) and WSL2 adapters are implemented but not yet validated in automated CI — tracked in Milestone M5
- `go install github.com/ks1686/gpm@latest` works on any platform with Go installed

## v0.1.0-beta.1 - 2026-03-17

First public pre-release of `gpm`.

Highlights:

- core CLI commands: `add`, `remove`, `list`, `apply`, `edit`, `clean`, and `version`
- `gpm.json` schema v1 and validation
- declarative apply flow backed by `gpm.lock.json`
- resolver and adapter support for Linux, macOS, and WSL2-oriented environments
- GitHub release automation with versioned binaries and checksums
