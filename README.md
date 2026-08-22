# genv

Track, sync, and reproduce your software environment across **macOS**, **Windows**, **Arch Linux**, **Ubuntu-like Linux**, and **WSL2**.

`genv` is a thin layer over the package managers you already use. Desired state lives in one git-friendly `genv.json`. Applied state lives in a machine-local lock file. Run `genv apply` and the machine matches the spec.

**Current release:** [latest](https://github.com/ks1686/genv/releases/latest) (v4.1.0+) — schema **v7** PowerShell parity, schema **v8** portable multi-target configs.

```bash
genv add git                          # track + install
genv apply --dry-run                  # preview reconcile
genv apply --yes                      # apply without prompt
genv status                           # show drift
genv migrate --write                  # upgrade a legacy spec to v8
genv export --target ubuntu --out ./u # single-target snapshot + report
genv map --target arch                # assist-only manager suggestions
```

---

## Install

| Platform | Install |
| -------- | ------- |
| macOS | `brew tap ks1686/tap && brew install --cask genv` |
| Arch / Manjaro | `paru -S genv` or `yay -S genv` (or `genv-bin`) |
| Other Linux | GitHub release tarball (see below) |
| Windows | Scoop (self-hosted bucket) or GitHub release zip (see below) |
| Any (from source) | `go install github.com/ks1686/genv@latest` (Go 1.24+) |

**Linux x86-64 example** (replace the version to match [Releases](https://github.com/ks1686/genv/releases/latest)):

```bash
curl -Lo genv.tar.gz https://github.com/ks1686/genv/releases/latest/download/genv_4.2.1_linux_amd64.tar.gz
tar -xzf genv.tar.gz
sudo mv genv /usr/local/bin/
genv version
```

**Windows (Scoop, after Scoop itself is installed):**

```powershell
scoop bucket add ks1686 https://github.com/ks1686/scoop-bucket
scoop install genv
```

The bucket is self-hosted (not Scoop extras). `scoop install genv` needs a root
`genv.json` on that bucket, which the first stable tag uploaded.

**Windows (PowerShell zip):**

```powershell
Invoke-WebRequest -Uri https://github.com/ks1686/genv/releases/latest/download/genv_4.2.1_windows_amd64.zip -OutFile genv.zip
Expand-Archive genv.zip -DestinationPath .
# put genv.exe on PATH, then:
genv version
```

winget and Chocolatey installers for the **genv binary** are not published
(GoReleaser Pro). Once genv is on `PATH`, it still **manages packages** through
winget, Scoop, and Chocolatey.

Release archives ship cosign-signed checksums (keyless). Darwin binaries are also Developer ID signed and notarized when Apple secrets are configured — see [SECURITY.md](SECURITY.md).

Platform walkthroughs: [macOS](docs/macos-install.md) · [Windows](docs/windows-install.md) · [WSL2](docs/wsl2-install.md) · [multi-machine](docs/multi-machine.md)

---

## Quick start

### One machine

```bash
genv init                              # optional wizard
genv add git
genv add neovim --version "0.10.*"
genv scan                              # bulk-adopt what's already installed
genv status
genv apply --dry-run
genv apply --yes
```

Default paths (respect `$XDG_CONFIG_HOME`):

| File | Location | Role |
| ---- | -------- | ---- |
| Spec | `~/.config/genv/genv.json` | Desired state — edit / commit this |
| Lock | `~/.config/genv/genv.lock.json` | Applied state — **machine-local, never commit** |

### Multiple machines (schema v8)

Put shared settings in `defaults`, OS-specific packages under `targets.*`, commit the spec, then apply per machine:

```bash
genv migrate --file genv.json --write   # if upgrading from v1–v7
genv map --target ubuntu                # see manager gaps (read-only)
genv apply --target ubuntu --dry-run
genv apply --target ubuntu --yes
```

Active target resolution: `--target` → `$GENV_TARGET` → host classification.

Full guide: [docs/multi-machine.md](docs/multi-machine.md).

---

## Targets and package managers

| Target | Detected when | Typical managers |
| ------ | ------------- | ---------------- |
| `macos` | macOS | `brew`, `mas` |
| `windows` | native Windows | `winget`, `scoop`, `choco` |
| `arch` | native Arch / Arch-like | `pacman`, `paru`, `yay` |
| `ubuntu` | Ubuntu-like Linux **or** Ubuntu-like WSL2 | `apt`, `snap`, `linuxbrew` |
| `wsl-arch` | Arch-like WSL2 | `pacman`, `paru`, `yay` |
| `linux` | optional catch-all (set via `--target` / `GENV_TARGET`) | `apt`, `dnf`, `apk`, `snap`, `linuxbrew`, … |

WSL2 does **not** inherit native `arch` automatically. Put shared bits in `defaults`; use `targets.ubuntu` or `targets.wsl-arch` for distro-specific packages.

**Also available** (explicit `prefer` / `managers`): `bun`, `npm`, `pnpm`, `yarn`, `deno`, `volta`, `uv`, `pipx`, `pip-user`, `poetry`, `conda`, `mamba`, `pixi`, `cargo`, `go`, `rustup`, `gem`, `composer`, `dotnet-tool`, `ghcup`, `stack`, `opam`, `juliaup`, `sdkman`, `asdf`, `mise`, `krew`, `helm`, `vscode`.

`external` is a track-only pseudo-manager: packages installed outside any manager (official installers, vendor downloads). Apply records them in the lock when the binary is on PATH; genv never installs or removes them itself.

Native `apt`, `dnf`, and `apk` adapters are registered system managers (`prefer: apt|dnf|apk`). Use `genv map` / `genv export` when moving a spec across Linux families.

---

## Spec format (schema v8)

Recommended shape for new configs:

```json
{
  "schemaVersion": "8",
  "defaults": {
    "env": {
      "EDITOR": { "value": "nvim" }
    },
    "shell": {
      "aliases": {
        "ll": { "value": "ls -lah" }
      }
    }
  },
  "targets": {
    "macos": {
      "packages": [
        { "id": "git", "prefer": "brew" },
        { "id": "ripgrep", "prefer": "brew" }
      ]
    },
    "ubuntu": {
      "packages": [
        { "id": "git", "prefer": "apt" },
        { "id": "ripgrep", "prefer": "apt" }
      ],
      "env": {
        "EDITOR": null
      }
    },
    "windows": {
      "packages": [
        {
          "id": "git",
          "prefer": "winget",
          "managers": { "winget": "Git.Git", "scoop": "git", "choco": "git" }
        }
      ],
      "shell": {
        "aliases": {
          "ll": { "value": "Get-ChildItem", "shell": "powershell" }
        }
      }
    }
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

**v8 rules (short):**

- Desired state lives under `defaults` and/or `targets.<id>` — not as top-level `packages` / `env` / `shell` / `files` / `services` / `hooks`.
- No per-record `host` in v8 (use target buckets).
- Target map entries may be `null` **tombstones** to drop a default for one OS (`EDITOR` above).
- `repo` and `updates` stay top-level.
- Schema **v7** adds `"shell": "powershell"` (POSIX-only when omitted). On native Windows, genv prefers `pwsh`, else Windows PowerShell, for profile fragments and hooks.

Legacy **v1–v7** specs still load. Convert with `genv migrate`. Field-by-field reference: [SCHEMA.md](SCHEMA.md).

---

## How apply and locks work

1. Read the spec and the lock.
2. For v8: resolve the active target, merge `defaults` + target (+ tombstones).
3. Refuse a **foreign lock** (wrong target / OS / unavailable managers). Recover with `genv apply --force-new-lock` (backs up the lock) or by removing it locally.
4. Install packages in the spec but not the lock; uninstall lock entries removed from the spec.
5. Reconcile env, shell, files, services, hooks as configured.
6. Update the lock (v8 records `target` + `goos`).

Convenience commands (`add` / `remove` / `adopt` / `disown` / `scan`) update the spec and usually the live system in one step. `genv add` installs first and only persists the spec after a successful install (unresolved or failed installs exit `4` and leave the spec unchanged; use `adopt` to track without installing). On v8 they write into `targets.<active>` (`--target` or `$GENV_TARGET` / classification).

`genv pull` fetches `genv.json` **and** relative `files` assets from `repo.url`. It never overwrites the lock or secrets.

---

## CLI reference

| Command | Purpose |
| ------- | ------- |
| `add` / `remove` (`rm`) | Track + install / untrack + uninstall |
| `adopt` / `disown` | Track without install / untrack without uninstall |
| `scan` | Bulk-adopt installed packages (`--dry-run`, `--yes`) |
| `list` (`ls`) | Show lock-tracked packages |
| `status` | Spec ↔ lock drift (`--files`, `--offline`, `--target`) |
| `apply` | Reconcile (`--dry-run`, `--yes`, `--json`, `--force`, `--backup`, `--strict`, `--quiet`, `--skip-packages`, `--timeout <d>`, `--no-hooks`, `--hook-timeout <d>`, `--target`, `--force-new-lock`) |
| `validate` | Validate spec + genv-managed agent executables |
| `upgrade` | Upgrade outdated tracked packages (`--all`, `--only`, `--skip`, `--only-manager`, `--skip-manager`, `--target`) |
| `updates` | Background checker (`check` / `start` / `stop` / `status`; `--target`, `--only`, `--skip`, `--only-manager`, `--skip-manager` on check/start) |
| `profile` | Named overlays (`list` / `create` / `switch`; refused on schema v8) |
| `pull` | Fetch spec + file assets from `repo` |
| `migrate` | v1–v7 → v8 targets |
| `export` | Single-target snapshot + report + assets |
| `map` | Assist-only manager mapping suggestions |
| `init` / `edit` | Wizard / `$EDITOR` |
| `env` / `shell` / `service` | Env vars, aliases, user services |
| `completion` | `bash` / `zsh` / `fish` / `powershell` |
| `clean` | Clear detected manager caches |
| `version` / `help` | Build info / usage |

### Shell completions

Install for your shell (auto-detects from `$SHELL` when omitted):

```bash
genv completion install        # bash, zsh, or fish
genv completion install powershell
```

Tab completion on `add` / `adopt` suggests package names from available managers (Homebrew-style local dumps when possible). After you accept a name, interactive `genv add` still asks which manager to use when multiple match.

### Common flags

- `--file <path>` — spec path (default under `~/.config/genv/`)
- `--lock-file <path>` — lock path (default `genv.lock.json` in the genv config dir)
- `--target <id>` — v8 target for apply / status / upgrade / updates / mutate / export / map / scan
- `--host <name>` — legacy host filter override for v1–v7 records / hooks (defaults via host **classification**, not hostname)
- `--json` — machine-readable envelope on stdout; subprocess noise on stderr

### Apply / portability flags worth knowing

```bash
genv apply --target windows --yes                 # adopt live apps, install only the missing
genv apply --skip-packages --yes                  # links + env only
genv apply --timeout 30m --hook-timeout 2m        # cap each subprocess / hook (default 10m; 0 disables)
genv apply --target ubuntu --dry-run --json
genv apply --force --backup --yes                 # overwrite mismatched files; keep *.backup.*
genv apply --target ubuntu --force-new-lock --yes   # after a foreign lock refuse
genv status --target windows                      # present vs missing vs ok
genv adopt cursor --target windows                # lock Anysphere.Cursor if already installed
genv export --target macos --out ./dist/macos --strict
genv migrate --write
```

File mismatches without `--force` no longer block packages/services: non-conflicting file ops still apply, each mismatched path is printed, and apply exits `4` if any remain.

### Updates checker

`genv updates start` registers a user systemd timer (Linux) or launchd job (macOS). Default behavior is check / log / notify only. Set `"autoApply": true` in the `updates` block to apply upgrades automatically. Details: [SCHEMA.md](SCHEMA.md#updates).

### Exit codes

| Code | Meaning |
| ---- | ------- |
| 0 | Success |
| 1 | Bad arguments / unknown command |
| 2 | I/O or serialization error |
| 3 | Spec failed validation |
| 4 | Semantic error (also `status` when drift exists; foreign lock refuse) |

---

## Resolution and upgrades (summary)

**Install resolution:** detect available managers → honor `prefer` → try `managers` map → fall back to system managers using the package `id`. Language / toolchain / plugin managers are **explicit-only** (must set `prefer` or `managers`) so `git` never silently resolves through npm.

**Upgrades:** `genv upgrade` and `genv updates check` share a planner. By default they plan packages with a detected update; `--all` plans every unconstrained tracked package. Version-constrained packages are skipped unless an adapter can guarantee a compatible target. Batched where the manager allows (`brew`, `pacman`/`paru`/`yay`, `apt`/`dnf`/`apk`, `mas`, `snap`, `scoop`, `choco`, …).

---

## Project status

| Area | State |
| ---- | ----- |
| Core CLI + declarative apply | Stable |
| macOS / Windows / Arch / Ubuntu / WSL targets | Stable (v4.0.0) |
| Schema v7 PowerShell profiles | Stable |
| Schema v8 portable targets | Stable |
| Background `updates` + profiles + hooks | Stable |
| apt / dnf / apk adapters | Stable |
| Publish genv to Scoop | Self-hosted bucket `ks1686/scoop-bucket`; uploads when `SCOOP_BUCKET_GITHUB_TOKEN` is set |
| Publish genv to winget / choco | Not published; publishers are GoReleaser Pro-only |

Historical milestone checklists: [ROADMAP.md](ROADMAP.md). Release notes: [CHANGELOG.md](CHANGELOG.md). Tag-driven publishing: [RELEASING.md](RELEASING.md).

---

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md).

## License

MIT
