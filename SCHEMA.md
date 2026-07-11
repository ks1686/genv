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
| v6      | `"6"`                 | expanded lifecycle `hooks`, `updates` block |

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

Lifecycle shell commands. Each hook is an object with exactly one executable
source:

- `command` — literal shell command string executed by the hook runner
- `file` — script path to execute; supports the common path expansion rules
- `host` — optional host selector

Phases:

- `preUpgrade` — run before package upgrades
- `postApply` — run after a successful apply
- `postUpgrade` — run after package upgrades

## v6 fields

A v6 file adds the following behavior to v5.

### Expanded `hooks`

Schema v6 keeps the v5 hook arrays and adds lifecycle phases for package and
apply operations:

- `preApply`, `postApply`
- `preAdd`, `postAdd`
- `preRemove`, `postRemove`
- `preUpgrade`, `postUpgrade`

Hooks may use either `command` or `file`, but not both. Hook execution provides
deterministic context environment variables including `GENV_EVENT`,
`GENV_PHASE`, `GENV_HOST`, `GENV_PROFILE`, `GENV_INSTALLED`, `GENV_REMOVED`,
`GENV_UPGRADED`, `GENV_FAILED`, and `GENV_SKIPPED`.

Hooks execute as the current user and are arbitrary code by design. Treat hook
entries as trusted configuration, not as untrusted data.

### `updates`

Configures the managed background updates checker:

- `enabled` — `true` allows `genv updates start` to register the checker
- `interval` — positive Go duration such as `"24h"`
- `autoApply` — when true, the checker applies tracked upgrades instead of only
  logging/notifying
- `notify` — request best-effort desktop notifications
- `onlyManagers`, `skipManagers`, `only`, `skip` — optional tracked-only filters
  matching the `genv upgrade` / `genv updates check` filter flags

The updates checker only plans genv-tracked packages. It does not perform a
topgrade-style system-wide update-all pass.

## Manager resolution and adapter scope

`prefer` and `managers` accept the registered manager IDs listed in the schema
registry, including system managers (`brew`, `pacman`, `winget`, `scoop`,
`choco`) and explicit language/tool/plugin managers (`npm`, `cargo`, `go`,
`krew`, `helm`, `vscode`, and the other ecosystem adapters).

When neither `prefer` nor `managers` selects a manager, resolver fallback is
limited to system package managers. Language, toolchain, and plugin managers are
explicit-only so a generic package ID cannot accidentally resolve through an
ecosystem tool merely because that tool is installed.

### `repo`

Source repository used by `genv pull`:

- `url` (required) — git URL or local path
- `ref` (optional) — branch, tag, or ref to checkout

## Lock file

`genv.lock.json` records the applied state. It lives at
`~/.config/genv/genv.lock.json` by default (or `$XDG_CONFIG_HOME/genv/genv.lock.json`),
regardless of where `genv.json` itself is located. As of v5 the lock file may
also contain a `files` array mirroring the applied file entries. As of v6 it can
also record `activeProfile`, used by `genv profile list` and `genv profile switch`.

Named profiles live in a `profiles/` directory next to the base `genv.json`.
`genv profile switch <name>` merges `profiles/<name>.json` over the base spec,
applies the result, and records the active profile in the lock file.
