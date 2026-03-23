# Releasing gpm

This repository publishes GitHub releases, a Homebrew formula, and an AUR package
automatically when an annotated tag is pushed. GoReleaser handles all three.

---

## Versioning

| Tag | Meaning |
|-----|---------|
| `v0.1.0-beta.1` | First public prerelease (shipped) |
| `v0.1.0` | First stable release — M1 and M2 complete on Linux |
| `v0.2.0` | M3–M5 complete (scan, status, JSON output, --yes/--timeout/--debug, macOS + WSL2 validation) |
| `v0.3.0` | M6 complete (API stability, test coverage, performance benchmarks, security audit) |
| `v0.4.0` | M7 complete (shell completions, gpm validate/upgrade/init, improved errors) |

Use pre-release suffixes (`-beta.N`, `-rc.N`) for any release that is not fully
validated. GoReleaser's `skip_upload: auto` setting skips the Homebrew and AUR
publishers for pre-release tags automatically, so only stable tags reach those
channels.

---

## One-time setup (before the first stable release)

These steps only need to be done once. After that, every `v*` tag publishes
automatically.

### 1. GitHub Actions permissions

In the repository settings → Actions → General, confirm:

- "Allow all actions" or allow the specific actions used
- "Read and write permissions" for the default `GITHUB_TOKEN` (needed to create releases)

### 2. Homebrew tap

GoReleaser pushes the formula to a separate `homebrew-tap` repo.

1. Create the repo **`ks1686/homebrew-tap`** on GitHub (public, empty is fine).
2. In the **`ks1686/gpm`** repository settings → Secrets and variables → Actions,
   add a repository secret named **`HOMEBREW_TAP_GITHUB_TOKEN`**.
   - Generate a fine-grained PAT at GitHub Settings → Developer Settings → Personal access tokens → Fine-grained tokens.
   - Grant it **Contents: Read and write** on the `ks1686/homebrew-tap` repository only.
   - No other permissions are needed.

Users install after setup:

```bash
brew tap ks1686/tap
brew install gpm
```

### 3. AUR (`gpm-bin`)

GoReleaser pushes a PKGBUILD to AUR via SSH. It updates an existing package — it does
not create a new one. The first publish must be done manually.

**3a. Create an AUR account** at https://aur.archlinux.org/ if you don't have one.

**3b. Generate an SSH key** for AUR (use a dedicated key, not your main one):

```bash
ssh-keygen -t ed25519 -C "aur-gpm" -f ~/.ssh/aur_gpm
# Leave passphrase empty — GoReleaser needs a passphrase-free key.
```

Add the public key to your AUR account: https://aur.archlinux.org/account/ → SSH keys.

**3c. Create the `gpm-bin` package on AUR** (one-time manual step):

```bash
# Clone the (empty) AUR repo — this creates the package namespace
git clone ssh://aur@aur.archlinux.org/gpm-bin.git /tmp/gpm-bin
cd /tmp/gpm-bin

# Write an initial PKGBUILD pointing at the v0.1.0 release
# (GoReleaser will update this on every subsequent tag push)
cat > PKGBUILD << 'EOF'
# Maintainer: ks1686 <ks1686@users.noreply.github.com>
pkgname=gpm-bin
pkgver=0.1.0
pkgrel=1
pkgdesc="Track, sync, and reproduce your software environment across Linux, macOS, and WSL2."
arch=('x86_64' 'aarch64')
url="https://github.com/ks1686/gpm"
license=('MIT')
provides=('gpm')
conflicts=('gpm')
source_x86_64=("https://github.com/ks1686/gpm/releases/download/v${pkgver}/gpm_${pkgver}_linux_amd64.tar.gz")
source_aarch64=("https://github.com/ks1686/gpm/releases/download/v${pkgver}/gpm_${pkgver}_linux_arm64.tar.gz")
# Fill in sha256sums after downloading the release artifacts:
# sha256sum gpm_0.1.0_linux_amd64.tar.gz gpm_0.1.0_linux_arm64.tar.gz
sha256sums_x86_64=('SKIP')
sha256sums_aarch64=('SKIP')

package() {
    install -Dm755 "./gpm" "${pkgdir}/usr/bin/gpm"
}
EOF

# Generate .SRCINFO (required by AUR)
makepkg --printsrcinfo > .SRCINFO

git add PKGBUILD .SRCINFO
git commit -m "Initial release v0.1.0"
git push
```

> **Note on SKIP:** Replace `SKIP` with the real sha256sums from the release
> `checksums.txt` before pushing. AUR will flag the package as untrustworthy
> if SKIP is left in place.

**3d. Add the AUR SSH private key as a repository secret:**

In `ks1686/gpm` → Settings → Secrets and variables → Actions, add a secret named
**`AUR_KEY`** containing the contents of `~/.ssh/aur_gpm` (the private key).

```bash
cat ~/.ssh/aur_gpm
# Copy the entire output including -----BEGIN/END----- lines into the secret value
```

Users install after setup:

```bash
paru -S gpm-bin   # or: yay -S gpm-bin
```

---

## Release checklist

1. **Make sure `main` is the commit you want to ship.**

2. **Run local CI** to catch any issues before tagging:

   ```bash
   go test ./...
   goreleaser release --clean --snapshot  # dry-run: builds artifacts, no publish
   ```

3. **Update CHANGELOG.md** — move the `Unreleased` section to the new version with today's date.

4. **Create and push an annotated tag:**

   ```bash
   git checkout main
   git pull --ff-only origin main
   git tag -a v0.1.0 -m "gpm v0.1.0"
   git push origin v0.1.0
   ```

5. **Watch GitHub Actions → Release workflow.** It will:
   - Run `go test ./...`
   - Build binaries for linux/darwin/windows × amd64/arm64
   - Bundle them as `.tar.gz` (`.zip` for Windows)
   - Generate `checksums.txt`
   - Publish a GitHub Release with all artifacts
   - Push the Homebrew formula to `ks1686/homebrew-tap`
   - Push an updated PKGBUILD to AUR (`gpm-bin`)

6. **Verify** by downloading one artifact and running:

   ```bash
   ./gpm version
   # Expected: gpm v0.1.0
   ```

7. **Verify Homebrew** (if you have brew):

   ```bash
   brew update && brew upgrade gpm
   gpm version
   ```

8. **Verify AUR** (on any Arch machine):

   ```bash
   paru -Sy gpm-bin
   gpm version
   ```

---

## Release note framing

For each release, the notes should cover:

- what milestone is complete
- any known limitations or partially-validated surfaces (e.g., adapters not tested in CI)
- any breaking changes to `gpm.json` schema or lock format

GoReleaser auto-generates a changelog from `feat:` and `fix:` commits as the release
body. Edit it on GitHub after publish, or use `release.notes` in `.goreleaser.yml`
to provide a custom body before tagging.

---

## If you want to dry-run packaging locally

Install GoReleaser, then run:

```bash
goreleaser release --clean --snapshot
```

Artifacts land in `./dist/`. Nothing is published.

---

## Future distribution channels

These are not yet set up but are candidates for later milestones:

| Channel | Complexity | Notes |
|---------|-----------|-------|
| Snap Store | Medium | Needs `snapcraft.yaml` + `SNAPCRAFT_STORE_CREDENTIALS` secret |
| Flathub | High | Requires a PR to `flathub/flathub`; not fully automatable |
| apt PPA | High | Needs a Launchpad account and `.deb` packaging |
| dnf COPR | High | Needs a Fedora account and `.spec` file |
| Scoop | Low | GoReleaser supports it natively; relevant once M5 (Windows) is targeted |
| winget | Low | GoReleaser supports it natively; relevant for M5 |
