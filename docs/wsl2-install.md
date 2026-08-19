# WSL2 Install and Bootstrap Guide

---

## Step 1 — Open PowerShell as Admin

- Hit `Windows key`, type `PowerShell`
- Right-click → **Run as Administrator**

---

## Step 2 — Install WSL2

```powershell
wsl --install
```

- If it asks to reboot → reboot, then come back here

---

## Step 3 — Open Ubuntu

- Hit `Windows key`, type `Ubuntu`, open it
- Wait for it to finish first-time setup (asks for username/password)

---

## Step 4 — Install genv

Download the latest Linux binary from the [Releases](https://github.com/ks1686/genv/releases/latest) page:

```bash
curl -Lo genv.tar.gz https://github.com/ks1686/genv/releases/latest/download/genv_4.0.13_linux_amd64.tar.gz
tar -xzf genv.tar.gz
sudo mv genv /usr/local/bin/
rm genv.tar.gz
```

Update the version segment in the URL when a newer release is current.

Verify:

```bash
genv version
```

---

## Step 5 — Create your config

For schema v8 configs, WSL2 is classified by its Linux userland:

- Ubuntu or Ubuntu-like WSL2 uses the `ubuntu` target.
- Arch-based WSL2 (for example ArchWSL) uses the `wsl-arch` target.
- There is no blanket inheritance from native `arch` to WSL2. Put shared entries
  in `defaults`, then add distro-specific package managers in `targets.ubuntu`
  or `targets.wsl-arch`.

Manager availability is detected separately from the target.

**Ubuntu / non-Arch WSL2** prefers native `apt` when it is available, then
`snap` or `linuxbrew` (Homebrew on Linux):

```bash
mkdir -p ~/.config/genv && cat > ~/.config/genv/genv.json << 'EOF'
{
  "schemaVersion": "8",
  "targets": {
    "ubuntu": {
      "packages": [
        {
          "id": "jq",
          "prefer": "apt",
          "managers": {
            "apt": "jq",
            "snap": "jq",
            "linuxbrew": "jq"
          }
        }
      ]
    }
  }
}
EOF
```

Apply with `genv apply --target ubuntu`, or rely on automatic classification
when this WSL distro is Ubuntu-like.

> Snap needs `snapd`, which relies on systemd being enabled in WSL2. If that is
> not set up, install [Homebrew on Linux](https://docs.brew.sh/Homebrew-on-Linux)
> and use `"prefer": "linuxbrew"` instead.

**Arch-based WSL2** (for example ArchWSL) supports `pacman` plus the `paru`/`yay`
AUR helpers. Use the `wsl-arch` target, not a native `arch` bucket:

```bash
mkdir -p ~/.config/genv && cat > ~/.config/genv/genv.json << 'EOF'
{
  "schemaVersion": "8",
  "targets": {
    "wsl-arch": {
      "packages": [
        {
          "id": "jq",
          "prefer": "pacman"
        }
      ]
    }
  }
}
EOF
```

Apply with `genv apply --target wsl-arch`, or rely on automatic classification
when this WSL distro is Arch-like.

If you share one file between native Arch, Ubuntu WSL2, and Arch WSL2, keep the
common parts in `defaults`:

```json
{
  "schemaVersion": "8",
  "defaults": {
    "env": {
      "EDITOR": { "value": "nvim" }
    }
  },
  "targets": {
    "arch": {
      "packages": [{ "id": "jq", "prefer": "pacman" }]
    },
    "ubuntu": {
      "packages": [{ "id": "jq", "prefer": "apt" }]
    },
    "wsl-arch": {
      "packages": [{ "id": "jq", "prefer": "pacman" }]
    }
  }
}
```

Legacy schema v1-v7 files can still use per-record `host`, but `wsl2` no longer
inherits `arch`. Migrate them with:

```bash
genv migrate --file ~/.config/genv/genv.json
genv migrate --file ~/.config/genv/genv.json --write
```

Review any warning that says bare `wsl2` entries were placed in `targets.wsl-arch`;
Ubuntu WSL users should move those records to `targets.ubuntu`.

---

## Step 6 — Test `genv apply`

```bash
genv apply --dry-run   # preview what will happen
genv apply             # apply it
```

Confirm it installed via a Linux manager (not a Windows binary):

```bash
jq --version
```

Confirm genv tracked it:

```bash
genv list
```

- `apply` output should show `apt`, `snap`, or `linuxbrew` as the adapter (`pacman`/`paru`/`yay` on Arch-based WSL2)
- `jq --version` should print a version number ✅
- `genv list` should show `jq` as an installed package ✅

---

## Step 7 — Sanity check: confirm no Windows path leakage

```bash
echo $PATH
```

- You may see `/mnt/c/...` paths — that's normal for WSL2
- genv strips these automatically so Windows binaries don't shadow Linux ones

---

## Step 8 — Done!

Your `genv.json` lives at `~/.config/genv/genv.json`. Add more packages with:

```bash
genv add <package>
```

Or bulk-adopt everything already installed:

```bash
genv scan
```

Then run `genv apply` to sync after editing the spec directly.

---

**Focus tip:** Steps 1–3 are in Windows. Steps 4–8 are inside the Ubuntu terminal. Don't mix them up.
