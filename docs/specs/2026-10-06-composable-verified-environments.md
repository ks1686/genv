# Composable, verified environments

Status: **M1 (local composition) and M2 (verified service changes) shipped in v4.7.0.** M3–M5 remain separately scoped follow-up milestones; this document records their boundaries, not an approval to implement them.
Date: 2026-10-06

## Goal and boundaries

Compose local environment modules, resolve an explicit effective environment, and reconcile related resources in dependency order with observable outcomes. Preserve genv as a thin layer over native package managers and supervisors.

Approved direction: local repository modules, optional roles/machine selection, dependency-aware service upgrades, then input-aware tasks and run history. Remote module distribution, a module marketplace, fleet execution, general package dependency solving, and universal rollback are excluded.

Do not change the user's live configuration during development. Existing schema v1–v9 behavior remains compatible. No commit, push, issue creation, or release is authorized by this planning work.

## Delivery boundaries

1. **M1 — local composition:** schema v10, module loading, resource identity, provenance/conflicts, config/explain commands, integration with existing commands and portable bundles.
2. **M2 — verified service changes:** common apply execution, dependency ordering, explicit package/file watches, conditional restarts, readiness checks, shared foreground/background behavior.
3. **M3 — roles and machine selection:** explicit selection, same-OS machines, selection-change safety.
4. **M4 — input-aware tasks:** declared inputs/outputs, no-op checks, dependencies; hooks remain compatible.
5. **M5 — run history/recovery:** durable outcomes, interrupted-run diagnosis and re-observed resume; bounded restoration only.

M1 and M2 have executable tasks in the companion implementation plan. M3–M5 are separately scoped follow-ups, not permission to implement undefined schema fields.

## M1: composition contract

### Schema and selection

Schema v10 extends portable v9. Root `modules` maps a kebab-case module name to a repository-relative JSON file path. Portable bundles gain `useModules`, an ordered list of registered names. Defaults and the active target contribute selections additively with first-occurrence deduplication; this new field does not change existing array replacement rules.

A module document has exactly `schemaVersion: "10"`, optional `requiresModules`, optional `defaults`, and optional `targets`. Module targets use the existing known OS target IDs. An absent module target contributes only defaults. A module must contain defaults or targets. Module bundles cannot contain `useModules`; dependency selection belongs in `requiresModules`. Modules cannot declare another registry, repo, updates, or adapters. Custom adapters remain root-owned.

Example root:

```json
{
  "schemaVersion": "10",
  "modules": { "proxy-service": "modules/proxy-service.json" },
  "targets": { "macos": { "useModules": ["proxy-service"] } }
}
```

Example module (illustrative software, not the user's live configuration):

```json
{
  "schemaVersion": "10",
  "defaults": {
    "packages": [{ "id": "ripgrep", "prefer": "brew" }]
  }
}
```

Resolve dependency closure before resource composition. Reject missing registrations and cycles with the full module path. Use stable dependency-first ordering, declared list order, and sorted map traversal. Load each module once. Parse every registered module during validate; active commands compose only the selected closure. Limits: 256 registered modules, 1 MiB per module document, dependency depth 64; reject over-limit input with an actionable error.

### Paths and trust

Module documents must be regular, non-symlink files under the root spec directory. Reject absolute paths, traversal, environment/home expansion, and symlink components in module paths. Source-root overrides relocate the complete config tree, including module documents.

Relative asset paths declared inside a module are relative to that module file. Normalize them to root-relative paths when lowering into the effective spec. Preserve original origins separately. Reject module-relative asset escapes and symlink escapes; apply existing handling for explicit absolute/home/env asset paths, which remain non-portable. Resolve relative external verification-key assets and hook files as well as file/service templates. Module parsing and composition never execute commands or contact a network.

Module content remains trusted configuration, not sandboxed code. Safe loading does not make later hooks or service commands safe to run from an untrusted repository.

### Identity, ownership, and merging

Use structured identities `(kind, key)` internally; display them as `kind:key` without reparsing display strings. Packages use package ID; services/env/aliases/functions use their declared map name; files use normalized destination; legacy hooks use origin plus phase and index; shell sources use normalized path. Origin records contain document path, JSON field path, module name, and target bucket, never secret values.

First materialize each contributor's own defaults/target using current overlay semantics. Then union contributors by identity:

- Identical declarations coalesce and retain all owners.
- Different declarations for the same identity fail with both origins.
- The root is a contributor, not an implicit higher-priority override.
- Within-contributor target tombstones retain existing semantics; they do not delete another module's resource.
- Hook ordering is root first, then dependency-first modules, preserving order within phases. Hooks with different origins are not coalesced.
- File normalization is target-aware: Windows path rules apply even on a Unix planning host. Detect cross-kind destination ownership collisions; duplicate identical directories may coalesce. Expand/check active-host paths again before mutation to detect runtime aliases.

Arbitrary cross-module overrides/removals are intentionally not part of M1. Refactor conflicting declarations or edit their owner. Explicit override syntax belongs with M3, with its own review; do not ship a placeholder field.

Deselecting a module removes only its ownership. Shared resources remain desired. Existing removal behavior remains in force: M1 does not add recursive directory deletion or new file deletion semantics.

### Integration boundary

`internal/schema` validates document shapes without I/O. `internal/genvfile` reads/writes raw documents. New `internal/compose` loads module documents and returns an effective flat spec plus identities, provenance, selected module names, and a non-secret structural selection fingerprint. The fingerprint excludes env values and other credential-bearing content.

Keep raw specs separate from effective specs: never write a flattened environment over the root. All consumers of desired state must use one composition entry point, including completion, scan, status, service commands, upgrade, scheduler, hooks, map, export, and validation. Lock-only `list` remains lock-only.

`genv config --target <id> [--json]` displays the effective environment with existing sensitive-value redaction. `genv explain <resource> --target <id> [--json]` returns ownership and selection reasons, not raw commands or env values. File resources are selected using the displayed `file:<destination>` selector, preserving everything after the first colon as the key. No command probes run for these commands.

For M1, commands that would mutate a module-owned declaration fail before hooks, subprocesses, spec writes, or lock writes, naming the owning file. Root-owned mutations remain supported. Scan excludes already desired module packages and adds only new root-owned entries. No automatic multi-file writes or `--module` mutation flag in M1. Package removals and disown must not uninstall/untrack a still-module-owned package.

Export flattens the selected environment into a portable snapshot and includes provenance in its report; it does not export machine-local state or secrets. Pull bundles registered module documents and their portable assets and validates a complete staged tree before publishing it locally. Existing symlink/secret exclusions stay in force.

## M2: execution contract

Add v10 service fields `requires` and `watch`, containing resource selectors for packages, files, or services. `requires` blocks service action when a prerequisite is unavailable/failed; `watch` implies a prerequisite and requests reconciliation when observed content changes. Reject unknown references, self edges, and cycles before mutations.

Add `restartPolicy` with `never` (default) and `ifRunning`. Capture state before upgrades; never start an intentionally stopped service merely because a watched package changed. Existing first-apply service behavior remains unchanged. Use declared restart argv, native supervisor restart, or an explicit stop/start sequence for custom services; never invent shell commands.

Add optional `healthCheck` with `command` argv, `timeout` positive duration (default 30s), `interval` positive duration (default 1s), and `allowBackground` (default false). Exit zero means ready; retry within the total deadline. Checks run only after authorized start/restart, never in ordinary status, config, explain, or dry-run. Scheduled tasks without a check report registration outcome, not application readiness. A required health check not permitted unattended defers the dependent service-affecting upgrade with an explicit reason.

Compare before/after live package versions and available managed-file/artifact digests. A successful no-op command is not a change; missing evidence is `unknown`, not unchanged. Unknown change evidence defers restart and reports manual verification required. A failed or partially failed package operation never produces a readiness claim for dependent services, even if its version changed. Coalesce multiple watch triggers into one restart per service per run.

Persist a non-secret pending service action under the existing mutation lock before changing a watched package, so interruption or readiness failure cannot lose the need to reconcile. Clear it only after the required action/verification succeeds. This is a minimal pending-action receipt, not M5's general run journal. An interrupted restart remains uncertain; the next explicit retry re-observes rather than blindly repeats it. Unattended retries do not silently replay an uncertain restart.

Unify text/JSON execution behind a common result model. Files precede dependent services; failures block dependents while independent work can finish. Initially execute serially in stable topological order; do not parallelize package-manager transactions. Preserve existing output envelopes and add optional explanations/outcomes rather than silently changing their meaning.

Foreground upgrades and the tracked-only background checker use the same service-change coordinator. Background execution still excludes OS/firmware work, respects noninteractive elevation policy, and skips incompatible actions before upgrading their watched packages. Dependency-aware removal stops a service being removed before uninstalling its prerequisites; retained services with removed required resources fail planning.

## Follow-up contracts

M3 separates target (OS), role (module selection), and machine (target plus roles). No hostname guessing. Selection changes are planned before acceptance; an unattended worker refuses mismatched accepted selection. Local selection state is never exported or pulled. Existing `profile` v1–v7 is not silently repurposed.

M4 uses new named task resources, not guessed input hashing for arbitrary hooks. Tasks need explicit side-effect boundaries and background eligibility. Keep manual services manual.

M5 records metadata and outcomes with private permissions, redaction, retention, and crash-safe writes. Resume obtains the mutation lock, re-observes state, and produces a new plan. Recovery never claims package/OS transactions are universally reversible.

## Acceptance

An unchanged v9 fixture behaves as before. A v10 root composes two local modules with explainable ownership and deterministic conflicts. Every command sees the same desired package/service set. Removing one shared owner does not remove the resource. No dry-run executes new custom commands. A proven watched package change restarts an already-running opted-in service exactly once, after its prerequisites, and reports readiness or an actionable failure. No real host packages, services, or live configuration are needed for these tests.
