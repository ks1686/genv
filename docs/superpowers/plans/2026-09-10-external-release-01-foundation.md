# External Release Management: Foundation Plan

## Goal

Add schema version 9, portable-version handling, external recipe validation,
machine-local receipt types, release-provider resolution, and deterministic
platform/asset selection. This slice discovers and plans releases but does not
download or install them.

## Architecture, Tools, And Constraints

- Follow the approved design in
  `docs/superpowers/specs/2026-09-10-external-release-management-design.md`.
- Preserve schema versions 1 through 8 and legacy presence-only `external`.
- Version 9 inherits the version 8 `defaults`/`targets` model.
- Use `net/http`, `encoding/json`, `regexp`, and `httptest`; add no dependency
  unless the standard library cannot meet an approved requirement.
- All source responses, regexes, redirects, and timeouts are bounded.
- This plan depends only on the Snap-removal change and is the base for plans 02,
  03, and 04.

## File Map

- `internal/schema/schema.go`: v9 constants and external recipe structs.
- `internal/schema/validate.go`, `internal/schema/unknown.go`: union and security
  validation.
- `internal/schema/merge.go`: deep-copy recipe fields during target materialization.
- `internal/schema/*_test.go`: compatibility, validation, and copy-isolation tests.
- `schema/v9/genv.json`: public JSON Schema mirror.
- `internal/genvfile/lockfile.go`: optional external receipt types.
- `internal/genvfile/lockfile_test.go`: old/new lock compatibility.
- `internal/external/provider.go`: provider contract and resolved release model.
- `internal/external/github.go`: GitHub release provider.
- `internal/external/http.go`: structured HTTP provider.
- `internal/external/select.go`: platform and unique asset selection.
- `internal/external/*_test.go`: local HTTP fixtures and platform matrix.
- `main.go`, `main_updates.go`, `main_updates_worker.go`,
  `main_updates_daemon.go`, `main_map.go`, `main_export.go`: replace v8-only
  portable checks with the shared helper where behavior applies to v9.
- `SCHEMA.md`: v9 contract and legacy external compatibility.

## Task 1: Schema V9 Red-Green Cycle

1. Add table tests that expect v9 to parse the complete GitHub and HTTP examples,
   reject external recipes on v1-v8, reject recipes without `prefer: "external"`,
   and reject malformed union combinations, selectors, regexes, verification
   policy, install actions, and unknown fields.
2. Run `go test ./internal/schema -run 'External|Version9|Portable' -count=1` and
   confirm the new tests fail because v9 and recipe fields do not exist.
3. Add the v9 data model, `schema.IsPortableVersion`, validation, unknown-field
   walking, deep copies, and `schema/v9/genv.json`.
4. Run the targeted command again and then `go test ./internal/schema -count=1`.

## Task 2: Portable-Version Migration

1. Add regression tests proving v9 materializes targets for apply, status,
   upgrade, updates, env, shell, service, map, and export command paths while v8
   behavior is unchanged.
2. Run `go test . -run 'Version9|V9|Portable' -count=1` and confirm the v9 cases
   fail at existing `schema.Version8` equality checks.
3. Replace behavior-level equality checks with `schema.IsPortableVersion`.
   Preserve commands whose contract explicitly produces v8, especially legacy
   migration when no external recipe exists.
4. Run `go test . -run 'Version9|V9|Portable|V8' -count=1`.

## Task 3: Lock Receipt Types

1. Add lock round-trip tests for source identity, release ID/tag, artifact digest,
   verifier summary, recipe digest, install type, ownership, and path receipts.
2. Add compatibility tests proving old locks without receipts still parse.
3. Run `go test ./internal/genvfile -run External -count=1` and confirm failure.
4. Add optional receipt structs without changing existing required lock fields.
5. Run `go test ./internal/genvfile -count=1`.

## Task 4: Provider Resolution

1. Add `httptest` cases for stable/prerelease/any GitHub selection, tag
   normalization, GitHub Enterprise base URL, optional token headers, API errors,
   malformed responses, response limits, and cancellation.
2. Add structured HTTP cases for JSON Pointer and text regex extraction, content
   type rejection, HTTPS policy, redirects, templates, and malformed versions.
3. Run `go test ./internal/external -run 'Provider|GitHub|HTTP' -count=1` and
   confirm failure before implementation.
4. Implement provider resolution behind injected HTTP clients and limits. Never
   persist or return authorization headers in errors or result objects.
5. Run `go test ./internal/external -run 'Provider|GitHub|HTTP' -count=1`.

## Task 5: Platform And Asset Selection

1. Add a matrix for Linux/macOS/Windows, amd64/arm64, glibc/musl, WSL-to-Linux,
   no match, ambiguous platform entries, no asset, and ambiguous assets.
2. Run `go test ./internal/external -run 'Select|Platform|Asset' -count=1` and
   confirm failure.
3. Implement deterministic selection with injected host facts and RE2 matching.
4. Run `go test ./internal/external -count=1`.

## Verification And Review Gate

Run:

```bash
go test ./internal/schema ./internal/genvfile ./internal/external .
go test ./...
go vet ./...
goreleaser check
git diff --check
```

Review all v8 equality checks found by
`rg 'SchemaVersion\s*[!=]=\s*schema\.Version8' --glob '*.go'` and justify any
remaining occurrence. Run the repository security scan before publishing. Do not
start plan 02 until this slice is reviewed and its full suite is green.
