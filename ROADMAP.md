# Roadmap and Implementation Checklist

This document is the public source of truth for delivery milestones, scope, and acceptance criteria.

## Status Legend

- [ ] Not started
- [x] Complete

## Milestone M1 - Core CLI Foundation

Goal: Ship a reliable CLI with a stable `genv.json` contract and safe execution defaults.

Target outcomes:

- `genv add`, `genv remove`, `genv list`, and `genv apply` are usable end to end.
- `genv apply --dry-run` is deterministic and readable.
- Invalid `genv.json` files fail with actionable errors.

Checklist:

- [x] Define and publish `genv.json` schema v1 (`schemaVersion: "1"`).
- [x] Implement schema validation with line-aware error messages.
- [x] Implement `genv add <id> [--version <ver>] [--manager <mgr>]` — adds to spec and installs immediately.
- [x] Implement `genv remove <id>` — removes from spec and uninstalls immediately.
- [x] Implement `genv list` — shows packages currently installed by genv (reads lock file).
- [x] Implement `genv edit` — opens genv.json in `$EDITOR`.
- [x] Implement `genv apply --dry-run` planning output.
- [x] Implement `genv apply` reconcile execution with confirmation prompt.
- [x] Add `--strict` behavior for unresolved packages.
- [x] Add structured exit codes (success, partial success, failed).
- [x] Add unit tests for parser, validation, and command argument handling.

Acceptance criteria:

- [x] A clean machine can run `genv apply --dry-run` against sample specs without panic/crash.
- [x] Malformed specs produce clear validation errors.
- [x] CLI help text documents all v1 commands and flags.

## Milestone M2 - Resolver, Adapter Layer, and Tracking

Goal: Resolve package IDs across available Linux managers and provide fine-grained tracking control.

Target outcomes:

- Linux hosts pick a valid manager when possible and resolution is transparent in dry-run output.
- Already-installed packages can be adopted into genv without reinstalling.
- Tracked packages can be disowned without uninstalling them.

Checklist:

- [x] Build adapter interface: detect, query, plan install, plan uninstall, plan cache clean, and normalize package IDs.
- [x] Implement Linux adapters: `apt`, `dnf`, `pacman`, `paru`, `yay`, `flatpak`, `snap`.
- [x] Implement macOS adapters: `brew` (formulae and casks), `macports`.
- [x] Implement Linuxbrew path support where available.
- [x] Implement host manager detection and capability reporting.
- [x] Implement package candidate scoring (`prefer` then OS priority).
- [x] Add unresolved package warnings in non-strict mode.
- [x] Add strict-mode hard failures with package-level diagnostics.
- [x] Add integration tests with mocked adapter responses.
- [x] Implement declarative `genv apply`: reconcile desired (genv.json) vs applied state (genv.lock.json).
- [x] Implement `genv.lock.json` write path — records manager and concrete package name per installed package.
- [x] Implement per-adapter uninstall commands (removals use the manager recorded in the lock).
- [x] Implement per-adapter cache-clean commands, deduplicated per manager after removals.
- [x] `genv add` and `genv remove` update both the spec and the lock atomically.
- [x] Implement `genv adopt <id>` — verify package is already installed and begin tracking it without reinstalling.
- [x] Implement `genv disown <id>` — stop tracking a package without uninstalling it.
- [x] Add E2E tests for adopt and disown in the Docker-based distro integration suite.

Acceptance criteria:

- [x] The same `genv.json` resolves sensibly on at least one Linux distro (validated in CI via Docker matrix).
- [x] Dry-run output includes selected manager and concrete package name per item.
- [x] Adding a package then removing it leaves the system and lock file in the original state.
- [x] `genv adopt` fails clearly when a package is not installed; succeeds and writes both files when it is.
- [x] `genv disown` removes a package from the spec and lock without touching the installed binary.

## Milestone M3 - Reproducibility Features

Goal: Improve environment portability and convergence behavior.

Target outcomes:

- Users can generate a baseline spec from an existing machine.
- Lock data can pin and replay resolved versions.
- `genv status` surfaces drift between the spec and the live system.

Checklist:

- [x] Implement `genv scan` to generate `genv.json` from currently installed packages (bulk adopt).
- [x] Implement package normalization and deduplication during scan.
- [x] Add lockfile version pinning (record resolved version after install).
- [x] Add lockfile precedence rules for version constraints.
- [x] Implement `genv status` — diff between spec, lock, and what is actually installed on the host.
- [x] Add regression tests for lock replay behavior.

Acceptance criteria:

- [x] Export from Machine A and install on Machine B completes with expected package coverage.
- [x] Re-running apply with lock data produces a stable, idempotent plan.
- [x] `genv status` correctly identifies packages in the spec but not installed, and vice versa.

## Milestone M4 - Reliability and Automation

Goal: Make the CLI production-ready for teams and unattended workflows.

Target outcomes:

- CI and dotfiles workflows can run `genv` safely and predictably.
- Releases are easy to consume and regression-resistant.

Checklist:

- [x] Add machine-readable output mode (`--json`) for plan/status/apply results.
- [x] Add non-interactive mode (`--yes`) to `genv apply` for CI and bootstrap scripts.
- [x] Add per-manager timeout and cancellation handling.
- [x] Add structured logs and debug mode (`--debug`) for issue triage.
- [x] Publish signed release binaries and checksums via GoReleaser.

Acceptance criteria:

- [x] CI can run `genv apply --dry-run --json` and parse stable output.
- [x] Non-interactive installs complete without prompts when `--yes` is set.
- [x] Release artifacts are published with reproducible version metadata.

## Milestone M5 - Cross-Platform Support (macOS and WSL2)

Goal: Validate and automate genv on macOS and WSL2 hosts.

Target outcomes:

- macOS users can rely on `brew` and `macports` adapters with the same guarantees Linux adapters carry.
- WSL2 is explicitly treated as Linux userland and uses Linux adapters without native Windows path leakage.
- Automated test coverage exists for both platforms.

Checklist:

- [x] Validate `brew` and `macports` adapters on a real macOS host (manual or self-hosted runner).
- [x] Add a macOS job to the integration workflow (self-hosted runner or `macos-latest` GitHub runner).
- [x] Validate WSL2 environment detection — confirm Linux adapters are selected, no Windows path leakage.
- [x] Add install and bootstrap documentation for macOS.
- [x] Add install and bootstrap documentation for WSL2.
- [x] Document known limitations for macOS (Homebrew install time, cask vs formula resolution).

Acceptance criteria:

- [x] `genv apply` on macOS with a `brew`-only spec installs and removes packages correctly.
- [x] `genv apply` inside WSL2 uses Linux adapters and produces identical output to a native Linux host.
- [x] The integration workflow runs and passes on macOS without manual intervention.

## Milestone M6 - API Stability and Quality

Goal: Establish stable, versioned contracts so users and tooling can rely on genv in production.

Target outcomes:

- The `--json` output schema is versioned and stable across patch releases.
- Test coverage is high enough that regressions are caught before users see them.
- The resolver and detection path are fast enough for interactive use.

Checklist:

- [x] Add `version` field to the `--json` output envelope so consumers can detect schema changes.
- [x] Define and document the formal deprecation policy (major version for breaking changes).
- [x] Achieve ≥80% unit test line coverage across all internal packages.
- [x] Add property-based / fuzz tests for version constraint logic and the resolver.
- [x] Add end-to-end smoke tests that run `genv apply` against real package managers in CI.
- [x] Benchmark resolver + manager detection; enforce a <200ms cold-start budget in CI.
- [x] Security review: audit every adapter's shell invocations for injection vectors.

Acceptance criteria:

- [x] `--json` output includes a `"version"` field and the schema is documented.
- [x] All internal packages reach ≥80% line coverage as reported by `go test -cover`.
- [x] CI enforces the cold-start budget via a benchmark gate.
- [x] No known shell-injection vectors in any adapter after the audit.

## Milestone M7 - Developer and User Experience

Goal: Make genv delightful to use daily and easy to integrate with existing workflows.

Target outcomes:

- Shell users get completion without extra setup.
- Common operations that currently require editing JSON can be done interactively.
- Error messages tell users what to do, not just what went wrong.

Checklist:

- [x] Implement shell completion for bash, zsh, and fish (`genv completion <shell>`).
- [x] Implement `genv validate` — validate genv.json schema without installing anything.
- [x] Implement `genv upgrade` — re-resolve pinned version constraints and update the lock.
- [x] Implement `genv init` — interactive wizard to create a new genv.json from scratch.
- [x] Improve error messages: include a suggestion or next step for every user-facing error.
- [x] Add `--quiet` flag to suppress plan output (useful in scripts alongside `--yes`).

Acceptance criteria:

- [x] `genv completion bash | source /dev/stdin` enables tab completion for all subcommands and flags.
- [x] `genv validate` exits 0 on a valid spec and 3 on an invalid one, with a clear error message.
- [x] `genv upgrade` updates `installedVersion` in the lock after successfully upgrading.
- [x] Every error message references a corrective action or relevant flag.

## Milestone M8 - Environment Variables and Shell Globals

Goal: Extend genv to manage global shell environment variables as part of the reproducible environment.

Target outcomes:

- Users can declare shell environment variables in `genv.json` and apply them to their shell profile.
- Variables are tracked in the lock file alongside packages so the full environment state is reproducible.
- `genv apply` handles variable addition, update, and removal cleanly without duplicating shell profile entries.

Checklist:

- [x] Extend `genv.json` schema (v2) to accept an `env` block mapping variable names to values.
- [x] Implement `genv env set <NAME> <value>` — add or update a variable in the spec.
- [x] Implement `genv env unset <NAME>` — remove a variable from the spec.
- [x] Implement `genv env list` — show all declared variables and their current resolved values.
- [x] Implement apply logic: write variables to a managed shell profile fragment (e.g. `~/.config/genv/env.sh`) sourced by the user's shell rc.
- [x] Track applied variable state in `genv.lock.json` so drift is detectable.
- [x] Implement `genv status` drift detection for env variables (declared vs. actually exported).
- [x] Support secret redaction in `--json` output for variables marked `sensitive: true`.
- [x] Add unit and integration tests for env apply, update, and removal.

Acceptance criteria:

- [x] `genv apply` on a spec with an `env` block exports the declared variables in a new shell session.
- [x] Removing a variable from the spec and re-running `genv apply` removes it from the managed fragment.
- [x] `genv status` surfaces variables that are in the spec but not currently exported, and vice versa.
- [x] Sensitive variables are redacted in `--json` output and log output.

## Milestone M9 - Shell Configuration Management

Goal: Extend genv to manage basic shell configuration (aliases, functions, and rc snippets) as a first-class environment concern.

Target outcomes:

- Users can declare shell aliases and small rc snippets in `genv.json` and have them applied consistently across machines.
- Shell config state is versioned and reproducible alongside packages and env vars.
- genv does not own the entire shell rc — it manages a scoped fragment and sources it, leaving user customizations intact.

Checklist:

- [x] Extend `genv.json` schema (v2 or v3) to accept a `shell` block with `aliases`, `functions`, and `source` fields.
- [x] Implement `genv shell alias set <name> <value>` and `genv shell alias unset <name>`.
- [x] Implement apply logic: write aliases and functions to a managed fragment (e.g. `~/.config/genv/shell.sh`).
- [x] Implement safe rc injection: detect whether the managed fragment is already sourced in `~/.bashrc`, `~/.zshrc`, etc., and add the source line only once.
- [x] Implement `genv shell status` — diff between declared shell config and what is currently active.
- [x] Support per-shell targeting (bash-only alias vs. zsh-only alias vs. both).
- [x] Add `genv shell edit` — open `genv.json` in `$EDITOR` to edit the shell block directly.
- [x] Track applied shell config in `genv.lock.json`.
- [x] Add unit and integration tests for fragment generation and rc injection.

Acceptance criteria:

- [x] `genv apply` on a spec with a `shell.aliases` block makes those aliases available in a new shell session.
- [x] Re-running apply after removing an alias cleanly removes it from the managed fragment.
- [x] The source line is injected exactly once into the user's rc file, even if `genv apply` is run multiple times.
- [x] `genv shell status` reports drift between the spec and the active shell session.

## Milestone M10 - Services Management

Goal: Extend genv to manage simple user-space services (e.g. background daemons) as part of the reproducible environment, with first-class integration into existing service managers where available.

Target outcomes:

- Users can declare services in `genv.json` with start/stop commands and have genv manage their lifecycle.
- Service state is tracked in the lock file and can be reconciled with the live system.
- genv provides commands to start, stop, and check the status of declared services.
- Where systemd (Linux) or launchd (macOS) is available, genv generates and installs the appropriate unit/plist files rather than managing raw processes.

Checklist:

- [x] Implement Linux adapter: `apk` (Alpine Linux).
- [x] Implement Linux adapter: `zypper` (openSUSE / SLES).
- [x] Implement Linux adapter: `xbps` (Void Linux).
- [x] Implement Linux adapter: `emerge` (Gentoo).
- [x] Publish genv as `.deb`, `.rpm`, and `.apk` release artifacts via GoReleaser `nfpms` (covers apt, dnf, and apk direct-install; zypper users covered by `.rpm`).
- [x] Publish genv to the Snap Store (`snapcraft.yaml` + GoReleaser snapcraft section).
- [x] Submit genv APKBUILD to Alpine Linux aports so `apk add genv` works without a direct download.
- [x] Submit genv Portfile to MacPorts ports tree.
- [x] Publish genv to Fedora COPR for one-line install (`dnf copr enable ks1686/genv && dnf install genv`).
- [x] Extend `genv.json` schema to accept a `services` block with per-service `start`, `stop`, and optional `restart` commands.
- [x] Implement `genv service add <name> --start <cmd> [--stop <cmd>]` and `genv service remove <name>`.
- [x] Implement `genv service start <name>`, `genv service stop <name>`, and `genv service status <name>`.
- [x] Implement apply logic: start services that are declared but not running; stop services that are in the lock but no longer in the spec.
- [x] Track service state in `genv.lock.json` and implement drift detection in `genv status`.
- [x] Add optional systemd user-unit generation (`~/.config/systemd/user/genv-<name>.service`) on Linux when systemd is available.
- [x] Add optional launchd plist generation (`~/Library/LaunchAgents/genv.<name>.plist`) on macOS when launchd is available.
- [x] Implement safe command execution with explicit argv slices (no shell interpolation).
- [x] Define and document failure semantics: what happens when a service fails to start, how to handle dependencies, and how to view logs.
- [x] Add unit and integration tests for service lifecycle management.

Acceptance criteria:

- [x] `genv apply` on a spec with a `services` block starts declared services and records their state in the lock file.
- [x] Removing a service from the spec and re-running `genv apply` stops and removes it cleanly.
- [x] `genv service status <name>` reports whether the service is running, and exits non-zero when it is not.
- [x] On systems with systemd available, genv generates a valid user unit and uses `systemctl --user` to manage it.

## Milestone M11 - Updates Daemon

Goal: Implement an optional background process that automatically checks for package updates on a configurable schedule, respecting version constraints in the lock file.

Target outcomes:

- Users can enable an updates daemon that runs in the background and checks for package updates at a specified interval.
- The daemon respects version constraints declared in `genv.json` and `genv.lock.json`, only flagging or applying updates that satisfy declared constraints.
- Users can configure whether updates are auto-applied or only announced.

Checklist:

- [ ] Implement `genv updates start` and `genv updates stop` to manage the daemon lifecycle (using the M10 service layer where possible).
- [ ] Add an `updates` block to `genv.json` with `enabled`, `interval`, and `autoApply` fields.
- [ ] Implement daemon logic: on each tick, call `genv upgrade --dry-run` per package, collect candidates, then either apply or log a notification.
- [ ] Respect pinned version constraints in the lock file — never upgrade a package beyond its constraint.
- [ ] Add structured logging to a genv-managed log file (`~/.config/genv/updates.log`) with rotation.
- [ ] Implement desktop notification support (via `notify-send` on Linux, `osascript` on macOS) when updates are available but `autoApply` is false.
- [ ] Add unit and integration tests for daemon configuration parsing and update-candidate selection.

Acceptance criteria:

- [ ] With `autoApply: true`, packages are upgraded automatically when a new version is available within the declared constraint.
- [ ] With `autoApply: false`, a desktop notification is sent and the update is recorded in the log without being applied.
- [ ] The daemon survives restarts gracefully and does not duplicate notifications.

## Milestone M12 - Named Profiles

Goal: Allow users to declare multiple named environment profiles in a single repository and switch between them, enabling distinct configurations for different contexts (e.g. work, personal, server, laptop).

Target outcomes:

- Users can maintain multiple named profiles (e.g. `work`, `home`, `server`) that each declare their own packages, env vars, and shell config.
- Switching profiles is a single command and triggers a reconcile to add/remove the delta.
- A `base` profile can define packages shared across all contexts, with named profiles inheriting and extending it.

Checklist:

- [ ] Define a profile schema: a `profiles/` directory of named `<profile>.json` files that extend a root `genv.json` base.
- [ ] Implement `genv profile switch <name>` — compute the diff between the current active profile and the target, then apply it.
- [ ] Implement `genv profile list` — list available profiles and mark the active one.
- [ ] Implement `genv profile create <name>` — scaffold a new profile file from the current environment.
- [ ] Track the active profile name in `genv.lock.json`.
- [ ] Implement inheritance: packages, env vars, and shell aliases declared in the base are always included; profiles add on top.
- [ ] Add `genv status` awareness: report the active profile and flag drift between the profile and the live system.
- [ ] Add unit and integration tests for profile switching, inheritance, and lock tracking.

Acceptance criteria:

- [ ] `genv profile switch work` installs packages only in the `work` profile and removes packages exclusive to the previous profile.
- [ ] `genv profile switch home` correctly reverts to the `home` profile state.
- [ ] Base-profile packages are never removed during a profile switch.
- [ ] `genv status` reports the active profile name alongside the drift summary.

## Milestone M13 - Hooks and Lifecycle Scripts

Goal: Allow users to declare shell hooks that run before or after specific genv lifecycle events, enabling custom bootstrapping, notifications, and integration with external tools.

Target outcomes:

- Users can declare `pre` and `post` hooks for events like `apply`, `add`, `remove`, and `upgrade`.
- Hooks are declared in `genv.json` and run in a predictable, sandboxed way with access to event context (e.g. which packages were installed).
- Hook failures are surfaced clearly without silently swallowing errors.

Checklist:

- [ ] Extend `genv.json` schema to accept a `hooks` block mapping event names to shell command strings.
- [ ] Implement hook execution in the apply, add, remove, and upgrade command paths.
- [ ] Pass event context to hooks via environment variables (e.g. `GENV_EVENT`, `GENV_INSTALLED`, `GENV_REMOVED`).
- [ ] Define and enforce a timeout for hook execution; surface timeout errors clearly.
- [ ] Implement `--no-hooks` flag on apply and related commands to skip hook execution.
- [ ] Support both inline commands and script file references (`file: ~/.config/genv/hooks/post-apply.sh`).
- [ ] Add unit tests for hook parsing, execution ordering, and error propagation.
- [ ] Document hook security implications: hooks run as the current user with full shell access; warn in docs.

Acceptance criteria:

- [ ] A `post.apply` hook declared in `genv.json` runs after every successful `genv apply`, with `GENV_INSTALLED` set to the list of installed package IDs.
- [ ] A failing hook exits the command with a non-zero code and prints the hook's stderr output.
- [ ] `genv apply --no-hooks` skips hook execution and exits 0 if the apply itself succeeded.
- [ ] Hook timeouts are enforced and reported clearly.

## Cross-Cutting Quality Gates

These gates apply to every milestone.

- [x] Document user-facing behavior changes in README and changelog.
- [x] Add tests for every new command or resolver rule.
- [x] Keep dry-run output human-readable and stable for CI snapshots.
- [x] Ensure commands are non-destructive unless explicitly requested.
- [x] Keep WSL2 behavior explicitly Linux-only (no native Windows installer scope creep).

## Release Plan

- [x] v0.1.0-beta.1 — first public prerelease; M1 complete, M2 partially validated
- [x] v0.1.0 — first stable release; M1 and M2 complete and validated on Linux
- [x] v0.2.0 — M3–M5 complete and validated, with cross-platform support, reproducibility, and reliability improvements
- [x] v1.0.0 — M6 and M7 complete; stable API and behavior guarantees, with a formal deprecation policy
- [x] v2.0.0 — M8 and M9 complete; full environment reproducibility: packages, global shell variables, and basic shell configuration managed as a single declarative spec
- [x] v2.1.0 — M10 complete; services management, new adapters (apk/zypper/xbps/emerge), Snap/Alpine/MacPorts/COPR packaging
- [ ] v2.2.0 — M13 complete; hooks and lifecycle scripts for custom bootstrapping and integration
- [ ] v2.3.0 — M12 complete; named profiles for context-switching between work, personal, and server environments
- [ ] v3.0.0 — M11 complete + first-party Windows support via native Windows package managers (e.g. Chocolatey, Scoop) and WSL2 improvements
- [ ] v4.0.0 — potential major release with support for language-specific package managers (e.g. npm, pip, cargo) and/or a plugin system for custom adapters

## How to Contribute Against This Roadmap

1. Pick one unchecked item.
2. Open an issue with the relevant milestone tag (`M11`, `M12`, or `M13`).
3. Link tests and sample output in the PR.
4. Update checklist state when merged.
