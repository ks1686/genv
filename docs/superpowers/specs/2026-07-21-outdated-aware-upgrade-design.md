# Outdated-Aware Upgrade Design

Date: 2026-07-21
Status: Approved

## Goal

Make `genv upgrade` plan and run only packages that actually have updates available by default, matching the hourly updates checker, while keeping an explicit escape hatch for the previous brute-force behavior. Expand outdated detection across managers that have trustworthy APIs, batch App Store upgrades, and keep successful upgrade output visible.

## Problem

`genv upgrade` currently builds a plan for every unconstrained tracked package (`DetectOutdated: false`). Managers then no-op already-current packages, which produces:

- Huge brew upgrade argv lists and floods of "already installed" warnings
- One mas upgrade line per tracked App Store app
- Upgrade plans that disagree with `genv updates check` / the scheduled worker, which already use `DetectOutdated: true`

The filtering machinery (`FilterOutdated` / `OutdatedLister`) already exists and is tested; upgrade simply does not enable it.

## Required behavior

### CLI defaults

- `genv upgrade` and `genv upgrade --dry-run` set `DetectOutdated: true` when building the plan.
- `--all` sets `DetectOutdated: false` and restores the previous "plan every unconstrained tracked package" behavior.
- Existing filters remain unchanged: `--only`, `--skip`, `--only-manager`, `--skip-manager`, version-constraint skips, confirmation/`-yes`, JSON, hooks, debug.
- When the filtered plan is empty, print a short "nothing to upgrade" message (or empty JSON batches) and exit 0.
- Human output still prints the upgrade plan and each real upgrade command/result that runs. Filtering removes only non-outdated packages from the plan; it does not hide successful upgrades.

### Shared planner semantics

- Updates check and the scheduled `__run-once` worker continue to pass `DetectOutdated: true` (no behavioral change on those paths beyond benefiting from new listers).
- Version-constrained packages remain skipped with the existing stable reason before outdated filtering.
- Managers without `OutdatedLister` remain on the conservative path: keep all of their filtered packages (do not silently skip).
- If a manager's `ListOutdated` call fails, keep that manager's packages and surface a warning (existing `FilterOutdated` contract).

### Outdated lister coverage

Already implemented (unchanged contract):

- brew / linuxbrew — `brew outdated --json=v2`
- mas — `mas outdated`
- bun — global install list vs registry latest

Add `OutdatedLister` in this change:

| Manager group | Detection approach |
|---------------|-------------------|
| npm, pnpm, yarn | Global list + registry `latest` (shared pattern with bun) |
| uv, pipx | Native list/outdated where available, else installed vs PyPI latest |
| cargo | Installed crates vs crates.io latest |
| winget, scoop, choco | Native outdated/status commands, intersect with tracked names |
| pacman, paru, yay | Native outdated query, intersect with tracked names |
| snap | Refresh/outdated listing, intersect with tracked names |

Deferred (conservative keep-all until detection is trustworthy): asdf, mise, sdkman, deno, go modules, volta, juliaup, ghcup, opam, stack, conda/mamba/pixi/poetry, vscode, krew/helm, gem, composer, dotnet, rustup, and similar URL- or pin-shaped adapters.

### Mas batching

- mas implements `BatchUpgrader` so multiple outdated App Store apps become one `mas upgrade id1 id2 …` action after filtering.
- Single-package upgrades continue to use `PlanUpgrade`.

### Documentation

- Update `genv upgrade` help text and README so default = outdated-only and `--all` = brute-force.
- Correct any docs that imply updates check is identical to upgrade dry-run without outdated queries.
- Update the planner comment that currently states upgrade leaves `DetectOutdated` false.
- CHANGELOG entry describing the default behavior change.

## Architecture

### Plan construction

```
lock ∩ spec
  → user filters
  → skip version pins
  → FilterOutdated (unless --all)
  → PlanUpgrade (batch where BatchUpgrader)
  → print / confirm / execute
```

`upgradeCmd` passes `DetectOutdated: !*allFlag` into `upgrade.BuildUpgradePlan`. No new planner stages are required.

### Lister implementation pattern

- Prefer one manager-native outdated query per adapter when the CLI provides structured or parseable output.
- For JS globals (npm/pnpm/yarn), reuse shared helpers already used by bun (installed map + `npmLatestVersion` / equivalent) so registry comparison stays consistent.
- Always intersect results with the tracked `pkgNames` when the manager reports a broader outdated set.
- Return `nil, nil` when nothing tracked is outdated; return `nil, err` only on real query failure so `FilterOutdated` can keep packages conservatively.

### Mas BatchUpgrader

`PlanUpgradeBatch(pkgNames)` returns `["mas", "upgrade", ...pkgNames]`. The resolver already prefers batch planning when a manager implements `BatchUpgrader` and more than one package remains for that manager.

## Error handling

- Outdated query failure → keep packages for that manager + warning (no silent drop).
- Upgrade command failures → existing per-action error reporting and exit behavior.
- Lock write after successful upgrades → unchanged.
- Empty outdated plan → success, not an error.

## Testing

- CLI/planner: default upgrade path uses outdated filtering; `--all` does not.
- Empty outdated plan produces empty actions and a clear human/JSON empty result.
- Each new `ListOutdated` implementation has fake-binary / fixture unit tests (success, intersection, nothing outdated, command failure).
- Mas `PlanUpgradeBatch` shape test; resolver batches multiple mas packages into one action.
- Existing outdated and updates-check tests remain green.

## Non-goals

- Suppressing Homebrew's own "already installed" warnings when the user explicitly passes `--all`.
- Adding outdated detection for every remaining adapter in this change.
- Changing scheduled `autoApply` policy or notification rules.
- Running `brew update` (or equivalent refresh) before every outdated query; stale index behavior remains as today.

## Success criteria

- For a brew+mas+bun lock with only a few real updates, default `genv upgrade --dry-run` lists those packages (not the full tracked set).
- Successful upgrades still print clearly in human mode.
- `genv upgrade --all --dry-run` restores the previous full unconstrained plan.
- `genv updates check` and the hourly worker continue to report only outdated packages, now with broader manager coverage where listers were added.
