# Changelog

All notable changes to this project will be documented in this file.

## Unreleased

## v4.3.1 - 2026-09-05

### Fixed

- `genv scan` no longer proposes non-package ids: `-` from `uv tool list`
  entrypoint bullets, `npm` from `npm list -g` reporting itself, or
  `toolchain:*` from rustup. uv parses only `name v<version>` headers;
  rustup `ListForScan` is empty (ListInstalled still reports toolchains
  for apply/status); `--all` still drops those three shapes.

- `genv upgrade --json` wet-run now requires `--yes` to execute (or `--dry-run`
  to plan only), matching the human confirmation path. The refused envelope
  still includes the planned batches.
- Leftover `genv upgrade <id>` arguments now apply as `--only` filters, matching
  shell completions. `--only` and positionals merge.
- Upgrade and index-refresh planning skip a manager when `Available()` is false
  and record an explicit reason, instead of emitting commands that cannot run.
- `genv apply --skip-packages` no longer inventories live package managers or
  prints the per-package `(up to date)` table. The header names files/env/services
  instead of a package count, and JSON omits the package plan the same way.
  Env, shell, files, and services still apply; lock packages are left untouched.
- Native Windows `genv updates` no longer flashes a console on each Task
  Scheduler run. The task still uses `InteractiveToken` (so `updates.notify`
  can toast) and still starts through `schtasks /Run` (so OpenSSH cannot kill
  it). The action is now `wscript.exe //B //Nologo` plus a `.vbs` host that
  `WshShell.Run`s the existing `.cmd` with window style 0. `<Hidden>true</Hidden>`
  alone does not hide `cmd.exe`. `updates.log` and one completion notification
  are unchanged. Do not flip `genv.exe` to the Windows GUI subsystem.
  `updates stop` now ends the running task before delete and retries removing
  the `.vbs` host if `wscript` still has it open (sharing violation).
- The `vscode` adapter now prefers `cursor` on PATH, then `code`. Cursor-only
  hosts can scan, adopt, and upgrade extensions without a `code` shim.
  VS Code-only hosts (`code`, no `cursor`) are unchanged.

### Added

- Tracked-package planning now refreshes each index-based manager once before
  outdated detection. `genv upgrade`, `genv updates check`, and the hourly
  `__run-once` / `autoApply` worker share that path. Refresh argv: `brew update`
  (brew and linuxbrew share one call), `sudo apt-get update`,
  `sudo pacman -Sy --noconfirm` (not `-Syu`), `paru`/`yay -Sy --noconfirm`,
  `sudo dnf makecache`, `sudo apk update`, `scoop update`,
  `winget source update`. Live registries (mas, bun/npm/pnpm/yarn, uv/pipx,
  pip-user, cargo, volta, choco, snap, vscode) stay as-is. A failed or timed-out
  refresh keeps that manager's packages and warns on wet upgrade, check, and
  `updates.log`. `--all` still refreshes. Human plans show the refresh command
  (e.g. `brew  ==> brew update`). `brew outdated` now passes `--greedy` so
  auto-updating casks are not dropped after the fetch.

### Changed

- `genv scan` now proposes user-facing installs by default: Homebrew
  `brew leaves` plus casks, Ruby gems that are not default or bundled,
  and pip-user packages that are not dependencies of other user-site
  packages (minus installer/stdlib-like noise). npm/pnpm/yarn were
  already top-level (`--depth=0`). Pass `--all` or `--deps` for the
  previous full `ListInstalled` inventory. `--dry-run` on a host whose
  brew leaves and casks are already tracked stays near-empty aside from
  real extras.
- README documents the hard skip for any non-empty package `version` during
  upgrade (range-satisfying upgrades are not implemented).

## v4.3.0 - 2026-09-03

### Added

- Native Windows `genv updates start` registers a per-user Task Scheduler
  job (`schtasks`) so the hourly updates checker has the same start / status /
  stop lifecycle as systemd --user and launchd. The task runs at logon plus
  `updates.interval`, sets a PATH that includes scoop/winget shims, and is
  started through the scheduler service so OpenSSH job objects cannot kill it.
- `genv upgrade` now runs a named step runner after the existing tracked-package
  planner: OS vendor updates for the active target, then firmware when a clean
  tool exists. macOS uses `sudo softwareupdate -i -a`. Windows uses the built-in
  Windows Update Agent COM API via `pwsh` (else `powershell` / `powershell.exe`)
  — no extra modules, and not `winget` (which upgrades packages, not the OS).
  Arch and wsl-arch use `sudo pacman -Syu --noconfirm` (paru/yay stay
  tracked-package adapters). Ubuntu uses `sudo apt-get update` then
  `sudo apt-get upgrade -y` (or `apt` if `apt-get` is absent); snap stays
  tracked-only. Linux firmware is `sudo fwupdmgr update` when `fwupdmgr` is
  present; macOS firmware is delivered by the system step; Windows firmware is
  skipped as vendor-specific. Missing tools are skipped with a reason; elevation
  is part of the planned command, not silently dropped. Step failures do not
  abort later steps. `genv updates check`, the timer, and `updates.autoApply`
  remain tracked packages only. Further steps (rustup, editors, containers, and
  other extra tools) land in follow-ups.

### Fixed

- `genv updates check` / `genv upgrade` no longer treat VS Code/Cursor
  extensions as perpetually outdated. The vscode adapter now queries the
  editor's marketplace (from `product.json`, so Cursor uses
  marketplace.cursorapi.com) and compares against the newest **stable**
  version. Pre-release gallery versions are skipped; the emitted command
  remains `code --install-extension <id> --force`, which cannot install
  pre-releases. Previously vscode had no `OutdatedLister`, so every
  tracked extension was kept on every check.
- `genv apply` recovers when the lock and the manager disagree: uninstalling
  a package that is already gone is treated as success and the lock entry is
  dropped. `genv remove` writes the spec only after uninstall succeeds, and
  `genv disown` can clear lock-only leftovers. Package removal failures no
  longer skip post-apply hooks unless there is a real unresolved file mismatch.
- asdf, sdkman, and stack `Query` no longer report absent for packages they
  cannot inspect. After apply's Query-based uninstall recovery, that false
  absent dropped the lock while the package was still installed. Those adapters
  now return an error so uninstall still runs.

## v4.2.2 - 2026-08-22

### Fixed

- README now documents every apply/status/upgrade flag it references (including
  `--skip-packages`, `--timeout`, and the upgrade filter flags), and install
  examples pin the current release.
- Shell completions offer `status --offline` and `completion install
  powershell` in all four shells; six stale `--host` help strings now say the
  default is host classification, matching reality and the README.
- CI workflows cancel superseded runs via concurrency groups instead of
  queueing duplicates.
- The scheduled updates worker no longer abandons an in-flight upgrade when
  its job budget expires: it waits on a short shutdown grace so package
  managers are not killed mid-transaction, and drains in-flight desktop
  notifications (bounded by their own 3s timeout) before closing the audit
  log.
- RELEASING.md now describes AUR publishing accurately (separate macOS job,
  not GoReleaser) and the real changelog exclude filters.
- Spec and lock file writes are now durable across crashes: both are fsynced
  before the publishing rename (and the directory afterwards), so a power
  loss can no longer leave an empty or partial `genv.json` /
  `genv.lock.json`.
- `genv pull` writes the pulled spec through a temp file and rename, so an
  interrupted pull can no longer leave `genv.json` truncated.
- Every package-manager probe is now bounded. `genv scan`, `genv search`,
  upgrade version capture, the outdated check, and service status probes cap
  each manager subprocess (30s default), so a hung manager — winget's
  first-run source sync can stall for minutes on fresh profiles — can no
  longer wedge the command. Timed-out probes surface as errors or
  conservative fallbacks: a timed-out outdated query keeps all packages, so
  real upgrades are never silently skipped.

### Changed

- `internal/resolver`: exported `DefaultLiveListTimeout` and added `CallTimed`
  / `RunTimed` helpers so commands that inventory managers directly share the
  same per-spawn deadline machinery as apply/status.
- Adapter probes (`ListInstalled`, `QueryVersion`, `Query`, `ListOutdated`,
  availability checks) run under a shared deadline and set `WaitDelay`, so a
  killed manager's orphaned children cannot hold output pipes open.

## v4.2.1 - 2026-08-20

### Fixed

- `genv apply` / `genv status` only inventory managers needed by unlocked spec
  packages, and each listing times out after 30s. A hung `composer global show`
  no longer stalls Windows CI or a real apply.

## v4.2.0 - 2026-08-20

### Fixed

- `genv apply` no longer treats an empty lock as “install everything”. Packages
  already present in winget/scoop are adopted into the lock (no upgrade).
- `winget install` uses `--disable-interactivity --no-upgrade`. Apply’s
  per-subprocess timeout defaults to 10m (`--timeout 0` disables).
- A failed or hung package no longer skips env/files.
- `genv adopt <id>` reads `managers.<mgr>` from the spec (e.g. `Anysphere.Cursor`)
  and can lock a package that is already listed in the spec.
- Windows status/apply no longer report zsh aliases or `HOMEBREW_*` as missing.
- Scoop subprocesses find git via the versioned `scoop/apps/git/<ver>/cmd`
  directory when the `current` junction is invisible (OpenSSH).

### Added

- `genv apply --skip-packages`
- `genv status --offline` (lock-only). Default status probes live managers;
  installed-but-unlocked packages are `present`.
- `external` manager for apps genv tracks but does not install.

## v4.1.0 - 2026-08-19

### Added

- Self-hosted Scoop install channel on stable tags: GoReleaser publishes
  `genv.json` to `ks1686/scoop-bucket`. v4.0.13 shipped GitHub/Homebrew; this
  release is the first tag that uploads the Scoop manifest from CI.

### Fixed

- Scoop publisher token template matches Homebrew (`{{ .Env.SCOOP_BUCKET_GITHUB_TOKEN }}`).
  GoReleaser 2.17 has no `envOrDefault`, which aborted the v4.0.13 scoop upload.
- `TestRunSubcmd_PerSpawnTimeout` gives the follow-up spawn 2s (not 50ms) so
  Windows CI does not fail `go env GOVERSION` after killing `sleep`.

## v4.0.13 - 2026-08-19

### Added

- Self-hosted Scoop install channel: GoReleaser publishes a manifest to
  `ks1686/scoop-bucket` when `SCOOP_BUCKET_GITHUB_TOKEN` is set, and skips that
  upload (without failing the rest of the release) when the token is empty or
  unset.

## v4.0.12 - 2026-08-15

### Fixed

- Release config no longer declares GoReleaser Pro-only `wingets` / `chocolateys` keys. OSS GoReleaser 2.17 rejected them and aborted before publishing `v4.0.11`. Scoop remains configured with `skip_upload: true`.

## v4.0.11 - 2026-08-15

### Fixed

- `genv add` now installs first and only then writes `genv.json` / the lock. Unresolved or failed installs leave the spec unchanged and exit `4`. Use `genv adopt` to track without installing.
- New specs from `genv init`, first `add`/`scan`, and `genvfile.New` are schemaVersion 8 (`defaults` + known `targets.*`). Named profiles are refused on v8 specs.
- Files apply rejects relative sources that escape `SourceRoot`. `--force-new-lock --dry-run` no longer renames the lock. Alias/function names are POSIX-safe. `runSubcmd` rejects empty argv.
- `apk` name/version split now understands `-rN` releases. `pip-user` and `volta` implement `OutdatedLister`. `krew` availability requires the krew plugin, not just `kubectl`.
- Apply unit tests that plant brew locks now seed schemaVersion 1 so Linux CI is not refused by the v8 foreign-lock gate. E2E spec assertions read v8 `targets.*` packages. Adapter `installFakeBinary` works on Windows (`PathListSeparator` + `.cmd` shim).
- CI `govulncheck` sets `GOTOOLCHAIN=auto` so the scanner can build on Go 1.25 while unit tests stay on go.mod 1.24.3.
- Windows unit tests isolate `USERPROFILE` (not only `HOME`), JSON-escape file paths, and treat execute-bit checks as Unix-only. Portable path helpers (`isAbsolutePath`, `brewStableBin`) no longer follow host `filepath` rules for POSIX/Homebrew strings. Completion search on Windows now stays inside a timeout instead of hanging on live `npm` queries.

### Added

- Native `apt`, `dnf`, and `apk` adapters (install/uninstall/query/search/outdated, default-fallback eligible). Ubuntu mapping prefers `apt` ahead of snap/linuxbrew.
- GoReleaser Scoop stub (`skip_upload: true`). winget / Chocolatey publishers are GoReleaser Pro-only and are not declared in OSS config.
- `Regression` GitHub Actions workflow: fail-closed add, v8 defaults, safety, adapter, files e2e, and actionlint.
- MIT `LICENSE` and `contents: read` on non-release CI workflows.

### Changed

- Install docs use `brew install --cask genv` and current archive names. WSL Ubuntu examples prefer native `apt`.
- Removed leftover Superpowers/handoff session notes and the historical `SECURITY_AUDIT.md` dump. Live security policy remains [SECURITY.md](SECURITY.md).

## v4.0.10 - 2026-08-12

### Added

- Darwin release binaries are signed with a Developer ID Application certificate and notarized via App Store Connect (GoReleaser `notarize.macos` / quill) when `MACOS_*` GitHub Actions secrets are set. See [RELEASING.md](RELEASING.md).

### Changed

- Homebrew cask `post_install` no longer strips `com.apple.quarantine` (notarized binaries do not need it). It still re-runs `genv updates start` when the updates LaunchAgent is present so launchd picks up the new Caskroom path after upgrades.

## v4.0.9 - 2026-08-01

### Fixed

- Lifecycle hooks now inherit stdin (same as package actions), so interactive prompts in hooks work at a TTY.
- `--yes` is exposed to hooks as `GENV_YES=true|false` (alongside existing `GENV_*` context env) so hooks can opt into noninteractive flags.

## v4.0.8 - 2026-08-01

### Added

- Shell completions for `genv add` / `genv adopt` suggest repository package names via
  `genv __complete repo-packages` (cached manager dumps + live search fallback).

## v4.0.7 - 2026-07-27

### Fixed

- `updates __run-once` no longer blocks on desktop notifications after a successful plan. Under launchd, `osascript` could ignore cancel and hold the process until the 5m job deadline (`updates.check.timeout` → exit 4) even though outdated detection had already finished in seconds.

## v4.0.6 - 2026-07-27

### Added

- Scheduled `updates __run-once` logs per-manager outdated-query timings (and total plan duration) so a slow launchd run can be distinguished from a silent timeout fallback.

## v4.0.5 - 2026-07-27

### Fixed

- `updates start` derives Homebrew `bin/genv` from Caskroom/Cellar versioned paths even when the brew symlink is missing or dangling mid-upgrade (no longer depends on `SameFile` alone).
- Scheduled updates PATH no longer retains Homebrew shim directories captured from cask `post_install`.
- `updates __run-once` enforces a wall-clock deadline (launchd `TimeOut` / systemd `TimeoutStartSec`, plus an in-process timeout) so a TLS/keychain hang cannot wedge the hourly checker permanently; `brew outdated` and notifications use bounded command contexts.

## v4.0.4 - 2026-07-27

### Fixed

- `genv updates start` prefers a PATH-stable self path (e.g. Homebrew `bin/genv`) over a version-pinned Caskroom path from `os.Executable` / cask `post_install`, so the updates LaunchAgent/systemd unit survives upgrades. `updates status` warns and `validate` fails when genv-managed agents point at a missing executable.
- Release workflow: AUR publish retries transient `aur.archlinux.org` SSH drops, and `workflow_dispatch` supports `aur-only` repair for an existing GitHub release without re-running GoReleaser.

## v4.0.3 - 2026-07-26

### Fixed

- When `genv apply` leaves unresolved file mismatches, it now prints that post-apply hooks are being skipped (services still run before files; only hooks are gated).

### Added

- `genv scan --dry-run` previews packages that would be adopted without writing the spec or lock.
- `genv scan` text mode confirms before adopting unless `--yes` is set (JSON still writes without a prompt, matching `apply --json`).

## v4.0.2 - 2026-07-26

### Fixed

- `genv apply` no longer aborts the whole run when a managed file mismatches: packages, env, shell, and services still apply; non-conflicting file ops still run; mismatched paths are named; exit `4` if any file issues remain.
- Text apply plans print per-file `create` / `update` / `mismatch` / `ok` lines (not only a count).
- `genv service status` works for `brew_formula` services without a `status` argv (delegates to `brew services list`).
- `genv updates help` / `--help` / `-h` print usage and exit 0.
- After Homebrew cask upgrades, `post_install` re-runs `genv updates start` when the updates LaunchAgent is present; `updates status` and successful `genv upgrade` of package `genv` hint to re-register when launchd codesign kills the agent.
- Unit tests for service apply no longer write live launchd plists under the real `$HOME`.

### Added

- `genv apply --backup` backs up mismatched targets before `--force` overwrite (same effect as per-entry `backup: true`).

### Changed

- `genv migrate` warns when host-unscoped packages are copied into every migrated target bucket.

## v4.0.1 - 2026-07-26

### Fixed

- On schemaVersion 8 specs, `status`, `upgrade`, `updates check` / `__run-once`, `env list`, `shell status`, `service list|start|stop|status`, `adopt --files`, and add/remove lifecycle hooks now materialize the active target via the same `Resolve` + `MergeTarget` path as `apply` (with `--target` / `$GENV_TARGET`). Previously they read empty top-level fields, so status reported every lock entry as `extra` and upgrade/updates silently planned nothing after `genv migrate`.

### Changed

- Integration CI adds an Arch Docker **v8 command matrix** (`scripts/docker-v8-command-matrix.sh` / `make integration-v8`) that builds genv and exercises every top-level CLI command against a pacman-backed schemaVersion 8 spec.

## v4.0.0 - 2026-07-26

### Added

- Native Windows PowerShell parity (schema **v7**): `env`/`shell` profile backends write `env.ps1` / `shell.ps1` and inject the CurrentUser CurrentHost profile when `pwsh` or Windows PowerShell is on `PATH` (prefer `pwsh`). Aliases/functions may set `"shell": "powershell"`; omitted `shell` stays POSIX-only. Hooks on Windows use the detected PowerShell engine (`-NoProfile -Command` / `-File`), with `cmd /C` fallback. `genv completion powershell` embeds and installs `completions/genv.ps1`.
- Multi-machine portability (schema **v8**): `defaults` plus `targets.*` let one git-tracked `genv.json` carry macOS, Windows, native Arch, Ubuntu-like Linux, Ubuntu WSL2, and Arch WSL2 desired state without sharing lock files. `genv migrate` converts legacy host predicates to target buckets, `genv map --target` prints assist-only manager mapping suggestions, and `genv export --target --out` writes single-target snapshots plus reports while omitting locks and sensitive env values. `genv apply --target` selects the active bucket and refuses foreign locks unless `--force-new-lock` backs them up first.

### Changed

- CI now enforces a statement-coverage floor (`COVER_MIN`, default 80%) via `make cover-gate` and runs `make bench-gate` for the cold-start budget (`BENCH_MAX_MS`; local default 200ms, CI uses 400ms for shared-runner noise). Previously claimed but not wired into GitHub Actions.
- Documentation cleanup: roadmap/release/security wording aligned with the v3.x/v4.x line; winget/Scoop/Chocolatey install channels explicitly deferred; outdated-upgrade plan and scheduler handoff marked completed/historical; `SECURITY_AUDIT.md` annotated with 2026-07-25 remediation status.
- Host classification no longer treats WSL2 as a blanket Arch match. Targets are `macos`, `windows`, `arch`, `ubuntu`, `wsl-arch`, and optional `linux`.
- Documentation pass for the v4.0.0 line: README rewritten around schema v8 targets and current install/CLI surface; SCHEMA, SECURITY, ROADMAP, and platform install guides updated for PowerShell (v7), portable targets (v8), and versioned release asset names.

### Fixed

- README `--lock-file` help text now correctly defaults to the genv config directory (not “next to the resolved spec”).
- `genv pull` copies relative `files` assets with the spec (not only `genv.json`), and refuses to ship locks/secrets in pull/export bundles.

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
- Public docs now explicitly distinguish genv's tracked-only updates checker from a full-machine update-all runner.
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
- All internal packages reach ≥80% line coverage as reported by `go test -cover`. *(Clarified 2026-07: CI enforces an 80% **total** statement-coverage floor via `COVER_MIN` / `make cover-gate`; per-package totals vary.)*
- Property-based and fuzz tests added for version constraint logic and the resolver.
- End-to-end smoke tests run `genv apply` against real package managers in CI.
- Resolver + manager detection benchmarked; <200ms cold-start budget enforced as a CI gate. *(Wired into GitHub Actions in Unreleased.)*
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
