# Remove Snap Distribution

## Goal

Stop publishing the nonfunctional strictly confined Snap package. Keep Snap as a
package-manager adapter; only the installation/distribution channel is removed.

## Files And Responsibilities

- `.goreleaser.yml`: remove Snap package generation and publication.
- `.github/workflows/release.yml`: remove Snapcraft setup and Store credentials.
- `RELEASING.md`: remove claims and checklist steps for Snap publication.
- `ROADMAP.md`: mark the historical Snap distribution outcome as withdrawn.
- `CHANGELOG.md`: record the channel removal and its confinement rationale.
- Release-config test: prevent Snap publication settings from returning unnoticed.

## Tasks

1. Add a failing repository check that rejects `snapcrafts:` in `.goreleaser.yml`
   and Snap publishing steps or credentials in `.github/workflows/release.yml`.
2. Remove Snap packaging, setup, and credentials from the release pipeline; verify
   with the targeted check and `goreleaser check`.
3. Update release documentation and changelog while preserving documentation of
   genv's `snap` package-manager adapter.
4. Run `go test ./...`, the release-config check, and `goreleaser check`; inspect
   the final diff and leave unrelated worktree content untouched.

## Constraints And Publication Gate

- Do not remove or alter `internal/adapter/snap.go` or Snap-managed package support.
- Store-side closure of the existing listing is a separate publisher action.
- Do not tag a release until the workflow no longer expects Snap tooling or
  `SNAPCRAFT_STORE_CREDENTIALS`.
