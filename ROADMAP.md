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

- [x] Implement `gpm scan` to generate `gpm.json` from currently installed packages (bulk adopt).
- [x] Implement package normalization and deduplication during scan.
- [x] Add lockfile version pinning (record resolved version after install).
- [x] Add lockfile precedence rules for version constraints.
- [x] Implement `gpm status` — diff between spec, lock, and what is actually installed on the host.
- [x] Add regression tests for lock replay behavior.

Acceptance criteria:

- [x] Export from Machine A and install on Machine B completes with expected package coverage.
- [x] Re-running apply with lock data produces a stable, idempotent plan.
- [x] `gpm status` correctly identifies packages in the spec but not installed, and vice versa.

## Milestone M4 - Reliability and Automation

Goal: Make the CLI production-ready for teams and unattended workflows.

Target outcomes:

- CI and dotfiles workflows can run `gpm` safely and predictably.
- Releases are easy to consume and regression-resistant.

Checklist:

- [x] Add machine-readable output mode (`--json`) for plan/status/apply results.
- [x] Add non-interactive mode (`--yes`) to `gpm apply` for CI and bootstrap scripts.
- [x] Add per-manager timeout and cancellation handling.
- [x] Add structured logs and debug mode (`--debug`) for issue triage.
- [x] Publish signed release binaries and checksums via GoReleaser.

Acceptance criteria:

- [x] CI can run `gpm apply --dry-run --json` and parse stable output.
- [x] Non-interactive installs complete without prompts when `--yes` is set.
- [x] Release artifacts are published with reproducible version metadata.

## Milestone M5 - Cross-Platform Support (macOS and WSL2)

Goal: Validate and automate gpm on macOS and WSL2 hosts.

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

- [x] `gpm apply` on macOS with a `brew`-only spec installs and removes packages correctly.
- [x] `gpm apply` inside WSL2 uses Linux adapters and produces identical output to a native Linux host.
- [x] The integration workflow runs and passes on macOS without manual intervention.

## Milestone M6 - API Stability and Quality

Goal: Establish stable, versioned contracts so users and tooling can rely on gpm in production.

Target outcomes:

- The `--json` output schema is versioned and stable across patch releases.
- Test coverage is high enough that regressions are caught before users see them.
- The resolver and detection path are fast enough for interactive use.

Checklist:

- [ ] Add `version` field to the `--json` output envelope so consumers can detect schema changes.
- [ ] Define and document the formal deprecation policy (major version for breaking changes).
- [ ] Achieve ≥80% unit test line coverage across all internal packages.
- [ ] Add property-based / fuzz tests for version constraint logic and the resolver.
- [ ] Add end-to-end smoke tests that run `gpm apply` against real package managers in CI.
- [ ] Benchmark resolver + manager detection; enforce a <200ms cold-start budget in CI.
- [ ] Security review: audit every adapter's shell invocations for injection vectors.

Acceptance criteria:

- [ ] `--json` output includes a `"version"` field and the schema is documented.
- [ ] All internal packages reach ≥80% line coverage as reported by `go test -cover`.
- [ ] CI enforces the cold-start budget via a benchmark gate.
- [ ] No known shell-injection vectors in any adapter after the audit.

## Milestone M7 - Developer and User Experience

Goal: Make gpm delightful to use daily and easy to integrate with existing workflows.

Target outcomes:

- Shell users get completion without extra setup.
- Common operations that currently require editing JSON can be done interactively.
- Error messages tell users what to do, not just what went wrong.

Checklist:

- [ ] Implement shell completion for bash, zsh, and fish (`gpm completion <shell>`).
- [ ] Implement `gpm validate` — validate gpm.json schema without installing anything.
- [ ] Implement `gpm upgrade` — re-resolve pinned version constraints and update the lock.
- [ ] Implement `gpm init` — interactive wizard to create a new gpm.json from scratch.
- [ ] Improve error messages: include a suggestion or next step for every user-facing error.
- [ ] Add `--quiet` flag to suppress plan output (useful in scripts alongside `--yes`).

Acceptance criteria:

- [ ] `gpm completion bash | source /dev/stdin` enables tab completion for all subcommands and flags.
- [ ] `gpm validate` exits 0 on a valid spec and 3 on an invalid one, with a clear error message.
- [ ] `gpm upgrade` updates `installedVersion` in the lock after successfully upgrading.
- [ ] Every error message references a corrective action or relevant flag.

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
- [ ] v1.0.0 — M6 and M7 complete; stable API and behavior guarantees, with a formal deprecation policy
- [ ] v1.1.0+ — iterate on user feedback, add features, and expand platform support as needed
- [ ] v2.0.0 — potential major release with first-party Windows support via native Windows package managers (e.g. Chocolatey, Scoop) and WSL2 improvements
- [ ] v3.0.0 — potential major release with support for language-specific package managers (e.g. npm, pip) and/or a plugin system for custom managers

## How to Contribute Against This Roadmap

1. Pick one unchecked item.
2. Open an issue with milestone tag (`M6` or `M7`).
3. Link tests and sample output in the PR.
4. Update checklist state when merged.
