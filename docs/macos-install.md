# macOS Install and Bootstrap Guide

---

## Step 1 — Install Homebrew (if not already installed)

Open **Terminal** and run:

```bash
/bin/bash -c "$(curl -fsSL https://raw.githubusercontent.com/Homebrew/install/HEAD/install.sh)"
```

- Follow the prompts — it may ask for your password
- If Homebrew is already installed, skip this step

Verify:

```bash
brew --version
```

---

## Step 2 — Install gpm

```bash
brew tap ks1686/tap
brew install gpm
```

Verify:

```bash
gpm version
```

---

## Step 3 — Create your config

```bash
mkdir -p ~/.config/gpm && cat > ~/.config/gpm/gpm.json << 'EOF'
{
  "schemaVersion": "1",
  "packages": []
}
EOF
```

---

## Step 4 — Add your first package

```bash
gpm add jq
```

- This adds `jq` to your `gpm.json` and installs it immediately via `brew`

Verify:

```bash
jq --version
gpm list
```

- `jq --version` should print a version number ✅
- `gpm list` should show `jq` as a tracked package ✅

---

## Step 5 — Preview and apply a full spec

Edit your spec to add more packages:

```bash
gpm edit
```

Then preview what will change before applying:

```bash
gpm apply --dry-run
```

When ready:

```bash
gpm apply
```

---

## Step 6 — Done!

Your `gpm.json` lives at `~/.config/gpm/gpm.json`. Commit it to your dotfiles repo and run `gpm apply` on any new Mac to reproduce your environment.

---

## Known limitations on macOS

- **Homebrew install time** — `brew install` for large packages (e.g. gcc, llvm) can take several minutes. This is a Homebrew limitation, not gpm's.
- **Cask vs formula resolution** — Some packages exist as both a cask and a formula (e.g. `firefox`). gpm currently treats Homebrew as a single `brew` manager and defaults to formulae, relying on Homebrew's own resolution rules. If you specifically need the cask variant, install it manually with `brew install --cask <name>` or manage that application outside of gpm for now.

- **Apple Silicon vs Intel** — Homebrew installs to `/opt/homebrew` on Apple Silicon and `/usr/local` on Intel. gpm handles both automatically via PATH detection.
- **macports** — If you use MacPorts instead of Homebrew, set `"prefer": "macports"` on packages where you want it used.

---

**Focus tip:** Steps 1–2 are one-time setup. Steps 3–5 are what you repeat on each new machine.
