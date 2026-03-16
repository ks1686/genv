# Roadmap and Implementation Checklist

This document is the public source of truth for delivery milestones, scope, and acceptance criteria.

## Status Legend

- [ ] Not started
- [x] Complete

## Milestone M1 - Core CLI Foundation

Goal: Ship a reliable CLI with a stable `gpm.json` contract and safe execution defaults.

Target outcomes:

- `gpm add`, `gpm remove`, `gpm list`, and `gpm install` are usable end to end.
- `gpm install --dry-run` is deterministic and readable.
- Invalid `gpm.json` files fail with actionable errors.

Checklist:

- [ ] Define and publish `gpm.json` schema v1 (`schemaVersion: "1"`).
- [ ] Implement schema validation with line-aware error messages.
- [ ] Implement `gpm add <id> [--version <ver>] [--manager <mgr>]`.
- [ ] Implement `gpm remove <id>`.
- [ ] Implement `gpm list`.
- [ ] Implement `gpm install --dry-run` planning output.
- [ ] Implement `gpm install` execution path with confirmation prompt.
- [ ] Add `--strict` behavior for unresolved packages.
- [ ] Add structured exit codes (success, partial success, failed).
- [ ] Add unit tests for parser, validation, and command argument handling.

Acceptance criteria:

- [ ] A clean machine can run `gpm install --dry-run` against sample specs without panic/crash.
- [ ] Malformed specs produce clear validation errors.
- [ ] CLI help text documents all v1 commands and flags.

## Milestone M2 - Resolver and Adapter Layer

Goal: Resolve package IDs across available managers on each host platform.

Target outcomes:

- Linux, macOS, and WSL2 hosts pick a valid manager when possible.
- Resolution decisions are transparent in dry-run output.

Checklist:

- [ ] Build adapter interface: detect, query, plan install, and normalize package IDs.
- [ ] Implement Linux adapters: `apt`, `dnf`, `pacman`, `flatpak`, `snap`.
- [ ] Implement macOS adapter: `brew` (formulae and casks).
- [ ] Implement Linuxbrew path support where available.
- [ ] Implement host manager detection and capability reporting.
- [ ] Implement package candidate scoring (`prefer` then OS priority).
- [ ] Add unresolved package warnings in non-strict mode.
- [ ] Add strict-mode hard failures with package-level diagnostics.
- [ ] Add integration tests with mocked adapter responses.

Acceptance criteria:

- [ ] The same `gpm.json` resolves sensibly on at least one Linux distro and one macOS host.
- [ ] WSL2 environment is treated as Linux userland and uses Linux adapters.
- [ ] Dry-run output includes selected manager and concrete package name per item.

## Milestone M3 - Reproducibility Features

Goal: Improve environment portability and convergence behavior.

Target outcomes:

- Users can generate a baseline spec from an existing machine.
- Lock data can pin and replay resolved versions.
- `gpm sync` applies minimal changes from current state to desired state.

Checklist:

- [ ] Implement `gpm scan` to generate `gpm.json` from installed packages.
- [ ] Implement package normalization and deduplication during scan.
- [ ] Introduce `gpm.lock.json` write path after install.
- [ ] Add lockfile read support and precedence rules.
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

- [ ] v0.1.0 - M1 complete (core CLI)
- [ ] v0.2.0 - M2 complete (resolver + adapters)
- [ ] v0.3.0 - M3 complete (scan, lock, sync)
- [ ] v0.4.0 - M4 complete (reliability and automation)

## How to Contribute Against This Roadmap

1. Pick one unchecked item.
2. Open an issue with milestone tag (`M1`, `M2`, `M3`, or `M4`).
3. Link tests and sample output in the PR.
4. Update checklist state when merged.
