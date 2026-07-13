# Scheduler and Platform Correctness Implementation Plan

> **Execution:** Follow this plan task by task with strict RED-GREEN-REFACTOR cycles. Do not implement or test Developer ID signing/notarization.

**Goal:** Make automatic manager suggestions platform-native, make scheduled updates run in a reliable environment with truthful status, prevent unsafe constrained upgrades, and preserve actionable scheduled-run diagnostics.

**Architecture:** Add a reusable automatic-manager policy in `internal/adapter` and call it only from search, scan, and implicit resolver fallback. Extend scheduled jobs with deterministic environment rendering and parse supervisor state into a typed status. Keep version-constraint safety in the shared upgrade planner. Make the hidden updates worker require an audit logger and capture bounded, redacted diagnostics.

**Tech stack:** Go 1.24, standard library, launchd, systemd user units, existing manual CLI/test seams.

---

## Task 1: Platform-native automatic package managers

**Files:**

- Create: `internal/adapter/platform.go`
- Create: `internal/adapter/platform_test.go`
- Modify: `internal/search/search.go`
- Modify: `internal/search/search_test.go`
- Modify: `internal/resolver/resolver.go`
- Modify: `internal/resolver/resolver_test.go`
- Modify: `main.go`
- Modify: `main_test.go`

1. Add table-driven tests for `AutomaticOnGOOS(manager, goos)` proving Darwin accepts `brew` and rejects `linuxbrew`, Linux does the inverse, and unrelated managers are unchanged.
2. Add search tests through an OS-parameterized helper proving only the native Homebrew identity is queried.
3. Add resolver tests proving implicit fallback uses the native identity while explicit `prefer` and `managers` can still choose either available identity.
4. Extract the scan adapter selection loop into a helper that accepts GOOS and add a CLI-level test proving a Darwin scan does not call Linuxbrew when both identities are detected.
5. Run the focused tests and confirm they fail for the missing policy:

   ```bash
   go test ./internal/adapter ./internal/search ./internal/resolver . -run 'AutomaticOnGOOS|Platform|Scan.*Homebrew'
   ```

6. Implement the minimal policy and thread `runtime.GOOS` through public production entry points while keeping test-only helpers parameterized.
7. Rerun the focused tests to green, then run the complete touched-package tests.

## Task 2: Deterministic scheduled-job PATH

**Files:**

- Create: `internal/service/scheduled_environment.go`
- Create: `internal/service/scheduled_environment_test.go`
- Modify: `internal/service/scheduled.go`
- Modify: `internal/service/service_test.go`
- Modify: `main_updates_daemon.go`
- Modify: `main_test.go`

1. Add table-driven tests for PATH construction: retain only non-empty absolute paths, de-duplicate in first-seen order, and append Darwin/Linux defaults.
2. Extend scheduled content tests to require systemd `Environment=` and launchd `EnvironmentVariables` output with correctly escaped values.
3. Extend `TestUpdatesStart_valid_config_registers_scheduler_without_auto_apply` to assert the scheduled job receives the sanitized augmented PATH captured from the invoking process.
4. Run the focused tests and record RED:

   ```bash
   go test ./internal/service . -run 'Scheduled.*(Path|Environment)|UpdatesStart_valid'
   ```

5. Add `Environment map[string]string` to `ScheduledJob`, deterministic environment renderers, and `ScheduledPath(pathValue, goos)`.
6. Pass the constructed PATH from `updates start` to the backend and render it in both supervisor formats.
7. Rerun focused and complete service/main tests to green.

## Task 3: Truthful scheduled-job status

**Files:**

- Create: `internal/service/scheduled_status.go`
- Create: `internal/service/scheduled_status_test.go`
- Modify: `internal/service/scheduled.go`
- Modify: `main_updates_daemon.go`
- Modify: `main_test.go`

1. Define expected typed status fixtures for unsupported, unregistered, registered idle, executing, last-run success, last-run failure, and unknown/malformed supervisor output.
2. Add launchd parser fixtures for `state`, `last exit code`, and `last exit reason`.
3. Add systemd parser fixtures using stable `LoadState`, `ActiveState`, `SubState`, `Result`, and `ExecMainStatus` properties.
4. Expand the fake supervisor and CLI tests so `updates status` prints explicit messages for each shared state.
5. Run focused tests and confirm RED:

   ```bash
   go test ./internal/service . -run 'Scheduled.*Status|UpdatesStatus'
   ```

6. Extend `ScheduledJobStatus` with registration, activity, last-run outcome, exit code, and detail fields. Add backend command helpers that capture supervisor output and distinguish not-found from inspection errors.
7. Parse backend output conservatively: never equate a loaded idle one-shot job with active execution, and never hide a recorded failure.
8. Update CLI formatting to consume only the shared typed fields and rerun focused tests to green.

## Task 4: Version-constraint-safe upgrade planning

**Files:**

- Modify: `internal/upgrade/planner.go`
- Modify: `internal/upgrade/planner_test.go`
- Modify: `main_test.go` if output coverage is not already exercised through the planner result

1. Add planner tests proving an unconstrained package still plans normally, a constrained package is skipped with `version-constrained package requires an explicit compatible target`, and a mixed batch excludes only constrained packages.
2. Add filter-composition cases proving current `--only`/`--skip` precedence remains deterministic.
3. Run focused tests and confirm RED:

   ```bash
   go test ./internal/upgrade -run 'Constraint|BuildPlan'
   ```

4. Replace the allowed-ID set with a package-by-ID index. Apply existing filters first, then separate constrained packages before `resolver.PlanUpgrade`.
5. Append constraint skips in stable lock order, preserve existing warnings, and rerun focused tests to green.
6. Run upgrade and updates command tests to prove the shared planner protects both interactive and scheduled flows.

## Task 5: Scheduled update audit logging and bounded diagnostics

**Files:**

- Create: `main_updates_diagnostics.go`
- Create: `main_updates_diagnostics_test.go`
- Modify: `main_updates_worker.go`
- Modify: `main_test.go`

1. Add tests for a bounded diagnostic writer that caps retained bytes, includes an explicit truncation marker, and redacts synthetic credential assignments.
2. Add a worker test where logger directory creation/opening fails; assert an IO exit and zero calls to `updatesRunUpgrade`.
3. Add a worker test returning multiple execution errors and synthetic stderr; assert each failure is logged separately with affected IDs/error and the bounded diagnostics are recorded.
4. Run focused tests and confirm RED:

   ```bash
   go test . -run 'Updates(Diagnostic|Logger|RunOnce.*Failure)'
   ```

5. Change logger initialization to return an error. Emit a minimal stderr message when no audit log can be opened and stop before planning/execution that could auto-apply.
6. Pass a bounded diagnostic writer as scheduled stderr, sanitize its rendered output, and log it once after execution only when non-empty.
7. Log each `UpgradeRunResult.Errors` entry separately. Derive affected IDs from the plan/action ordering without logging environment state or command credentials.
8. Rerun focused tests to green and confirm existing check-only/notification tests remain green.

## Task 6: Documentation, regression suite, and manual QA

**Files:**

- Modify: `docs/superpowers/specs/2026-07-12-scheduler-platform-correctness-design.md`
- Modify: `README.md` if scheduler status/PATH or constrained-upgrade behavior is user-facing there
- Modify: `CHANGELOG.md` if the repository records unreleased fixes there

1. Mark the approved design status accurately and document user-visible behavior where the repository currently documents updates/upgrades.
2. Format and inspect the diff:

   ```bash
   gofmt -w <changed-go-files>
   git diff --check
   ```

3. Run focused touched-package suites, then the repository gates:

   ```bash
   go test ./internal/adapter ./internal/search ./internal/resolver ./internal/service ./internal/upgrade .
   go test -race -shuffle=on -count=1 ./...
   make lint
   make build
   make bench
   ```

4. Build a fresh binary and manually exercise non-mutating surfaces with fixture specs under `.omo/session-work/`: `search`/`add` candidate selection, `updates check --json`, and `updates status`. Do not execute a real upgrade or register/replace a real scheduler job during QA.
5. Inspect generated launchd/systemd content using pure rendering tests or a small Go test fixture, confirming only the expected PATH and status fields appear.
6. Review the complete diff against every approved requirement, run the post-implementation review workflow, fix validated findings, rerun affected gates, and ensure no test processes or scratch artifacts remain.

