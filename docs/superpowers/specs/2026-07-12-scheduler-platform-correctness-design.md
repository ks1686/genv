# Scheduler and Platform Correctness Design

Date: 2026-07-12
Status: Approved

## Goal

Fix the correctness and observability defects exposed while enabling unattended genv updates, and prevent macOS from suggesting Linuxbrew alongside native Homebrew. Developer ID signing and notarization are explicitly out of scope.

## Required behavior

### Platform-aware manager suggestions

- On macOS, automatic search, scan, and resolution suggestions include `brew` and exclude `linuxbrew`.
- On Linux, automatic suggestions include `linuxbrew` and exclude `brew` when presenting the two Homebrew identities.
- Explicit configuration remains authoritative: a package may still name or prefer either manager when that manager is available on the current host.
- Platform filtering applies only to automatic candidate generation. It must not make an explicitly configured manager invalid.
- The platform-policy seam must accept an OS value in tests rather than requiring tests to run on multiple operating systems.

### Scheduled-job environment

- Scheduled jobs receive a deterministic PATH that can resolve supported user-level package managers.
- PATH construction starts with the invoking process PATH, retains only non-empty absolute directories, removes duplicates, and appends missing platform defaults.
- macOS defaults include Apple Silicon Homebrew, Intel Homebrew/local, and system binary directories.
- Linux defaults include Linuxbrew, local, and system binary directories.
- Both launchd and systemd render the scheduled environment so behavior does not depend on a supervisor's restricted default PATH.
- Generated environment values are escaped using the target supervisor's existing escaping rules.

### Truthful scheduler status

- Status distinguishes unsupported, unregistered, registered/idle, executing, last-run success, and last-run failure.
- An idle one-shot timer remains healthy and registered; it is not reported as currently executing.
- A loaded job whose last execution failed reports that failure instead of the generic `running` message.
- launchd parsing recognizes state, last exit code, and last exit reason from `launchctl print` output.
- systemd obtains equivalent timer/service state through stable `systemctl show` properties.
- Human output is explicit, while structured internal status remains typed and testable.

### Version-constrained upgrades

- The upgrade planner must never schedule a broad latest-version upgrade for a package with a non-empty version constraint unless the selected adapter can guarantee the target satisfies that constraint.
- In the current adapter model, constrained packages are skipped with a stable, actionable reason rather than upgraded unsafely.
- Unconstrained packages retain current behavior.
- Filters continue to compose with constraint skips, and JSON/human output exposes the skip reason.
- This conservative behavior applies to both `genv upgrade` and `genv updates` because they share the planner.
- A future version-aware adapter capability can replace the conservative skip without changing the planner contract.

### Scheduled update observability

- Failure to create, rotate, or open `updates.log` is an explicit worker error and prevents auto-apply from running without an audit trail.
- Every failed upgrade action is logged with its affected tracked IDs and error.
- Package-manager diagnostics are captured with a bounded buffer and logged without unbounded memory or log growth.
- Successful scheduled runs retain the existing aggregate completion record.
- Logging changes must not expose environment variables or credentials, and tests use synthetic non-sensitive diagnostics.

## Architecture

### Platform policy

A small internal policy function decides whether a manager may appear as an automatic suggestion for a supplied GOOS. Search, scan, and implicit resolver candidate construction call this function. Explicit `prefer` and `managers` paths bypass automatic-suggestion filtering after normal availability checks.

This targeted seam avoids adding a platform method to every adapter while remaining extensible for future platform-specific manager pairs.

### Scheduled environment and status

`ScheduledJob` gains a typed environment map. The updates lifecycle constructs the sanitized augmented PATH once and passes it to the scheduled backend. launchd emits `EnvironmentVariables`; systemd emits `Environment=` entries.

`ScheduledJobStatus` grows explicit registration, activity, and last-run fields. Backend-specific parsers translate supervisor output into that shared type. CLI formatting consumes the typed status instead of inferring health from command success.

### Constraint policy

`BuildUpgradePlan` indexes complete `schema.Package` values by ID instead of storing only allowed IDs. Before creating upgrade actions, it separates non-empty constraints into skipped packages with the stable reason `version-constrained package requires an explicit compatible target`. Existing manager/package filters run consistently and preserve their current precedence.

### Diagnostic capture

The scheduled worker opens its logger through a function returning an error. Auto-apply uses a bounded writer for manager stderr and records each structured execution error after `RunUpgrade`. The normal interactive `genv upgrade` output path is unchanged.

## Error handling

- Malformed supervisor output returns a conservative registered/unknown-health status plus detail rather than claiming success.
- Failure to inspect status returns the existing IO exit category.
- Invalid or unusable PATH entries are omitted; if the resulting PATH is empty, platform defaults remain.
- Constrained packages are skips, not planner failures, so unconstrained tracked packages may still upgrade.
- Logger initialization failure returns an IO exit code before any upgrade command executes.

## Test strategy

Every behavior follows RED to GREEN.

1. Platform suggestions:
   - macOS automatic candidates contain `brew` and not `linuxbrew`.
   - Linux automatic candidates contain `linuxbrew` and not `brew`.
   - Explicit cross-platform manager selection remains accepted when available.
2. Scheduled environment:
   - PATH sanitizer removes relative, empty, and duplicate entries.
   - launchd plist contains escaped `EnvironmentVariables`.
   - systemd unit contains the equivalent PATH.
3. Scheduler status:
   - fixtures cover unregistered, idle successful, executing, and failed launchd/systemd states.
   - CLI output names each state truthfully.
4. Constraints:
   - unconstrained packages still plan upgrades.
   - constrained packages are skipped with the stable reason.
   - filters and constraint skips compose deterministically.
5. Observability:
   - logger-open failure prevents the executor call.
   - multiple action failures are individually logged.
   - diagnostics are bounded and truncation is visible.

Verification includes targeted tests, `go test -race -shuffle=on -count=1 ./...`, formatting/lint gates available in the repository, and CLI manual QA using a freshly built binary with fixture specs. Manual QA must not execute real package upgrades.

## Compatibility and migration

- Existing genv schema remains version 6; no configuration migration is required.
- Existing update filters and unconstrained upgrade behavior remain compatible.
- Human `updates status` wording becomes more precise but command and exit-code contracts remain stable.
- Rerunning `genv updates start` regenerates the current user's scheduler artifact with the corrected environment, removing the need for manual plist edits.

## Out of scope

- Developer ID signing, Apple notarization, or acquiring an Apple developer account.
- Redesigning every adapter around generalized platform metadata.
- Implementing manager-specific compatible-version discovery in this change.
- Executing real upgrades as part of tests or manual QA.
