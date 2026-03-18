# gpm — Global Package Manager

Track, sync, and reproduce your software environment across Linux, macOS, and WSL2.

```bash
gpm add git                     # add and immediately install a package
gpm remove git                  # remove from spec and immediately uninstall
gpm adopt git                   # track an already-installed package without reinstalling
gpm disown git                  # stop tracking a package without uninstalling it
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

### macOS

```bash
brew tap ks1686/tap
brew install gpm
```

### Linux — Arch / Manjaro

```bash
paru -S gpm-bin      # or: yay -S gpm-bin
```

### Linux — other distros

Download a pre-built binary from [Releases](../../releases/latest):

```bash
# example for x86-64
curl -Lo gpm.tar.gz https://github.com/ks1686/gpm/releases/latest/download/gpm_linux_amd64.tar.gz
tar -xzf gpm.tar.gz
sudo mv gpm /usr/local/bin/
```

### Windows (WSL2)

Use the Linux instructions above inside your WSL2 shell, or download the Windows binary from [Releases](../../releases/latest) and add it to your `PATH`.

### Any platform — Go install

```bash
go install github.com/ks1686/gpm@latest
```

Requires Go 1.21+. The binary is placed in `$GOPATH/bin`.

---

Verify the installation:

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

# Adopt a package that's already installed — track it without reinstalling
gpm adopt ripgrep

# Disown a package — stop tracking it without uninstalling it
gpm disown ripgrep

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

`gpm adopt <id>` and `gpm disown <id>` give you fine-grained tracking control without touching the system: adopt starts tracking a package that's already installed (no install runs), and disown stops tracking one without uninstalling it.

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
| `gpm remove <id>` | Remove package from spec and uninstall it now (alias: `rm`) |
| `gpm adopt <id> [flags]` | Track an already-installed package without reinstalling |
| `gpm disown <id>` | Stop tracking a package without uninstalling it |
| `gpm list` | List packages currently tracked by gpm (from lock file) (alias: `ls`) |
| `gpm apply [--dry-run] [--strict]` | Reconcile system state with gpm.json |
| `gpm clean [--dry-run]` | Clear the cache of all detected package managers |
| `gpm edit` | Open gpm.json in `$EDITOR` |
| `gpm version` | Show build version, commit, and date |
| `gpm help` | Show help text |

### `gpm add` / `gpm adopt` flags

- `--version <ver>` — version constraint, e.g. `"0.10.*"`
- `--prefer <mgr>` — preferred manager, e.g. `brew`
- `--manager <mgr:name,...>` — manager-specific names, e.g. `flatpak:org.mozilla.firefox`

### `gpm apply` flags

- `--dry-run` — print the reconcile plan without executing
- `--strict` — exit with an error if any package cannot be resolved

### `gpm clean` flags

- `--dry-run` — print the clean commands without executing

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
- [x] M2: Resolver + adapter layer, declarative apply, adopt/disown, cache clean
- [ ] M3: `gpm scan`, lock file version pinning, `gpm status`
- [ ] M4: Reliability, automation, and release hardening
- [ ] M5: macOS and WSL2 validation and automated testing

## Releasing

The repository includes a tag-driven GitHub release workflow. The release process is documented in [RELEASING.md](RELEASING.md).

---

## Contributing

This project is in early development. The core spec and CLI come first. Adapter quality (how well each manager backend works) is the main product risk, so that's where contributions matter most.

See [CONTRIBUTING.md](CONTRIBUTING.md) for details.

---

## License

MIT
