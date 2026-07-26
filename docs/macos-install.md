# macOS install and bootstrap

## 1. Homebrew

```bash
/bin/bash -c "$(curl -fsSL https://raw.githubusercontent.com/Homebrew/install/HEAD/install.sh)"
brew --version
```

Skip if Homebrew is already installed.

## 2. Install genv

```bash
brew tap ks1686/tap
brew install genv
genv version
```

## 3. Create a schema v8 config

```bash
mkdir -p ~/.config/genv
cat > ~/.config/genv/genv.json << 'EOF'
{
  "schemaVersion": "8",
  "defaults": {
    "env": {
      "EDITOR": { "value": "nvim" }
    }
  },
  "targets": {
    "macos": {
      "packages": []
    }
  }
}
EOF
```

On this Mac, classification selects `macos` automatically. Use `--target macos` when you want to be explicit.

## 4. Add packages

```bash
genv add jq
genv list
genv apply --dry-run
genv apply --yes
```

`genv add` installs immediately via `brew` when available and writes into `targets.macos` on a v8 spec.

## 5. Dotfiles / other machines

Commit `~/.config/genv/genv.json` (and any relative `files` assets). **Do not** commit `genv.lock.json`.

On another Mac: clone, then `genv apply --target macos --yes`.

For Linux/Windows siblings of the same repo, see [multi-machine.md](multi-machine.md).

## Notes

- Large formulae can take minutes — that is Homebrew, not genv.
- Some names exist as both formula and cask; genv uses Homebrew’s own resolution. Prefer explicit `managers` / manual cask install when you need a specific variant.
- Apple Silicon (`/opt/homebrew`) and Intel (`/usr/local`) are both detected via `PATH`.
