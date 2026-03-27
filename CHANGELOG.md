# Changelog

All notable changes to this project will be documented in this file.

## Unreleased — v2.2.0

Targets Milestone M13: hooks and lifecycle scripts. Users will be able to declare `pre` and `post` hooks in `genv.json` for events like `apply`, `add`, `remove`, and `upgrade`, with event context passed via environment variables.

---

## v2.1.0 - 2026-03-27

Milestone M10 is complete. genv now supports virtually every mainstream Linux package manager and adds full user-space service lifecycle management.

### New adapters

- **`apk`** — Alpine Linux package manager; supports install, remove, cache clean, version query, and list-installed.
- **`zypper`** — openSUSE / SLES; full adapter parity with the existing Linux adapters.
- **`xbps`** — Void Linux's native package manager (`xbps-install`, `xbps-remove`, `xbps-query`).
- **`emerge`** — Gentoo Portage; installs via `emerge`, removes via `emerge --unmerge`, queries via `qlist`.

Complete Linux adapter matrix: `apt`, `dnf`, `zypper`, `apk`, `pacman`, `paru`, `yay`, `flatpak`, `snap`, `linuxbrew`, `xbps`, `emerge`.

### Services management (`genv service`)

- **`genv service add <name> --start <cmd> [--stop <cmd>]`** — declare a user-space service in the spec.
- **`genv service remove <name>`** — remove a service from the spec.
- **`genv service start <name>`** / **`genv service stop <name>`** — imperatively manage a declared service.
- **`genv service status <name>`** — report whether the service is running; exits non-zero when it is not.
- `genv apply` starts services declared in the spec that are not running and stops services removed from the spec.
- Service state is tracked in `genv.lock.json` and surfaces in `genv status` drift output.
- On Linux with systemd: generates a user unit at `~/.config/systemd/user/genv-<name>.service`, managed via `systemctl --user`.
- On macOS with launchd: generates a plist at `~/Library/LaunchAgents/genv.<name>.plist`, managed via `launchctl`.
- All service commands use explicit argv slices — no shell interpolation.

### Schema (v4)

- `genv.json` schema v4 adds the `services` block.

### Distribution channels

- `.deb`, `.rpm`, and `.apk` packages published to GitHub Releases via GoReleaser `nfpms`.
- Snap package published to the Snap Store (`snap install genv`).
- APKBUILD submitted to Alpine Linux aports (`apk add genv`).
- Portfile submitted to MacPorts ports tree (`sudo port install genv`).
- Fedora COPR repo: `dnf copr enable ks1686/genv && dnf install genv`.

---

## v2.0.0 - 2026-03-26

Milestones M8 and M9 are complete. genv now manages the full reproducible environment: packages, global shell variables, and shell configuration in a single declarative spec.

### Environment variables (`genv env`) — M8

- **`genv env set <NAME> <value>`** — add or update a variable in the spec.
- **`genv env unset <NAME>`** — remove a variable from the spec.
- **`genv env list`** — show all declared variables and their current resolved values.
- `genv apply` writes variables to `~/.config/genv/env.sh` and injects a source line into the user's shell rc exactly once.
- Variable state is tracked in `genv.lock.json`; `genv status` surfaces drift between declared and exported values.
- Variables marked `sensitive: true` are redacted in `--json` and log output.

### Shell configuration (`genv shell`) — M9

- **`genv shell alias set <name> <value>`** / **`genv shell alias unset <name>`** — manage shell aliases.
- `genv apply` writes aliases and rc snippets to `~/.config/genv/shell.sh` and sources it from the user's rc file; source-line injection is idempotent.
- **`genv shell status`** — diff between declared shell config and what is currently active.
- **`genv shell edit`** — open `genv.json` in `$EDITOR` to edit the shell block directly.
- Per-shell targeting: aliases can be scoped to `bash`, `zsh`, or both.
- Shell config state is tracked in `genv.lock.json`.

### Schema (v2 and v3)

- `genv.json` schema v2 adds the `env` block; schema v3 adds the `shell` block.

---

## v2.0.1 - 2026-03-25

Patch release.

- **fix(pacman):** remove stale `download-*` temp files left in the cache directory before running `pacman -Sc`; previously these could prevent the cache-clean from completing cleanly.
- Internal formatting cleanup in `internal/adapter/adapter_test.go` (gofmt).

---

## v1.0.0 - 2026-03-24

Milestones M6 and M7 are complete. The CLI surface, JSON output schema, and `genv.json` format are now stable with a formal deprecation policy.

### API stability and quality (M6)

- `--json` output envelope gains a `"version"` field; schema is versioned and documented.
- Formal deprecation policy established: breaking changes require a major version bump.
- All internal packages reach ≥80% line coverage as reported by `go test -cover`.
- Property-based and fuzz tests added for version constraint logic and the resolver.
- End-to-end smoke tests run `genv apply` against real package managers in CI.
- Resolver + manager detection benchmarked; <200ms cold-start budget enforced as a CI gate.
- Security audit: all adapter shell invocations reviewed for injection vectors; none found.

### Developer and user experience (M7)

- **`genv completion <bash|zsh|fish>`** — print shell completion script; pipe directly into your rc.
- **`genv validate`** — validate `genv.json` without installing anything; exits 3 on invalid spec.
- **`genv upgrade`** — re-resolve version constraints and update `installedVersion` in the lock.
- **`genv init`** — interactive wizard to scaffold a new `genv.json` from scratch.
- Every user-facing error now includes a corrective action or relevant flag reference.
- **`--quiet`** flag on `genv apply` — suppresses plan output for scripts alongside `--yes`.

---

## v1.0.1 - 2026-03-24

Patch release.

- **refactor:** simplified `upgradeCmd` — eliminated redundant `adapter.ByName()` call by storing the adapter at plan-build time; replaced O(n²) `InstalledVersion` update loop with an O(1) map lookup.
- **refactor:** switched `initCmd` stdin reading from `bufio.Scanner` to `bufio.NewReader` to match the `confirm()` helper pattern used throughout the rest of the CLI.

---

## v0.2.0 - 2026-03-23

Second stable release of `genv`. Milestones M3, M4, and M5 are complete. All five delivery milestones are now done.

### New commands

- **`genv scan`** — discovers every package installed across all available managers and bulk-adopts them into `genv.json` and the lock file. Deduplicates packages that appear in multiple managers (e.g. `paru` and `yay` both surface the pacman DB).
- **`genv status`** — compares `genv.json` against `genv.lock.json` and reports drift, missing installs, and orphaned lock entries. Exits with code 4 when actionable drift is found, making it usable as a CI gate.

### New flags

- `genv apply --yes` — skips the confirmation prompt; safe for CI pipelines and bootstrap scripts.
- `genv apply --json`, `genv status --json`, `genv scan --json` — emits a stable JSON envelope to stdout and routes subprocess output to stderr, keeping stdout clean for `jq` and other tools.
- `genv apply --timeout <duration>` — sets a per-subprocess deadline (e.g. `--timeout 5m`); the process is canceled cleanly when the deadline fires.
- `genv apply --debug`, `genv status --debug`, `genv scan --debug` — enables debug-level structured logging to stderr via `log/slog`, including subprocess spawn events with elapsed duration.

### Reproducibility

- Lock file now records `installedVersion` after each successful install (best-effort via per-adapter `QueryVersion`).
- `genv apply` detects version drift: if the recorded version no longer satisfies the spec constraint, the package is queued for reinstall.
- Packages with no recorded version (old lock entries) are never treated as drifted — full backward compatibility with existing lock files.

### Release hardening

- Binaries are built with `-trimpath` for reproducible output across machines.
- `checksums.txt` is signed with [cosign](https://docs.sigstore.dev/cosign/overview/) using keyless (OIDC) signing. The `.sig` and `.pem` files are attached to every GitHub release.

### Cross-platform (M5)

- macOS `brew` and `macports` adapters validated in the `macos-latest` CI runner.
- WSL2 detection sanitizes `$PATH` to strip Windows-host binary paths, preventing Windows binaries from shadowing Linux ones.
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

First stable release of `genv`. Milestones M1 and M2 are complete and validated on Linux.

Highlights:

- core CLI commands: `add`, `remove`, `adopt`, `disown`, `list`, `apply`, `edit`, `clean`, and `version`
- `genv.json` schema v1 with line-aware validation errors
- declarative apply flow backed by `genv.lock.json`
- `genv adopt` — track an already-installed package without reinstalling it
- `genv disown` — stop tracking a package without uninstalling it
- resolver and adapter support for `apt`, `dnf`, `pacman`, `paru`, `yay`, `flatpak`, `snap`, `brew`, `macports`, and `linuxbrew`
- Docker-based integration tests validating all Linux adapters in CI
- Homebrew tap and AUR (`genv-bin` pre-compiled, `genv` source) distribution

Notes:

- macOS (`brew`, `macports`) and WSL2 adapters are implemented but not yet validated in automated CI — tracked in Milestone M5
- `go install github.com/ks1686/genv@latest` works on any platform with Go installed

## v0.1.0-beta.1 - 2026-03-17

First public pre-release of `genv`.

Highlights:

- core CLI commands: `add`, `remove`, `list`, `apply`, `edit`, `clean`, and `version`
- `genv.json` schema v1 and validation
- declarative apply flow backed by `genv.lock.json`
- resolver and adapter support for Linux, macOS, and WSL2-oriented environments
- GitHub release automation with versioned binaries and checksums
