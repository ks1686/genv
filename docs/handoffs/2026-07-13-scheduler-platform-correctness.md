# Scheduler and Platform Correctness Handoff

Date: 2026-07-13  
Repository: `/Users/ks1686/Documents/Repos/genv`  
Branch: `codex/scheduler-platform-correctness`  
Starting commit: `dc31d11`  
Status: implementation is nearly complete, uncommitted, with one validated correctness blocker remaining

## Read this first

The approved design and execution plan are:

- `docs/superpowers/specs/2026-07-12-scheduler-platform-correctness-design.md`
- `docs/superpowers/plans/2026-07-12-scheduler-platform-correctness.md`

Task evidence is under `.superpowers/sdd/task-{1..5}-report.md`. The durable progress ledger is `.superpowers/sdd/progress.md`.

Do not stage, commit, push, install the development binary, or replace the live scheduler job unless the user explicitly authorizes that action. Developer ID signing/notarization is intentionally out of scope.

## Original objective and decisions

The user wanted every genv-supported tracked package covered by managed updates and asked to fix the core defects discovered while enabling that workflow.

Binding decisions:

- macOS automatic search, scan, and implicit resolution use `brew`, not `linuxbrew`;
- Linux automatic paths use `linuxbrew`, not `brew`;
- explicit `prefer` or `managers` configuration can still select either identity when the shared `brew` executable is available;
- scheduled jobs inherit a sanitized, augmented package-manager `PATH`;
- scheduler status must distinguish unsupported, unregistered, idle, executing, last success, and last failure;
- a constrained package must not receive an unsafe broad latest-version upgrade;
- scheduled auto-apply must fail closed without an audit log and retain bounded, credential-safe diagnostics;
- no real package upgrades may run during tests or QA.

## Completed implementation

### Platform-native automatic managers

- New policy: `internal/adapter/platform.go`
- Search filter: `internal/search/search.go`
- Implicit resolver filter: `internal/resolver/resolver.go`
- Scan filter and test seam: `main.go`
- Tests cover Darwin/Linux automatic behavior, explicit cross-platform selection, and a real `scan` command no-call regression.

The physical availability map remains deliberately unfiltered so explicit configuration still works.

### Scheduled environment

- New PATH construction/rendering: `internal/service/scheduled_environment.go`
- `ScheduledJob` now carries `Environment map[string]string`.
- Both launchd and systemd render deterministic environment data.
- `updates start` captures the invoking PATH, removes empty/relative/duplicate entries, and appends platform defaults.

### Scheduler status

- New typed parser/status boundary: `internal/service/scheduled_status.go`
- CLI formatting: `main_updates_daemon.go`
- Launchd parsing already handles:
  - top-level state without nested coalition-state overwrite;
  - `last exit code = 78: EX_CONFIG`;
  - `last exit code = (never exited)`;
  - missing job versus inspection failure.
- Systemd timer and service queries already use different property sets.

One systemd ambiguity remains and is described below.

### Constraint-safe upgrades

- Shared planner change: `internal/upgrade/planner.go`
- Non-empty package constraints are skipped with the exact reason:

  `version-constrained package requires an explicit compatible target`

- Filters retain their existing precedence.
- Constrained entries are removed before batch construction, while unconstrained packages continue.
- Both `genv upgrade` and `genv updates` use this planner.

### Scheduled audit logging and diagnostics

- New bounded/redacted diagnostics: `main_updates_diagnostics.go`
- Logger initialization errors now stop the hidden worker before planning or execution.
- Typed `UpgradeFailure` data preserves tracked IDs through resolver and upgrade layers.
- Each failure is logged individually; unmatched legacy errors still get a fallback event.
- Raw capture and post-redaction output are both capped at 64 KiB with a visible truncation marker.
- Credential-bearing lines and IDs are redacted; stdin and environment are never captured.
- Interactive upgrade stream behavior is unchanged.

### Documentation

- `README.md` documents scheduler PATH/status, fail-closed audit logging, and conservative constraint behavior.
- `CHANGELOG.md` has Unreleased fixes for all four areas.

## Remaining correctness blocker

The final goal review found that systemd status can falsely report a never-executed oneshot service as a successful completed run.

Current logic in `internal/service/scheduled_status.go` treats:

```text
Result=success
ExecMainStatus=0
```

as proof of a completed success. systemd can expose those zero/success defaults before the service has ever executed, so this violates the required distinction between “registered and idle; no completed run is known” and “last run succeeded.”

### Recommended fix

Use a stable service execution marker, preferably `ExecMainStartTimestampMonotonic`:

1. Add `ExecMainStartTimestampMonotonic` to the service `systemctl --user show` property list in `systemdScheduledStatus`.
2. Require it in the service completeness check in `parseSystemdScheduledStatus`.
3. Parse it as an unsigned integer.
4. If it is empty or zero, keep `LastRun=ScheduledRunUnknown` regardless of default `Result=success` and `ExecMainStatus=0`.
5. If the service is currently executing, report `Executing=true` and do not claim a completed outcome for the active invocation; `LastRun=unknown` is the conservative value because the current start has replaced the service execution fields.
6. Only interpret `Result` and `ExecMainStatus` when the start timestamp is non-zero and the service is no longer executing.
7. Malformed timestamp text should produce conservative registered/unknown status with a malformed-output detail.

Verify the property contract against official systemd documentation if network access is available. Previous web/HTTP attempts failed because the Codex session exhausted process descriptors; no documentation result was obtained.

### Required RED tests

Add realistic fixtures in `internal/service/scheduled_status_test.go`:

- never run: inactive/dead, `Result=success`, `ExecMainStatus=0`, `ExecMainStartTimestampMonotonic=0` -> registered, idle, last run unknown;
- completed success: inactive/dead, success/0, positive start timestamp -> last run success;
- completed failure: failed/failed, non-zero status, positive timestamp -> last run failure;
- first execution in progress: active/running, positive timestamp, default success/0 -> executing, last run unknown;
- malformed timestamp -> registered/unknown with explanatory detail;
- command assertion verifies the service query requests `--property=ExecMainStartTimestampMonotonic`, while the timer query does not.

Run before production changes and capture the expected failure:

```bash
go test ./internal/service . -run 'Scheduled.*Status|UpdatesStatus' -count=1
```

Then implement minimally, format, and rerun:

```bash
gofmt -w internal/service/scheduled_status.go internal/service/scheduled_status_test.go
go test ./internal/service . -run 'Scheduled.*Status|UpdatesStatus' -count=1
go test -race -shuffle=on -count=1 ./internal/service .
```

Append RED/GREEN evidence to `.superpowers/sdd/task-3-report.md`.

## Verification already completed

Successful gates on the current diff before the remaining systemd fix:

```text
go test -count=1 ./internal/adapter ./internal/search ./internal/resolver ./internal/service ./internal/upgrade .
PASS

go test -race -shuffle=on -count=1 ./...
PASS, all packages

go vet ./...
PASS

go build -o .omo/session-work/genv-qa .
PASS

make ci
PASS, total statement coverage 68.1%

git diff --check
PASS
```

Relevant resolver benchmarks passed:

```text
BenchmarkPlanUpgradeAdapterLookup-14    84000 ns/op
BenchmarkDetect-14                  114711542 ns/op
BenchmarkResolve-14                     16875 ns/op
```

Command:

```bash
go test -run '^$' -bench 'Benchmark(Detect|Resolve|PlanUpgradeAdapterLookup)$' -benchtime=1x ./internal/resolver
```

`make bench` is not a useful bounded gate in this repository: it also runs `BenchmarkExecuteApply`, which spawns `true` 1,000 times. It was stopped after 78.235 seconds with no result. Use the relevant benchmark selection above unless intentionally waiting for the subprocess benchmark.

### Known pre-existing lint failures

`make lint` fails on 12 findings outside this change. Do not silently fold these into this fix:

- unchecked errors and one ineffectual assignment in `main_profile_test.go`;
- unchecked `in.Close()` in `main_pull.go`;
- an ST1005 test sentinel in `internal/files/links_test.go`;
- SA4006 and an unused helper in pre-existing `main.go` apply code.

All changed files passed formatting and the compiler/race suite. If Claude chooses to repair lint, treat that as a separate scope decision and report it clearly.

## Manual QA evidence

A fresh binary under `.omo/session-work/genv-qa` was used; no install or upgrade executed.

- Automatic macOS apply dry-run resolved `jq` through `brew`.
- Explicit `prefer: linuxbrew` apply dry-run resolved `jq` through `linuxbrew`.
- Constrained `upgrade --dry-run` printed no upgrade plan and the exact skip reason.
- `updates check --json` returned zero batches and the constraint skip.
- Live launchd status printed:

  `updates checker is registered and idle; last run succeeded (status 0) (genv.updates).`

- Bad `updates status` flag exited 1 with `flag provided but not defined`.
- Check-only hidden worker exited 0 and wrote `updates.check.planned ... auto_apply=false`.
- With `XDG_CONFIG_HOME` pointing to a regular file, the hidden worker exited 2 and printed only:

  `genv updates: audit log unavailable`

- Real macOS `scan` completed against scratch files:
  - 283 packages added;
  - brew=211, bun=3, gem=48, mas=19, npm=2;
  - linuxbrew=0.
- The real config `/Users/ks1686/.config/genv/genv.json` validates and has:

  ```json
  {"enabled":true,"interval":"24h","autoApply":true}
  ```

- A read-only `updates check --json` against the real config/lock planned 91 unfiltered tracked batches. No upgrade was executed.

The final goal review noted that the plan literally asked for a search/add candidate-selection QA flow. The automatic and explicit resolution paths were exercised through `apply --dry-run`, and the real scan path was exercised, but no interactive `add` search was run. If practical, add a bounded non-mutating `add` search QA scenario; do not permit it to install anything. If the CLI cannot search without proceeding to installation, document why the apply/scan coverage is the safe equivalent.

## Live system state

- Installed binary: `/opt/homebrew/bin/genv`
- Installed version: `genv 3.0.1`, commit `f428c0f147e2de0065af168ff9723624daefcfef`
- The installed binary is not this development build.
- Live plist: `~/Library/LaunchAgents/genv.updates.plist`
- Current plist already contains a PATH with Homebrew/system directories, runs every 86,400 seconds, and points at `/opt/homebrew/bin/genv`.
- The live job was only inspected, not replaced or stopped.

Do not install the development build or rerun `updates start` without explicit authorization. Once a fixed genv binary is installed, rerunning `genv updates start` will regenerate the supervisor artifact using the corrected environment renderer.

## Final verification and review checklist

After fixing the systemd execution marker:

1. Run the focused Task 3 tests and touched-package race tests above.
2. Run:

   ```bash
   gofmt -w <all changed Go files>
   git diff --check
   go test -race -shuffle=on -count=1 ./...
   go vet ./...
   make ci
   go build -o .omo/session-work/genv-qa .
   go test -run '^$' -bench 'Benchmark(Detect|Resolve|PlanUpgradeAdapterLookup)$' -benchtime=1x ./internal/resolver
   ```

3. Repeat the relevant manual status scenario with the fresh binary.
4. Rerun final goal, QA, code-quality, security, and context/history reviews. The previous five-lane review produced one substantive FAIL (the systemd issue) and four infrastructure failures (`Bad file descriptor` / disconnected streams); those four failures were not counted as passes.
5. Address every validated blocking finding and rerun the affected review lane.
6. Update `.superpowers/sdd/progress.md` to mark Task 6 complete and create `.superpowers/sdd/task-6-report.md` with final evidence.
7. Confirm no test, benchmark, debugger, or QA process remains.

## Cleanup before handoff completion

Current scratch artifacts intentionally remain because the work is not finished:

- `.omo/session-work/` contains the QA binary, fixture specs/locks, generated scan results, logs, and captured error output;
- `coverage.out` was generated by `make ci`;
- `.debug-journal.md` contains the runtime hypotheses and cleanup ledger;
- `.superpowers/sdd/` contains task briefs/reports/progress.

After final verification, use the recoverable `trash` command on macOS for disposable scratch artifacts rather than `rm`:

```bash
trash .omo/session-work
trash coverage.out
trash .debug-journal.md
```

Decide whether `.superpowers/sdd/` is retained as implementation evidence or trashed as workflow scratch. Keep the approved design, implementation plan, and this handoff unless the user asks for them removed.

Finish with:

```bash
git status --short
git diff --check
git diff --stat
pgrep -af 'go test|genv-qa|dlv|lldb' || true
```

The final diff should contain only the requested implementation, direct tests, README/CHANGELOG updates, approved design/plan, and this handoff. Report the pre-existing lint failures separately. Do not claim the branch is complete while the systemd never-run case or any required review lane remains unresolved.
