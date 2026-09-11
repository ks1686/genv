# Linux install and bootstrap

## Install genv

Arch and Manjaro users can install `genv` or `genv-bin` from the AUR. Other
Linux distributions should use the archive attached to the latest GitHub
Release:

```bash
version=4.3.0
curl -fLO "https://github.com/ks1686/genv/releases/download/v${version}/genv_${version}_linux_amd64.tar.gz"
curl -fLO "https://github.com/ks1686/genv/releases/download/v${version}/checksums.txt"
sha256sum --ignore-missing -c checksums.txt
tar -xzf "genv_${version}_linux_amd64.tar.gz"
sudo install -m 0755 genv /usr/local/bin/genv
genv version
```

Use the matching `arm64` archive on 64-bit ARM. Check the Releases page for the
exact current asset names. The former Snap distribution is discontinued because
strict confinement prevented genv from observing and controlling host package
managers. This does not remove genv's `snap` adapter for managing other snaps.

## Configure and apply

Use `ubuntu`, `arch`, or an explicit `linux` target as appropriate:

```bash
genv init
genv validate
genv apply --dry-run
genv apply
```

Schema v9 managed external recipes can install verified GitHub or structured
HTTP releases when no native package exists. Direct binaries, ZIP/tar archives,
and explicit installer scripts are supported. See
[SCHEMA.md](../SCHEMA.md#managed-external-releases-v9) for the recipe and
unattended-execution security rules.
