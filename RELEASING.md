# Releasing genv

This repository publishes GitHub releases, a Homebrew formula, and an AUR package
automatically when an annotated tag is pushed. GoReleaser handles GitHub releases,
Homebrew, AUR, and the Snap Store automatically. Alpine aports, MacPorts, and Fedora
COPR require manual PR/spec updates after the GitHub release artifacts are published.

---

## Versioning

| Tag | Meaning |
| --- | --- |
| `v0.1.0-beta.1` | First public prerelease (shipped) |
| `v0.1.0` | First stable release — M1 and M2 complete on Linux |
| `v0.2.0` | M3–M5 complete (scan, status, JSON output, --yes/--timeout/--debug, macOS + WSL2 validation) |
| `v1.0.0` | M6 and M7 complete (stable API/quality bar + UX command set) |
| `v2.0.0` | M8 and M9 complete (env and shell configuration management) |
| `v2.1.0` | M10 complete (services management, new adapters: apk/zypper/xbps/emerge, expanded packaging/distribution) |
| `v2.2.0` | M13 complete (hooks and lifecycle scripts) |
| `v2.3.0` | M12 complete (named profiles) |
| `v3.0.0` | M11 complete + first-party Windows support (Chocolatey, Scoop) |

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
2. In the **`ks1686/genv`** repository settings → Secrets and variables → Actions,
   add a repository secret named **`HOMEBREW_TAP_GITHUB_TOKEN`**.
   - Generate a fine-grained PAT at GitHub Settings → Developer Settings → Personal access tokens → Fine-grained tokens.
   - Grant it **Contents: Read and write** on the `ks1686/homebrew-tap` repository only.
   - No other permissions are needed.

Users install after setup:

```bash
brew tap ks1686/tap
brew install genv
```

### 3. AUR (`genv-bin` and `genv`)

Two AUR packages are published on every stable release. Both use the same `AUR_KEY` secret.

- **`genv-bin`** — installs a pre-compiled binary downloaded from the GitHub release. Fast to install; no Go toolchain required.
- **`genv`** — compiles from source using `go build`. Takes longer but lets users audit the build on their own machine.

The two packages `conflict` with each other so users can only have one installed at a time.
Each CI script updates an existing AUR package — it does not create a new one. The first publish of each must be done manually.

**3a. Create an AUR account** at <https://aur.archlinux.org/> if you don't have one.

**3b. Generate an SSH key** for AUR (use a dedicated key, not your main one):

```bash
ssh-keygen -t ed25519 -C "aur" -f ~/.ssh/aur
# Leave passphrase empty — the CI script needs a passphrase-free key.
```

Add the public key to your AUR account: <https://aur.archlinux.org/account/> → SSH keys.

**3c. Create the `genv-bin` package on AUR** (one-time manual step):

```bash
# Clone the (empty) AUR repo — this creates the package namespace
git clone ssh://aur@aur.archlinux.org/genv-bin.git /tmp/genv-bin-aur
cd /tmp/genv-bin-aur

# Write an initial PKGBUILD pointing at the v0.2.0 release
# (CI will update this on every subsequent tag push)
cat > PKGBUILD << 'EOF'
# Maintainer: ks1686 <ks1686@users.noreply.github.com>
pkgname=genv-bin
pkgver=0.2.0
pkgrel=1
pkgdesc="Track, sync, and reproduce your software environment across Linux, macOS, and WSL2."
arch=('x86_64' 'aarch64')
url="https://github.com/ks1686/genv"
license=('MIT')
provides=('genv')
conflicts=('genv')
source_x86_64=("https://github.com/ks1686/genv/releases/download/v${pkgver}/genv_${pkgver}_linux_amd64.tar.gz")
source_aarch64=("https://github.com/ks1686/genv/releases/download/v${pkgver}/genv_${pkgver}_linux_arm64.tar.gz")
# Fill in sha256sums after downloading the release artifacts:
# sha256sum genv_0.2.0_linux_amd64.tar.gz genv_0.2.0_linux_arm64.tar.gz
sha256sums_x86_64=('SKIP')
sha256sums_aarch64=('SKIP')

package() {
    install -Dm755 "./genv" "${pkgdir}/usr/bin/genv"
}
EOF

# Generate .SRCINFO (required by AUR)
makepkg --printsrcinfo > .SRCINFO

git add PKGBUILD .SRCINFO
git commit -m "Initial release v0.2.0"
git push
```

> **Note on SKIP:** Replace `SKIP` with the real sha256sums from the release
> `checksums.txt` before pushing. AUR will flag the package as untrustworthy
> if SKIP is left in place.

**3c-2. Create the `genv` source package on AUR** (one-time manual step):

```bash
git clone ssh://aur@aur.archlinux.org/genv.git /tmp/genv-src-aur
cd /tmp/genv-src-aur

cat > PKGBUILD << 'EOF'
# Maintainer: ks1686 <ks1686@users.noreply.github.com>
pkgname=genv
pkgver=0.2.0
pkgrel=1
pkgdesc="Track, sync, and reproduce your software environment across Linux, macOS, and WSL2."
arch=('x86_64' 'aarch64')
url="https://github.com/ks1686/genv"
license=('MIT')
makedepends=('go')
conflicts=('genv-bin')
source=("${pkgname}-${pkgver}.tar.gz::https://github.com/ks1686/genv/archive/refs/tags/v${pkgver}.tar.gz")
sha256sums=('SKIP')

build() {
    cd "genv-${pkgver}"
    go build -trimpath -ldflags "-s -w -X main.version=${pkgver}" -o genv .
}

package() {
    cd "genv-${pkgver}"
    install -Dm755 genv "${pkgdir}/usr/bin/genv"
}
EOF

makepkg --printsrcinfo > .SRCINFO
git add PKGBUILD .SRCINFO
git commit -m "Initial release v0.2.0"
git push
```

**3d. Add the AUR SSH private key as a repository secret:**

In `ks1686/genv` → Settings → Secrets and variables → Actions, add a secret named
**`AUR_KEY`** containing the contents of `~/.ssh/aur` (the private key).

```bash
cat ~/.ssh/aur
# Copy the entire output including -----BEGIN/END----- lines into the secret value
```

Users install after setup:

```bash
paru -S genv-bin   # pre-compiled binary (fast)
paru -S genv       # builds from source
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
   git tag -a v0.1.0 -m "genv v0.1.0"
   git push origin v0.1.0
   ```

5. **Watch GitHub Actions → Release workflow.** It will:
   - Run `go test ./...`
   - Build binaries for linux/darwin/windows × amd64/arm64
   - Bundle them as `.tar.gz` (`.zip` for Windows)
   - Generate `checksums.txt`
   - Publish a GitHub Release with all artifacts
   - Push the Homebrew formula to `ks1686/homebrew-tap`
   - Push updated PKGBUILDs to AUR (`genv-bin` pre-compiled and `genv` source)

6. **Verify** by downloading one artifact and running:

   ```bash
   ./genv version
   # Expected: genv v0.1.0
   ```

7. **Verify Homebrew** (if you have brew):

   ```bash
   brew update && brew upgrade genv
   genv version
   ```

8. **Verify AUR** (on any Arch machine):

   ```bash
   paru -Sy genv-bin && genv version   # pre-compiled
   # or
   paru -Sy genv && genv version       # from source
   ```

9. **Update manual packaging channels** (after GitHub release artifacts are published):

   **Alpine aports (GitLab PR):**
   - Bump `pkgver` in `packaging/alpine/APKBUILD` to the new version.
   - Compute the sha512 of the new source tarball: `sha512sum genv-<ver>.tar.gz`.
   - Update `sha512sums` in the APKBUILD and submit/update the aports MR.

   **MacPorts (GitHub PR):**
   - Bump the version in `packaging/macports/Portfile`.
   - Run `port checksum` against the new tarball to get rmd160/sha256/size values.
   - Update the `checksums` stanza and submit/update the macports-ports PR.

   **Fedora COPR:**
   - Bump `Version` and add a `%changelog` entry in `packaging/fedora/genv.spec`.
   - **Fedora policy (Oct 2025):** if Claude or another AI tool assisted in writing the spec,
     include `Assisted-by: Claude Sonnet 4.6` in the commit message or COPR PR description.
   - Trigger a COPR build from the new tag or push the updated spec.

   **Snap Store:** handled automatically by GoReleaser's `snapcrafts` section — no manual step needed.

---

## Release note framing

For each release, the notes should cover:

- what milestone is complete
- any known limitations or partially-validated surfaces (e.g., adapters not tested in CI)
- any breaking changes to `genv.json` schema or lock format

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

Candidates for v3.0.0 (Windows support milestone):

| Channel | Complexity | Notes |
| --- | --- | --- |
| Scoop | Low | GoReleaser supports it natively; relevant for v3.0.0 Windows support |
| winget | Low | GoReleaser supports it natively; relevant for v3.0.0 Windows support |
| apt PPA | High | Needs a Launchpad account; `.deb` artifacts already ship via GitHub Releases |
