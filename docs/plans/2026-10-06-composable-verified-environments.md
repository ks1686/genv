# Plan: composable, verified environments (M1 composition + M2 service changes)

Design baseline: [docs/specs/2026-10-06-composable-verified-environments.md](../specs/2026-10-06-composable-verified-environments.md)
Created: 2026-10-06 · Status: **completed (v4.7.0)** · Fast-path: no
Scope here was M1 and M2 only. M3–M5 remain separately scoped follow-up milestones. Task 9's shared apply execution engine was deliberately not implemented: it was high-regression-risk infrastructure that the shipped service-result model did not need.

## Goal and architecture

Goal: let genv compose **local repository modules** into one effective environment with explicit ownership, then reconcile related resources **in dependency order with observable outcomes** (service restart after a proven binary change, readiness checks, honest "unknown" states).

Architecture decisions that shape every task:

1. **New schema v10** extends the portable model (v8 `defaults` + `targets.*`). v10 adds root `modules` (name → repo-relative JSON path) and bundle `useModules`. v1–v9 behavior is unchanged; `IsPortableVersion` covers v8, v9, v10.
2. **Three new internal units** so `internal/schema` stays pure validation and `internal/genvfile` stays raw document IO:
   - `internal/compose` — loads module documents, resolves selection closure, unions resources with provenance/conflict detection, returns the effective flat spec + identities + fingerprints.
   - `internal/plan` — builds the dependency graph over composed resources (`requires`, `watch`) and returns a stable topological order with reasons.
   - `internal/apply` — shared execution engine producing one result model consumed by text output, JSON output, and the background updates worker.
3. **One composition entry point.** Every desired-state consumer (apply, status, upgrade, updates, scan, add/remove/disown/adopt, env/shell/service/files, completion, validate, export, map, pull) goes through it. `genv list` stays lock-only.
4. **No silent resolution.** Conflicting declarations are errors with both origins. Root is a contributor, not an override. No cross-module override syntax in M1.
5. **No false change claims.** A successful command is not proof of change; missing evidence is `unknown`, which defers restarts and asks for manual verification.
6. **No live-config edits during development.** All tests use temp dirs, `testing/fstest`, and `internal/testutil` helpers.

Tools: Go 1.24.3, existing `Makefile` targets, `go test -race`, `cover-gate` (COVER_MIN=80), `bench-gate`. No new dependencies.

Global constraints:

- No commits, pushes, PRs, issue edits, or releases without explicit request.
- Do not modify `~/.config/genv/genv.json` or any live machine state while implementing.
- Schema additions need: Go validation, `schema/v10/genv.json` mirror, `SCHEMA.md`, `README.md`, completions, `man/genv.1`, CHANGELOG entry.
- New commands need dispatch entries in `run()` (main.go), usage text, and all four completion scripts.
- Every task keeps `go test ./...` green; no task leaves a failing test behind.

## File and responsibility map

New files (M1):

| File | Responsibility |
|---|---|
| `internal/schema/module.go` | `ModuleDoc` shape, module parse/validate, module-specific unknown-key walk, module path syntax validation |
| `internal/schema/module_test.go` | red/green tests for module documents |
| `internal/compose/compose.go` | `Composition` type, `Resolve` entry point, per-root composition |
| `internal/compose/load.go` | module document IO: regular file, no symlink components, size limit, path-safety |
| `internal/compose/select.go` | selection closure, dependency order, cycle/missing-module detection |
| `internal/compose/merge.go` | identity union, coalescing, conflict detection, hook ordering, asset path normalization |
| `internal/compose/identity.go` | structured resource identities + target-aware file path normalization |
| `internal/compose/provenance.go` | `Origin` (document path, JSON field path, module, target) + `Provenance` index |
| `internal/compose/fingerprint.go` | non-secret structural fingerprint of the effective environment |
| `internal/compose/errors.go` | typed errors: `ErrConflict`, `ErrCycle`, `ErrMissingModule`, `ErrPathEscape`, `ErrLimit` |
| `internal/compose/*_test.go` | table-driven tests for each file above |
| `main_compose.go` | `configCmd`, `explainCmd`, mutation ownership guards used by main |
| `main_compose_test.go` | tests for the two commands and the guards |

New files (M2):

| File | Responsibility |
|---|---|
| `internal/schema/servicechange.go` | `requires` / `watch` / `restartPolicy` / `healthCheck` validation on `Service` |
| `internal/plan/plan.go` | resource graph, stable topological order, blocked reasons |
| `internal/plan/plan_test.go` | ordering, cycle, missing-reference tests |
| `internal/apply/apply.go` | shared execution result model + engine used by text/JSON/worker |
| `internal/apply/apply_test.go` | engine tests with fake executors |
| `internal/verify/health.go` | readiness check with retry/timeout; exit 0 = ready |
| `internal/service/changes.go` | change evidence (version compare, file digests), pending action receipt, restart coordinator |

Modified (M1): `internal/schema/schema.go`, `internal/schema/validate.go`, `internal/schema/unknown.go`, `internal/genvfile/rewrite.go`, `internal/output/json.go`, `internal/genvfile/lockfile.go`, `internal/lockgate/gate.go`, `internal/export/export.go`, `internal/export/report.go`, `internal/pull/assets.go`, `internal/complete/repo.go`, `main.go`, `main_help.go`, `main_scan_*`, `main_files.go`, `main_pull.go`, `main_upgrade*`, `main_updates_worker.go`, `main_service_test.go`, `completions/*`, `schema/v10/genv.json` (new), `SCHEMA.md`, `README.md`, `CHANGELOG.md`, `man/genv.1`, `docs/multi-machine.md`, `scripts/docker-v8-command-matrix.sh` (add v10 module fixture step).

Modified (M2): `internal/schema/schema.go` (service fields), `internal/schema/unknown.go` (serviceFields), `internal/service/*`, `main.go` (`runApplyText`, `runApplyJSON`, `upgradeCmd`), `main_updates_worker.go`.

## Task order

Serial chain: 1 → 2 → 3. Then parallel-safe: 4, 5, 6. Then serial: 7 → 8. M2: 9 → (10 ∥ 11) → 12 → 13.

## Tasks

### Task 1 — schema v10 documents and validation (serial)

Files: `internal/schema/schema.go`, `internal/schema/module.go` (new), `internal/schema/validate.go`, `internal/schema/unknown.go`, `schema/v10/genv.json` (new).

Contracts:

- `const Version10 = "10"`; append to `versionOrder`; `IsPortableVersion` returns true for 8, 9, 10.
- `GenvFile.Modules map[string]string \`json:"modules,omitempty"\``; `TargetBundle.UseModules []string \`json:"useModules,omitempty"\``.
- `internal/schema/module.go`: `type ModuleDoc struct { SchemaVersion string; RequiresModules []string; Defaults *TargetBundle; Targets map[string]*TargetBundle }`; `ParseAndValidateModule(data []byte) (*ModuleDoc, []ValidationError, error)`; `ValidateModuleName(name string) bool` (kebab-case, ≤64 chars, not colliding with a built-in manager); `ValidateModulePath(p string) bool` (relative, no `..` segment, no `~`, no `$`).
- `modules` requires v10; `useModules` requires v10; a module doc requires `schemaVersion: "10"`, at least one of `defaults`/`targets`, no `modules`/`useModules`/`repo`/`updates`/`adapters` inside it, and target keys from `KnownTargets`.
- `internal/schema/unknown.go`: add `"modules"` to `rootFields`, `"useModules"` to `bundleFields`, and a `moduleFields` walk for module documents.

Red tests (`internal/schema/module_test.go`, `schema/validate_test.go`): `TestParseAndValidateModule_accepts_v10_document`, `TestModule_v8_schemaVersion_rejected`, `TestModule_rejects_nested_modules`, `TestValidateModulePath_rejects_traversal`, `TestPortable_v10_requires_useModules_version`, `TestRootModules_preserved_by_marshal`. First run: `go test ./internal/schema/ -run 'Module|Portable_v10'` fails to compile (`undefined: Version10`) → implement → passes.

Verify: `go test ./internal/schema/ -run 'Module|Portable' && go test ./internal/schema/`.

Note: `genvfile.New()` still creates v8; M1 keeps new empty specs at v8 unless the user opts into v10. Do not change `New()`.

### Task 2 — module loading with path safety (serial)

Files: `internal/compose/load.go`, `internal/compose/errors.go`, tests.

Contracts: `LoadDocument(root, relPath string) (*schema.ModuleDoc, string, error)`; `LoadAllRegistered(root string, registered map[string]string) (map[string]*loadedModule, error)` where `loadedModule{Doc *schema.ModuleDoc; RelPath string; AbsPath string}`. Enforce: `os.Lstat` regular file, no symlink in any path component (`filepath.EvalSymlinks` on the parent must equal the parent), size ≤ 1 MiB, root containment via `genvfile.WithinDir`, registered count ≤ 256. Loading never executes commands or touches the network. Typed errors: `ErrPathEscape`, `ErrMissingModule`, `ErrLimit`, `ErrInvalidModule`.

Red tests: `TestLoadDocument_rejects_symlinked_module`, `TestLoadDocument_rejects_traversal`, `TestLoadDocument_rejects_oversize`, `TestLoadAllRegistered_reports_missing_registration`. First run: compile failure → implement → pass.

Verify: `go test ./internal/compose/`.

### Task 3 — selection closure, ordering, and composition (serial)

Files: `internal/compose/select.go`, `compose.go`, `identity.go`, `provenance.go`, `fingerprint.go`, `merge.go`, tests.

Contracts:

- `Select(reg map[string]string, docs map[string]*loadedModule, selections []string) ([]string, error)` — dependency closure, cycle detection (`ErrCycle` with the full path), deterministic order (dependency-first, declared list order, sorted map traversal).
- `type Origin struct { Document string; Field string; Module string; Target string }`; `type Provenance struct { Owners map[Identity][]Origin }`.
- `type Identity struct { Kind string; Key string }` with `String() "kind:key"` (display only; never reparsed).
- `Resolve(rootSpecPath, sourceRoot string, f *schema.GenvFile, targetID string, selections []string) (*Composition, error)`; `Composition{Effective *schema.GenvFile; Provenance; SelectedModules []string; Fingerprint string}`.
- Merge rules: materialize each contributor's own `defaults`+target with current overlay semantics first; then union by identity. Identical declarations coalesce (all owners kept). Different declarations for the same identity → `ErrConflict` naming both origins. Root is a contributor. Within-contributor tombstones keep current semantics and never delete another module's resource. Hooks keep order (root first, then dependency-first modules) and never coalesce across origins. File identity uses the normalized destination with target-aware Windows rules, plus cross-kind destination collision detection. Module-declared relative asset paths are normalized to root-relative in `Effective`, with origins preserved.
- `Fingerprint` is a structural, non-secret hash: exclude env values, include resource identity/structure/selection.

Red tests: `TestSelect_orders_dependencies_first`, `TestSelect_detects_cycle`, `TestResolve_conflicting_services_error`, `TestResolve_identical_declarations_coalesce_with_both_owners`, `TestResolve_root_tombstone_does_not_delete_module_resource`, `TestResolve_hook_order_root_first`, `TestResolve_normalizes_module_asset_paths_to_root`, `TestFingerprint_excludes_env_values`, `TestResolve_v8_spec_unchanged`. First run: compile failure → implement → pass.

Verify: `go test ./internal/compose/`.

### Task 4 — single composition entry point in the CLI (serial after 3)

Files: `main.go` (`resolveEffectiveSpec`, `materializeSpecForCommand`, `readMaterializedSpec`, `materializedHooks`, `addCmd`, `runRemove`, `adoptCmd`, `disownCmd`, `envSetCmd`, `envUnsetCmd`, `shellAliasSetCmd`, `shellAliasUnsetCmd`, `serviceAddCmd`, `serviceRemoveCmd`, `adoptFilesCmd`, `scanCmd`, `completeInternalCmd`, `validateCmd`, `mapCmd`, `exportCmd`, `pullCmd`), `main_scan_dryrun_test.go`, `main_test.go`, `internal/complete/repo.go`, `internal/genvfile/rewrite.go`.

Contracts: add `composeEntry(file, f, hostFlag, targetFlag, sourceRoot string) (*compose.Composition, int)`; `resolveEffectiveSpec` and `materializeSpecForCommand` return the composed effective spec for v10 and keep the exact current code path for v1–v9. `--source-root` relocates module documents as well. `rewrite.go`: `sameNonPackageState`/`bundleMetaEqual` must treat `Modules`/`UseModules` as non-package state so additive package rewrites never drop them. Lock metadata (`internal/genvfile/lockfile.go`, stamped by `stampLockTarget` and the lock-write path) records the selected module list and the composition fingerprint; `internal/lockgate/gate.go` treats a selection/fingerprint change as a **warning**, never a foreign-lock refusal, because changing module selection is an ordinary spec edit rather than a machine mismatch.

Cycle rule for this task: mutation commands (`add`, `remove`, `disown`, `adopt`, `env`, `shell`, `service`) keep their current raw-spec reading. They only consume composition through the guard helper that Task 5 defines, so Task 5 depends on Task 4's `composeEntry` but Task 4 does not wait on Task 5's guard.

Red tests: `TestMaterializeSpecForCommand_v10_composes_modules`, `TestAdopt_v10_adopts_into_root_target`, `TestScan_v10_skips_module_owned_packages`, `TestRewritePackagesInPlace_preserves_modules_and_useModules`, `TestComplete_repo_packages_includes_module_packages`, `TestLockGate_selection_change_warns_without_refusing`. First run: v10 spec is rejected as unknown version → implement → pass.

Verify: `go test ./... -run 'Materialize|Adopt|Scan|Rewrite|Complete'` then `go test ./...`.

### Task 5 — ownership guards for mutation commands (parallel-safe with 4)

Files: `main_compose.go`, `main.go` (call sites), `main_compose_test.go`, `internal/commands/helpers.go`.

Contracts: `func refuseModuleOwned(c *compose.Composition, kind, key string) int` returns `exitLogic` with the owning document path when a resource is module-owned; root-owned resources pass. Apply to: `remove`, `disown`, `adopt --files`, `env set/unset`, `shell alias set/unset`, `service add/remove`. `remove`/`disown` must additionally refuse when the package is still owned by another module even if the root entry was dropped. `genv add` on a module-owned package fails with a pointer to edit the owning module (no auto multi-file writes in M1).

Red tests: `TestRemove_refuses_module_owned_package_before_subprocess`, `TestDisown_refuses_module_owned_package`, `TestServiceAdd_refuses_module_owned_service`, `TestEnvSet_refuses_module_owned_variable`, `TestRemove_root_owned_succeeds`. First run: guard missing → implement → pass.

Verify: `go test ./ -run 'refuses_module|root_owned'`.

### Task 6 — export, pull, and portability of modules (parallel-safe with 4)

Files: `internal/export/export.go`, `internal/export/report.go`, `internal/pull/assets.go`, `main_pull.go`, `main_export.go`, tests in `internal/export/`, `main_export_test.go`, `main_pull_test.go`.

Contracts: `export.BuildWithOptions` gains `Modules map[string]string` so `genv export --target` writes a **flattened single-target snapshot** (no `modules`/`useModules` keys; same source `schemaVersion`) plus a report section listing selected modules and their provenance. `pull.BundleAssetSources` includes registered module documents and their portable relative assets, honoring the existing symlink/secret exclusions. `main_pull.go` composes the staged clone in a temp dir and fails before publishing when module documents are missing, invalid, or cyclic.

Red tests: `TestExport_v10_snapshot_is_flattened_and_valid`, `TestExport_report_lists_module_provenance`, `TestPull_v10_bundles_module_documents`, `TestPull_v10_refuses_missing_module_document`, `TestPull_v10_refuses_symlinked_module_asset`. First run: compile/missing symbol → implement → pass.

Verify: `go test ./internal/export/ ./internal/pull/ && go test ./ -run 'Export|Pull'`.

### Task 7 — `genv config` and `genv explain` commands (serial after 4, 5)

Files: `main_compose.go`, `main.go` (`run()` dispatch, `printUsage`), `main_help.go`, `internal/output/json.go`, `completions/genv.{bash,zsh,fish,ps1}`, `man/genv.1`, tests `main_compose_test.go`, `internal/output/json_test.go`.

Contracts:

- `genv config [--target <id>] [--json]` — effective environment summary: resolved target, selected modules, per-resource counts, and fingerprint. Existing `commands.RedactValue` for sensitive env. No probes.
- `genv explain <resource> [--target <id>] [--json]` — one `Kind:Key` selector (for files everything after the first colon is the key); prints owners with document/field/module/target and the selection reason. Never prints env values, hook command bodies, or raw commands.
- `output.Evelope` gains optional `ConfigResult` / `ExplainResult` data types only; `SchemaVersion` stays `"1"` (additive fields).

Red tests: `TestConfigCmd_v10_lists_modules_redacted`, `TestExplainCmd_reports_owners`, `TestExplainCmd_file_selector_key_preserved`, `TestExplainCmd_unknown_resource_exits_nonzero`, `TestCompletionScripts_mention_config_and_explain`. First run: `unknown command` → implement → pass.

Verify: `go test ./ -run 'Config|Explain|Completion' && go test ./internal/output/`.

### Task 8 — M1 integration, docs, and gates (serial after 7)

Files: `scripts/docker-v8-command-matrix.sh` (add a v10 two-module fixture step), `SCHEMA.md` (v10 section), `README.md` (composition + `config`/`explain` rows), `docs/multi-machine.md`, `CHANGELOG.md`, `e2e/e2e_test.go` (compose fixture), `internal/schema/version_test.go` (v10 in `AtLeastVersion`).

Contracts: matrix exercises `validate`, `config`, `explain`, `apply --dry-run`, `status`, `upgrade --dry-run`, `export`, `pull` against a v10 spec with two modules and one shared, coalesced resource. Docs state: modules are local and trusted; conflicts are errors; no override syntax; `useModules` is additive.

Verify: `make ci` (vet, gofmt, race, cover-gate, bench-gate) → `make lint` → `make integration-v8` → `go test ./e2e/ -run Compose` (if e2e needs Docker, note it and rely on the matrix).

### Task 9 — shared execution result model (serial, start of M2)

Files: `internal/apply/apply.go` (new), `internal/apply/apply_test.go`, `main.go` (`runApplyText`, `runApplyJSON`), `internal/output/json.go`.

Contracts: `type StepResult struct { Kind string; Key string; Action string; Status string; Reason string; DurationMs int64 }`; statuses `applied`, `skipped`, `blocked`, `failed`, `unknown`; `type Result struct { Steps []StepResult; Installed, Removed, Failed []string; ExitCode int }`. The engine receives callbacks for packages/files/env/shell/services/hooks so both text and JSON paths share ordering, blocking, and success determination; presentation stays in main. Existing exit-code semantics preserved: 0 ok, 4 semantic, and `apply --json` stays plan-only without `--yes`.

Red tests (in `internal/apply`): `TestEngine_blocks_dependents_on_failure`, `TestEngine_reports_unknown_without_probe`, `TestEngine_preserves_exit_codes`, plus existing `main_apply_files_test.go`, `main_apply_portability_test.go`, and `main_verify_test.go` fixtures as the regression net that the refactor must not change. First run: engine tests fail to compile because `internal/apply` does not exist → implement engine → wire both `runApplyText` and `runApplyJSON` to it → all green with byte-identical v8 envelopes.

Verify: `go test ./internal/apply/ && go test ./... && make bench-gate`.

### Task 10 — service `requires` / `watch` / `restartPolicy` / `healthCheck` + graph (parallel-safe with 9)

Files: `internal/schema/servicechange.go`, `internal/schema/schema.go`, `internal/schema/unknown.go`, `internal/plan/plan.go`, `internal/plan/plan_test.go`, `internal/verify/health.go`, tests.

Contracts:

- `Service.Requires []string`, `Service.Watch []string`, `Service.RestartPolicy string` (`never` default, `ifRunning`), `Service.HealthCheck *HealthCheck{Command []string; Timeout string; Interval string; AllowBackground bool}` — all v10-only.
- `internal/plan.Build(graph) (Plan, error)` — nodes are composed identities; stable Kahn order with reason strings; cycle and unknown-reference errors.
- `internal/verify.Health(ctx, spec, probe func() bool) error` — retries `probe` until `Timeout` (default 30s, positive), interval default 1s; used only after authorized start/restart.

Red tests: `TestPlan_orders_dependencies_before_dependents`, `TestPlan_detects_cycle_with_path`, `TestValidateService_v10_fields_require_v10`, `TestHealth_retries_until_ready`, `TestHealth_times_out`. First run: compile failure → implement → pass.

Verify: `go test ./internal/plan/ ./internal/schema/ ./internal/verify/`.

### Task 11 — change evidence and restart coordinator (serial after 9, parallel with 10)

Files: `internal/service/changes.go`, `internal/genvfile/lockfile.go` (`LockedPendingAction`), tests in `internal/service/`.

Contracts:

- `func ChangedEvidence(before, after schema.Package, queryVersion func(string) (string, error)) Evidence` — `changed`, `unchanged`, or `unknown`; a successful no-op command with equal versions is `unchanged`; an unavailable version query is `unknown`.
- `type PendingAction struct { Name string; Package string; Reason string; RecordedAt string }` persisted in the lock under the existing mutation lock **before** changing a watched package; cleared only after the action and readiness succeed.
- `func PlanServiceRestarts(c *Composition, plan upgrade.UpgradePlan, evidenceFor func(string) Evidence) []RestartDecision` — one decision per service per run, coalescing multiple triggers; `ifRunning` never starts a stopped service.

Red tests: `TestChangedEvidence_noop_command_with_same_version_is_unchanged`, `TestChangedEvidence_missing_version_is_unknown`, `TestPlanServiceRestarts_coalesces_triggers`, `TestPlanServiceRestarts_ifRunning_skips_stopped_service`, `TestPendingAction_persisted_before_package_change`. First run: compile failure → implement → pass.

Verify: `go test ./internal/service/ ./internal/genvfile/`.

### Task 12 — wire foreground upgrade and background worker (serial after 11)

Files: `main.go` (`upgradeCmd`), `main_updates_worker.go`, `internal/apply/apply.go`, `internal/schema/validate.go` (unknown selector validation), tests `main_upgrade_runner_test.go`, `main_updates_unattended_test.go`, `main_updates_worker_timeout_test.go`.

Contracts: both paths use the same coordinator from Task 11. Foreground `upgrade` restarts and verifies; the background worker **defers** upgrades whose watched service has a health check without `allowBackground: true` and **skips incompatible actions before upgrading their watched packages**. OS/firmware steps stay interactive-only. An uncertain (interrupted) pending action is reported, not silently replayed unattended.

Red tests: `TestUpgrade_restarts_watched_service_after_proven_change`, `TestUpgrade_defers_restart_when_evidence_unknown`, `TestUpdatesWorker_defers_health_check_without_allow_background`, `TestUpdatesWorker_reports_uncertain_pending_action`. First run: no restart behavior → implement → pass.

Verify: `go test ./ -run 'Upgrade_restarts|Upgrade_defers|UpdatesWorker' && go test ./...`.

### Task 13 — M2 integration, docs, and gates (serial after 12)

Files: `SCHEMA.md` (v10 service fields), `README.md` (updates checker + service restart section), `man/genv.1`, `CHANGELOG.md`, `internal/schema/unknown.go` (serviceFields update), `scripts/docker-v8-command-matrix.sh`.

Contracts: docs state exactly: a successful command is not proof of a change; `unknown` defers; `ifRunning` preserves stopped state; readiness checks run only after authorized restart and never in dry-run/status; unattended deferral is logged with a reason.

Verify: `make ci` → `make lint` → `make integration-v8`.

## Parallelization map

- Serial: 1 → 2 → 3 (each depends on the previous).
- Parallel-safe after 3: 4 (main integration), 5 (guards), 6 (export/pull) — disjoint files.
- Serial after 4 and 5: 7 (commands, completions, docs).
- Serial: 8 (integration/docs/gates).
- M2: 9 serial; 10 ∥ 11 after 9; 12 after 11; 13 after 12.

Kit dispatch suggestion for the implement phase: one `implement` subagent per parallel-safe slice (4, 5, 6), each in its own worktree under `~/Documents/Worktrees/genv/`. Pi has no subagent API, so Pi runs these inline, serially.

## Acceptance coverage

| Design-doc acceptance item | Task |
|---|---|
| Unchanged v9 fixture behaves as before | 4 (v1–v9 path untouched), 8 (`make ci` + v8/v9 regression fixtures) |
| v10 root composes two local modules with explainable ownership | 3, 4, 7 |
| Deterministic conflicts instead of silent override | 3 (`ErrConflict`), 5 (guards) |
| Every command sees the same desired set | 4, 6 |
| Deselecting one shared owner keeps the resource | 3 (coalesced owners), 5 (`remove`/`disown` refuse while another owner remains) |
| No dry-run executes new custom commands | 7 (`config`/`explain` never probe), 10 (`healthCheck` gated on authorized restart) |
| Watched change restarts a running service once, after prerequisites, with readiness | 9, 10, 11, 12 |
| No real host state needed for tests | 2–7 (temp dirs, `internal/testutil`), 9–12 (fake executors) |

## Final verification (after Task 13)

```bash
make ci            # vet + gofmt + race tests + cover-gate (80) + bench-gate
make lint          # golangci-lint
make integration-v8 # real Arch container command matrix incl. v10 fixture
go test ./e2e/     # e2e compose fixture (Docker; note if skipped)
```

Review gate (before any push, only when requested):

1. Intent vs plan: every acceptance item in the design doc is either implemented or explicitly deferred with a reason.
2. `verify` (green checks above), `review` (intent, regressions, no secrets), `security-scan` (path traversal, symlink, subprocess argv, redaction).
3. Record with `~/.config/genv/git/review-record.sh` (no arguments) before any push. Never hand-write `.git/review-passed`.
4. Pre-push hook re-scans the pushed range with `gitleaks`.

Publication gates (not authorized by this plan):

- Push/PR/release only on explicit request; PRs via `gh stack` for dependent slices.
- Issue #218 stays open until Task 12 ships and CI is green on the release branch.
- Schema v10 release notes: v10 is additive; v1–v9 specs keep working without `genv migrate`.

## Risks and mitigations

| Risk | Mitigation |
|---|---|
| Composition regressions in v8/v9 | Task 4 keeps the existing code path for v1–v9; `main_v8_status_upgrade_test.go` and `main_v9_portability_test.go` remain the regression gate |
| Conflict false positives from normalization | Task 3 normalizes file destinations target-aware and coalesces identical declarations; Windows CI job validates cross-OS |
| Symlink/traversal in module paths | Task 2 lstat + component-wise symlink rejection + root containment; Task 6 reuses `pull` exclusions |
| Coverage drop below 80 | Every new package ships table-driven tests in the same task; `cover-gate` fails the build |
| Cold-start regression from composition | `bench-gate` (200ms) after Task 4 and Task 9; module loading is I/O and stays off `BenchmarkDetect` path |
| Main.go growth | New logic lives in `internal/compose`, `internal/plan`, `internal/apply`, `main_compose.go`; main.go only wires |
| Scope creep into M3–M5 | Plan stops at M2; follow-ups are separate specs/plans per the lifecycle rule |