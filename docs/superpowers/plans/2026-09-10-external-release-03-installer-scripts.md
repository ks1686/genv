# External Release Management: Installer Scripts Plan

## Goal

Add verified downloaded installer scripts, explicit uninstall actions,
cross-platform interpreter selection, elevation policy, and recipe-controlled
background execution.

## Architecture, Tools, And Constraints

- Depends on plans 01 and 02.
- Scripts are downloaded files, never shell pipelines or remote recipe content.
- Supported interpreters are `sh`, `bash`, `pwsh`, and `powershell`.
- Installer argv and environment are explicit and template substitution is
  allowlisted.
- Verification authenticates the script but cannot guarantee payloads fetched by
  that script; unattended execution therefore needs an additional recipe opt-in.
- `allowUnverified` is always interactive and never scheduler-eligible.
- A script without an uninstall action cannot be reported as removed.

## File Map

- `internal/schema/schema.go`, `internal/schema/validate.go`,
  `schema/v9/genv.json`: finalized script, uninstall, scope, environment, and
  background-policy fields.
- `internal/external/script.go`: interpreter resolution and execution.
- `internal/external/policy.go`: interactive/unattended decision matrix.
- `internal/external/template.go`: allowlisted release/platform substitutions.
- `internal/external/elevate.go`: explicit system-scope elevation planning.
- `internal/external/engine.go`: script install/upgrade/uninstall transaction.
- `internal/profilebackend/powershell.go`: reuse PowerShell discovery behavior
  without requiring PowerShell on non-Windows hosts.
- `internal/resolver/resolver.go`: pass execution mode, confirmation, stdin, and
  timeout context into external policy.
- `main.go`, `main_updates_worker.go`: distinguish interactive, `--yes`, and
  scheduler execution modes.
- Tests beside each changed package.

## Task 1: Script Schema And Templates

1. Add validation tests for each interpreter, argv/env templates, unknown
   placeholders, forbidden shell strings, conflicting install fields, uninstall
   argv, system scope, and background policy.
2. Run `go test ./internal/schema ./internal/external -run 'Script|Template' -count=1`
   and confirm failure.
3. Implement strict allowlisted substitution for `version`, `tag`, `os`, `arch`,
   staged script path, and declared destination values.
4. Run the targeted tests again.

## Task 2: Policy Matrix

1. Add a table covering verified/unverified, asset/script, interactive/`--yes`/
   scheduler, HTTP/HTTPS, and `allowBackgroundExecution` combinations.
2. Assert that unverified and insecure-transport recipes never run unattended;
   scripts run unattended only when verified and explicitly opted in.
3. Run `go test ./internal/external -run Policy -count=1` and confirm failure.
4. Implement one centralized policy decision function used by plan, apply,
   upgrade, and the updates worker.
5. Run `go test ./internal/external -run Policy -count=1`.

## Task 3: Cross-Platform Script Execution

1. Add fake-interpreter tests for POSIX and Windows argv, private script modes,
   environment isolation, inherited stdin, timeout/cancellation, exit status,
   redaction, and cleanup.
2. Add tests proving PowerShell is gated by availability and prefers `pwsh` over
   Windows PowerShell consistently with the profile backend.
3. Run `go test ./internal/external -run 'Script|PowerShell' -count=1` and confirm
   failure.
4. Implement execution without invoking an extra shell layer.
5. Run `go test ./internal/external -run 'Script|PowerShell' -count=1`.

## Task 4: Scope And Elevation

1. Add tests proving user scope cannot target outside the home directory, system
   scope is visible in plans, elevation is explicit, and permission failures do
   not silently retry as root.
2. Run `go test ./internal/external -run 'Scope|Elevat' -count=1` and confirm
   failure.
3. Reuse existing command execution and confirmation semantics for `sudo` and
   elevated PowerShell where supported; fail clearly where no backend exists.
4. Run the targeted tests again.

## Task 5: Script Install, Upgrade, And Uninstall

1. Add resolver tests for verified interactive install, opted-in scheduler
   upgrade, scheduler refusal, unverified confirmation despite `--yes`, explicit
   uninstall success/failure, absent uninstall refusal, post-detection failure,
   and unchanged spec/lock on failure.
2. Run `go test ./internal/resolver . -run ExternalScript -count=1` and confirm
   failure.
3. Integrate script actions and store non-owned script receipts with verification
   and recipe identity. Never infer files created by the script.
4. Run `go test ./internal/resolver . -run ExternalScript -count=1`.

## Verification And Review Gate

Run:

```bash
go test ./internal/external ./internal/resolver .
go test -race ./internal/external ./internal/resolver
go test ./...
make bench
go vet ./...
git diff --check
```

Run the repository security scan. Smoke-test a harmless local shell installer in
Ubuntu and a harmless local PowerShell installer in native Windows. Do not use a
real vendor script for CI. Do not start plan 04 until every unattended-policy
denial test passes.
