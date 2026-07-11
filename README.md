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
| Any of the above  | `bun`, `npm`, `pnpm`, `yarn`, `deno`, `volta` (global JS/TS tools), `uv`, `pipx`, `pip-user`, `poetry`, `conda`, `mamba`, `pixi` (global Python/data tools), `cargo` (global crates), `go` (`go install` binaries), `rustup` (explicit Rust toolchains/components/targets), `gem` (global Ruby gems), `composer` (global PHP packages), `dotnet-tool` (global .NET tools), `ghcup`/`stack` (Haskell toolchains/tools), `opam` (OCaml switch packages), `juliaup` (Julia channels), `sdkman`/`asdf`/`mise` (universal tool/version managers), `krew` (kubectl plugins), `helm` (Helm plugins), `vscode` (VS Code extensions) |

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
  "schemaVersion": "6",
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
    "preApply": [
      {
        "file": "~/.config/genv/hooks/pre-apply.sh"
      }
    ],
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
  "updates": {
    "enabled": true,
    "interval": "24h",
    "autoApply": false,
    "notify": true
  },
  "repo": {
    "url": "https://github.com/example/dotfiles",
    "ref": "main"
  }
}
```

**Fields:**

- `schemaVersion` — `"1"` through `"6"`; older versions still load
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
- `hooks` — optional lifecycle hooks
  - Existing schema v5 hook arrays remain supported unchanged: `preUpgrade`, `postApply`, `postUpgrade`
  - Schema v6 additionally supports `preApply`, `postApply`, `preAdd`, `postAdd`, `preRemove`, `postRemove`, `preUpgrade`, and `postUpgrade`
  - Each hook is `{ "command": "inline shell command", "host": ... }` or `{ "file": "~/path/to/script.sh", "host": ... }`; set exactly one of `command` or `file`
  - Hook environment includes `GENV_EVENT`, `GENV_PHASE`, `GENV_HOST`, `GENV_PROFILE`, `GENV_INSTALLED`, `GENV_REMOVED`, `GENV_UPGRADED`, `GENV_FAILED`, and `GENV_SKIPPED` as deterministic comma-separated lists
  - Security: hooks run as the current user and can execute arbitrary shell/script code with the same filesystem and network access as `genv`; script-file hooks are arbitrary code execution by design. `genv` passes script paths as argv elements and does not interpolate package names into hook command strings beyond the explicit inline shell-command contract.
- `updates` — optional managed background update checker settings (schema v6)
  - `enabled` — set `true` before `genv updates start` will register a checker
  - `interval` — positive Go duration such as `"24h"`, used as the systemd timer / launchd `StartInterval` cadence
  - `autoApply` — default `false`; upgrades are only applied by the checker when explicitly set to `true`
  - `notify` — send a best-effort desktop notification (`notify-send` on Linux, `osascript` on macOS) when available
  - `onlyManagers`, `skipManagers`, `only`, `skip` — optional tracked-only filters matching `genv upgrade`
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
| `genv profile list`                               | List available profiles and mark the active one                        |
| `genv profile create <name>`                      | Scaffold a new profile file                                            |
| `genv profile switch <name> [flags]`              | Switch to a named profile and reconcile the system                     |
| `genv apply [flags]`                              | Reconcile system state with genv.json                                  |
| `genv validate [flags]`                           | Validate genv.json without changing the system                         |
| `genv upgrade [flags]`                            | Upgrade tracked packages in per-manager batches and refresh lock versions |
| `genv updates check [flags]`                      | Check available updates for genv-tracked packages without mutating anything |
| `genv updates start [flags]`                      | Register the managed background updates checker                             |
| `genv updates stop`                               | Stop and unregister the managed background updates checker                   |
| `genv updates status`                             | Show managed background updates checker status                              |
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
- `--no-hooks` — skip pre-add and post-add hooks without skipping the install
- `--hook-timeout <duration>` — per-hook deadline, e.g. `5m` or `30s` (0 = no timeout)
- `--host <name>` — host name for host-specific hooks (defaults to `$GENV_HOST` or the machine hostname)
- `--lock-file <path>` — path to the lock file (defaults next to the resolved spec)

### `genv remove` flags

- `--no-hooks` — skip pre-remove and post-remove hooks without skipping the uninstall
- `--hook-timeout <duration>` — per-hook deadline, e.g. `5m` or `30s` (0 = no timeout)
- `--host <name>` — host name for host-specific hooks (defaults to `$GENV_HOST` or the machine hostname)
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
- `--no-hooks` — skip pre-apply and post-apply hooks without skipping the apply operation
- `--hook-timeout <duration>` — per-hook deadline, e.g. `5m` or `30s` (0 = no timeout)
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
- `--no-hooks` — skip pre-upgrade and post-upgrade hooks without skipping package upgrades
- `--json` — emit machine-readable JSON to stdout instead of human-readable text
- `--only <id-or-pkg-name>[,...]` — upgrade only matching tracked packages
- `--skip <id-or-pkg-name>[,...]` — skip matching tracked packages
- `--only-manager <manager>[,...]` — upgrade only matching tracked managers
- `--skip-manager <manager>[,...]` — skip matching tracked managers
- `--hook-timeout <duration>` — per-hook deadline, e.g. `5m` or `30s` (0 = no timeout)
- `--debug` — emit debug-level structured logs to stderr
- `--host <name>` — host name for host-specific records (defaults to `$GENV_HOST` or the machine hostname)
- `--lock-file <path>` — path to the lock file (defaults next to the resolved spec)

### `genv updates check` flags

`genv updates check` uses the same shared dry-run planner as `genv upgrade --dry-run` and reports **genv-tracked packages only**. It never writes the lock file, never executes package-manager commands, and never runs upgrade hooks.

- `--json` — emit machine-readable JSON to stdout instead of human-readable text
- `--only <id-or-pkg-name>[,...]` — check only matching tracked packages
- `--skip <id-or-pkg-name>[,...]` — skip matching tracked packages
- `--only-manager <manager>[,...]` — check only matching tracked managers
- `--skip-manager <manager>[,...]` — skip matching tracked managers
- `--host <name>` — host name for host-specific records (defaults to `$GENV_HOST` or the machine hostname)
- `--lock-file <path>` — path to the lock file (defaults next to the resolved spec)

### `genv updates start|stop|status`

`genv updates start` registers a user-level scheduled job using `systemd --user` on Linux or `launchd` on macOS. The scheduled command is a one-shot `genv updates __run-once` inside the genv binary; the platform supervisor owns the interval and restart lifecycle. Unsupported platforms report a clear message instead of crashing.

The managed checker logs to `~/.config/genv/updates.log` (respecting `$XDG_CONFIG_HOME`) and rotates that file to `updates.log.1` after roughly 1 MiB. On each interval it runs the shared upgrade planner against tracked packages only, logs the result, and sends a best-effort notification when `updates.notify:true` and a notifier binary is available.

The default background behavior is **check/log/notify only**. It does not apply package upgrades unless the spec explicitly sets:

```json
{
  "schemaVersion": "6",
  "updates": {
    "enabled": true,
    "interval": "24h",
    "autoApply": true
  }
}
```

`genv updates start` refuses a missing, disabled, or invalid `updates` block with a corrective hint. Use `genv updates status` to inspect the registered checker and `genv updates stop` to remove it.

`genv updates start` flags:

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

# Check status or tracked-only updates in a script
genv status --json | jq '.ok'
genv updates check --json | jq '.data.dryRun'

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
4. Falls back to the first available system package manager in the registry, using the package ID as the name.

Language, toolchain, and plugin managers such as `npm`, `cargo`, `go`, `krew`, `helm`, and `vscode` are explicit-only during resolution: use `prefer` or `managers` when a package should be handled by one of them. This prevents a generic package such as `git` from accidentally resolving to an ecosystem manager just because that tool is installed.

Unresolved packages (no compatible manager found) produce a warning. Use `--strict` to treat them as a hard error.

---

## How upgrades work

`genv upgrade` updates the packages recorded in `genv.lock.json` without touching packages that are installed but not tracked by genv.

To minimize the number of subprocesses, genv groups tracked packages by their recorded package manager and issues one batched upgrade command per manager when the underlying tool supports it:

- `pacman`, `paru`, `yay` — `pacman -S pkg1 pkg2 ...`
- `brew` / `linuxbrew` — `brew upgrade pkg1 pkg2 ...`
- `choco` — `choco upgrade -y pkg1 pkg2 ...`
- `scoop` — `scoop update pkg1 pkg2 ...`
- `snap` — `snap refresh pkg1 pkg2 ...`

Managers that do not provide a selective multi-package upgrade command (`uv`, `pipx`, `pip-user`, `poetry`, `conda`, `mamba`, `pixi`, `mas`, `bun`, `npm`, `pnpm`, `yarn`, `deno`, `volta`, `cargo`, `go`, `rustup`, `gem`, `composer`, `dotnet-tool`) still upgrade one package at a time. JavaScript, Python, Rust, and Go managers are tracked-only: `npm` upgrades with `npm install --global <pkg>`, `pnpm` with `pnpm add --global <pkg>`, Yarn Classic with `yarn global add <pkg>`, Volta with `volta install <pkg>`, and none of them run broad global update commands; `cargo` upgrades named crates with `cargo install <pkg>` and never runs broad install-update commands; `go` upgrades named module paths with `go install <module>@latest` (or the explicit `@version` you track) and never runs broad Go updaters; `rustup` upgrades only explicit IDs such as `toolchain:stable`, `component:rustfmt@stable`, or `target:aarch64-unknown-linux-gnu@stable`. Python managers are similarly scoped: `uv` upgrades with `uv tool install --upgrade <pkg>`, `pipx` upgrades with `pipx install --force <pkg>`, `pip-user` with `python3 -m pip install --user --upgrade <pkg>` (note: this shares the user site-packages directory), `poetry` with `poetry self add <pkg>`, `pixi` with `pixi global upgrade <pkg>`, and `conda`/`mamba` require explicit `<env>:<pkg>` targeting.

Ruby, PHP, and .NET managers are tracked-only globals: `gem` installs/upgrades with `gem install <pkg>` and uninstalls with `gem uninstall -x -a <name>` (which removes every installed version of the tracked gem); `composer` uses `composer global require <pkg>`/`composer global remove <pkg>` and never runs a project-local `composer update`; `dotnet-tool` uses `dotnet tool install/uninstall/update --global` only, never a local tool manifest.

Language-ecosystem toolchain managers use explicit, non-project targeting: `ghcup` tracks Haskell tool versions by a `<tool>:<version>` id where tool is `ghc`, `cabal`, `hls`, or `stack` (e.g. `ghc:9.4.8`); `stack` installs global executables with `stack install <pkg>` (it has no safe global uninstall, so removal is reported as unsupported rather than guessed); `opam` requires a `<switch>:<pkg>` id and pins every operation to that switch with `--switch`; `juliaup` tracks a single Julia channel id such as `release`, `lts`, or `1.10` via `juliaup add/remove/update <channel>`. Ambiguous or unsupported ids fail with an actionable, non-mutating command.

Universal version managers require explicit tool/version ids and never run broad "update all" commands: `sdkman` uses `<candidate>:<version>` (e.g. `java:21.0.2-tem`) mapped to `sdk install/uninstall <candidate> <version>` and never `sdk selfupdate`; `asdf` uses `plugin:<name>` for plugins (`asdf plugin add/remove`) or `tool:<plugin>@<version>` for tool versions (`asdf install/uninstall <plugin> <version>`) and never `asdf plugin update --all`; `mise` uses `<tool>@<version>` (e.g. `node@22.11.0`) mapped to `mise use -g <tool>@<version>` / `mise uninstall <tool>@<version>`. Invalid formats fail with an actionable, non-mutating command.

Kubernetes plugin managers are tracked per plugin: `krew` uses `kubectl krew install/uninstall/upgrade <plugin>` (never the broad `kubectl krew upgrade` that updates every plugin); `helm` manages Helm plugins only (`helm plugin install/uninstall/update <name>`), never Helm repositories or project chart dependencies. Helm plugin installs need a source URL, so track them with a `managers.helm` url override (or a `name=url` value); an id with no url fails with an actionable, non-mutating command.

The `vscode` adapter tracks individual VS Code extensions by their `<publisher>.<name>` id via `code --install-extension`/`--uninstall-extension`, and upgrades a single tracked extension with `code --install-extension <id> --force` — never a broad "update all extensions" command. Extension ids are matched case-insensitively against `code --list-extensions --show-versions`.

**Deferred plugin managers.** Editor/shell/tmux frameworks whose plugin lifecycle depends on an interactive shell session or a self-update-only model are intentionally **not** implemented as genv managers, because they cannot expose safe, non-interactive, per-plugin install/remove/upgrade commands that fit genv's tracked-only model. This includes shell frameworks (`oh-my-zsh`, `zinit`, `antidote`, `fisher`) and the tmux plugin manager (`tpm`). Manage these through genv `hooks` (e.g. a `postApply` script that runs the framework's own installer/updater) rather than as tracked packages. This keeps genv from sourcing interactive RC files or depending on an attached tmux session.

Yarn support targets Yarn Classic's `yarn global` commands; Yarn Berry project-scoped package management is not modeled as global packages. Deno global scripts need both a command name and a module URL. Because genv package entries have a single manager override string, use either `"id":"serve","managers":{"deno":"https://deno.land/std/http/file_server.ts"}` or the explicit `"managers":{"deno":"serve=https://deno.land/std/http/file_server.ts"}` form; invalid or missing Deno URLs fail with an actionable command instead of running a partial install.

After each command, genv refreshes the installed versions in the lock file. For managers that can list all installed versions in one call, that list is used instead of querying each package individually.

Because batching means a single command may upgrade several packages at once, a failure is reported for the whole batch while still recording the versions of any packages that did upgrade.

Lifecycle hooks run by default for non-dry-run add/remove/apply/upgrade commands. Pass `--no-hooks` to skip hooks without suppressing the primary package operation; upgrade JSON output records this as `data.filters.hooksSkipped`. Use `--hook-timeout` to bound each hook independently.

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
- [x] M11: updates daemon/checker (`genv updates`)
- [x] M12: named profiles (`genv profile`)
- [x] M13: hooks and lifecycle scripts

The v3.0.0 line closes the currently committed roadmap backlog. Future large ideas should be tracked as issues or non-committed proposals rather than open roadmap milestones.

## Releasing

The repository includes a tag-driven GitHub release workflow. The release process is documented in [RELEASING.md](RELEASING.md).

---

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md) for details.

---

## License

MIT
