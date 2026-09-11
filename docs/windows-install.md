# Windows install and bootstrap

Native Windows is the `windows` target (not WSL). Use a separate Linux install inside WSL — see [wsl2-install.md](wsl2-install.md).

## 1. Install genv

**Scoop** (self-hosted bucket, not Scoop extras):

```powershell
scoop bucket add ks1686 https://github.com/ks1686/scoop-bucket
scoop install genv
```

`scoop install genv` needs a `genv.json` manifest on that bucket, which the
first stable tag after merge uploads. Until then, use the zip:

```powershell
Invoke-WebRequest -Uri https://github.com/ks1686/genv/releases/latest/download/genv_4.1.0_windows_amd64.zip -OutFile genv.zip
Expand-Archive genv.zip -DestinationPath .\genv
New-Item -ItemType Directory -Force $HOME\bin
Copy-Item .\genv\genv.exe $HOME\bin\genv.exe
# ensure $HOME\bin is on PATH
genv version
```

Update the version segment when a newer release is current.

winget and Chocolatey installers for genv itself (`winget install ks1686.genv`,
`choco install genv`) are not published (GoReleaser Pro). Those managers remain
available for **packages** genv manages.

## 2. Install a Windows package manager

| Manager | Notes |
| ------- | ----- |
| `winget` | Built into recent Windows; good default |
| `scoop` | User-local installs |
| `choco` | Often needs an elevated shell |

`bun` and `uv` global tools also work when on `PATH`.

## 3. Schema v8 config

```powershell
New-Item -ItemType Directory -Force $HOME\.config\genv
@'
{
  "schemaVersion": "8",
  "defaults": {
    "env": {
      "EDITOR": { "value": "nvim" }
    }
  },
  "targets": {
    "windows": {
      "packages": [
        {
          "id": "git",
          "prefer": "winget",
          "managers": {
            "winget": "Git.Git",
            "scoop": "git",
            "choco": "git"
          }
        }
      ],
      "shell": {
        "aliases": {
          "ll": { "value": "Get-ChildItem", "shell": "powershell" }
        }
      }
    }
  }
}
'@ | Set-Content -Encoding UTF8 $HOME\.config\genv\genv.json
```

Classification selects `windows`. Pass `--target windows` when explicit is better.

## 4. Apply

```powershell
genv validate
genv apply --dry-run
genv apply --yes
genv list
genv status
```

## 5. OS upgrades (`genv upgrade`)

After tracked packages, `genv upgrade` runs a **system** step that drives the
Windows Update Agent COM API through `pwsh` (else `powershell` /
`powershell.exe`). This is not `winget` — winget upgrades packages, not the OS.

The COM install often needs an **elevated** PowerShell session (Administrator).
genv does **not** auto-elevate. If the step fails with a WUA `ResultCode`
(for example `4` Failed), re-run `genv upgrade` from elevated PowerShell, or
finish updates in **Settings → Windows Update**. A reboot message is printed
when WUA reports `RebootRequired`. `SucceededWithErrors` without any
per-update Failed result is treated as soft success with a warning.

Firmware updates stay skipped on Windows (vendor-specific).

## 6. Updates checker

`genv updates start` registers a per-user Task Scheduler job (`schtasks`)
that runs `genv updates __run-once` at logon and then on `updates.interval`.
`genv updates status` / `stop` talk to the same task. The job is started by
the scheduler service (not as a child of the current shell) so an OpenSSH
session teardown cannot kill it.

Registration is the **unelevated current-user** path: the task XML pins
`UserId` to your account with `InteractiveToken` + `LeastPrivilege`. An
elevated shell is not required in the normal case. If create fails with
Access denied, retry from elevated PowerShell, check Task Scheduler / Group
Policy permissions, or delete a foreign-owned `genv-updates` task and retry.

```powershell
genv updates start
genv updates status
```

Until this is registered, UniGetUI or `genv upgrade` still work as a manual
workaround.

## PowerShell env, shell, and hooks

On native Windows, apply prefers **PowerShell 7+ (`pwsh`)**, then Windows PowerShell 5.1:

- Shared `env` → `%USERPROFILE%\.config\genv\env.ps1` + CurrentUser CurrentHost profile inject
- Aliases/functions with `"shell": "powershell"` (schema **v7+**) → `shell.ps1`
- Omitted `shell` stays POSIX-only (also written if a POSIX shell/rc is already present)
- Missing PowerShell engine → warn and skip `.ps1` paths; apply continues
- Hooks: `pwsh|powershell -NoProfile -Command` / `-File`; else `cmd /C` with a warning

```powershell
genv completion powershell
genv completion install powershell
# then: . $HOME\.config\genv\completions\genv.ps1
```

## Limitations

- `.ps1` profile writes are Windows-only (macOS/Linux never write them even if `pwsh` exists).
- Symlinks may need Developer Mode or an elevated shell.
- Native Windows and WSL2 are different targets and binaries — model them as `targets.windows` vs `targets.ubuntu` / `targets.wsl-arch` in one v8 spec (see [multi-machine.md](multi-machine.md)). Do **not** rely on legacy `host` selectors for new configs.
- Schema v9 managed external recipes support Windows direct executables, ZIP archives, and downloaded `pwsh`/`powershell` scripts when the artifact is verified. PowerShell availability is checked before execution, and scheduler execution additionally requires `allowBackgroundExecution`. See [SCHEMA.md](../SCHEMA.md#managed-external-releases-v9).
