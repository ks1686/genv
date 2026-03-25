#!/usr/bin/env bash
# Publishes a source-build PKGBUILD to the AUR for the genv package.
# Users install this package by compiling from source with `go build`.
# Usage: publish-aur-src.sh <version>   (version without leading 'v', e.g. 2.0.1)
# Requires: AUR_KEY env var containing the SSH private key.
set -euo pipefail

VERSION="$1"
PKGBASE="genv"
REPO="https://github.com/ks1686/genv"

# ── Fetch source tarball and compute sha256sum ─────────────────────────────────
curl -fsSL "${REPO}/archive/refs/tags/v${VERSION}.tar.gz" -o /tmp/genv-src.tar.gz
SHA256SRC=$(sha256sum /tmp/genv-src.tar.gz | awk '{print $1}')

# ── SSH setup ─────────────────────────────────────────────────────────────────
mkdir -p ~/.ssh
printf '%s\n' "${AUR_KEY}" > ~/.ssh/aur
chmod 600 ~/.ssh/aur
ssh-keyscan -H aur.archlinux.org >> ~/.ssh/known_hosts 2>/dev/null
export GIT_SSH_COMMAND="ssh -i ~/.ssh/aur -o StrictHostKeyChecking=yes"

# ── Clone AUR repo ────────────────────────────────────────────────────────────
git clone "ssh://aur@aur.archlinux.org/${PKGBASE}.git" /tmp/aur-src-pkg
cd /tmp/aur-src-pkg

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
makedepends=('go')
conflicts=('genv-bin')
source=("${pkgname}-${pkgver}.tar.gz::https://github.com/ks1686/genv/archive/refs/tags/v${pkgver}.tar.gz")
sha256sums=('__SHA256SRC__')

build() {
	cd "genv-${pkgver}"
	go build -trimpath -ldflags "-s -w -X main.version=${pkgver}" -o genv .
}

package() {
	cd "genv-${pkgver}"
	install -Dm755 genv "${pkgdir}/usr/bin/genv"
	install -Dm644 "completions/genv.zsh"  "${pkgdir}/usr/share/zsh/site-functions/_genv"
	install -Dm644 "completions/genv.bash" "${pkgdir}/usr/share/bash-completion/completions/genv"
	install -Dm644 "completions/genv.fish" "${pkgdir}/usr/share/fish/vendor_completions.d/genv.fish"
}
PKGEOF

sed -i \
  -e "s/__PKGBASE__/${PKGBASE}/g" \
  -e "s/__VERSION__/${VERSION}/g" \
  -e "s/__SHA256SRC__/${SHA256SRC}/g" \
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
  printf '\tmakedepends = go\n'
  printf '\tconflicts = genv-bin\n'
  printf '\tsource = genv-%s.tar.gz::%s/archive/refs/tags/v%s.tar.gz\n' \
    "${VERSION}" "${REPO}" "${VERSION}"
  printf '\tsha256sums = %s\n' "${SHA256SRC}"
  printf '\n'
  printf 'pkgname = %s\n'   "${PKGBASE}"
} > .SRCINFO

# ── Commit and push ───────────────────────────────────────────────────────────
git config user.name  "ks1686"
git config user.email "ks1686@users.noreply.github.com"
git add PKGBUILD .SRCINFO
git diff --cached --quiet || git commit -m "Update to ${VERSION}"
git push origin master
