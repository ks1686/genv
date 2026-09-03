# Releasing genv

This repository publishes GitHub releases, a Homebrew cask, a Scoop manifest,
and an AUR package automatically when an annotated tag is pushed. GoReleaser
handles GitHub releases, Homebrew, Scoop, and the Snap Store automatically; a
follow-up macOS workflow job publishes the AUR packages.
— no external reviewer sign-off required.

---

## Versioning

| Tag | Meaning |
| --- | --- |
| `v0.1.0-beta.1` | First public prerelease (shipped) |
| `v0.1.0` | First stable release — M1 and M2 complete on Linux |
| `v0.2.0` | M3–M5 complete (scan, status, JSON output, --yes/--timeout/--debug, macOS + WSL2 validation) |
| `v1.0.0` | M6 and M7 complete (stable API/quality bar + UX command set) |
| `v2.0.0` | M8 and M9 complete (env and shell configuration management) |
| `v2.1.0` | M10 complete (services management, new adapters: zypper/xbps/emerge, Snap Store publishing) |
| `v2.2.0` | Scoped M13 surface (schema v5 `files`/`hooks`, host selectors, `pull` / `status --files` / `adopt --files`) |
| `v2.3.0` | Native Windows support (`windows` host, `winget`/`scoop`/`choco`, merge-dir file links) |
| `v3.0.0` | M11 updates checker, M12 named profiles, full M13 lifecycle hooks, upgrade JSON/filtering, tracked ecosystem adapters |
| `v3.2.x` | Outdated-aware `genv upgrade` / `updates check` (default plan only packages with detected updates) |
| `v4.0.0` | Schema v7 PowerShell parity + schema v8 portable multi-target (`defaults`/`targets`, migrate/export/map, foreign-lock gate) |
| `v4.0.1` | Fix schemaVersion 8 materialize gaps (`status` / `upgrade` / `updates` / hooks / env·shell·service reads) + Arch CLI matrix CI |
| `v4.0.2` | Apply continues past file mismatches; human file plans + `--backup`; brew-formula service status; updates `--help`; launchd re-register after genv upgrade |
| `v4.0.3` | Explicit skip message for post-apply hooks on file mismatch; `scan --dry-run` + confirmation/`--yes` |
| `v4.0.4` | Prefer PATH-stable genv path for updates LaunchAgent/systemd; agent dangling-path checks in status/validate; AUR publish retry + aur-only repair |
| `v4.0.5` | Brew bin derivation without SameFile mid-upgrade; strip Homebrew shims from scheduled PATH; `__run-once` / brew / notify timeouts so launchd cannot wedge |
| `v4.0.6` | Per-manager outdated timing in updates.log to diagnose slow launchd checks vs timeout fallback |
| `v4.0.7` | `__run-once` no longer hangs on osascript notify until 5m deadline (exit 4) |
| `v4.0.8` | `add`/`adopt` Tab completions via `genv __complete repo-packages` (cached dumps + search fallback) |
| `v4.0.9` | Lifecycle hooks inherit stdin + receive `GENV_YES` when `--yes` is set |
| `v4.0.10` | Darwin GitHub Release / Homebrew binaries are Developer ID signed and notarized |
| `v4.0.11` | Fail-closed `add`, schema v8 defaults, native apt/dnf/apk, Windows CI/test hardening (tag exists; GitHub Release aborted on Pro-only GoReleaser keys) |
| `v4.0.12` | Drop Pro-only winget/chocolatey GoReleaser keys so OSS publish can succeed |
| `v4.0.13` | Self-hosted Scoop bucket (`ks1686/scoop-bucket`) |
| `v4.1.0` | First tagged Scoop upload from CI (token template matches Homebrew) |
| `v4.2.0` | Windows apply skip-if-present, 10m timeout, live status, `--skip-packages`, `external` manager |
| `v4.2.1` | Apply/status list only needed managers; 30s listing timeout so hung Composer cannot stall Windows CI |
| `v4.2.2` | Every manager probe bounded (scan/search/upgrade/outdated/services); crash-safe fsync'd spec & lock writes; atomic pull; docs/completions/CI fixes; updates worker shutdown grace |
| `v4.3.0` | Windows updates scheduler, vscode stable-only outdated detection, lock/apply recovery, and `genv upgrade` OS/firmware steps |

Use pre-release suffixes (`-beta.N`, `-rc.N`) for any release that is not fully
validated. GoReleaser's `skip_upload: auto` setting skips the Homebrew, Scoop,
and Snap publishers for pre-release tags, and the AUR publish job skips
pre-release versions itself — so only stable tags reach those channels.

---

## One-time setup (before the first stable release)

These steps only need to be done once. After that, every `v*` tag publishes
automatically.

### 1. GitHub Actions permissions

In the repository settings → Actions → General, confirm:

- "Allow all actions" or allow the specific actions used
- "Read and write permissions" for the default `GITHUB_TOKEN` (needed to create releases)

### 2. Apple Developer ID signing and notarization

Darwin release binaries are signed with a **Developer ID Application** certificate
and notarized via App Store Connect (GoReleaser → anchore/quill on `ubuntu-latest`).
Cosign continues to sign `checksums.txt` separately.

Without these secrets, GoReleaser skips `notarize.macos` and ships adhoc-signed
Darwin binaries (the pre-v4.1 behavior).

**2a. Create a Developer ID Application certificate**

1. Open [Certificates](https://developer.apple.com/account/resources/certificates/list).
2. Create a certificate of type **Developer ID Application**.
3. Upload a Certificate Signing Request (CSR). Generate one locally:

   ```bash
   mkdir -p .local/apple-signing
   openssl req -new -newkey rsa:2048 -nodes \
     -keyout .local/apple-signing/DeveloperID.key \
     -out .local/apple-signing/DeveloperID.csr \
     -subj "/emailAddress=YOU@example.com/CN=Developer ID Application/C=US"
   ```

4. Download the `.cer`, double-click to import into Keychain Access (keep the
   private key that matched the CSR on the same Mac).
5. In Keychain Access, select the **Developer ID Application** identity →
   Export → `.p12`, and choose a strong password. Store the `.p12` and password
   outside the repo (`.local/` is gitignored).

**2b. Create an App Store Connect API key**

1. Open [Users and Access → Integrations → Team Keys](https://appstoreconnect.apple.com/access/integrations/api).
2. Generate a key with **Developer** access (or App Manager).
3. Download the `.p8` once (`AuthKey_<KEY_ID>.p8`). Note the **Key ID** and
   **Issuer ID** (UUID on the same page).

**2c. Add GitHub Actions secrets**

Encode and upload (or use the helper):

```bash
./scripts/set-macos-signing-secrets.sh \
  --p12 /path/to/Certificates.p12 \
  --p12-password 'your-export-password' \
  --p8 /path/to/AuthKey_XXXXXXXXXX.p8 \
  --key-id XXXXXXXXXX \
  --issuer-id 00000000-0000-0000-0000-000000000000
```

Secrets created:

| Secret | Contents |
| --- | --- |
| `MACOS_SIGN_P12` | base64 of the `.p12` |
| `MACOS_SIGN_PASSWORD` | password that opens the `.p12` |
| `MACOS_NOTARY_KEY` | base64 of the `.p8` |
| `MACOS_NOTARY_KEY_ID` | API Key ID |
| `MACOS_NOTARY_ISSUER_ID` | Issuer UUID |

**2d. Verify a Darwin artifact after the next release**

Team ID for this project: `7R2VPW8GH4`  
Identity: `Developer ID Application: KARIM SMIRES (7R2VPW8GH4)`

```bash
codesign -dv --verbose=4 ./genv
# Expect: Authority=Developer ID Application: KARIM SMIRES (7R2VPW8GH4)
#         TeamIdentifier=7R2VPW8GH4

# Bare CLI tools often report "does not seem to be an app" from spctl -a;
# that is normal. Prefer exec assessment / successful launch under quarantine:
spctl -a -t exec -vv ./genv
./genv version
```

### 3. Homebrew tap

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
brew install --cask genv
```

### 4. Scoop bucket

GoReleaser pushes a Scoop manifest to a separate `scoop-bucket` repo. It does
not create that repo; create it first (`gh repo create`). The release workflow
`GITHUB_TOKEN` cannot push to another repository.

1. Create the repo **`ks1686/scoop-bucket`** on GitHub (public). Do **not** add
   the `scoop-bucket` GitHub topic — that lists the bucket on scoop.sh.
2. In the **`ks1686/genv`** repository settings → Secrets and variables → Actions,
   add a repository secret named **`SCOOP_BUCKET_GITHUB_TOKEN`**.
   - Generate a fine-grained PAT at GitHub Settings → Developer Settings → Personal access tokens → Fine-grained tokens.
   - Grant it **Contents: Read and write** on the `ks1686/scoop-bucket` repository only.
   - Do not reuse `HOMEBREW_TAP_GITHUB_TOKEN` unless that token is also granted
     on `scoop-bucket`.
   - Do not put a `gh` OAuth token (`gho_…`) into Actions secrets.
3. Leave `scoops.directory` unset so manifests live at the bucket root
   (`scoop install genv` cannot find the manifest otherwise).
4. If the secret is missing, GoReleaser skips the Scoop upload (`skip_upload: true`)
   and the rest of the release still publishes. With the secret set, `skip_upload`
   is `auto` (stable tags upload; prereleases do not).

Users install after a tagged release has pushed `genv.json`:

```powershell
scoop bucket add ks1686 https://github.com/ks1686/scoop-bucket
scoop install genv
```

### 5. AUR (`genv-bin` and `genv`)

Two AUR packages are published on every stable release. Both use the same `AUR_KEY` secret.

- **`genv-bin`** — installs a pre-compiled binary downloaded from the GitHub release. Fast to install; no Go toolchain required.
- **`genv`** — compiles from source using `go build`. Takes longer but lets users audit the build on their own machine.

The two packages `conflict` with each other so users can only have one installed at a time.
Each CI script updates an existing AUR package — it does not create a new one. The first publish of each must be done manually.

**5a. Create an AUR account** at <https://aur.archlinux.org/> if you don't have one.

**5b. Generate an SSH key** for AUR (use a dedicated key, not your main one):

```bash
ssh-keygen -t ed25519 -C "aur" -f ~/.ssh/aur
# Leave passphrase empty — the CI script needs a passphrase-free key.
```

Add the public key to your AUR account: <https://aur.archlinux.org/account/> → SSH keys.

**5c. Create the `genv-bin` package on AUR** (one-time manual step):

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

**5c-2. Create the `genv` source package on AUR** (one-time manual step):

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

**5d. Add the AUR SSH private key as a repository secret:**

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
   - Push `genv.json` to `ks1686/scoop-bucket` (Scoop pipe continues on error — confirm the file landed)
   - Push updated PKGBUILDs to AUR (`genv-bin` pre-compiled and `genv` source)

   If GitHub/Homebrew succeeded but AUR failed (transient `aur.archlinux.org` SSH),
   do **not** re-run the whole Release job (GoReleaser will hit `already_exists`).
   Instead dispatch an AUR-only repair:

   ```bash
   gh workflow run Release -f mode=aur-only -f version=4.0.3
   gh run watch  # pick the new run
   ```

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

8. **Verify Scoop** (on Windows, after the bucket has `genv.json`):

   ```powershell
   scoop bucket add ks1686 https://github.com/ks1686/scoop-bucket
   scoop update
   scoop install genv
   genv version
   ```

9. **Verify AUR** (on any Arch machine):

   ```bash
   paru -Sy genv-bin && genv version   # pre-compiled
   # or
   paru -Sy genv && genv version       # from source
   ```

10. **Snap Store:** handled automatically by GoReleaser's `snapcrafts` section — no manual step needed.

---

## Release note framing

For each release, the notes should cover:

- what milestone is complete
- any known limitations or partially-validated surfaces (e.g., adapters not tested in CI)
- any breaking changes to `genv.json` schema or lock format

GoReleaser auto-generates a changelog for the release body from commit messages,
excluding `docs:`, `test:`, and `chore:` commits. Edit it on GitHub after
publish, or provide a custom body in the GoReleaser changelog config before
tagging.

---

## If you want to dry-run packaging locally

Install GoReleaser, then run:

```bash
goreleaser release --clean --snapshot
```

Artifacts land in `./dist/`. Nothing is published.

---

## Future distribution channels

Scoop is live as a self-hosted bucket (`ks1686/scoop-bucket`), not Scoop extras.
winget and Chocolatey stay unpublished (community review + GoReleaser Pro).
GitHub Release zips remain a supported Windows path.

| Channel | Status | Notes |
| --- | --- | --- |
| winget | Deferred | Publisher is GoReleaser Pro-only; default source needs microsoft/winget-pkgs review |
| Chocolatey | Deferred | Publisher is GoReleaser Pro-only; community repo has the same review gate |
| apt PPA | Deferred | `.deb` artifacts already ship via GitHub Releases |
