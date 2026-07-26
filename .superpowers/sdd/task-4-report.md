# Task 4 Report: Lock metadata + foreign-lock gate

## Status

Implemented.

## Changes

- Added `target` and `goos` metadata fields to `genvfile.LockFile`.
- Added `internal/lockgate` with:
  - `Decision`
  - `Check(lf *genvfile.LockFile, activeTarget, goos string, available map[string]bool) Decision`
- `Check` treats a completely empty lock (`len(Packages)==0 && Target=="" && GOOS==""`) as local.
- `Check` flags foreign locks for target mismatch, GOOS mismatch, or any non-empty package manager that is unavailable/missing/false in the bool availability map.

## Tests

- Added failing tests first for lock metadata round-trip and lockgate decisions.
- Confirmed expected initial failure:
  - `go test ./internal/lockgate ./internal/genvfile -count=1`
- After implementation:
  - `go test ./internal/lockgate ./internal/genvfile -count=1` passed.
  - `make ci` failed in existing root-package `TestProfileSwitch`.
  - `go test ./... -count=1` failed in existing root-package `TestProfileSwitch`.

## Concerns

Broader root-package test failure appears unrelated to this task:

- `TestProfileSwitch` expects packages in the lock after profile switch but observes an empty package list.
- New task code is not wired into apply/profile paths, per task scope.
