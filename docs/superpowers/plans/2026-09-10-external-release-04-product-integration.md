# External Release Management: Product Integration Plan

## Goal

Complete status, update discovery, upgrade filtering, scheduled execution,
human/JSON output, export behavior, user documentation, and cross-platform
integration coverage for managed external releases.

## Architecture, Tools, And Constraints

- Depends on plans 01 through 03.
- `status` remains local; remote discovery belongs to `updates check` and
  `upgrade` planning.
- Provider failures are visible and never silently interpreted as current.
- Existing package/manager filters and version constraints apply.
- v9 export preserves local recipe declarations and bundles local public-key
  files through the existing asset exporter.
- No recipe registry, HTML scraper, or large flag-based recipe builder is added.

## File Map

- `internal/resolver/outdated.go`, `internal/resolver/resolver.go`: per-package
  external update discovery and execution planning.
- `internal/output/json.go`: release, verification, scope, policy, and drift
  fields for machine-readable output.
- `main.go`: human rendering plus status, apply, remove, upgrade, dry-run, and
  help integration.
- `main_updates.go`, `main_updates_worker.go`: check and scheduler policy.
- `internal/export/export.go`, `internal/pull/assets.go`: v9 preservation and
  local verifier-key assets.
- `main_*_test.go`, `internal/output/*_test.go`, `internal/export/*_test.go`:
  command and output contracts.
- `e2e/e2e_test.go`: local-fixture lifecycle test.
- `.github/workflows/integration.yml`: Linux/Windows non-mutating integration jobs.
- `README.md`, `SCHEMA.md`, `docs/linux-install.md`, `docs/windows-install.md`,
  `docs/macos-install.md`, `docs/wsl2-install.md`: user guidance and examples.
- `CHANGELOG.md`, completions under `completions/`: release and CLI surfaces.

## Task 1: Local Status And Drift

1. Add tests for absent binaries, unparsable versions, lock-version drift,
   recipe-hash drift, modified owned files, adopted non-owned files, and provider
   non-use during status.
2. Run `go test ./internal/resolver . -run ExternalStatus -count=1` and confirm
   failure.
3. Add local detection and receipt drift to existing status structures and human/
   JSON renderers without making network calls.
4. Run the targeted tests again.

## Task 2: Update Discovery And Filters

1. Add tests for GitHub and HTTP latest selection, equal/different versions,
   exact and wildcard constraints, prerelease channels, provider failure,
   `--only`, `--skip`, `--only-manager external`, and mixed-manager plans.
2. Run `go test ./internal/resolver . -run 'ExternalUpdate|ExternalUpgrade' -count=1`
   and confirm failure.
3. Extend the shared tracked-package planner with a per-package external path;
   do not force recipe-backed updates through the manager-wide `OutdatedLister`
   contract.
4. Run the targeted tests and existing outdated-planner tests.

## Task 3: Scheduled Update Enforcement

1. Add worker tests for verified direct assets, opted-in verified scripts,
   non-opted-in scripts, unverified recipes, insecure transport, stale recipe
   hashes, and changed signer policy between planning and execution.
2. Run `go test . -run 'Updates.*External|External.*Scheduled' -count=1` and
   confirm failure.
3. Re-resolve and re-check policy immediately before execution. Notifications
   count available but blocked updates without claiming they were applied.
4. Run the targeted tests and all existing updates tests.

## Task 4: Dry-Run, Output, And Export

1. Add golden tests proving dry-run resolves metadata and selection without
   downloading artifacts, and output redacts credentials while showing version,
   asset, verifier, install type, scope, and script background policy.
2. Add v9 export tests for preserving recipes, bundling local minisign/OpenPGP key
   files, rewriting relative paths, and omitting lock receipts and secrets.
3. Run `go test ./internal/output ./internal/export . -run External -count=1` and
   confirm failure.
4. Implement output and export behavior, then rerun the targeted tests.

## Task 5: Documentation And Completion

1. Add `docs/linux-install.md` with GitHub archive installation for genv itself
   and clarify that the discontinued Snap is not supported.
2. Document complete GitHub asset, structured HTTP asset, shell script, and
   PowerShell script recipes, including every verification method and scheduler
   restriction.
3. Update README installation links, SCHEMA v9 reference, command help, and shell
   completions only for actual CLI changes.
4. Record the feature in `CHANGELOG.md` without rewriting historical releases.
5. Verify references with focused `rg` searches and `git diff --check`.

## Task 6: End-To-End Coverage

1. Add a local test server fixture that serves release JSON, checksums,
   signatures, direct binaries, archives, and harmless scripts without external
   network access.
2. Exercise apply, status, updates check, upgrade, remove, failed verification,
   rollback, and scheduler denial on Linux.
3. Exercise path, ZIP, direct executable, and PowerShell policy behavior on
   Windows. Keep host-mutating macOS actions to unit tests and a documented manual
   smoke test.
4. Run the targeted workflow locally where possible and verify its YAML syntax.

## Full Verification, Review, And Publication Gate

Run:

```bash
make ci
make integration-v8
goreleaser check
git diff --check
```

Also run the new local-fixture integration tests on Ubuntu and native Windows,
the repository security scan, dependency vulnerability/license checks, and a
manual review of every download/execution boundary. Confirm the legacy
presence-only external tests still pass. Do not publish until documentation,
JSON Schema, completion scripts, and both scheduled-policy denial paths match the
implemented behavior.
