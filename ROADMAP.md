# Roadmap and Implementation Checklist

This document is the public source of truth for delivery milestones, scope, and acceptance criteria.

## Status Legend

- [ ] Not started
- [x] Complete

## Milestone M1 - Core CLI Foundation

Goal: Ship a reliable CLI with a stable `gpm.json` contract and safe execution defaults.

Target outcomes:

- `gpm add`, `gpm remove`, `gpm list`, and `gpm apply` are usable end to end.
- `gpm apply --dry-run` is deterministic and readable.
- Invalid `gpm.json` files fail with actionable errors.

Checklist:

- [x] Define and publish `gpm.json` schema v1 (`schemaVersion: "1"`).
- [x] Implement schema validation with line-aware error messages.
- [x] Implement `gpm add <id> [--version <ver>] [--manager <mgr>]` — adds to spec and installs immediately.
- [x] Implement `gpm remove <id>` — removes from spec and uninstalls immediately.
- [x] Implement `gpm list` — shows packages currently installed by gpm (reads lock file).
- [x] Implement `gpm edit` — opens gpm.json in `$EDITOR`.
- [x] Implement `gpm apply --dry-run` planning output.
- [x] Implement `gpm apply` reconcile execution with confirmation prompt.
- [x] Add `--strict` behavior for unresolved packages.
- [x] Add structured exit codes (success, partial success, failed).
- [x] Add unit tests for parser, validation, and command argument handling.

Acceptance criteria:

- [x] A clean machine can run `gpm apply --dry-run` against sample specs without panic/crash.
- [x] Malformed specs produce clear validation errors.
- [x] CLI help text documents all v1 commands and flags.

## Milestone M2 - Resolver, Adapter Layer, and Tracking

Goal: Resolve package IDs across available Linux managers and provide fine-grained tracking control.

Target outcomes:

- Linux hosts pick a valid manager when possible and resolution is transparent in dry-run output.
- Already-installed packages can be adopted into gpm without reinstalling.
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
- [x] Implement declarative `gpm apply`: reconcile desired (gpm.json) vs applied state (gpm.lock.json).
- [x] Implement `gpm.lock.json` write path — records manager and concrete package name per installed package.
- [x] Implement per-adapter uninstall commands (removals use the manager recorded in the lock).
- [x] Implement per-adapter cache-clean commands, deduplicated per manager after removals.
- [x] `gpm add` and `gpm remove` update both the spec and the lock atomically.
- [x] Implement `gpm adopt <id>` — verify package is already installed and begin tracking it without reinstalling.
- [x] Implement `gpm disown <id>` — stop tracking a package without uninstalling it.
- [x] Add E2E tests for adopt and disown in the Docker-based distro integration suite.

Acceptance criteria:

- [x] The same `gpm.json` resolves sensibly on at least one Linux distro (validated in CI via Docker matrix).
- [x] Dry-run output includes selected manager and concrete package name per item.
- [x] Adding a package then removing it leaves the system and lock file in the original state.
- [x] `gpm adopt` fails clearly when a package is not installed; succeeds and writes both files when it is.
- [x] `gpm disown` removes a package from the spec and lock without touching the installed binary.

## Milestone M3 - Reproducibility Features

Goal: Improve environment portability and convergence behavior.

Target outcomes:

- Users can generate a baseline spec from an existing machine.
- Lock data can pin and replay resolved versions.
- `gpm status` surfaces drift between the spec and the live system.

Checklist:

- [ ] Implement `gpm scan` to generate `gpm.json` from currently installed packages (bulk adopt).
- [ ] Implement package normalization and deduplication during scan.
- [ ] Add lockfile version pinning (record resolved version after install).
- [ ] Add lockfile precedence rules for version constraints.
- [ ] Implement `gpm status` — diff between spec, lock, and what is actually installed on the host.
- [ ] Add regression tests for lock replay behavior.

Acceptance criteria:

- [ ] Export from Machine A and install on Machine B completes with expected package coverage.
- [ ] Re-running apply with lock data produces a stable, idempotent plan.
- [ ] `gpm status` correctly identifies packages in the spec but not installed, and vice versa.

## Milestone M4 - Reliability and Automation

Goal: Make the CLI production-ready for teams and unattended workflows.

Target outcomes:

- CI and dotfiles workflows can run `gpm` safely and predictably.
- Releases are easy to consume and regression-resistant.

Checklist:

- [ ] Add machine-readable output mode (`--json`) for plan/status/apply results.
- [ ] Add non-interactive mode (`--yes`) to `gpm apply` for CI and bootstrap scripts.
- [ ] Add per-manager timeout and cancellation handling.
- [ ] Add structured logs and debug mode (`--debug`) for issue triage.
- [ ] Publish signed release binaries and checksums via GoReleaser.

Acceptance criteria:

- [ ] CI can run `gpm apply --dry-run --json` and parse stable output.
- [ ] Non-interactive installs complete without prompts when `--yes` is set.
- [ ] Release artifacts are published with reproducible version metadata.

## Milestone M5 - Cross-Platform Support (macOS and WSL2)

Goal: Validate and automate gpm on macOS and WSL2 hosts.

Target outcomes:

- macOS users can rely on `brew` and `macports` adapters with the same guarantees Linux adapters carry.
- WSL2 is explicitly treated as Linux userland and uses Linux adapters without native Windows path leakage.
- Automated test coverage exists for both platforms.

Checklist:

- [ ] Validate `brew` and `macports` adapters on a real macOS host (manual or self-hosted runner).
- [ ] Add a macOS job to the integration workflow (self-hosted runner or `macos-latest` GitHub runner).
- [ ] Validate WSL2 environment detection — confirm Linux adapters are selected, no Windows path leakage.
- [ ] Add install and bootstrap documentation for macOS.
- [ ] Add install and bootstrap documentation for WSL2.
- [ ] Document known limitations for macOS (Homebrew install time, cask vs formula resolution).

Acceptance criteria:

- [ ] `gpm apply` on macOS with a `brew`-only spec installs and removes packages correctly.
- [ ] `gpm apply` inside WSL2 uses Linux adapters and produces identical output to a native Linux host.
- [ ] The integration workflow runs and passes on macOS without manual intervention.

## Cross-Cutting Quality Gates

These gates apply to every milestone.

- [ ] Document user-facing behavior changes in README and changelog.
- [ ] Add tests for every new command or resolver rule.
- [ ] Keep dry-run output human-readable and stable for CI snapshots.
- [ ] Ensure commands are non-destructive unless explicitly requested.
- [ ] Keep WSL2 behavior explicitly Linux-only (no native Windows installer scope creep).

## Release Plan

- [x] v0.1.0-beta.1 — first public prerelease; M1 complete, M2 partially validated
- [ ] v0.1.0 — first stable release; M1 and M2 complete and validated on Linux
- [ ] v0.2.0 — M3 complete (scan, version pinning, status)
- [ ] v0.3.0 — M4 complete (reliability and automation)
- [ ] v0.4.0 — M5 complete (macOS and WSL2 support)

## How to Contribute Against This Roadmap

1. Pick one unchecked item.
2. Open an issue with milestone tag (`M1`, `M2`, `M3`, `M4`, or `M5`).
3. Link tests and sample output in the PR.
4. Update checklist state when merged.
