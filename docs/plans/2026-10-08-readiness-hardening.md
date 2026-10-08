# Plan: post-v4.7.0 readiness hardening

Created: 2026-10-08 · Status: **in progress** (owner: "follow the full plan until
it's complete as a release on GitHub") · Fast-path: no
Baseline: `main` @ `9184917` (v4.7.0 tagged and released 2026-10-08).
Worktree: `~/Documents/Worktrees/genv/readiness-hardening`.
Audit evidence (local, may be gone): `/tmp/genv-readiness/`.

## Handoff rules (read first)

This plan is written so any agent can resume it cold. Rules:

1. Work on branch `chore/readiness-hardening` (create it from `main` if missing).
   One writer per worktree.
2. Do tasks in order. Each task is one commit (Conventional Commits). Before
   starting, run `git log --oneline main..HEAD` to see which tasks already landed;
   tick boxes in this file in the same commit as the work.
3. TDD where behavior changes: failing test first, then the fix.
4. Gate after every task: `make ci` must pass. After Task 1, `make lint` must
   also stay at **0 findings**.
5. The owner authorized publishing this slice: push the branch, open the PR,
   triage and fix every review comment (Copilot or human), merge, tag, and
   publish the stable release. Anything outside that scope still needs a new
   approval. Before each push run `~/.config/genv/git/review-record.sh`.
6. Do not touch the user's live `~/.config/genv`. The test harness already
   redirects `$HOME`/config root; keep it that way.
7. Out of scope (do not start): range-satisfying upgrades, watching a peer
   service, Task 9 shared apply engine, M3–M5, winget/Chocolatey publishing.
   Note: Task 3 (`genv service restart`) *is* in scope — the owner added it.
8. If something in this plan proves wrong against the code, fix the plan in the
   same commit and say why — do not silently diverge.

## Baseline at audit time

| Gate | Result |
| ---- | ------ |
| `make ci` (vet, gofmt, race, cover-gate, bench-gate) | pass — 82.2% vs 80% floor; 141ms vs 200ms |
| `make lint` (golangci-lint) | **fail — 36 findings** (errcheck 21, staticcheck 13, ineffassign 2) |
| `govulncheck ./...`, `actionlint` | pass |
| `go test -tags integration ./e2e -run 'TestE2ECompose\|TestFiles_'` | pass |
| GitHub CI/Integration/Regression/Release/CodeQL on HEAD | pass |
| Open issues / PRs | 0 / 0 |

Package coverage below 80% (`go test -cover ./...`): `internal/testutil` 38.9,
`internal/external` 69.3, `internal/selfpath` 72.1, `internal/pull` 72.9,
`internal/files` 77.0, `internal/migrate` 78.8. Root package 80.7.
74 functions at 0% (list: `go tool cover -func=coverage.out | awk '$NF=="0.0%"'`).

## Tasks

### Task 1 — `make lint` to zero  (`chore(lint): …`)

- [x] Fix all 36 findings (plus 4 revealed after the first pass: 36 → 7 → 0). Known list at audit time:
  - errcheck, production: `internal/adapter/gem.go:227-228`,
    `internal/adapter/registry.go:65`, `internal/adapter/vscode_gallery.go:230`,
    `internal/complete/cache.go:73,109,112,117`, `internal/export/export.go:605`,
    `internal/external/archive.go:129,167,175`, `internal/external/download.go:41`,
    `internal/external/engine.go:91`, `internal/external/verify_sigstore.go:38`.
    Use `defer func() { _ = x.Close() }()` for read-side closes; for write-side
    closes, return the error. Cleanup removes: `_ = os.Remove(...)`.
  - errcheck, tests: `internal/adapter/outdated_test.go:453,455,491`,
    `internal/external/download_test.go:16,42`, `internal/external/engine_test.go:29`.
  - ineffassign: `main_helpers_coverage_test.go:245,251`.
  - staticcheck: `archive.go:206` (`tar.TypeRegA` deprecated — keep accepting
    old archives: compare against the byte `'\x00'` with a comment, or drop it
    only if Go's reader already normalizes it; add a test either way),
    `download.go:27`, `provider.go:72`, `validate.go:604` (De Morgan),
    `verify_sigstore.go:41` (lowercase error string — check no test matches the
    old text), `schema.go:227` (type conversion), `validate.go:455` (tagged
    switch), `validate.go:512` (precompile regexp outside loop),
    `main.go:3308` (dead `lf` read in scan — **verify** the second read under
    the mutex at ~3425 is the one used, then delete the first read and its
    error path), test-only S1021/SA9003 in `main_apply_files_test.go:267`,
    `main_spec_adapter_test.go:109,151`, `main_helpers_coverage_test.go:338`
    (SA9003 empty branch — the assertion is missing; make it a real `t.Error`).
- [x] Add a pinned `Lint` job to `.github/workflows/ci.yml` (golangci-lint-action@v8, v2.14.0) plus an explicit `.golangci.yml`
  (`.github/workflows/test.yml`); otherwise add a lint job there. Ask the owner
  before changing CI if unsure.
- Done when: `make lint` exits 0, `make ci` passes.

### Task 2 — published JSON Schemas match the code  (`fix(schema): …`)

Defect: `schema/v8`, `v9`, `v10` `genv.json` contain **none** of the 49 manager
names in `internal/schema/schema.go` `KnownManagers` (no `prefer`/`managers`
enum); `schema/v1` still lists `flatpak`, which is not a known manager. No test
ties these files to the code.

- [x] Write a failing test (`internal/schema/jsonschema_files_test.go`, package `schema_test`) that
  loads each `schema/v*/genv.json`, extracts the manager enum(s), and compares
  them to `KnownManagers` (v1: the managers valid for v1 — check `validate.go`
  for per-version restrictions rather than assuming all 49).
- [x] Fix the schema files so the test passes (v1 closed enum; v8+ `anyOf` enum + adapter-name pattern). Decide per version whether
  `prefer`/`managers` should be an enum or a free string (custom `adapters` in
  v8+ make `prefer` open — then document it as `string` and test that instead).
- [x] Remove `flatpak` from v1 (genv has never shipped a flatpak adapter).
- [x] Every file validated with `check-jsonschema` (draft-07): a v8 spec with `prefer: paru`, a v8 spec with a custom adapter, a v1 spec and a v10 spec all pass; `prefer: flatpak` in v1 is rejected.
- Done when: test passes and would fail if a manager is added to `KnownManagers`
  without updating the schemas.

### Task 3 — `genv service restart <name>`  (`feat(service): …`)

Owner request: today a misbehaving service needs two commands
(`genv service stop <name>` then `genv service start <name>`), with ordering and
readiness left to the human. Ship a real `genv service restart`.

Design (decide before coding, state the choice in the commit body):

- **Precedence**, matching the backend ownership rules already used by
  `service start`/`service stop`: `brew_formula` → `brew services restart`;
  `scheduled_task` → refuse (a trigger is not a resident process); `launchd`
  → boot out + bootstrap; `systemd` → `systemctl --user restart`; declared raw
  `restart` argv → run once; otherwise raw `stop` then `start`. A raw service
  without `restart` or `stop` is refused rather than half-restarted.
- **Dependency policy (changed after implementation review):** restart *only*
  the named service. Restarting a healthy database because an API is sick is a
  surprising, potentially destructive blast radius. Check `requires` first;
  when a dependency is down, name it and touch nothing. Do **not** implement
  `--all` or implicitly restart dependencies.
- **Readiness:** after the restart, if the service declares `health_check`, run
  `verify.Health` and report readiness failure separately from restart failure
  (same distinction `restartOutcome.ReadinessError` makes). A readiness failure
  is not silently "ok".
- **Contracts:** exit 0 success; `exitLogic` restart failure; `exitLogic`
  readiness failure with its own message; `exitUsage` missing name/unknown
  subcommand; unknown service → `exitLogic` like `start`/`stop`.
- **Consistency:** `--json` output? The other `service` subcommands are text
  only; decide once and document. Tests must prove both paths.

Files: `main.go` (dispatch + usage text), `main_service_restart_cmd.go`,
`internal/service/restart.go`, `internal/service/supervisor.go` (test seam for
`launchctl print`), tests, `README.md`, `SCHEMA.md`, `man/genv.1`,
`completions/`, `scripts/docker-v8-command-matrix.sh`, `CHANGELOG.md`.

- [x] Spec the precedence + dependency rules above in the `RestartDeclared` and
  CLI doc comments; TDD CLI/backend tests were written before the implementation.
- [x] `RestartDeclared` in `internal/service` with hermetic unit tests for brew,
  launchd, systemd, raw and scheduled-task refusal; no real supervisor is run.
- [x] CLI subcommand, usage/help, exit codes. It remains text-only to match
  `service start` and `service stop`; this is documented in `SCHEMA.md`.
- [x] Preflight dependency liveness check and separate health-check readiness
  verdict; an unhealthy restarted service exits non-zero without being reported
  as a failed restart.
- [x] Update help test, README, SCHEMA, man page, completions, command matrix,
  CHANGELOG; completion scripts now have a regression test.
- [x] Done when: `genv service restart` covers brew, launchd, systemd, raw and
  dependency cases under `go test ./...`, and the docs match the behavior.

### Task 4 — stale docs and repo debris  (`docs: …`)

- [x] Updated README's current-release line to v4.7.0/schema v10 and retained
  the `releases/latest` link. Existing platform stability text was already current.
- [x] Marked the composition spec M1+M2 shipped in v4.7.0; M3–M5 remain deferred.
- [x] Marked the composition plan completed and recorded Task 9 as deliberately
  skipped for regression-risk reasons.
- [x] Marked v4.7.0 release-readiness as published 2026-10-08 and corrected the
  CHANGELOG date.
- [x] Kept completed plans in place but changed their status lines: this repo has
  no `docs/archive/` convention yet, so moving them would add unrelated structure.
- [x] Deleted the unreferenced tracked `baseline.txt` benchmark capture.
- [x] Deleted discontinued `snap/snapcraft.yaml`; `release_config_test.go` now
  fails if it returns. The Snap package-manager adapter was untouched.
- [x] `.cursor/plans/` has no working-tree files (it is ignored), so there is no
  stale local stamp to update in this worktree.
- [x] Done when: no current doc claims a state that contradicts git tags/releases.

### Task 5 — input hardening  (`fix(schema): …`, `fix(genvfile): …`)

From the deleted `SECURITY_AUDIT.md` (restore with
`git log --diff-filter=D --name-only -- SECURITY_AUDIT.md` then `git show <sha>^:SECURITY_AUDIT.md`),
items still open after reconciliation:

- [x] **Version constraint format/length (audit #11).** `internal/version/version.go`
  `Satisfies` supports only `""`, `"*"`, exact, and `prefix.*`. Add validation in
  `internal/schema/validate.go` (wherever package `version` is checked) that
  rejects: length > 128, control/whitespace characters, leading `-`, and a `*`
  anywhere except a trailing `.*` or the whole value. Failing tests first;
  confirm existing fixtures/e2e specs still validate. Document in `SCHEMA.md`.
  Mention in CHANGELOG as a validation tightening (could reject previously
  accepted junk — that is intended).
- [x] **Size cap on `genv.json` and the lock (audit #13).** Modules are capped at
  1 MiB (`internal/compose/load.go`), but `genvfile.Read` (`genvfile.go:106`) and
  `ReadLock` (`lockfile.go:142`) use unbounded `os.ReadFile`. Add a shared cap
  (suggest 4 MiB, error naming the path and limit). Failing tests first.
- [x] **Lock-mutex regression guard (#97 residual).** Manual check at audit time:
  every lock-writing command (apply, upgrade, remove, add, adopt, disown, scan,
  files, service restart, updates worker) acquires `genvfile.LockMutation`. Add a
  test that fails if a `genvfile.WriteLock` call site in `main*.go` is not
  preceded by a `LockMutation` in the same function (AST or source scan), or an
  equivalent behavioral test. Keep it simple and deterministic.
- Audit items confirmed closed (no work): #9 package names
  (`schema.ValidPackageName`), #10 manager values, module limits; #7 `--file` is
  open by design.

### Task 6 — coverage  (`test: …`)

Targets: total ≥ 85%, every non-test-helper package ≥ 75%. Then lock it in.

**Outcome:** total 80.7% → 83.2% (floor raised 80 → 82; Linux CI measures ~0.8 pt below macOS, so 82 keeps headroom); `adapter` 83.5 → 86.6,
`external` 69.3 → 73.2, `selfpath` 72 → 84, `pull` 73 → 80+, `files` 77 → 78,
`migrate` 78.8 → 80.8. The 85% / 75%-per-package stretch targets were not fully
met (`external` stays below 75: sigstore/openpgp/minisign verify and `sudo`
elevation need real keys or a real sudo). Recorded as a known limit.

- [x] `internal/external` (69.3%) first: archive staging (tar gz/xz/zstd, zip,
  strip, path traversal rejection), download errors, verifier failure paths,
  `defaultRunElevated` via injected runner. Use `httptest` and in-memory archives;
  no network.
- [x] Root package: `reportApplyRestarts` (45.5%), `updatesCheckReadSpecError`
  (55.6%), and other low funcs from `go tool cover -func`.
- [x] `internal/pull` (72.9%, `copyBundleDir` 0%), `internal/files` (77.0%,
  `ResolveSource`/`ExpandPath`/`removePath` 0%), `internal/migrate` (78.8%,
  `cloneGenvFile` 0%), `internal/selfpath` (72.1%).
- [x] Adapter 0% funcs: `apt`/`dnf` `Search*`/`ListNames*`/`ListInstalledVersions`,
  `apk` versions, `pip_user`, `ghcup`, `opam`, `conda`, `composer`, `mas`
  completion, `command` adapter `NormalizeID`/`PlanClean`, `external` adapter,
  `fallback.DefaultFallbackEligible`. Use the existing fake-command/stub runner
  pattern in `internal/adapter/*_test.go` — never call real managers.
- [ ] Remaining 0% funcs: `compose.BundleFromEffective`/`NewProvenance`/
  `funcIdentity`, `env.RcFiles`, `genvfile.LockPathIn`, `plan.Names`,
  `schema.MarshalJSON`/`durationFields`/`hasDefaultService`,
  `service.ScheduledJobTimeOut`/`schtasksServiceHint`, `complete.RepoPackages`.
  Platform-only funcs (`processElevated`, `sudoNoninteractiveOK`,
  `ProbeSchtasksServiceRunning`) may stay uncovered if untestable — note them.
- [x] Raise `COVER_MIN` in `Makefile` to (achieved total − 1, rounded down), and
  add a per-package floor to `scripts/cover-gate.sh` only if cheap.
- Rule: tests must assert behavior, not just execute lines.

### Task 7 — final verification and publish  (`release:` …)

- [x] `make ci` (83.2% vs floor 82, bench 102ms), `make lint` 0 issues, `govulncheck` 0 reachable, `actionlint` clean.
- [ ] `go test -tags integration ./e2e/...` for the hermetic subsets.
- [x] `make integration-v8` equivalent: 108 PASS / 0 FAIL on Arch amd64 (includes `service restart`).
- [x] Add a CHANGELOG `Unreleased` section summarizing user-visible changes
  (validation tightening, size caps, schema files, `genv service restart`).
- [ ] Update this plan's status line and the baseline table with final numbers.
- [ ] Security pass on the pushed range: `gitleaks`, `trivy fs`, `semgrep
  p/gosec`, `govulncheck ./...`, `actionlint`. Triage everything new; note
  anything that also exists on `main` as pre-existing, but fix it if it is cheap.
- [ ] Triage **every** review comment on the PR (Copilot or human): verify each
  against the code rather than trusting it, fix the real ones, reply with the
  evidence, and re-push. Re-run the review gate before every push.
- [ ] Follow CI to green, merge, tag (patch bump, e.g. `v4.7.1`), and let the
  release workflow publish. Homebrew + GitHub Release = release success; if AUR
  fails, repair AUR only, do not re-run the whole Release workflow.
- [ ] Report the result with the final numbers.

## Owner decisions (record answers here)

- [x] Delete `snap/snapcraft.yaml`? — **yes, delete it** (owner: "fix all findings"); extend `release_config_test.go` to guard it
- [x] #84 (bootstrap-from-spec for curl installs): treat as done via external
  `script` installs? — _pending; default: yes, no work_
- [x] #92 (sensitive env values stored in the 0600 lock; hash-only rows as
  follow-up): leave as is? — _pending; default: leave_
- [x] Wire `make lint` into CI? — **yes**

## Known limits (deliberate, not backlog)

Range-satisfying upgrades (`version` set ⇒ upgrade skipped); watching a peer
service; Task 9 shared apply engine; M3–M5 (roles, tasks, run history);
winget/Chocolatey publishing; `updates.autoApply` is tracked-packages-only.
