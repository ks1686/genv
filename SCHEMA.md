# genv.json schema

Canonical structs: `internal/schema/schema.go`. Validation: `internal/schema/validate.go`. JSON Schema mirror for v8: `schema/v8/genv.json` (Go validator remains source of truth).

## Supported versions

| Version | `schemaVersion` | Adds |
| ------- | --------------- | ---- |
| v1 | `"1"` | `packages` |
| v2 | `"2"` | `env` |
| v3 | `"3"` | `shell` |
| v4 | `"4"` | `services` |
| v5 | `"5"` | `files`, `hooks`, per-record `host`, `repo` |
| v6 | `"6"` | expanded lifecycle hooks, `updates` |
| v7 | `"7"` | `"shell": "powershell"` targeting |
| v8 | `"8"` | portable `defaults` + `targets.*`; optional top-level `adapters` |

Older versions still load. Prefer **v8** for new multi-machine specs. Convert with `genv migrate`.

## Common rules

- Field names are `lowerCamelCase`.
- Empty optional objects/arrays are omitted when marshaling (`omitempty`).
- Paths support `~` and `$VAR` / `${VAR}` expansion.
- **v1–v7:** optional per-record `host` is a string or string array (`"macos"` or `["arch","macos"]`). Empty means “all hosts”. Legacy literal `"wsl2"` is obsolete for classification (see [WSL guide](docs/wsl2-install.md)); migrate to `ubuntu` / `wsl-arch` targets.
- **v8:** `host` is illegal. Use `targets.<id>` buckets.

## v8 — portable targets (recommended)

```json
{
  "schemaVersion": "8",
  "defaults": {
    "env": { "EDITOR": { "value": "nvim" } }
  },
  "targets": {
    "macos": {
      "packages": [{ "id": "ripgrep", "prefer": "brew" }]
    },
    "ubuntu": {
      "packages": [{ "id": "ripgrep", "prefer": "apt" }],
      "env": { "EDITOR": null }
    }
  },
  "repo": { "url": "https://github.com/example/dotfiles", "ref": "main" },
  "updates": { "enabled": true, "interval": "24h" }
}
```

### Known targets

| ID | Meaning |
| -- | ------- |
| `macos` | macOS |
| `windows` | native Windows |
| `arch` | native Arch / Arch-like |
| `ubuntu` | Ubuntu-like Linux **or** Ubuntu-like WSL2 |
| `wsl-arch` | Arch-like WSL2 |
| `linux` | optional catch-all (explicit `--target` / `GENV_TARGET`) |

### Rules

- Top-level `packages`, `env`, `shell`, `files`, `services`, and `hooks` are **invalid** in v8. Put them under `defaults` and/or `targets.<id>`.
- `targets` must be non-empty; keys must be known target IDs.
- `repo`, `updates`, and `adapters` remain top-level.
- Bundles support the same blocks as the flat schema: `packages`, `env`, `shell`, `files`, `services`, `hooks`.

### Merge and tombstones

Apply resolves one active target: `--target` → `GENV_TARGET` → host classification. Missing `targets.<active>` fails (no fallback).

Merge order: copy `defaults`, overlay `targets.<id>`. Arrays defined on the target replace defaults; omitted arrays keep defaults. Map keys in the target win. Set a map value to JSON `null` under a **target** (not under `defaults`) to tombstone an inherited env / alias / function / service key.

### Portability commands

| Command | Role |
| ------- | ---- |
| `genv migrate [--write]` | v1–v7 → v8 buckets |
| `genv map --target <id>` | assist-only mapping suggestions (never mutates) |
| `genv export --target <id> --out <dir>` | single-target snapshot + `report.json` / `report.md` + relative assets; omits locks and sensitive env |

### Locks

`~/.config/genv/genv.lock.json` is machine-local. v8 locks may record `target` and `goos`. A foreign lock is refused; use `genv apply --force-new-lock` to back it up and start fresh. Never commit locks.

Each applied `files.links[]` (`link` / `managed-link`) and `files.templates[]` entry may record `contentHash` (`sha256:<hex>` of the link source or rendered template). `genv status --files` reports `drifted` when the live hash differs. Older locks omit the field and stay topology-only. Apply refreshes the hash after a successful link/template op and never reverts the body.

Guide: [docs/multi-machine.md](docs/multi-machine.md).

## v7 — PowerShell

Aliases/functions may set `"shell": "powershell"`. Omitted `shell` stays POSIX-oriented. On native Windows, apply prefers `pwsh`, else Windows PowerShell, for `.ps1` fragments and hooks. See [docs/windows-install.md](docs/windows-install.md).

## v6 — updates and hooks

### `updates`

- `enabled`, `interval` (positive Go duration, e.g. `"24h"`)
- `autoApply` (default false — check/log/notify only)
- `notify`
- `onlyManagers`, `skipManagers`, `only`, `skip` — same filters as `genv upgrade`’s tracked-package step

Tracked packages only; the checker never plans or applies OS vendor or firmware updates. Use `genv upgrade` for those. Not a remote SSH updater.

`genv updates start` registers the checker with systemd --user (Linux),
launchd (macOS), or Task Scheduler / `schtasks` (Windows).

### Hooks

Phases: `preApply` / `postApply`, `preAdd` / `postAdd`, `preRemove` / `postRemove`, `preUpgrade` / `postUpgrade` (v5 also had `preUpgrade` / `postApply` / `postUpgrade`).

Each hook is `{ "command": "..." }` or `{ "file": "..." }` (exactly one), optional `name`, optional `continueOnError`, optional `host` on v1–v7.

Context env: `GENV_EVENT`, `GENV_PHASE`, `GENV_HOST`, `GENV_PROFILE`, `GENV_SPEC_FILE`, `GENV_SPEC_DIR`, `GENV_LOCK_FILE`, `GENV_YES`, `GENV_INSTALLED`, `GENV_REMOVED`, `GENV_UPGRADED`, `GENV_FAILED`, `GENV_SKIPPED`.

`continueOnError: true` reports a non-zero hook and continues the phase instead of failing the command. After a phase, genv prints a hook summary: `name` (or the first 40 characters of the command), exit code, and duration, in run order.

Hooks run as the current user and are arbitrary code by design — treat the spec as trusted.

## v5 — files, hooks, host, repo

### `files`

- `links[]` — `source`, `target`, `mode` (`link` default, `managed-link`, or `merge-dir`), optional `host`, `backup`, optional `perm`
  - `mode` is the link kind, not a Unix mode
  - `merge-dir` symlinks each file from source into target so multiple records can layer into one directory
  - `perm` is an octal string (`0644`, `0700`); apply chmods the source file (managed-link) or source directory (merge-dir)
- `templates[]` — copy after `__HOME__` / `__USER__` / `__HOST__` / `__OS__` / `__ARCH__` rendering; optional `perm` on the rendered file
- `dirs[]` — ensure directories exist; optional `perm` on the directory
- `perm` is 3 or 4 octal digits. Apply sets it after creating the entry; a second apply is a no-op when the mode already matches. `genv status` reports `perm-mismatch`. `mode` on `dirs[]` is rejected (unknown field).

### `repo`

- `url` (required), `ref` (optional) — used by `genv pull`

## v4 — services

Map of name → `{ start, stop, restart, status }` argv arrays and/or `brew_formula` (mutually exclusive with `start`).

## v3 / v2 / v1

- v3: `shell` with `aliases`, `functions`, `source`
- v2: `env` map of `{ value, sensitive? }`
- v1: `packages[]` with `id`, optional `version`, `prefer`, `managers`

## Manager resolution

`prefer` and `managers` accept registered manager IDs (see README table) or a v8 `adapters` name. Without an explicit selection, fallback uses **system** package managers only. Language, toolchain, and plugin managers are explicit-only.

`external` is a track-only manager for apps with an official installer (not winget/scoop). Apply records them when the binary is on PATH; it never installs them.

## Spec adapters (v8)

Top-level `adapters` defines installable command adapters for plugin CLIs that genv does not ship built-in. `external` stays track-only. A package with `prefer: <adapter-name>` then participates in apply, adopt, status, scan, updates, and upgrade.

```json
{
  "schemaVersion": "8",
  "adapters": {
    "claude-plugin": {
      "list": "claude plugin list --json",
      "install": "claude plugin install {{id}} --scope user",
      "remove": "claude plugin uninstall {{id}}",
      "upgrade": "claude plugin update {{id}}",
      "idField": "name",
      "versionField": "version"
    },
    "gh-extension": {
      "list": "gh extension list",
      "install": "gh extension install {{id}}",
      "remove": "gh extension remove {{id}}",
      "upgrade": "gh extension upgrade {{id}}",
      "listMatch": "(?m)^(?P<id>\\S+)\\s+(?P<version>\\S+)"
    }
  },
  "targets": {
    "macos": {
      "packages": [
        { "id": "slack@claude-plugins-official", "prefer": "claude-plugin" },
        { "id": "github/gh-copilot", "prefer": "gh-extension" }
      ]
    }
  }
}
```

### Adapter fields

| Field | Required | Role |
| ----- | -------- | ---- |
| `list` | yes | Inventory command. Used for Query, scan, status, and apply adopt-if-present. |
| `install` | yes | Argv template. `{{id}}` and `{{name}}` expand to the package id. |
| `remove` | yes | Uninstall template. |
| `upgrade` | no | Upgrade template. Omitted → same argv as `install`. |
| `version` | no | Per-package version command (`{{id}}`). Omitted → `versionField` from `list`. |
| `outdated` | no | Optional outdated inventory, parsed like `list`. Omit to keep the built-in “no detector → keep all” updates behavior. |
| `idField` | no | JSON object field (or dotted path, e.g. `plugins.name`) for the package id. |
| `versionField` | no | JSON field on the same object for the installed version. |
| `listMatch` | no | Regex. Capture group 1, or named `(?P<id>…)` / `(?P<name>…)` and `(?P<version>…)`. |

List parsing order: JSON when `idField` is set and stdout looks like JSON; else `listMatch`; else the first whitespace-separated field per line.

Commands are split into argv (quotes supported). There is no shell piping or redirection — wrap in `sh -c` if needed. Adapter names are `claude-plugin`-style kebab-case and must not collide with a built-in manager. Spec adapters are explicit-only: they never win `genv add git` fallback.

`genv scan` runs each available adapter’s `list` and, for spec adapters, writes `prefer` so the adopted package stays bound. `genv export` copies `adapters` into the snapshot so `prefer` still validates.

`genv apply` consults a live inventory (`ListInstalled` per available manager) and adopts already-installed packages into the lock instead of reinstalling. `genv upgrade` remains the only upgrade path. Apply `--timeout` defaults to 10m. `--skip-packages` applies env/shell/files/services without inventorying or planning packages. `--source-root <dir>` resolves `files.links` / `files.templates` sources against that directory instead of the spec file directory (lock, env, and shell paths stay where `--file` / `--lock-file` / `--state-dir` put them).

`genv status` probes live managers by default (`--offline` is lock-only). Unlocked but installed packages are `present`.

## Profiles

Named profiles live in `profiles/<name>.json` beside the base spec on schema v1–v7. `genv profile switch` merges the profile over the base, applies, and stores `activeProfile` in the lock. Schema v8 refuses named profiles — use `defaults` plus `targets.*` instead.
