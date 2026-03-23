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
curl -Lo genv.tar.gz https://github.com/ks1686/genv/releases/latest/download/genv_linux_amd64.tar.gz
tar -xzf genv.tar.gz
sudo mv genv /usr/local/bin/
rm genv.tar.gz
```

Verify:

```bash
genv version
```

---

## Step 5 — Create your config

```bash
mkdir -p ~/.config/genv && cat > ~/.config/genv/genv.json << 'EOF'
{
  "schemaVersion": "1",
  "packages": [
    {
      "id": "jq",
      "prefer": "apt"
    }
  ]
}
EOF
```

---

## Step 6 — Test `genv apply`

```bash
genv apply --dry-run   # preview what will happen
genv apply             # apply it
```

Confirm it installed via apt (not a Windows binary):

```bash
jq --version
```

Confirm genv tracked it:

```bash
genv list
```

- `apply` output should show `apt` as the adapter ✅
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
