# genv.json schema

This document describes the `genv.json` spec schema versions. The canonical
structs live in `internal/schema/schema.go`; validation is performed by
`internal/schema/validate.go`.

## Supported versions

| Version | `schemaVersion` value | What's added |
| ------- | --------------------- | ------------ |
| v1      | `"1"`                 | `packages` array only |
| v2      | `"2"`                 | `env` block |
| v3      | `"3"`                 | `shell` block |
| v4      | `"4"`                 | `services` block |
| v5      | `"5"`                 | `files`, `hooks`, per-record `host`, `repo` |

Older versions are loaded and validated without error.

## Common field rules

- All JSON field names are `lowerCamelCase`.
- Optional objects/arrays are omitted when empty (`omitempty`).
- `host` is a per-record selector. It can be a single string (`"macos"`) or an
  array of strings (`["arch", "wsl2"]`). An empty/absent selector matches every
  host.
- Path fields support leading `~` expansion to the user's home directory and
  `$VAR`/`${VAR}` expansion from the process environment.

## v4 fields

A v4 file contains the v1-v3 fields plus:

- `services` — map of service name to service declaration
  - `start`, `stop`, `restart`, `status`: command argv arrays
  - `brew_formula`: Homebrew formula name for `brew services` (mutually
    exclusive with `start`)

## v5 fields

A v5 file adds the following top-level blocks to the v4 fields.

### `files`

Reconciles filesystem entries on the host.

- `links` — array of symlink declarations
  - `source` (required) — path relative to the spec repo root
  - `target` (required) — absolute path after expansion
  - `mode` — `"link"` (default) or `"managed-link"`
  - `host` — optional host selector
  - `backup` — whether a forced overwrite keeps a `.backup.<timestamp>` copy
- `templates` — array of files to copy after placeholder rendering
  - `source`, `target`, `host`, `backup`
- `dirs` — array of directories to ensure exist
  - `target`, `host`

### `hooks`

Lifecycle shell commands. Each hook is an object with:

- `command` (required) — literal shell command string executed via `sh -c`
- `host` — optional host selector

Phases:

- `preUpgrade` — run before package upgrades
- `postApply` — run after a successful apply
- `postUpgrade` — run after package upgrades

### `repo`

Source repository used by `genv pull`:

- `url` (required) — git URL or local path
- `ref` (optional) — branch, tag, or ref to checkout

## Lock file

`genv.lock.json` records the applied state. It lives at
`~/.config/genv/genv.lock.json` by default (or `$XDG_CONFIG_HOME/genv/genv.lock.json`),
regardless of where `genv.json` itself is located. As of v5 the lock file may
also contain a `files` array mirroring the applied file entries.
