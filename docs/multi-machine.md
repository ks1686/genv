# Multi-machine and cross-OS genv configs

Use one schema **v8** `genv.json` as the git source of truth for every machine.
Keep machine-local state (locks, secrets) out of git. Companion overview:
[README](../README.md) · [SCHEMA](../SCHEMA.md).

## What belongs in git

Commit the portable source files:

- `genv.json`
- relative assets referenced by `files.links[].source` or `files.templates[].source`
- documentation for your target layout

Never commit:

- `genv.lock.json` or lock backups
- exported bundles that contain machine-specific snapshots
- secrets, tokens, private keys, or sensitive env values

The lock file records what one machine applied, including target, OS, managers,
and package names. Sharing it with another target can cause false "up to date"
results or unsafe uninstall plans, so treat it like local cache state.

## Model your machines with targets

Schema v8 moves desired state under `defaults` and `targets.*`:

```json
{
  "schemaVersion": "8",
  "defaults": {
    "env": {
      "EDITOR": { "value": "nvim" }
    }
  },
  "targets": {
    "macos": {
      "packages": [{ "id": "ripgrep", "prefer": "brew" }]
    },
    "ubuntu": {
      "packages": [{ "id": "ripgrep", "prefer": "apt" }]
    },
    "wsl-arch": {
      "packages": [{ "id": "ripgrep", "prefer": "pacman" }]
    }
  }
}
```

Known target IDs are `macos`, `windows`, `arch`, `ubuntu`, `wsl-arch`, and
`linux`. `genv apply` resolves the active target from `--target`, then
`GENV_TARGET`, then host classification. If no matching `targets.<id>` exists,
apply fails instead of applying another target.

Use `defaults` for shared env, shell aliases/functions, packages, files,
services, and hooks. A target overlays `defaults`; map entries in a target can
use `null` tombstones to remove default env vars, aliases, functions, or
services for that target.

## Migrate, inspect, and apply

For an older host-scoped file:

```bash
genv migrate --file genv.json          # preview schema v8 JSON on stdout
genv migrate --file genv.json --write  # rewrite genv.json after review
```

Then inspect package-manager portability:

```bash
genv map --target ubuntu --file genv.json
genv export --target ubuntu --out ./dist/ubuntu
```

`genv map` is assist-only and never edits your spec. `genv export` writes a
single-target schema v8 snapshot plus `report.json` and `report.md`; it omits
locks and sensitive env values, and copies relative file assets into the bundle.
Use `--strict` when an export with report errors should fail CI.

On each machine:

```bash
git clone <your-dotfiles-repo>
cd <your-dotfiles-repo>
genv apply --target ubuntu --dry-run
genv apply --target ubuntu --yes
```

You can omit `--target` when classification is enough, or set `GENV_TARGET` in
automation for a deterministic target.

## Foreign-lock recovery

When a lock file appears to belong to a different target, OS, or unavailable
manager set, `genv apply` refuses to use it:

```text
genv apply: foreign lock refused: lock target "macos" does not match active target "ubuntu"
```

Recover by inspecting and moving/removing the lock, or let genv back it up and
start fresh:

```bash
genv apply --target ubuntu --force-new-lock
```

`--force-new-lock` renames the foreign lock to `genv.lock.json.bak-<timestamp>`
and creates a new local lock after a successful apply. Do not copy the backup to
other machines; it is only for local recovery.
