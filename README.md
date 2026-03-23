# genv — Global Environment Manager

Track, sync, and reproduce your software environment across Linux, macOS, and WSL2.

```bash
genv add git                       # add and immediately install a package
genv remove git                    # remove from spec and immediately uninstall
genv adopt git                     # track an already-installed package without reinstalling
genv disown git                    # stop tracking a package without uninstalling it
genv scan                          # bulk-adopt all installed packages into genv.json
genv status                        # show drift between genv.json and the lock file
genv apply                         # reconcile system state with genv.json
genv apply --dry-run               # preview what will change
genv apply --yes                   # apply without a confirmation prompt (CI-safe)
genv apply --dry-run --json        # machine-readable plan output
```

---

## What it is

`genv` is a thin layer on top of your existing package managers. It tracks what you want installed in a single `genv.json` file, then figures out how to install each package on whatever machine you're on.

It works like NixOS's declarative model: **you edit the spec file, and `genv apply` makes reality match it** — installing packages that were added and uninstalling ones that were removed. A `genv.lock.json` file records what genv last applied, so it only acts on the delta.

Move to a new machine? Clone your dotfiles, run `genv apply`, and you're done.

---

## Supported platforms and package managers

| Platform | Managers                                                               |
|----------|------------------------------------------------------------------------|
| Linux    | `apt`, `dnf`, `pacman`, `paru`, `yay`, `linuxbrew`, `flatpak`, `snap`  |
| macOS    | `brew` (formulae + casks), `macports`                                  |
| Windows  | WSL2 (targets the Linux userland inside WSL2)                          |

`genv` detects which managers are available on the current host and picks the best one automatically, or uses your preference.

---

## Install

### macOS

```bash
brew tap ks1686/tap
brew install genv
```

### Linux — Arch / Manjaro

```bash
paru -S genv-bin      # or: yay -S genv-bin
```

### Linux — other distros

Download a pre-built binary from [Releases](../../releases/latest):

```bash
# example for x86-64 Linux
curl -Lo genv.tar.gz https://github.com/ks1686/genv/releases/latest/download/genv_linux_amd64.tar.gz
tar -xzf genv.tar.gz
sudo mv genv /usr/local/bin/
```

### Windows (WSL2)

Use the Linux instructions above inside your WSL2 shell. See the [WSL2 install guide](docs/wsl2-install.md) for a full walkthrough.

### Any platform — Go install

```bash
go install github.com/ks1686/genv@latest
```

Requires Go 1.21+. The binary is placed in `$GOPATH/bin`.

---

Verify the installation:

```bash
genv version
```

Release binaries are signed with [cosign](https://docs.sigstore.dev/cosign/overview/) using keyless signing. The signature and certificate are attached to every GitHub release alongside `checksums.txt`.

---

## Quick start

```bash
# Add packages — each one is tracked in genv.json and installed immediately
genv add git
genv add neovim --version "0.10.*"
genv add firefox --manager flatpak:org.mozilla.firefox

# Bulk-adopt all packages already installed on this machine
genv scan

# Adopt a single already-installed package — track it without reinstalling
genv adopt ripgrep

# Disown a package — stop tracking it without uninstalling it
genv disown ripgrep

# Check if genv.json and the lock file are in sync
genv status

# See what is currently tracked by genv (reads genv.lock.json)
genv list

# Edit genv.json directly in your $EDITOR
genv edit

# Reconcile — installs newly added packages, removes deleted ones
genv apply --dry-run   # preview the delta first
genv apply             # apply it (prompts for confirmation)
genv apply --yes       # apply without prompting (for CI / scripts)

# Machine-readable output for pipelines
genv apply --dry-run --json
genv status --json

# Remove a package — uninstalls it and removes it from the spec
genv remove git
```

Your `genv.json` lives at `~/.config/genv/genv.json` by default (respects `$XDG_CONFIG_HOME`). It is just a file — commit it, share it, version it.

---

## How the declarative model works

`genv` maintains two files side by side:

| File | Default location | Purpose |
| --- | --- | --- |
| `genv.json` | `~/.config/genv/genv.json` | **Desired state** — what you want installed. Edit via `genv add`/`genv remove`/`genv edit`/`genv scan`. |
| `genv.lock.json` | `~/.config/genv/genv.lock.json` | **Applied state** — what genv last installed, via which manager. Auto-managed; do not edit by hand. |

When you run `genv apply`:

1. genv reads `genv.json` (desired) and `genv.lock.json` (last applied).
2. Packages in desired but not in the lock → **install**.
3. Packages in the lock but not in desired → **uninstall** (using the manager recorded in the lock, then clean cache).
4. Packages in both → **skip** (already up to date).
5. Lock file is updated to reflect what actually succeeded.

`genv add <id>` and `genv remove <id>` are convenience commands that update the spec **and** immediately install or uninstall the single package, keeping the lock in sync.

`genv adopt <id>` and `genv disown <id>` give you fine-grained tracking control without touching the system: adopt starts tracking a package that's already installed (no install runs), and disown stops tracking one without uninstalling it.

`genv scan` discovers every package currently installed across all available managers and bulk-adopts them into your spec and lock — useful for generating a baseline spec from an existing machine.

`genv status` compares your spec and lock file and reports any drift — packages in the spec but not yet applied, packages in the lock but removed from the spec, and version constraint violations.

---

## genv.json format

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

- `id` — canonical name for the package (used by genv)
- `version` — optional version constraint; omit for latest; supports `"x.y.*"` prefix wildcards
- `prefer` — optional hint for which manager to use first
- `managers` — optional map of manager-specific package identifiers (for packages with different names across managers)

---

## CLI reference

| Command | Description |
| ------- | ----------- |
| `genv add <id> [flags]` | Add package to spec and install it now |
| `genv remove <id>` | Remove package from spec and uninstall it now (alias: `rm`) |
| `genv adopt <id> [flags]` | Track an already-installed package without reinstalling |
| `genv disown <id>` | Stop tracking a package without uninstalling it |
| `genv scan [flags]` | Bulk-adopt all installed packages into genv.json |
| `genv status [flags]` | Show drift between genv.json and the lock file |
| `genv list` | List packages currently tracked by genv (from lock file) (alias: `ls`) |
| `genv apply [flags]` | Reconcile system state with genv.json |
| `genv clean [--dry-run]` | Clear the cache of all detected package managers |
| `genv edit` | Open genv.json in `$EDITOR` |
| `genv version` | Show build version, commit, and date |
| `genv help` | Show help text |

### `genv add` / `genv adopt` flags

- `--version <ver>` — version constraint, e.g. `"0.10.*"`
- `--prefer <mgr>` — preferred manager, e.g. `brew`
- `--manager <mgr:name,...>` — manager-specific names, e.g. `flatpak:org.mozilla.firefox`

### `genv apply` flags

- `--dry-run` — print the reconcile plan without executing
- `--strict` — exit with an error if any package cannot be resolved
- `--yes` — skip the confirmation prompt (for CI and scripts)
- `--json` — emit machine-readable JSON to stdout instead of human-readable text
- `--timeout <duration>` — per-subprocess deadline, e.g. `5m` or `30s` (0 = no timeout)
- `--debug` — emit debug-level structured logs to stderr

### `genv status` flags

- `--json` — emit machine-readable JSON to stdout
- `--debug` — emit debug-level structured logs to stderr

### `genv scan` flags

- `--json` — emit machine-readable JSON to stdout
- `--debug` — emit debug-level structured logs to stderr

### `genv clean` flags

- `--dry-run` — print the clean commands without executing

### Common flag

- `--file <path>` — path to genv.json (default: `$XDG_CONFIG_HOME/genv/genv.json` or `~/.config/genv/genv.json`)

---

## Machine-readable output (`--json`)

When `--json` is passed, the command writes a single JSON object to stdout and routes all subprocess output to stderr, keeping stdout clean for piping.

```bash
# Parse the plan in CI
genv apply --dry-run --json | jq '.data.toInstall[].id'

# Check status in a script
genv status --json | jq '.ok'

# Non-interactive apply in a bootstrap script
genv apply --yes --json 2>/dev/null
```

The envelope format:

```json
{
  "command": "apply",
  "ok": true,
  "data": { ... },
  "errors": []
}
```

`ok` is `false` when the command encountered an error or found drift (`genv status`). Exit codes are unchanged regardless of `--json`.

---

## How resolution works

When genv needs to install a package it:

1. Detects which package managers are available on the host.
2. Honours the `prefer` hint if that manager is available.
3. Falls back to the first available manager listed in the `managers` map.
4. Falls back to the first available manager in the registry, using the package ID as the name.

Unresolved packages (no compatible manager found) produce a warning. Use `--strict` to treat them as a hard error.

---

## Exit codes

| Code | Meaning |
|------|---------|
| 0 | Success |
| 1 | Bad arguments or unknown command |
| 2 | Filesystem or serialisation error |
| 3 | `genv.json` fails schema validation |
| 4 | Semantic error — also returned by `genv status` when drift or extra entries exist |

---

## Roadmap

Implementation milestones and detailed checklists are tracked in [ROADMAP.md](ROADMAP.md).

Current focus (v1.0.0):

- [x] M1: Core CLI and `genv.json` spec validation
- [x] M2: Resolver + adapter layer, declarative apply, adopt/disown, cache clean
- [x] M3: `genv scan`, lock file version pinning, `genv status`
- [x] M4: `--json`, `--yes`, `--timeout`, `--debug`, signed releases
- [x] M5: macOS and WSL2 validation and automated testing
- [ ] M6: API stability, test coverage, performance benchmarks, security audit
- [ ] M7: Shell completions, `genv validate`, `genv upgrade`, `genv init`, improved errors

## Releasing

The repository includes a tag-driven GitHub release workflow. The release process is documented in [RELEASING.md](RELEASING.md).

---

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md) for details.

---

## License

MIT
