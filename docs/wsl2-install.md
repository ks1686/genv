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

## Step 4 — Install Go

The version of Go in apt is often outdated. Install the official binary instead:

```bash
cd /tmp
curl -Lo go.tar.gz https://go.dev/dl/go1.22.3.linux-amd64.tar.gz
sudo tar -C /usr/local -xzf go.tar.gz
echo 'export PATH=$PATH:/usr/local/go/bin' >> ~/.bashrc
source ~/.bashrc
```

Verify:

```bash
go version
```

---

## Step 5 — Clone the repo

```bash
git clone https://github.com/ks1686/gpm.git
cd gpm
```

---

## Step 6 — Run the integration tests

```bash
go test -tags integration ./internal/adapter/
```

- All tests pass? ✅ Continue
- Something fails? ❌ Screenshot it, message me

---

## Step 7 — Create your config

```bash
mkdir -p ~/.config/gpm && cat > ~/.config/gpm/gpm.json << 'EOF'
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

## Step 8 — Test `gpm apply`

From inside the repo directory:

```bash
go run . apply
```

Confirm it installed via apt (not a Windows binary):

```bash
jq --version
```

Confirm gpm tracked it:

```bash
go run . list
```

- `apply` output should show `apt` as the adapter ✅
- `jq --version` should print a version number ✅
- `list` should show `jq` as an installed package ✅

---

## Step 9 — Sanity check: confirm no Windows path leakage

```bash
echo $PATH
```

- You should see `/mnt/c/...` paths — that's normal for WSL2
- gpm strips these automatically so Windows binaries don't shadow Linux ones

---

## Step 10 — Done!

Your `gpm.json` lives at `~/.config/gpm/gpm.json`. Add more packages with:

```bash
go run . add <package>
```

Or edit the spec directly:

```bash
go run . edit
```

Then run `go run . apply` to sync.

---

**Focus tip:** Steps 1–3 are in Windows. Steps 4–10 are inside the Ubuntu terminal. Don't mix them up.
