# genv.json schema

Canonical structs: `internal/schema/schema.go` and `internal/schema/module.go`. Validation: `internal/schema/validate.go`. JSON Schema mirrors: `schema/v8/genv.json`, `schema/v9/genv.json`, and `schema/v10/genv.json` (Go validator remains source of truth).

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
| v9 | `"9"` | managed external release recipes |
| v10 | `"10"` | local modules: root `modules` registry + per-bundle `useModules` |

Older versions still load. Prefer **v8** unless a managed external recipe requires
v9 or composable modules require v10. All three use portable `defaults` and
`targets.*` buckets. Convert legacy specs with `genv migrate`.

## Common rules

- Field names are `lowerCamelCase`.
- Empty optional objects/arrays are omitted when marshaling (`omitempty`).
- Paths support `~` and `$VAR` / `${VAR}` expansion.
- **v1–v7:** optional per-record `host` is a string or string array (`"macos"` or `["arch","macos"]`). Empty means “all hosts”. Legacy literal `"wsl2"` is obsolete for classification (see [WSL guide](docs/wsl2-install.md)); migrate to `ubuntu` / `wsl-arch` targets.
- **v8-v10:** `host` is illegal. Use `targets.<id>` buckets.

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
| `genv export --target <id> --out <dir>` | single-target snapshot + `report.json` / `report.md` + relative assets; omits locks and sensitive env. `--verify` queries live managers and records install proof failures in the report |

### Locks

`~/.config/genv/genv.lock.json` is machine-local. v8 locks may record `target` and `goos`. A foreign lock is refused; use `genv apply --force-new-lock` to back it up and start fresh. Never commit locks.

Each applied `files.links[]` (`link` / `managed-link`) and `files.templates[]` entry may record `contentHash` (`sha256:<hex>` of the link source or rendered template). `genv files adopt` records the same hash after a successful seed+link. `genv status --files` reports `drifted` when the live hash differs. Older locks omit the field and stay topology-only. Apply refreshes the hash after a successful link/template op and never reverts the body.

Guide: [docs/multi-machine.md](docs/multi-machine.md).

## v7 — PowerShell

Aliases/functions may set `"shell": "powershell"`. Omitted `shell` stays POSIX-oriented. On native Windows, apply prefers `pwsh`, else Windows PowerShell, for `.ps1` fragments and hooks. See [docs/windows-install.md](docs/windows-install.md).

## v6 — updates and hooks

### `updates`

- `enabled`, `interval` (positive Go duration, e.g. `"24h"`)
- `autoApply` (default false — check/log/notify only)
- `notify`
- `onlyManagers`, `skipManagers`, `only`, `skip` — same filters as `genv upgrade`’s tracked-package step

Tracked packages only; the checker never plans or applies OS vendor or firmware updates. Use `genv upgrade` for those. Not a remote SSH updater. The scheduled `__run-once` job is non-interactive: it never prompts for sudo/UAC. Packages that need elevation are skipped and logged; run `genv upgrade` from a terminal (or an elevated Windows session) to apply them.

`genv updates start` registers the checker with systemd --user (Linux),
launchd (macOS), or Task Scheduler / `schtasks` (Windows).

### Hooks

Phases: `preApply` / `postApply`, `preAdd` / `postAdd`, `preRemove` / `postRemove`, `preUpgrade` / `postUpgrade` (v5 also had `preUpgrade` / `postApply` / `postUpgrade`).

Each hook is `{ "command": "..." }` or `{ "file": "..." }` (exactly one), optional `name`, optional `continueOnError`, optional `host` on v1–v7. `file` and `command` must not contain newlines — hooks run through `sh -c`, PowerShell `-Command`, or `cmd /C`, so a newline would smuggle a second command past what the author wrote.

A relative `file` resolves against the spec directory (`--source-root` overrides), which is where `genv pull` and `genv export` place bundled hook scripts, so hooks work regardless of the working directory — including under the scheduled updates worker. `~/…` and `$VAR` prefixes are expanded; `~name` is another user's home and is **not** expanded.

Context env: `GENV_EVENT`, `GENV_PHASE`, `GENV_HOST`, `GENV_PROFILE`, `GENV_SPEC_FILE`, `GENV_SPEC_DIR`, `GENV_LOCK_FILE`, `GENV_YES`, `GENV_INSTALLED`, `GENV_REMOVED`, `GENV_UPGRADED`, `GENV_FAILED`, `GENV_SKIPPED`.

Hooks are expected to **check-then-act** and short-circuit when the work is already done. After a successful run they print a status line on stdout or stderr:

```
GENV_HOOK_STATUS=changed
GENV_HOOK_STATUS=skipped
```

Non-zero exit is `error` (the status line is ignored). Exit `0` without a `GENV_HOOK_STATUS` line is treated as `changed`, so existing exit-only hooks stay visible as work rather than a silent no-op. `GENV_HOOK_STATUS=error` on exit `0` is ignored.

`continueOnError: true` reports a non-zero hook and continues the phase instead of failing the command. After a phase, genv prints a hook summary in run order: `name` (or the first 40 characters of the command), status (`changed`, `skipped (no-op)`, or `error`), exit code, and duration.

Hooks run as the current user and are arbitrary code by design — treat the spec as trusted.

## v5 — files, hooks, host, repo

### `files`

- `links[]` — `source`, `target`, `mode` (`link` default, `managed-link`, or `merge-dir`), optional `host`, `backup` (`backup: true` replaces that entry's mismatched regular file without `--force`), optional `perm`
  - `mode` is the link kind, not a Unix mode
  - `merge-dir` symlinks each file from source into target so multiple records can layer into one directory
  - `perm` is an octal string (`0644`, `0700`); apply chmods the source file (managed-link) or source directory (merge-dir)
- `templates[]` — copy after `__HOME__` / `__USER__` / `__HOST__` / `__OS__` / `__ARCH__` rendering; optional `backup` (`backup: true` replaces that entry's mismatched regular file without `--force`), optional `perm` on the rendered file
- `dirs[]` — ensure directories exist; optional `perm` on the directory
- `perm` is 3 or 4 octal digits. Apply sets it after creating the entry; a second apply is a no-op when the mode already matches. `genv status` reports `perm-mismatch`. `mode` on `dirs[]` is rejected (unknown field).

### `repo`

- `url` (required), `ref` (optional) — used by `genv pull`

## v4 — services

Map of name → one backend:

- `{ start, stop, restart, status }` argv arrays
- `brew_formula` (mutually exclusive with `start`)
- `launchd: { "plist": "agents/com.example.agent.plist" }` — LaunchAgent template, rendered like `files.templates[]` (`__HOME__` / `__USER__` / `__HOST__` / `__OS__` / `__ARCH__`)
- `systemd: { "unit": "units/foo.service" }` — systemd --user unit template, same rendering
- `scheduled_task: { "action": "C:\\Program Files\\Acme\\agent.exe", ... }` — Windows Task Scheduler task, rendered by genv (no template file)

`launchd`, `systemd` and `scheduled_task` may coexist on one service (portable defaults). They are mutually exclusive with `start` and `brew_formula`.

#### `scheduled_task`

The declarative Windows backend. genv renders the task definition, registers it with `schtasks /Create /XML`, and deletes it again when the service leaves the spec.

| Field | Required | Notes |
| --- | --- | --- |
| `action` | yes | Absolute path. Task Scheduler does not search `PATH`, so a bare name would never resolve. Validated with Windows rules, not host rules, so a spec checks out on macOS and Linux CI too. |
| `args` | no | Passed to `action` verbatim, each quoted individually. |
| `trigger` | no | `logon` (default), `boot`, `daily`, `weekly`. |
| `at` | for `daily` / `weekly` | `HH:MM`, zero-padded, 24-hour. Rejected on the other triggers rather than silently ignored. |
| `day_of_week` | for `weekly` | `monday`..`sunday`. |
| `principal` | no | `user` (default) or `system`. |
| `description` | no | Lands in the task's registration info. |
| `restart_on_failure` | no | Re-runs a task that exits non-zero. |
| `retry_interval` | no | ISO 8601 duration (`PT10M`). Only with `restart_on_failure`. |
| `execution_time_limit` | no | ISO 8601 duration (`PT1H`). |

```json
"acme-agent": {
  "scheduled_task": {
    "action": "C:\\Program Files\\Acme\\agent.exe",
    "args": ["--serve", "--config", "C:\\etc\\acme.toml"],
    "trigger": "daily",
    "at": "03:00",
    "restart_on_failure": true,
    "retry_interval": "PT10M"
  }
}
```

The `user` principal registers unelevated, with `InteractiveToken` + `LeastPrivilege`, so a normal `genv apply` works without elevation. `principal: "system"` needs an elevated shell; genv says so when Task Scheduler denies the registration. Removing the service from the spec unregisters the task and deletes its generated `.cmd`, `.vbs` and `.xml` files under the genv state directory.

The action runs through a `.cmd` wrapper launched by a windowless `wscript.exe`, so no console window flashes on each trigger, and the wrapper propagates the exit code — which is what `restart_on_failure` keys on. `genv service status <name>` reads `schtasks /Query /FO LIST /V`; a `daily` or `logon` task is normally `Ready` rather than `Running`, which is not drift. A task registered outside genv, or deleted by hand, is re-registered on the next apply.

On a non-Windows host a declared `scheduled_task` is skipped, not an error, so one spec can target several platforms.

`genv apply` writes the rendered plist to `~/Library/LaunchAgents/<Label>.plist` and `launchctl bootstrap`s `gui/$UID` when the job is not loaded. A content change boots the job out and bootstraps again. `genv service status <name>` uses `launchctl print gui/$UID/<Label>`. Removing the service from the spec boots it out and deletes the plist.

On Linux, apply writes `~/.config/systemd/user/<basename>.service`, then `systemctl --user daemon-reload` and `enable --now`. Content changes restart the unit. Status uses `systemctl --user is-active`. Removal stops, disables, and deletes the unit.

Relative template paths resolve against the spec directory (or `repo.url` when set), same as `files.templates`.

## v3 / v2 / v1

- v3: `shell` with `aliases`, `functions`, `source`
- v2: `env` map of `{ value, sensitive? }`
- v1: `packages[]` with `id`, optional `version`, `prefer`, `managers`

Shell function bodies are wrapped unquoted in a generated function
(`name() { … }` / `function <name> { … }`), so a body must be plain text:
braces, `;`, `|`, `&`, backticks, `$`, `<`, `>`, `(`, `)` and newlines are
rejected at validation rather than allowed to close the wrapper.

Alias values follow the same rule only for `shell: "powershell"`, because genv
emits that form unquoted — PowerShell has no POSIX `alias` builtin, so an alias
value is the command its function body runs. POSIX aliases are emitted
single-quoted (`alias g='for i in {1..10}'`), which already contains every
metacharacter, so they are not restricted.

## Manager resolution

`prefer` and `managers` accept registered manager IDs (see README table) or a v8 `adapters` name. Without an explicit selection, fallback uses **system** package managers only. Language, toolchain, and plugin managers are explicit-only.

On v1-v8, `external` is a track-only manager for apps with an official installer
(not winget/scoop). Apply records them when the binary is on PATH; it never
installs them. Schema v9 permits an `external` recipe on a package with
`prefer: "external"`; the recipe declares local version detection, a GitHub
Release or structured HTTP source, platform artifacts, installation type, and
verification policy. Recipes without a verifier must explicitly set
`allowUnverified: true`.

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

### Managed external releases (v9)

Schema v9 keeps the v8 `defaults` and `targets.*` model and adds an optional
`external` recipe to packages whose `prefer` is `external`. A recipe contains:

- `detect`: explicit version command and one-capture-group `versionRegex`
- `source`: `githubRelease` or structured JSON/text `httpRelease` metadata
- `platforms`: OS/architecture/libc selectors, one artifact, and one install recipe
- `verify`: required verifier chain unless `allowUnverified` is explicitly true

Supported installs are `direct`, explicit-file `archive` mappings for ZIP and
tar/gzip/xz/zstd, and downloaded `script` installers using `sh`, `bash`, `pwsh`,
or `powershell`. Script argv and environment values use only `{version}`, `{tag}`,
`{os}`, `{arch}`, `{script}`, and `{destination}` placeholders. Script removals
require explicit uninstall argv.

Supported verification methods are GitHub asset digests, literal/metadata
SHA-256, checksum files, Sigstore bundles with pinned issuer/identity, minisign,
and OpenPGP with a pinned full fingerprint. HTTP transport requires
`allowInsecureHTTP`; it is never scheduler-eligible. Unverified actions always
require a dedicated interactive acknowledgement, even with `--yes`. Verified
scripts additionally require `allowBackgroundExecution` before unattended use.

The machine-local lock records release, artifact, verifier, recipe, ownership,
and installed-path digests. Status detects version, recipe, and owned-file drift.
Removal refuses modified owned files and never guesses files created by scripts.

`genv apply` consults a live inventory (`ListInstalled` per available manager) and adopts already-installed packages into the lock instead of reinstalling. `genv upgrade` remains the only upgrade path. Apply `--timeout` defaults to 10m. `--skip-packages` applies env/shell/files/services without inventorying or planning packages. `--source-root <dir>` resolves `files.links` / `files.templates` and service `launchd.plist` / `systemd.unit` sources against that directory instead of the spec file directory (lock, env, and shell paths stay where `--file` / `--lock-file` / `--state-dir` put them).

Apply injects one source line per rc file (`.zshrc` / `.bashrc`, and the PowerShell profile on Windows) inside a `# genv env` block:

```sh
if [ -r "$HOME/.config/genv/env.sh" ]; then . "$HOME/.config/genv/env.sh"; fi
```

Two properties matter. It is **guarded**, because the fragments are rendered output rather than tracked files and are legitimately absent before the first apply — an unguarded source printed `No such file or directory` on every shell start. The guard is an `if`, not `[ -r x ] && . x`, because the `&&` form leaves a false exit status when the fragment is missing. And it is **`$HOME`-relative** (`$env:USERPROFILE` on Windows), so a single committed rc template is correct on every host. A fragment outside the home directory — a custom `--state-dir` — keeps its absolute path but is still guarded.

genv recognises its own block by marker and **replaces** it rather than appending, so a template carrying another host's path is corrected in place instead of accumulating a second block. Rc injection only happens when the state directory is the default config directory; a custom `--state-dir` leaves your rc files alone.

`genv status` probes live managers by default (`--offline` is lock-only). Unlocked but installed packages are `present`. A package that is in both the spec and the lock but whose lock entry records no installed version is reported as `unknown` **only when a live inventory positively contradicts the lock** — the manager was inventoried and does not list the package. The entry's presence is not evidence of an install (that is the state a failed install leaves behind), but a missing version alone proves nothing either: many managers never report a version, and a real install through one of them produces the same version-less entry. So a manager that could not be inventoried, or that does list the package, leaves the entry as `ok`, and `genv status --offline` is always quiet. Apply re-queues exactly the entries `unknown` reports. The version column keeps its own independent meaning — `*` for no spec constraint and no recorded version, `?` for a constraint with no recorded version.


## v10 — local modules

A module is a second JSON document that declares part of the environment.
`genv.json` registers modules by name and each target bundle selects them with
`useModules`; composition unions the selections of every contributor.

```json
{
  "schemaVersion": "10",
  "modules": {
    "base": "modules/base.json",
    "dev":  "modules/dev.json"
  },
  "targets": {
    "macos": {
      "useModules": ["dev"],
      "packages": [{ "id": "ghostty" }]
    }
  }
}
```

```json
// modules/base.json
{
  "schemaVersion": "10",
  "defaults": {
    "packages": [{ "id": "jq" }],
    "env": { "EDITOR": { "value": "nvim" } }
  }
}

// modules/dev.json
{
  "schemaVersion": "10",
  "requiresModules": ["base"],
  "defaults": { "packages": [{ "id": "ripgrep" }] }
}
```

### Rules

- **Modules are local and trusted.** Paths resolve inside the spec's directory;
  a module that escapes it is refused. Module documents are not fetched and not
  signed — they are files you already have on disk.
- **`useModules` is additive.** Selecting `"dev"` also selects `"base"` because
  dev requires it. There is no way to un-select a dependency.
- **Modules compose per target.** Each module contributes its `defaults` and, when
  the active target exists, its `targets.<id>` overlay.
- **The root is a contributor, not an override.** A resource the root and a module
  declare *identically* is kept once, and every contributor is recorded as an
  owner. A resource they declare *differently* is an error naming both origins.
- **No override syntax.** To change what a module declares, edit the module or
  write a different module. A root entry cannot silently win.
- **Contributor-local merge rules are v8 rules.** Within one document, a target
  array replaces the defaults array and maps merge, exactly as in v8. Across
  documents, arrays union by identity.

  That replacement rule has a sharp edge worth stating plainly: a module that
  declares `defaults.packages` *and* `targets.macos.packages` does **not** get
  both. The target array wins and the default packages disappear from the
  composition — silently, because this is the behaviour every v8 spec already
  has. Put a module's packages in one place, or repeat them in the overlay.
  `genv explain package <id> --target macos` will tell you where a surviving
  declaration actually lives.
- **Module-owned resources are read-only from the CLI.** `add`, `remove`,
  `disown`, `adopt`, `env`, `shell`, `service`, and `files adopt` refuse with the
  owning module named, before running any subprocess or writing anything.
- **`genv validate` loads every registered module**, including ones no target
  selects, because a broken registration is a mistake worth reporting. Other
  commands load only the selection closure.
- **`genv migrate` refuses v10.** Converting is not needed and would lose the
  registry.

### Inspecting a composition

`genv config --target macos` prints what the target actually gets; `genv explain
package jq --target macos` prints who declares it and whether a command may
change it. `genv export` flattens the composition into a single-target snapshot
and records the materialized modules in `report.json`.

## v10 — service dependencies and change triggers

```json
{
  "schemaVersion": "10",
  "targets": {
    "macos": {
      "services": {
        "api": {
          "start": ["api-server"],
          "requires": ["db"],
          "watch": ["postgres"],
          "restart_policy": "ifRunning",
          "health_check": {
            "command": ["curl", "-fsS", "localhost:8080/health"],
            "timeout": "30s",
            "interval": "1s",
            "allow_background": false
          }
        }
      }
    }
  }
}
```

- `requires` — services this one starts **after** and stops **before**. It is an
  ordering constraint, not a change trigger. A cycle is a validation-time error
  naming the services involved, as is a `requires` entry no declared service
  provides.
- `watch` — the resources whose change should restart this service: **tracked
  packages**, whose installed version moved during an upgrade, and **managed
  file destinations**, which `genv apply` creates or rewrites. An entry that is
  neither is a validation error naming the service and the entry, because nothing
  could ever report it as changed and the service would silently never restart.
  Entries may carry the explicit `watch:` prefix, which is how a package and a
  file of the same name are told apart.

  A file entry matches the `target` of a `files.links`, `files.templates`, or
  `files.dirs` entry, written either literally or with `~`/`$VAR` expanded.
  Watching *another service* is still not a trigger: restarting because a peer
  changed is a different question from restarting because a resource changed,
  and answering it safely needs more than a package or file digest.
- `restart_policy` — `never` (default) leaves a stopped service stopped;
  `ifRunning` starts one, because the change is what it was waiting for.
- `health_check` — a readiness probe run **only after** an authorized start or
  restart. It never runs from `status`, `apply --dry-run`, `upgrade --dry-run`,
  or any planning path: a plan that runs commands is not a plan. `timeout`
  defaults to 30s and `interval` to 1s; both must be positive. `allow_background`
  must be `true` before the unattended updates worker may use the check.
- All four fields are **refused on v1–v9** rather than ignored. A silently dropped
  `watch` means a service that quietly never restarts.

### What upgrade does about it

| Situation | Outcome |
| --------- | ------- |
| Upgraded, installed version moved | service restarted (or started, under `ifRunning`), then health checked |
| `apply` created or rewrote a watched file | same: restarted, then health checked |
| Upgraded, installed version did not move | no restart — the upgrade was a no-op |
| Version could not be established before or after | **deferred**: nothing is restarted and nothing is claimed |
| Watched service stopped, policy `never` | skipped, and said so |
| Restart or readiness failed | reported, exit non-zero, and the pending record is kept |
| Pending record could not be written | restart proceeds, and the gap is reported — refusing would leave the old binary running |
| Dependency did not come back | the dependent is **not started**, and the failure names it |

The phase runs in `genv upgrade`, in `genv upgrade --json`, in `genv apply`, in
`genv apply --json`, and in the unattended `genv updates` worker. Every JSON
envelope reports the outcome under a `services` key — an output flag never
changes what the command does.

On `apply` the phase runs only when the apply itself succeeded. Restarting on top
of a half-applied environment would trade one broken state for another.

Restarts are ordered: services stop in reverse dependency order (a service goes
down before the ones that depend on it) and start in dependency order. A service
whose dependency failed to come back is not started, so nothing is reported as
successfully started against something that is down.

The pending record is written **before** a service is stopped and cleared only
after the action and its readiness check both succeed. An interrupted run
therefore leaves evidence: the next run reports the unconfirmed change instead of
assuming it worked. The unattended worker defers upgrades whose watched service
cannot be restarted *and verified* without a human, before touching any package,
and restarts the ones it can once the upgrade lands.

## Profiles

Named profiles live in `profiles/<name>.json` beside the base spec on schema v1–v7. `genv profile switch` merges the profile over the base, applies, and stores `activeProfile` in the lock. Schema v8 refuses named profiles — use `defaults` plus `targets.*` instead.
