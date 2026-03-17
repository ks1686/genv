# gpm — Global Package Manager

Track, sync, and reproduce your software environment across Linux, macOS, and WSL2.

```bash
gpm add git                     # add and immediately install a package
gpm remove git                  # remove from spec and immediately uninstall
gpm apply                       # reconcile system state with gpm.json
gpm apply --dry-run             # preview what will change
```

---

## What it is

`gpm` is a thin layer on top of your existing package managers. It tracks what you want installed in a single `gpm.json` file, then figures out how to install each package on whatever machine you're on.

It works like NixOS's declarative model: **you edit the spec file, and `gpm apply` makes reality match it** — installing packages that were added and uninstalling ones that were removed. A `gpm.lock.json` file records what gpm last applied, so it only acts on the delta.

Move to a new machine? Clone your dotfiles, run `gpm apply`, and you're done.

---

## Supported platforms and package managers

| Platform | Managers                                                              |
|----------|-----------------------------------------------------------------------|
| Linux    | `apt`, `dnf`, `pacman`, `paru`, `yay`, `linuxbrew`, `flatpak`, `snap` |
| macOS    | `brew` (formulae + casks), `macports`                                 |
| Windows  | WSL2 (targets the Linux userland inside WSL2)                         |

`gpm` detects which managers are available on the current host and picks the best one automatically, or uses your preference.

---

## Install

```bash
go install github.com/ks1686/gpm@latest
```

Or download a pre-built binary from [Releases](../../releases).

Published binaries report their embedded build metadata via:

```bash
gpm version
```

---

## Quick start

```bash
# Add packages — each one is tracked in gpm.json and installed immediately
gpm add git
gpm add neovim --version "0.10.*"
gpm add firefox --manager flatpak:org.mozilla.firefox

# See what is currently installed by gpm (reads gpm.lock.json)
gpm list

# Edit gpm.json directly in your $EDITOR
gpm edit

# Reconcile — installs newly added packages, removes deleted ones
gpm apply --dry-run   # preview the delta first
gpm apply             # apply it

# Remove a package — uninstalls it and removes it from the spec
gpm remove git
```

Your `gpm.json` is just a file — commit it, share it, version it.

---

## How the declarative model works

`gpm` maintains two files side by side:

| File | Purpose |
| --- | --- |
| `gpm.json` | **Desired state** — what you want installed. Edit via `gpm add`/`gpm remove`/`gpm edit`. |
| `gpm.lock.json` | **Applied state** — what gpm last installed, via which manager. Auto-managed; do not edit by hand. |

When you run `gpm apply`:

1. gpm reads `gpm.json` (desired) and `gpm.lock.json` (last applied).
2. Packages in desired but not in the lock → **install**.
3. Packages in the lock but not in desired → **uninstall** (using the manager recorded in the lock, then clean cache).
4. Packages in both → **skip** (already up to date).
5. Lock file is updated to reflect what actually succeeded.

`gpm add <id>` and `gpm remove <id>` are convenience commands that update the spec **and** immediately install or uninstall the single package, keeping the lock in sync.

---

## gpm.json format

```json
{
  "schemaVersion": "1",
  "packages": [
    {
      "id": "git"
    },
    {
      "id": "neovim",
      "version": "0.10.*",
      "prefer": "brew"
    },
    {
      "id": "firefox",
      "managers": {
        "flatpak": "org.mozilla.firefox",
        "brew":    "firefox",
        "snap":    "firefox"
      }
    }
  ]
}
```

**Fields:**

- `id` — canonical name for the package (used by gpm)
- `version` — optional version constraint; omit for latest
- `prefer` — optional hint for which manager to use first
- `managers` — optional map of manager-specific package identifiers (for packages with different names across managers)

---

## CLI reference

| Command | Description |
| ------- | ----------- |
| `gpm add <id> [flags]` | Add package to spec and install it now |
| `gpm remove <id>` | Remove package from spec and uninstall it now |
| `gpm list` | List packages currently installed by gpm (from lock file) |
| `gpm apply [--dry-run] [--strict]` | Reconcile system state with gpm.json |
| `gpm edit` | Open gpm.json in `$EDITOR` |
| `gpm help` | Show help text |

### `gpm add` flags

- `--version <ver>` — version constraint, e.g. `"0.10.*"`
- `--prefer <mgr>` — preferred manager, e.g. `brew`
- `--manager <mgr:name,...>` — manager-specific names, e.g. `flatpak:org.mozilla.firefox`

### `gpm apply` flags

- `--dry-run` — print the reconcile plan without executing
- `--strict` — exit with an error if any package cannot be resolved

### Common flag

- `--file <path>` — path to gpm.json (default: `./gpm.json`)

---

## How resolution works

When gpm needs to install a package it:

1. Detects which package managers are available on the host.
2. Honours the `prefer` hint if that manager is available.
3. Falls back to the first available manager listed in the `managers` map.
4. Falls back to the first available manager in the registry, using the package ID as the name.

Unresolved packages (no compatible manager found) produce a warning. Use `--strict` to treat them as a hard error.

---

## Roadmap

Implementation milestones and detailed checklists are tracked in [ROADMAP.md](ROADMAP.md).

Current focus:

- [x] M1: Core CLI and `gpm.json` spec validation
- [x] M2: Resolver + adapter layer, declarative apply, uninstall, cache clean
- [ ] M3: `gpm scan`, lock file pinning, and `gpm sync`
- [ ] M4: Reliability, automation, and release hardening

## Releasing

The repository includes a tag-driven GitHub release workflow. The release process is documented in [RELEASING.md](RELEASING.md).

The first public prerelease is `v0.1.0-beta.1`, intended to reflect the current usable state of the project. That release may include some Milestone 2 functionality that is still being validated across supported environments; the release notes should call that out explicitly.

---

## Contributing

This project is in early development. The core spec and CLI come first. Adapter quality (how well each manager backend works) is the main product risk, so that's where contributions matter most.

See [CONTRIBUTING.md](CONTRIBUTING.md) for details.

---

## License

MIT
