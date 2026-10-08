# Release readiness — v4.7.0

**Status: published as v4.7.0 on 2026-10-08.**
This is the historical release-readiness record; the branch was merged, tagged and released.

## What ships

Two capabilities under schemaVersion `"10"`, plus one CLI addition each.

### 1. Composable environments (local modules)

A spec can be assembled from several documents instead of one:

```json
{
  "schemaVersion": "10",
  "modules": { "base": "modules/base.json", "dev": "modules/dev.json" },
  "targets": { "macos": { "useModules": ["dev"] } }
}
```

`useModules` is additive and follows `requiresModules`. Every desired-state command
(`apply`, `status`, `upgrade`, `updates`, `scan`) resolves the composed
environment, so a module's packages install, lock, and report like root ones.

Design decisions worth reviewing before publish:

- **The root is a contributor, not an override.** Identical declarations
  coalesce and keep every owner; differing declarations are an error naming both
  origins. There is no override syntax — a silent winner is the failure mode
  this exists to remove.
- **Module-owned resources are read-only from the CLI** (`add`, `remove`,
  `disown`, `adopt`, `env`, `shell`, `service`, `files adopt`), refused before
  any subprocess or write.
- **Modules are local and trusted**: contained to the spec directory, component
  symlinks rejected, `O_NOFOLLOW` reads, size and depth limits. Nothing is
  fetched or signature-checked.
- **`validate` loads every registered module**; reconciliation loads only the
  selection closure.
- **`export` flattens** and records the materialized modules in `report.json`.
- **`migrate` refuses v10** — nothing to convert, and rewriting would discard the
  registry.

### 2. Dependency-aware service changes

```json
"api": {
  "start": ["api-server"],
  "requires": ["db"],
  "watch": ["postgres", "api.conf"],
  "restart_policy": "ifRunning",
  "health_check": { "command": ["curl", "-fsS", "localhost:8080/health"] }
}
```

Closes #218. `requires` (ordering) and `watch` (change trigger) are deliberately
different relationships.

The honest parts, which are the point:

- An upgrade that ran and left the installed version unchanged restarts nothing.
- A version genv cannot establish yields **`unknown`**, and the service is
  deferred — never restarted on a guess.
- Readiness failure is reported separately from restart failure.
- A health check **never runs from a plan** (status, any dry run).
- A pending record is written *before* a service is stopped and cleared only
  after the action and readiness succeed, so an interrupted run is reported.
- The unattended worker defers upgrades whose watched service it cannot restart
  *and* verify without a human, before touching any package.
- The four fields are **refused on v1–v9** rather than ignored.

### 3. `genv config` / `genv explain`

What a target actually gets, and why one resource is there. Attribution names
the declaring block (`defaults` vs `targets.<id>`) so it points at a line a user
can edit.

## Verification

| Gate | Result |
| ---- | ------ |
| `go test ./...` | **all green** (first green run on this branch) |
| `make ci` (vet, gofmt, race, cover-gate 82.1%, bench-gate 103ms/200ms) | pass |
| `make lint` | 36 findings — **identical to clean `main`**, zero added |
| `make integration-v8` (Arch Docker, amd64 emulation) | **107 PASS / 0 FAIL** |
| `go test -tags integration ./e2e -run TestE2ECompose` | pass (9 subtests) |
| `gitleaks` (worktree + `main..HEAD`) | no leaks |
| `trivy fs` | 2 HIGH in `golang.org/x/mod` — **same on `main`**, not introduced |
| `semgrep p/gosec` on new packages | 0 findings |
| v1–v9 output vs `main` binary | **byte-identical** across status/validate/apply --dry-run/list/scan --dry-run |

Review record: recorded by `review-record.sh` against the final commit (3 checks, all passed).

## Bugs found and fixed during verification

Three defects that unit tests could not have caught, found by running the real
thing:

1. **No service would ever have restarted.** `RunUpgrade` rewrites installed
   versions in the lock in place, so the "before" snapshot was taken afterwards
   and every comparison read "unchanged".
2. **`genv upgrade` deadlocked.** The restart phase re-acquired the lock mutex the
   upgrade path and the worker already hold; flock is not re-entrant.
3. **`service api: restartd`** — the completion line was built by appending `d`.

Also fixed: `validate` accepted a service `requires` cycle the documentation
promised it would reject, and `TestBrewServicesList` ran the real `brew`, which
made `make ci` red on a clean checkout and blocked the review gate.

## Second review pass: findings on the branch

A review of the pushed branch raised ten items. Each was re-verified against the
code rather than taken on trust; all ten were real, and two were worse than
reported.

**Composition**

- `shell.source` was dropped entirely on the module path. Fixed.
- Array fields used `len() > 0`, so an empty target array kept the defaults
  instead of clearing them — the same spec meant two things with and without
  modules. Fixed for packages, file links, templates, dirs and hook phases.
- Hooks were appended within a document, so a target phase could not replace the
  defaults phase. Fixed.
- A module-free spec produced an empty provenance index, so `genv explain`
  reported every v8/v9 resource as unowned. Fixed.
- A `requires` edge was judged against a synthetic file holding only the bundle
  being checked. **Worse than reported**: every reference outside that one bucket
  failed, including a service declared in another bucket of the same module. A
  module that ordered itself after the root database failed *every* command.
  Cross-document references now resolve against the composed union.
- A `watch` entry naming a file or another service was accepted and then
  silently ignored forever. Now a validation error naming the entry.

**Service restarts**

- `genv upgrade --json` returned before the restart phase: it upgraded the
  binary, left the service on the old code, and reported success. Fixed; restarts
  are reported under a new `services` key.
- The unattended worker deferred the packages needing a human and then never
  restarted anything at all, silently. Fixed.
- `health_check.timeout` bounded only the gap *between* probes, so `timeout:
  200ms` could hold the command open indefinitely on a hung probe. Fixed.
- The fingerprint skipped `requires`, `watch`, `restart_policy` and
  `health_check`, so a change to restart behavior left the lock looking current.
  Fixed.
- A pending record that could not be written was swallowed, while a comment
  claimed the outcome reported it. Fixed.

**One more, found while checking compatibility**

Validation output for an *invalid* spec was not deterministic: fifteen runs of
one spec produced three different orderings, because validators walk Go maps.
Pre-existing on `main`, not introduced here. Errors are now sorted once at the
boundary of `ParseAndValidate`/`ParseAndValidateModule`. Valid v1/v5/v7/v8
output is byte-for-byte unchanged; only the ordering of an invalid spec's errors
changed, and it no longer moves.

`TestMain` now always overrides the config root, and a regression test fails
against the old conditional. A full `go test ./...` no longer touches the live
lock or log.

A **second, larger instance of the same class** was found later, while adding the
file-watch feature: `$HOME` was still the developer's, and `launchctl` was not
shadowed. A test that applied a service wrote `~/Library/LaunchAgents/genv.*.plist`
and bootstrapped a real launchd job into the user's session. The job survived the
test run, and the next `genv validate` failed with a dangling
`ProgramArguments[0]` — which reads like a product regression and was not one.
`$HOME`/`%USERPROFILE%` are now redirected and `launchctl`/`systemctl` are
shadowed with fakes that succeed without touching session state. The stray job
was unloaded and the stray plist removed; the four real genv agents
(peaproxy, rclone-sync, searxng, updates) were never touched. Both fixes have
regression tests that fail against the old behavior.

## Known limits

- **Watching another service is not a trigger.** `watch` covers tracked packages
  (upgrade moves the installed version) and managed file destinations (apply
  creates or rewrites the file), both driving the restart phase on the human and
  JSON paths. Watching a *peer service* is still refused at validate time:
  restarting because another service changed is a different question, and the
  evidence for it does not exist.
- `make integration-v8` needs `--platform linux/amd64` on Apple Silicon;
  `archlinux:latest` has no arm64 manifest. The Makefile target is unchanged, so
  on arm64 hosts it must be run with the flag added (CI is amd64).
- Task 9 of the plan (refactoring `runApplyText`/`runApplyJSON` onto a shared
  execution engine) was **deliberately not done**. The result model the restart
  phase needs exists in `internal/service` instead; rewriting both apply output
  paths was high regression risk for infrastructure value the feature does not
  need. Everything else in the plan shipped.
- `updates.autoApply` remains tracked-packages-only; OS/firmware steps stay
  interactive-only.
- No remote module registry, no fleet execution, no general dependency solving,
  no rollback promise.

## Proposed publish sequence (needs approval)

1. `git push -u origin feat/compose-v10`
2. `gh pr create` — title: `feat: composable environments and verified service changes (schema v10)`
3. `gh stack` not needed (single slice)
4. `git tag -a v4.7.0 -m "..."` after merge to main
5. Release workflow publishes via GoReleaser; Homebrew + GitHub Release is
   release success

Release notes are the `## v4.7.0` CHANGELOG section, unchanged.