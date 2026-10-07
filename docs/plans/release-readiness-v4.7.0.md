# Release readiness — v4.7.0

**Status: ready to publish, awaiting explicit approval to push and tag.**
Nothing has been pushed. The branch is local in `~/Documents/Worktrees/genv/compose-v10`.

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
| `make ci` (vet, gofmt, race, cover-gate 81.6%, bench-gate 107ms/200ms) | pass |
| `make lint` | 36 findings — **identical to clean `main`**, zero added |
| `make integration-v8` (Arch Docker, amd64 emulation) | **107 PASS / 0 FAIL** |
| `go test -tags integration ./e2e -run TestE2ECompose` | pass (9 subtests) |
| `gitleaks` (worktree + `main..HEAD`) | no leaks |
| `trivy fs` | 2 HIGH in `golang.org/x/mod` — **same on `main`**, not introduced |
| `semgrep p/gosec` on new packages | 0 findings |
| v1–v9 output vs `main` binary | **byte-identical** across status/apply/validate |

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

## Test-harness hazard found and fixed

`TestMain` redirected `XDG_CONFIG_HOME` to a temp dir only when it was unset. Where
it is set (as on the author's machine), `go test ./...` resolved the real
`~/.config/genv` and ran the unattended updates worker against it three times
during verification. Impact: the live `genv.lock.json` was rewritten with
`upgraded=0`. The live `genv.json` was never modified and no package changed.
`TestMain` now always overrides the config root, and a regression test fails
against the old conditional. A full `go test ./...` no longer touches the live
lock or log.

## Known limits

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