#!/usr/bin/env bash
# Publishes a PKGBUILD and .SRCINFO to the AUR for the genv package.
# Usage: publish-aur.sh <version>   (version without leading 'v', e.g. 0.2.0)
# Requires: AUR_KEY env var containing the SSH private key.
set -euo pipefail

VERSION="$1"
PKGBASE="genv"
REPO="https://github.com/ks1686/genv"

# ── Fetch checksums ────────────────────────────────────────────────────────────
curl -fsSL "${REPO}/releases/download/v${VERSION}/checksums.txt" -o /tmp/checksums.txt

SHA256_AMD64=$(grep "genv_${VERSION}_linux_amd64.tar.gz" /tmp/checksums.txt | awk '{print $1}')
SHA256_ARM64=$(grep "genv_${VERSION}_linux_arm64.tar.gz" /tmp/checksums.txt  | awk '{print $1}')

# ── SSH setup ─────────────────────────────────────────────────────────────────
mkdir -p ~/.ssh
printf '%s\n' "${AUR_KEY}" > ~/.ssh/aur
chmod 600 ~/.ssh/aur
ssh-keyscan -H aur.archlinux.org >> ~/.ssh/known_hosts 2>/dev/null
export GIT_SSH_COMMAND="ssh -i ~/.ssh/aur -o StrictHostKeyChecking=yes"

# ── Clone AUR repo ────────────────────────────────────────────────────────────
git clone "ssh://aur@aur.archlinux.org/${PKGBASE}.git" /tmp/aur-pkg
cd /tmp/aur-pkg

# ── Generate PKGBUILD ─────────────────────────────────────────────────────────
# Single-quoted heredoc keeps ${pkgver} and ${pkgdir} as literals for makepkg.
# Placeholders are substituted via sed below.
cat > PKGBUILD << 'PKGEOF'
# Maintainer: ks1686 <ks1686@users.noreply.github.com>
pkgbase=__PKGBASE__
pkgname=__PKGBASE__
pkgver=__VERSION__
pkgrel=1
pkgdesc='Track, sync, and reproduce your software environment across Linux, macOS, and WSL2.'
arch=('x86_64' 'aarch64')
url='https://github.com/ks1686/genv'
license=('MIT')
source_x86_64=("https://github.com/ks1686/genv/releases/download/v${pkgver}/genv_${pkgver}_linux_amd64.tar.gz")
sha256sums_x86_64=('__SHA256_AMD64__')
source_aarch64=("https://github.com/ks1686/genv/releases/download/v${pkgver}/genv_${pkgver}_linux_arm64.tar.gz")
sha256sums_aarch64=('__SHA256_ARM64__')

package() {
	install -Dm755 "./genv" "${pkgdir}/usr/bin/genv"
}
PKGEOF

sed -i \
  -e "s/__PKGBASE__/${PKGBASE}/g" \
  -e "s/__VERSION__/${VERSION}/g" \
  -e "s/__SHA256_AMD64__/${SHA256_AMD64}/g" \
  -e "s/__SHA256_ARM64__/${SHA256_ARM64}/g" \
  PKGBUILD

# ── Generate .SRCINFO ─────────────────────────────────────────────────────────
# AUR requires tab-indented fields within each pkgbase/pkgname block.
{
  printf 'pkgbase = %s\n'    "${PKGBASE}"
  printf '\tpkgdesc = Track, sync, and reproduce your software environment across Linux, macOS, and WSL2.\n'
  printf '\tpkgver = %s\n'   "${VERSION}"
  printf '\tpkgrel = 1\n'
  printf '\turl = %s\n'      "${REPO}"
  printf '\tarch = x86_64\n'
  printf '\tarch = aarch64\n'
  printf '\tlicense = MIT\n'
  printf '\tsource_x86_64 = %s/releases/download/v%s/genv_%s_linux_amd64.tar.gz\n' \
    "${REPO}" "${VERSION}" "${VERSION}"
  printf '\tsha256sums_x86_64 = %s\n'   "${SHA256_AMD64}"
  printf '\tsource_aarch64 = %s/releases/download/v%s/genv_%s_linux_arm64.tar.gz\n' \
    "${REPO}" "${VERSION}" "${VERSION}"
  printf '\tsha256sums_aarch64 = %s\n'  "${SHA256_ARM64}"
  printf '\n'
  printf 'pkgname = %s\n'   "${PKGBASE}"
} > .SRCINFO

# ── Commit and push ───────────────────────────────────────────────────────────
git config user.name  "ks1686"
git config user.email "ks1686@users.noreply.github.com"
git add PKGBUILD .SRCINFO
git diff --cached --quiet || git commit -m "Update to ${VERSION}"
git push origin master
