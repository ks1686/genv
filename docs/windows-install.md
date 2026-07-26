# Windows Install and Bootstrap Guide

---

## Step 1 - Download genv

Native Windows is detected as the `windows` host class. Download the latest
Windows archive from the [Releases](https://github.com/ks1686/genv/releases/latest)
page:

```powershell
Invoke-WebRequest -Uri https://github.com/ks1686/genv/releases/latest/download/genv_windows_amd64.zip -OutFile genv.zip
Expand-Archive genv.zip -DestinationPath .\genv
```

Move `genv.exe` somewhere on your `PATH`, for example:

```powershell
New-Item -ItemType Directory -Force $HOME\bin
Copy-Item .\genv\genv.exe $HOME\bin\genv.exe
```

Verify:

```powershell
genv version
```

---

## Step 2 - Install a Windows package manager

genv can use any supported manager it finds on `PATH`:

| Manager | Notes |
| --- | --- |
| `winget` | Built into recent Windows releases; good default for Microsoft Store and WinGet packages. |
| `scoop` | User-local installs, usually no Administrator shell required. |
| `choco` | Chocolatey packages; many install/upgrade operations require an elevated shell. |

`bun` and `uv` global tools also work on native Windows when those CLIs are on
`PATH`.

---

## Step 3 - Create your config

Create `%USERPROFILE%\.config\genv\genv.json`:

```powershell
New-Item -ItemType Directory -Force $HOME\.config\genv
@'
{
  "schemaVersion": "7",
  "packages": [
    {
      "id": "Git.Git",
      "prefer": "winget",
      "host": "windows"
    }
  ]
}
'@ | Set-Content -Encoding UTF8 $HOME\.config\genv\genv.json
```

If you prefer Scoop or Chocolatey, use that manager explicitly:

```json
{
  "schemaVersion": "5",
  "packages": [
    {
      "id": "git",
      "prefer": "scoop",
      "managers": {
        "winget": "Git.Git",
        "scoop": "git",
        "choco": "git"
      },
      "host": "windows"
    }
  ]
}
```

---

## Step 4 - Preview and apply

```powershell
genv validate
genv apply --dry-run
genv apply --yes
```

Confirm genv tracked the result:

```powershell
genv list
genv status
```

---

## Native Windows shell, env, and hooks

On native Windows, `genv apply` prefers **PowerShell 7+ (`pwsh`)**, then
Windows PowerShell 5.1 (`powershell` / `powershell.exe`):

- Shared `env` values are written to `%USERPROFILE%\.config\genv\env.ps1` and
  injected into the CurrentUser CurrentHost profile.
- Shell aliases/functions with `"shell": "powershell"` (schema **v7**) go to
  `shell.ps1`. Entries with an omitted `shell` target stay POSIX-only.
- If a POSIX shell/rc is already present (e.g. Git Bash), genv also maintains
  `env.sh` / `shell.sh`.
- If no PowerShell engine is on `PATH`, genv warns and skips `.ps1` profiles
  without failing the whole apply.

Hooks on Windows run as `pwsh|powershell -NoProfile -Command …` (or `-File`
for script hooks). Without an engine they fall back to `cmd /C` with a warning.

Completions:

```powershell
genv completion powershell
genv completion install powershell
# then: . $HOME\.config\genv\completions\genv.ps1
```

---

## Native Windows limitations

- PowerShell apply targets are Windows-only; macOS/Linux never write `.ps1`
  profiles even if `pwsh` is installed.
- File links use Windows symlinks. If link creation fails, enable Developer Mode
  or run the shell as Administrator.
- WSL2 is separate from native Windows: run the Linux binary inside WSL2 and use
  Linux managers there. See [WSL2 Install and Bootstrap Guide](wsl2-install.md).

---

**Focus tip:** Native Windows and WSL2 use different binaries, host classes, and
package managers. Keep their `genv.json` records separated with `host` selectors
when one spec targets both environments.
