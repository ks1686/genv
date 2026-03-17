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

## Milestone M2 - Resolver and Adapter Layer

Goal: Resolve package IDs across available managers on each host platform.

Target outcomes:

- Linux, macOS, and WSL2 hosts pick a valid manager when possible.
- Resolution decisions are transparent in dry-run output.

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

Acceptance criteria:

- [ ] The same `gpm.json` resolves sensibly on at least one Linux distro and one macOS host.
- [ ] WSL2 environment is treated as Linux userland and uses Linux adapters.
- [ ] Dry-run output includes selected manager and concrete package name per item.
- [ ] Adding a package then removing it leaves the system and lock file in the original state.

## Milestone M3 - Reproducibility Features

Goal: Improve environment portability and convergence behavior.

Target outcomes:

- Users can generate a baseline spec from an existing machine.
- Lock data can pin and replay resolved versions.
- `gpm sync` applies minimal changes from current state to desired state.

Checklist:

- [ ] Implement `gpm scan` to generate `gpm.json` from installed packages.
- [ ] Implement package normalization and deduplication during scan.
- [ ] Add lockfile version pinning (record resolved version after install).
- [ ] Add lockfile precedence rules for version constraints.
- [ ] Implement `gpm status` host-vs-spec diff output.
- [ ] Implement `gpm sync` minimal-delta planner.
- [ ] Add conflict messaging for version or manager mismatches.
- [ ] Add regression tests for lock replay behavior.

Acceptance criteria:

- [ ] Export from Machine A and install on Machine B completes with expected package coverage.
- [ ] Re-running install with lock data produces stable plan output.
- [ ] `gpm sync` avoids reinstalling already-converged packages.

## Milestone M4 - Reliability and Automation

Goal: Make the CLI production-ready for teams and unattended workflows.

Target outcomes:

- CI and dotfiles workflows can run `gpm` safely and predictably.
- Releases are easy to consume and regression-resistant.

Checklist:

- [ ] Add machine-readable output mode (`--json`) for plan/status/install results.
- [ ] Add non-interactive mode (`--yes`) for CI and bootstrap scripts.
- [ ] Add retry/backoff policy for transient package-manager failures.
- [ ] Add per-manager timeout and cancellation handling.
- [ ] Add structured logs and debug mode for issue triage.
- [ ] Add cross-platform integration test matrix (Linux distro variants + macOS).
- [ ] Add benchmark checks for resolver and planner performance.
- [ ] Publish signed release binaries and checksums.

Acceptance criteria:

- [ ] CI can run `gpm install --dry-run --json` and parse stable output.
- [ ] Non-interactive installs complete without prompts when `--yes` is set.
- [ ] Release artifacts are published with reproducible version metadata.

## Cross-Cutting Quality Gates

These gates apply to every milestone.

- [ ] Document user-facing behavior changes in README and changelog.
- [ ] Add tests for every new command or resolver rule.
- [ ] Keep dry-run output human-readable and stable for CI snapshots.
- [ ] Ensure commands are non-destructive unless explicitly requested.
- [ ] Keep WSL2 behavior explicitly Linux-only (no native Windows installer scope creep).

## Release Plan (Suggested)

- [x] v0.1.0-beta.1 - first public prerelease representing the current usable state; M1 complete, may include partially validated M2 work
- [ ] v0.1.0 - first stable release
- [x] v0.2.0 - M2 complete and validated (resolver + adapters, declarative apply, lock file)
- [ ] v0.3.0 - M3 complete (scan, version pinning, sync)
- [ ] v0.4.0 - M4 complete (reliability and automation)

## How to Contribute Against This Roadmap

1. Pick one unchecked item.
2. Open an issue with milestone tag (`M1`, `M2`, `M3`, or `M4`).
3. Link tests and sample output in the PR.
4. Update checklist state when merged.
