# Task 3 Report: Classify target IDs + resolve

## Status

Completed.

## Changes

- Updated `host.Classify()` to ignore `GENV_HOST` and return portable target IDs:
  - `macos`
  - `windows`
  - `arch`
  - `ubuntu`
  - `wsl-arch`
- Added pure Linux classification helper coverage via `classifyLinux(osRelease, procVersion string)`.
- Removed WSL-to-Arch inheritance from `host.Match`.
- Added filter-level coverage that `wsl-arch` does not inherit bare `arch` records.
- Added `internal/target.Resolve(flag string)` with precedence:
  1. non-empty explicit flag
  2. non-empty `GENV_TARGET`
  3. `host.Classify()`
  4. guidance error on classifier failure

## TDD / Tests

1. Added failing tests first:
   - missing `classifyLinux`
   - missing `target.Resolve`
   - legacy WSL inheritance contract
2. Confirmed expected failure:
   - `go test ./internal/host ./internal/target -count=1` failed before implementation.
3. Implemented production code and reran:
   - `go test ./internal/host ./internal/target -count=1` passes.

## Additional Verification

- `go test ./... -count=1` fails in `TestProfileSwitch` because that test expects package installs without configuring an available package manager in this Ubuntu runner:
  - `main_profile_test.go:139: expected 2 packages in lock, got 0`
  - Reproduced with `go test . -run '^TestProfileSwitch$' -count=1 -v`.
  - This failure is outside the host/target package changes and matches the test's environment-sensitive setup.

## Concerns

- No apply wiring was added, per Task 3 scope.
- `Resolve` intentionally resolves source precedence only; target presence in a v8 spec remains a later apply/merge concern.
