# Task 2 Report: Merge defaults + target + tombstones

## Status

Completed.

## TDD

1. Added failing tests in `internal/schema/merge_test.go` for:
   - target package replacement
   - env tombstones
   - shell alias/function tombstones using `TargetShellConfig`
   - flat effective output (`Defaults`/`Targets` omitted)
   - missing target error
   - deep-copy/no input mutation
   - omitted array fields keeping defaults
2. Confirmed RED:
   - `go test ./internal/schema -count=1 -run TestMergeTarget`
   - failed on missing `TargetShellConfig` and `MergeTarget`
3. Implemented schema merge.
4. Confirmed GREEN:
   - `go test ./internal/schema -count=1 -run TestMergeTarget`
   - `go test ./internal/schema -count=1`

## Implementation Notes

- Added `TargetShellConfig` for v8 `defaults`/`targets` shell blocks with pointer-valued alias/function maps.
- Kept top-level `ShellConfig` value-typed for the flat effective document.
- Added `MergeTarget(f *GenvFile, targetID string) (*GenvFile, error)`.
- Merge behavior:
  - returns a new flat `GenvFile` with `SchemaVersion: Version8`
  - copies top-level `Repo` and `Updates`
  - clears `Defaults` and `Targets` on output
  - packages replace defaults when target packages are non-nil
  - env/services/shell maps overlay defaults and honor nil tombstones
  - files/hooks/source slices replace only when target slice is non-nil; otherwise defaults are kept
  - all returned data is deep-copied
- Updated v8 target bundle validation to validate `TargetShellConfig` and reject shell tombstones in `defaults`.

## Verification

- `go test ./internal/schema -count=1` passed.
- `git diff --check` passed.
- Additional exploratory `go test ./...` failed in existing root test `TestProfileSwitch` because host detection skipped host-specific records in this Linux environment; rerunning `go test . -count=1 -run '^TestProfileSwitch$'` reproduced the same unrelated failure.

## Concerns

- No concerns for Task 2 scope.

## Review Fix: Unknown Tombstones

- Added v8 validation errors for target env/service/shell alias/function tombstones that do not reference a non-null entry in the same `defaults` field.
- Kept `MergeTarget` behavior unchanged: known tombstones still delete during merge; validation is the gate for typos.
- RED evidence: `go test ./internal/schema -count=1` failed on `TestParseAndValidate_V8RejectsUnknownEnvTombstone` and `TestParseAndValidate_V8RejectsUnknownAliasTombstone`.
- GREEN evidence: `go test ./internal/schema -count=1` passed.
