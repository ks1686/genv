# Changelog

All notable changes to this project will be documented in this file.

## Unreleased

## v3.2.1 - 2026-07-21

### Changed

- Internal cleanup for outdated detection: shared registry HTTP helper, `Filters.All` as the single `--all` / outdated-filter switch (removed redundant `DetectOutdated`), co-located outdated helpers, and table-driven outdated adapter tests.

## v3.2.0 - 2026-07-21

### Changed

- `genv upgrade` now plans only packages with a detected update by default (same outdated filtering as `genv updates check`). Pass `--all` to restore the previous brute-force plan of every unconstrained tracked package. Outdated detection now also covers npm/pnpm/yarn, uv/pipx, cargo, winget/scoop/choco, pacman/paru/yay, and snap. Multiple Mac App Store upgrades are batched into one `mas upgrade` invocation.

## v3.1.0 - 2026-07-15

### Changed

- `genv updates check` and the background updates checker now report only packages that actually have an update available, instead of planning an upgrade for every tracked package. The checker previously emitted one upgrade batch per manager unconditionally, so its notification count never reflected reality and never dropped after upgrading. Outdated status is now detected per manager — `brew outdated --json=v2` (formulae and casks), `mas outdated`, and, because bun has no reliable global-outdated command, an npm registry `latest` comparison for global bun packages. A manager whose query fails keeps all of its packages (so a real update is never silently missed), and managers without outdated detection are unchanged. The scheduled-check notification now counts outdated packages rather than batches and stays silent when nothing is outdated. `genv upgrade` is unchanged and still upgrades every tracked package, letting each manager skip already-current ones.

## v3.0.5 - 2026-07-13

### Fixed

- The `gem` adapter no longer reports gems from an installation directory it cannot write to (e.g. macOS system Ruby at `/Library/Ruby/Gems`). Those gems are root-owned, so `genv scan` was adopting dozens of unmanageable packages that failed every `genv upgrade` with `Gem::FilePermissionError`. When the active gem install dir is not writable, the adapter now lists nothing.

## v3.0.4 - 2026-07-13

### Fixed

- `genv scan` no longer re-adopts packages whose friendly ID differs from their manager-specific name. Adapters like `mas` report installed apps by their manager name (a numeric App Store product ID), so an app already tracked as `{"id":"xcode","managers":{"mas":"497799835"}}` was adopted again on every scan as a duplicate bare-numeric `497799835` entry — doubling its App Store upgrade work. Scan now treats every `managers` value as already tracked.

## v3.0.3 - 2026-07-13

### Fixed

- The Homebrew cask now strips `com.apple.quarantine` on install. The release binaries are adhoc-signed (not notarized), so macOS quarantined them, and launchd refused to exec a quarantined adhoc binary (`OS_REASON_EXEC`) — breaking `genv updates` scheduled jobs after every cask install/upgrade.
- `bun` global packages are now upgraded with `bun add --global` instead of `bun update --global`. The latter looks for a local `package.json` and no-ops for globally-installed packages ("No package.json, so nothing to update"), so scheduled upgrades never actually updated bun globals and reported failures.

## v3.0.2 - 2026-07-13

### Fixed

- Automatic manager discovery is now platform-native: macOS search, scan, and implicit resolution use `brew` without duplicate `linuxbrew` suggestions, while Linux uses `linuxbrew`. Explicit `prefer` and `managers` configuration remains authoritative when the selected manager is available.
- Managed update jobs now receive a deterministic package-manager `PATH` under launchd and systemd, and `genv updates status` distinguishes registration, active execution, successful runs, and failed runs using real supervisor state.
- `genv upgrade` and managed updates conservatively skip version-constrained packages when an adapter cannot guarantee a compatible target instead of issuing an unsafe latest-version upgrade.
- Scheduled auto-apply now fails closed when its audit log cannot be opened, records each failed action with its tracked package IDs, and retains bounded, credential-redacted manager diagnostics.

## v3.0.1 - 2026-07-11

### Fixed

- `genv env set`, `genv shell alias set`, and `genv service add` no longer downgrade `schemaVersion` when editing a spec that already declares a newer version. Previously they rewrote the version to the minimum required by the newly-added block (`2`, `3`, and `4` respectively), which corrupted v5/v6 files (dropping support for `files`, `hooks`, and `updates` blocks) and caused the very next validation to fail. They now raise the version to the required minimum only when the current version is older.

## v3.0.0 - 2026-07-11

### Added

- `genv upgrade` now supports machine-readable `--json` output plus tracked-only filters: `--only`, `--skip`, `--only-manager`, and `--skip-manager`.
- `genv updates check/start/stop/status` adds a managed updates checker built on the same tracked-only upgrade planner. It defaults to check/log/notify behavior and only applies upgrades when `updates.autoApply` is explicitly enabled.
- Schema v6 adds the `updates` block and expands lifecycle hooks to apply/add/remove/upgrade phases with `--no-hooks`, hook timeouts, deterministic hook context environment, and script-file hook references.
- `genv profile list/create/switch` adds named profiles stored under `profiles/<name>.json`, merged over the base `genv.json`, with active profile state recorded in the lock file.
- Added tracked-only language/tool/plugin adapters for global JS/TS, Python/data, Rust, Go, Ruby/PHP/.NET, Haskell/OCaml/Julia, universal version managers, Kubernetes plugins, Helm plugins, and VS Code extensions.

### Changed

- Resolver fallback is now restricted to system package managers. Ecosystem, toolchain, and plugin managers such as `npm`, `cargo`, `go`, `krew`, `helm`, and `vscode` remain selectable through `prefer` or `managers`, but are never blind fallback targets.
- Public docs now explicitly distinguish genv's tracked-only upgrade model from topgrade-style system-wide update-all behavior.
- macOS user-facing manager choices dedupe `brew`/`linuxbrew` while preserving existing `linuxbrew` specs and locks.

### Documentation

- README, SCHEMA, and ROADMAP now document schema v6, updates, profiles, lifecycle hooks, tracked-only adapter semantics, and the v3.0.0 closure of the currently committed roadmap backlog.

## v2.4.0 - 2026-07-07

### Changed

- `genv upgrade` now batches tracked packages by package manager and issues one selective multi-package upgrade command per manager when the underlying tool supports it (`pacman`, `paru`, `yay`, `brew`, `linuxbrew`, `choco`, `scoop`, `snap`). Managers without selective multi-package upgrade syntax (`uv`, `mas`, `bun`) remain per-package. Post-upgrade version refresh also uses a single list command for managers that support it (`pacman`, `paru`, `yay`, `snap`, plus existing `bun`, `choco`, `scoop`, `uv`, `mas`), reducing the total number of subprocesses from O(packages) to O(managers). Untracked packages are never touched.

### Added

- New optional `adapter.BatchUpgrader` interface for adapters that can upgrade multiple named packages in one command while leaving untracked packages alone.
- `ListInstalledVersions` implementations for `pacman`, `paru`, `yay`, and `snap` to power the batched version refresh.

### Documentation

- README, CHANGELOG, ROADMAP, AGENTS, and CONTRIBUTING now reflect the batched upgrade behavior and the new `BatchUpgrader` extension.

## v2.3.4 - 2026-07-07

### Added

- **`genv completion install [shell] [--dir <path>]`** — installs the embedded completion script into the shell's standard completion directory so completions work with no manual setup: zsh → `$XDG_DATA_HOME/zsh/site-functions/_genv`, bash → `$XDG_DATA_HOME/bash-completion/completions/genv`, fish → `$XDG_CONFIG_HOME/fish/completions/genv.fish`. The shell is auto-detected from `$SHELL` when omitted, and `--dir` overrides the target directory (e.g. to install into a directory already on your zsh `$fpath`). The positional shell may appear before or after `--dir`.
- The shipped bash/zsh/fish completion scripts now complete the new `completion install` subcommand and its `--dir` flag.

## v2.3.3 - 2026-07-07

### Added

- **`mas` adapter** for managing Mac App Store apps through the `mas` CLI (macOS only). Apps are tracked by their numeric App Store product ID via the `managers.mas` field, e.g. `{"id": "xcode", "managers": {"mas": "497799835"}}`. Implements install/uninstall (`sudo`-prefixed, like `pacman`/`snap`)/upgrade planning, installed-membership and version queries, and the batch `VersionLister` path (a single `mas list` call) consumed by `genv scan` and `genv status`. `mas` is now a recognized manager in both Go schema validation and the published JSON schema.

### Fixed

- `genv upgrade` no longer prints raw lifecycle-hook command strings to the terminal. The per-hook "running hook" log line dropped from INFO to DEBUG, so it now appears only under `--debug` instead of trailing every upgrade with escaped shell commands.

## v2.3.2 - 2026-07-07

### Fixed

- Shortened Snap Store package summary metadata to satisfy snapcraft's 78-character validation limit.

## v2.3.1 - 2026-07-07

### Changed

- Rewrote repository history to preserve only the active contributors, Karim Smires and Omar Waseem, while removing bot/AI co-author trailers from commits.
- Reduced redundant package-manager listing work for `bun`, `choco`, `scoop`, and `uv`, and taught `genv scan` to use the new batch version-listing path instead of querying versions package-by-package.
- Parallelized package search across searchable adapters while preserving adapter-priority ordering and first-seen deduplication.
- Cached repeated adapter lookups in resolver upgrade/removal planning paths.

### Fixed

- Hooks now use `cmd /C` on native Windows instead of assuming `sh -c` exists.
- File symlink errors on Windows now include an actionable Developer Mode / Administrator hint while preserving the original error for unwrapping.
- Service unit/plist path construction now resolves the user home directory through `os.UserHomeDir()` instead of directly reading `HOME`.
- File-apply summary errors now preserve underlying errors for `errors.Is` / `errors.As` without changing the human-readable summary.
- Built-in help, bash/zsh/fish completions, and CI metadata now reflect the current command set and platform support.
- Release checksum signing now uses cosign v3's Sigstore bundle output instead of the removed separate certificate/signature output flags.

### Documentation

- Rewrote stale install and e2e documentation for schema v5 files, WSL2, macOS, and native Windows.
- Reframed contribution guidance around the project being personal/solo-maintained while preserving Omar Waseem's contributor attribution.
- Updated README, roadmap, release metadata, and agent guidance for the v2.3.x state of the tool.

## v2.3.0 - 2026-07-07

### Added

- **Native Windows support** (previously WSL2-only): a new `windows` host classification (`internal/host.Classify`), plus three new adapters — `winget`, `scoop`, and `choco`. `bun` and `uv` already worked cross-platform and now cover global installs on native Windows too. This was the Windows-support portion of the v3.0.0 milestone; the later v3.0.0 line completes the updates checker as well.
- `files.links[]` gains a third mode, `"merge-dir"`, alongside `"link"` and `"managed-link"`. Instead of symlinking an entire source directory as one unit, it symlinks each file under source individually into target. This lets multiple records target the *same* directory — e.g. one with no `host`, one `host`-filtered — and layer: a later record's same-named file wins over an earlier one without needing `--force`, so a shared base directory plus a small host-specific override directory can compose one target directory, instead of requiring a full separate source tree per host. `genv status --files` reports one entry per merged file for per-file drift detection.

### Fixed

- `Uv.PlanClean` returned `nil` ("no standard tool-only cache-clean command") — true for `uv tool`, but `uv` itself has a real global cache-clean command. `genv clean` now runs `uv cache clean` for uv, same as every other adapter with a real cache to clear.

### Documentation

- README's CLI reference table was missing `genv adopt --files`, `genv status --files`, and `genv pull` entirely (never added when they shipped). Documented all three, plus a `genv pull` flags section and the `--files` flag under `genv status` flags.

## v2.2.1 - 2026-07-06

### Fixed

- `Brew`/`Linuxbrew` adapters' `ListInstalled` ran `brew list --formula --1` (and `--cask --1`) — `--1` is not a recognized brew flag, so brew silently printed its usage banner and exited 0 instead of listing anything. This broke `genv scan` for every brew/Linuxbrew user: it always reported "0 added" regardless of how much was actually installed. Fixed to `-1` (one-per-line output), with new regression tests in `internal/adapter/brew_test.go` that check the exact arguments passed, not just that *some* output comes back.

## v2.2.0 - 2026-07-06

Ships the `tc-genv-migration` surface: schema v5 (`files` + `hooks` blocks, `host` selector, `repo` field), three new adapters, and the commands needed to move a dotfiles repo from shell scripts to a declarative `genv.json`. This is a **scoped subset** of Milestone M13 (hooks and lifecycle scripts) — see ROADMAP.md M13 for what's shipped versus still open (no `add`/`remove` hook wiring, no `GENV_EVENT`/`GENV_INSTALLED`/`GENV_REMOVED` env context, no `--no-hooks` flag, no script-file hook references, no hook-specific timeout).

### Schema v5

- `files` block with `link`, `copy`, `copy-template`, and `managed-link` modes, per-record `Host` selector, and a `Backup` flag controlling whether a forced overwrite preserves the old target.
- `hooks` block with three fixed phases: `hooks.preUpgrade`, `hooks.postApply`, `hooks.postUpgrade`. Hooks are literal shell command strings, host-filtered, executed via `sh -c`; the hook carries its own privilege (e.g. `sudo pacman -Syu`) — genv does not add `sudo` itself.
- `host` selector (`HostPredicate`, accepting a single string or a string array) on `packages`, `services`, `files`, and `hooks` records, matched against a runtime-detected `macos`/`arch`/`wsl2` host. WSL2 inherits every `host:"arch"` record.
- `repo` top-level field: the local path to the spec repo, consumed by `genv pull`.
- Lock file location changed: the default lock path is now `~/.config/genv/genv.lock.json` (from the genv config dir), no longer derived from the spec path. Overridable with `--lock-file`. v1–v4 specs still load unchanged.

### Re-added and new adapters

- **`pacman`** — Arch Linux official repositories only (`pacman -S --needed --noconfirm`, `sudo`-prefixed). Re-added because Arch official repositories are first-party to the distro, unlike the v2.1.2-removed apt/dnf-style managers that required submission to and approval by external repositories. AUR packages remain covered by the existing `paru` and `yay` adapters.
- **`bun`** — global installs only (`bun add --global <pkg>`); cwd-scoped installs are out of scope and belong in a hook.
- **`uv`** — global tool installs only (`uv tool install <pkg>`); venv-scoped installs are out of scope.

### New commands

- `genv pull` — self-pulls the spec repo declared in the `repo` field; refuses on a dirty working tree.
- `genv adopt --files` — registers already-managed files into the lock without rewriting them (template targets are rendered before comparison).
- `genv status --files` — live-filesystem parity check for the `files` block, separate from the existing spec-vs-lock `genv status` contract.

### Fixes

- `pacman`'s `PlanInstall`/`PlanUninstall`/`PlanUpgrade`/`PlanClean` commands are now `sudo`-prefixed, matching `snap`. Unlike `paru`/`yay`, bare `pacman` has no built-in privilege escalation, so `genv clean` (which runs every detected adapter's clean command) failed for any non-root user.
- The `internal/host` `Classify()` unit test no longer fails on a plain Linux CI runner that is neither Arch nor WSL2 (it now skips instead of asserting a known class).

---

## v2.1.2 - 2026-03-28

Narrowed supported package managers to only those with self-service deployment pipelines (no external approval required).

### Supported package managers

genv now targets exclusively:

- **`brew`** — Homebrew formulae and casks (macOS and Linux)
- **`linuxbrew`** — Homebrew on Linux (non-macOS path)
- **`paru`** — AUR helper (Arch Linux)
- **`yay`** — AUR helper (Arch Linux)
- **`snap`** — Snap Store (Ubuntu and other Linux distributions with snapd)
- **Raw binary** — pre-built binaries via GitHub Releases (`go install github.com/ks1686/genv@latest`)

### Removed adapters

Removed `apt`, `dnf`, `zypper`, `pacman`, `flatpak`, `xbps`, and `emerge` adapters, along with all associated tests, CI jobs, schema entries, and documentation. These managers require submission to and approval by external package repositories.

### Removed packaging channels

- `.deb` packages (required submission to apt repositories)
- `.rpm` packages (required submission to dnf/rpm repositories)

### Bug fix

- `genv scan` now validates the spec file before checking for available package managers, ensuring invalid files always return an error rather than silently succeeding.

---

## v2.1.1 - 2026-03-27

### Distribution channels

- Snap package now published to Snap Store (`snap install genv`); channel restricted to `stable` to match credential scope.
- Alpine APKBUILD and Fedora COPR packaging removed (require external maintainer approval).

---

## v2.1.0 - 2026-03-27

Milestone M10 is complete. genv now supports virtually every mainstream Linux package manager and adds full user-space service lifecycle management.

### New adapters

- **`zypper`** — openSUSE / SLES; full adapter parity with the existing Linux adapters.
- **`xbps`** — Void Linux's native package manager (`xbps-install`, `xbps-remove`, `xbps-query`).
- **`emerge`** — Gentoo Portage; installs via `emerge`, removes via `emerge --unmerge`, queries via `qlist`.

Complete Linux adapter matrix: `apt`, `dnf`, `zypper`, `pacman`, `paru`, `yay`, `flatpak`, `snap`, `linuxbrew`, `xbps`, `emerge`.

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

- `.deb` and `.rpm` packages published to GitHub Releases via GoReleaser `nfpms`.
- Snap package published to the Snap Store (`snap install genv`).

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

- macOS `brew` adapter validated in the `macos-latest` CI runner.
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
- resolver and adapter support for `apt`, `dnf`, `pacman`, `paru`, `yay`, `flatpak`, `snap`, `brew`, and `linuxbrew`
- Docker-based integration tests validating all Linux adapters in CI
- Homebrew tap and AUR (`genv-bin` pre-compiled, `genv` source) distribution

Notes:

- macOS (`brew`) and WSL2 adapters are implemented but not yet validated in automated CI — tracked in Milestone M5
- `go install github.com/ks1686/genv@latest` works on any platform with Go installed

## v0.1.0-beta.1 - 2026-03-17

First public pre-release of `genv`.

Highlights:

- core CLI commands: `add`, `remove`, `list`, `apply`, `edit`, `clean`, and `version`
- `genv.json` schema v1 and validation
- declarative apply flow backed by `genv.lock.json`
- resolver and adapter support for Linux, macOS, and WSL2-oriented environments
- GitHub release automation with versioned binaries and checksums
