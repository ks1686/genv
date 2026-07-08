# genv — Global Environment Manager

Track, sync, and reproduce your software environment across Linux, macOS, Windows, and WSL2.

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

It follows a declarative model: **you edit the spec file, and `genv apply` makes reality match it** — installing packages that were added and uninstalling ones that were removed. A `genv.lock.json` file records what genv last applied, so it only acts on the delta.

Move to a new machine? Clone your dotfiles, run `genv apply`, and you're done.

---

## Supported platforms and package managers

| Platform          | Managers                                                     |
| ----------------- | -------------------------------------------------------------- |
| Linux (Arch)      | `pacman` (official repos), `paru`/`yay` (AUR)                |
| Linux (other)     | `snap`, `linuxbrew`                                           |
| macOS             | `brew` (formulae + casks), `mas` (Mac App Store)             |
| Windows (native)  | `winget`, `scoop`, `choco`                                    |
| WSL2              | Targets the Linux userland inside WSL2 (treated as `arch`)    |
| Any of the above  | `bun` (global installs), `uv` (global tool installs)          |

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
paru -S genv      # or: yay -S genv
```

### Linux — other distros

Download a pre-built binary from [Releases](https://github.com/ks1686/genv/releases/latest):

```bash
# example for x86-64 Linux
curl -Lo genv.tar.gz https://github.com/ks1686/genv/releases/latest/download/genv_linux_amd64.tar.gz
tar -xzf genv.tar.gz
sudo mv genv /usr/local/bin/
```

### Windows (WSL2)

Use the Linux instructions above inside your WSL2 shell. See the [WSL2 install guide](docs/wsl2-install.md) for a full walkthrough.

### Windows (native)

Native Windows is a first-class host (detected as the `windows` host class). The
`genv` binary is not yet published to winget/scoop/choco as an install channel, so
download the Windows binary from [Releases](https://github.com/ks1686/genv/releases/latest):

```powershell
Invoke-WebRequest -Uri https://github.com/ks1686/genv/releases/latest/download/genv_windows_amd64.zip -OutFile genv.zip
Expand-Archive genv.zip -DestinationPath .
```

Once installed, `genv` on native Windows manages packages via `winget`, `scoop`, and `choco` (whichever are present), plus `bun`/`uv` for global installs. See the [Windows install guide](docs/windows-install.md) for a full walkthrough.

### Any platform — Go install

```bash
go install github.com/ks1686/genv@latest
```

Requires Go 1.24+. The binary is placed in `$GOPATH/bin`.

---

Verify the installation:

```bash
genv version
```

Release binaries are signed with [cosign](https://docs.sigstore.dev/cosign/overview/) using keyless signing. The Sigstore bundle is attached to every GitHub release alongside `checksums.txt`.

---

## Quick start

```bash
# Add packages — each one is tracked in genv.json and installed immediately
genv add git
genv add neovim --version "0.10.*"
genv add firefox --manager brew:firefox

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

`genv` reads the desired state from `genv.json` and tracks the applied state in
`genv.lock.json`. The lock file always lives in the genv configuration directory
so that a spec checked into a repo does not carry host-specific applied state:

| File             | Default location                | Purpose                                                                                                 |
| ---------------- | ------------------------------- | ------------------------------------------------------------------------------------------------------- |
| `genv.json`      | `~/.config/genv/genv.json`      | **Desired state** — what you want installed. Edit via `genv add`/`genv remove`/`genv edit`/`genv scan`. |
| `genv.lock.json` | `~/.config/genv/genv.lock.json` | **Applied state** — what genv last installed, via which manager. Auto-managed; do not edit by hand.     |

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
  "schemaVersion": "5",
  "packages": [
    {
      "id": "git"
    },
    {
      "id": "neovim",
      "version": "0.10.*",
      "prefer": "brew",
      "host": ["arch", "wsl2"]
    },
    {
      "id": "firefox",
      "managers": {
        "brew": "firefox",
        "snap": "firefox",
        "winget": "Mozilla.Firefox"
      }
    }
  ],
  "env": {
    "EDITOR": {
      "value": "nvim"
    }
  },
  "shell": {
    "aliases": {
      "ll": {
        "value": "ls -lah"
      }
    }
  },
  "services": {
    "syncthing": {
      "start": ["syncthing", "serve"],
      "stop": ["pkill", "-f", "syncthing"]
    },
    "postgresql": {
      "brew_formula": "postgresql@14"
    }
  },
  "files": {
    "links": [
      {
        "source": "shell/.zshrc",
        "target": "~/.zshrc",
        "mode": "link"
      }
    ],
    "templates": [
      {
        "source": "codex/config.toml",
        "target": "~/.config/codex/config.toml"
      }
    ],
    "dirs": [
      {
        "target": "~/.config/genv"
      }
    ]
  },
  "hooks": {
    "preUpgrade": [
      {
        "command": "brew upgrade",
        "host": "macos"
      }
    ],
    "postApply": [
      {
        "command": "echo 'apply complete'"
      }
    ]
  },
  "repo": {
    "url": "https://github.com/example/dotfiles",
    "ref": "main"
  }
}
```

**Fields:**

- `schemaVersion` — `"1"` through `"5"`; older versions still load
- `packages` — array of tracked packages
  - `id` — canonical name for the package (used by genv)
  - `version` — optional version constraint; omit for latest; supports `"x.y.*"` prefix wildcards
  - `prefer` — optional hint for which manager to use first
  - `managers` — optional map of manager-specific package identifiers (for packages with different names across managers)
  - `host` — optional host selector (string or array), e.g. `"macos"` or `["arch","wsl2"]`
- `env` — optional map of global shell environment variables managed by genv
- `shell` — optional shell config block for aliases/functions/source snippets
- `services` — optional service block for declarative user-space service lifecycle management
  - `start` — command array to start the service (raw, cross-platform)
  - `brew_formula` — homebrew formula name to manage via `brew services` (macOS only; mutually exclusive with `start`)
- `files` — optional filesystem reconciliation block (schema v5)
  - `links` — symlinks to create (`mode`: `"link"`, `"managed-link"`, or `"merge-dir"`)
    - `"merge-dir"` symlinks source's files individually into target (rather than symlinking the whole directory as one unit), so multiple records — e.g. one unfiltered, one `host`-filtered — can target the *same* directory and layer: a later record's same-named file wins over an earlier one without needing `--force`, letting you keep one shared base plus a small host-specific override directory instead of a full separate source tree per host
  - `templates` — files to copy after rendering `__HOME__` / `__USER__` / `__HOST__` / `__OS__` / `__ARCH__`
  - `dirs` — directories to ensure exist
  - per-record `host` and `backup` fields are supported
- `hooks` — optional lifecycle shell commands (schema v5)
  - `preUpgrade`, `postApply`, `postUpgrade` arrays of `{ command, host }`
- `repo` — optional source repository for `genv pull` (schema v5)
  - `url` — repository URL or local path
  - `ref` — optional branch/tag/ref

---

## CLI reference

| Command                                           | Description                                                            |
| ------------------------------------------------- | ---------------------------------------------------------------------- |
| `genv add <id> [flags]`                           | Add package to spec and install it now                                 |
| `genv remove <id> [flags]`                        | Remove package from spec and uninstall it now (alias: `rm`)            |
| `genv adopt <id> [flags]`                         | Track an already-installed package without reinstalling                |
| `genv adopt --files [flags]`                      | Register already-correct `files` block entries into the lock without rewriting them |
| `genv disown <id>`                                | Stop tracking a package without uninstalling it                        |
| `genv scan [flags]`                               | Bulk-adopt all installed packages into genv.json                       |
| `genv status [flags]`                             | Show drift between genv.json and the lock file                         |
| `genv status --files [flags]`                     | Check the `files` block against the live filesystem only (does not touch the spec-vs-lock check above) |
| `genv pull [flags]`                               | Fetch genv.json from the `repo.url`/`repo.ref` git remote configured in the spec |
| `genv list`                                       | List packages currently tracked by genv (from lock file) (alias: `ls`) |
| `genv apply [flags]`                              | Reconcile system state with genv.json                                  |
| `genv validate [flags]`                           | Validate genv.json without changing the system                         |
| `genv upgrade [flags]`                            | Upgrade tracked packages and refresh lock versions                     |
| `genv init [flags]`                               | Interactive wizard to create a new genv.json                           |
| `genv env <set\|unset\|list>`                     | Manage global environment variables in the spec                        |
| `genv shell <alias\|status\|edit>`                | Manage shell aliases and shell config drift                            |
| `genv service <add\|remove\|list\|start\|stop\|status>` | Manage declared user-space services (aliases: `rm`, `ls`)         |
| `genv completion <bash\|zsh\|fish>`               | Print shell completion script                                          |
| `genv completion install [shell] [--dir]`         | Install completion into the shell's completion dir (shell auto-detected from `$SHELL`) |
| `genv clean [--dry-run]`                          | Clear the cache of all detected package managers                       |
| `genv edit`                                       | Open genv.json in `$EDITOR`                                            |
| `genv version`                                    | Show build version, commit, and date (alias: `--version`)              |
| `genv help`                                       | Show help text                                                         |

### `genv add` flags

- `--version <ver>` — version constraint, e.g. `"0.10.*"`
- `--prefer <mgr>` — preferred manager, e.g. `brew`
- `--manager <mgr:name,...>` — manager-specific names, e.g. `winget:Mozilla.Firefox`
- `--no-search` — skip the interactive package search and use the id as-is
- `--lock-file <path>` — path to the lock file (defaults next to the resolved spec)

### `genv adopt` flags

- `--version <ver>` — version constraint, e.g. `"0.10.*"`
- `--prefer <mgr>` — preferred manager, e.g. `brew`
- `--manager <mgr:name,...>` — manager-specific names, e.g. `winget:Mozilla.Firefox`
- `--host <name>` — host name for host-specific records (defaults to `$GENV_HOST` or the machine hostname)
- `--files` — adopt matching `files` block entries into the lock without changing targets
- `--json` — emit machine-readable JSON to stdout instead of human-readable text
- `--lock-file <path>` — path to the lock file (defaults next to the resolved spec)

### `genv apply` flags

- `--dry-run` — print the reconcile plan without executing
- `--force` — overwrite mismatched managed files
- `--strict` — exit with an error if any package cannot be resolved
- `--yes` — skip the confirmation prompt (for CI and scripts)
- `--quiet` — suppress plan output (useful in scripts)
- `--json` — emit machine-readable JSON to stdout instead of human-readable text
- `--timeout <duration>` — per-subprocess deadline, e.g. `5m` or `30s` (0 = no timeout)
- `--debug` — emit debug-level structured logs to stderr
- `--host <name>` — host name for host-specific records (defaults to `$GENV_HOST` or the machine hostname)
- `--lock-file <path>` — path to the lock file (defaults next to the resolved spec)

### `genv status` flags

- `--json` — emit machine-readable JSON to stdout
- `--debug` — emit debug-level structured logs to stderr
- `--files` — check the `files` block against the live filesystem instead of the spec-vs-lock diff
- `--host <name>` — host name for host-specific records (defaults to `$GENV_HOST` or the machine hostname)
- `--lock-file <path>` — path to the lock file (defaults next to the resolved spec)

### `genv pull` flags

- `--url <url>` — override the repository URL configured in `repo.url`
- `--ref <ref>` — override the repository ref configured in `repo.ref` (default: `main`)
- `--dry-run` — print what would be pulled without writing `genv.json`

### `genv scan` flags

- `--json` — emit machine-readable JSON to stdout
- `--debug` — emit debug-level structured logs to stderr
- `--lock-file <path>` — path to the lock file (defaults next to the resolved spec)

### `genv upgrade` flags

- `--dry-run` — print the upgrade commands without executing
- `--yes` — skip the confirmation prompt
- `--debug` — emit debug-level structured logs to stderr
- `--host <name>` — host name for host-specific records (defaults to `$GENV_HOST` or the machine hostname)
- `--lock-file <path>` — path to the lock file (defaults next to the resolved spec)

### `genv clean` flags

- `--dry-run` — print the clean commands without executing

### `genv service add` flags

- `--start <cmd>` — command to start the service (e.g. `"syncthing serve"`)
- `--stop <cmd>` — command to stop the service
- `--restart <cmd>` — command to restart the service
- `--status <cmd>` — command to check if the service is running
- `--brew-formula <formula>` — homebrew formula to manage via `brew services` (macOS only; mutually exclusive with `--start`)

**Examples:**
```bash
# Raw command service (any platform)
genv service add syncthing --start "syncthing serve" --stop "pkill -f syncthing"

# Brew-managed service (macOS only)
genv service add postgresql --brew-formula postgresql@14
```

### Common flags

- `--file <path>` — path to genv.json (default: `$XDG_CONFIG_HOME/genv/genv.json` or `~/.config/genv/genv.json`)
- `--lock-file <path>` — path to the lock file (default: `genv.lock.json` in the genv config directory). Accepted by the spec-mutating and reconciling commands (`add`, `remove`, `adopt`, `disown`, `list`, `apply`, `scan`, `status`, `upgrade`).

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
2. Honors the `prefer` hint if that manager is available.
3. Falls back to the first available manager listed in the `managers` map.
4. Falls back to the first available manager in the registry, using the package ID as the name.

Unresolved packages (no compatible manager found) produce a warning. Use `--strict` to treat them as a hard error.

---

## Exit codes

| Code | Meaning                                                                           |
| ---- | --------------------------------------------------------------------------------- |
| 0    | Success                                                                           |
| 1    | Bad arguments or unknown command                                                  |
| 2    | Filesystem or serialization error                                                 |
| 3    | `genv.json` fails schema validation                                               |
| 4    | Semantic error — also returned by `genv status` when drift or extra entries exist |

---

## Roadmap

Implementation milestones and detailed checklists are tracked in [ROADMAP.md](ROADMAP.md).

Current focus (v2.x):

- [x] M1: Core CLI and `genv.json` spec validation
- [x] M2: Resolver + adapter layer, declarative apply, adopt/disown, cache clean
- [x] M3: `genv scan`, lock file version pinning, `genv status`
- [x] M4: `--json`, `--yes`, `--timeout`, `--debug`, signed releases
- [x] M5: macOS and WSL2 validation and automated testing
- [x] M6: API stability, test coverage, performance benchmarks, security audit
- [x] M7: Shell completions, `genv validate`, `genv upgrade`, `genv init`, improved errors
- [x] M8: global environment variable management (`genv env`)
- [x] M9: shell configuration management (`genv shell`)
- [x] M10: services management (`genv service`) + expanded packaging channels
- [ ] M11: updates daemon
- [ ] M12: named profiles
- [ ] M13: hooks and lifecycle scripts

## Releasing

The repository includes a tag-driven GitHub release workflow. The release process is documented in [RELEASING.md](RELEASING.md).

---

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md) for details.

---

## License

MIT
