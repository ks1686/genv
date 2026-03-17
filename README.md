# gpm — Global Package Manager

Track, sync, and reproduce your software environment across Linux, macOS, and WSL2.

```bash
gpm add git                     # track a package
gpm install                     # install everything in your gpm.json
gpm install --dry-run           # preview what will run
```

---

## What it is

`gpm` is a thin layer on top of your existing package managers. It tracks what you want installed in a single `gpm.json` file, then figures out how to install each package on whatever machine you're on.

Move to a new machine? Clone your dotfiles, run `gpm install`, and you're done.

Inspired by NixOS's declarative model, but without the learning curve: you declare an app by ID, pin a version if you care, and optionally hint at which manager to prefer.

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

---

## Quick start

```bash
# Add packages to your gpm.json
gpm add git
gpm add neovim --version "0.10.*"
gpm add firefox --manager flatpak

# See what's tracked
gpm list

# Install everything (dry-run first by default)
gpm install --dry-run
gpm install
```

Your `gpm.json` is just a file — commit it, share it, version it.

---

## gpm.json format

```json
{
  "schemaVersion": "1",
  "packages": [
    {
      "id": "git",
      "version": "*"
    },
    {
      "id": "neovim",
      "version": "0.10.*",
      "prefer": "brew"
    },
    {
      "id": "firefox",
      "version": "*",
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
- `version` — version constraint; `"*"` means latest
- `prefer` — optional hint for which manager to use first
- `managers` — optional map of manager-specific package identifiers (for packages with different names across managers)

---

## CLI reference

```bash
gpm add <id> [--version <ver>] [--manager <mgr>]
gpm remove <id>
gpm install [--dry-run] [--strict]
gpm list
gpm status          # diff current host state against gpm.json
```

`--strict` fails immediately if any package cannot be resolved on the current host. Without it, gpm warns and continues.

---

## How resolution works

When you run `gpm install` on a new machine:

1. gpm detects which package managers are available.
2. For each package in `gpm.json`, it finds the intersection of available managers and ones that carry the package.
3. It scores candidates by your `prefer` hint, then by a built-in priority order for the current OS.
4. It builds a command plan and shows you what will run before doing anything.

---

## Roadmap

Implementation milestones and detailed checklists are tracked in [ROADMAP.md](ROADMAP.md).

Current focus:

- [ ] M1: Core CLI and `gpm.json` spec validation
- [ ] M2: Install planner + resolver for Linux, macOS, and WSL2
- [ ] M3: `gpm scan`, lock file support, and `gpm sync`
- [ ] M4: Reliability, automation, and release hardening

---

## Contributing

This project is in early development. The core spec and CLI come first. Adapter quality (how well each manager backend works) is the main product risk, so that's where contributions matter most.

See [CONTRIBUTING.md](CONTRIBUTING.md) for details.

---

## License

MIT
