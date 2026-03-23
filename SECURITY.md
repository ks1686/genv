# Security Policy

## Supported Versions

`genv` is pre-1.0 software. Only the latest release receives security fixes.

| Version      | Supported          |
| ------------ | ------------------ |
| latest (0.x) | :white_check_mark: |
| older 0.x    | :x:                |

## Reporting a Vulnerability

Please **do not** open a public GitHub issue for security vulnerabilities.

Use GitHub's [private vulnerability reporting](https://github.com/ks1686/genv/security/advisories/new) to submit a report. This keeps the details confidential until a fix is released.

Include as much of the following as you can:

- A description of the vulnerability and its potential impact
- Steps to reproduce, or a minimal proof-of-concept
- The `genv version` output from the affected build
- Your operating system and package manager combination

You can expect an acknowledgement within **72 hours** and a status update at least every **7 days** while the issue is being investigated. If a fix is warranted, a patched release and a public advisory will be published together.

## Verifying Release Integrity

Every release binary is signed with [cosign](https://docs.sigstore.dev/cosign/overview/) using keyless signing. The signature and certificate are attached to each GitHub release alongside `checksums.txt`. Verify a downloaded archive before use:

```bash
cosign verify-blob \
  --certificate-identity "https://github.com/ks1686/genv/.github/workflows/release.yml@refs/tags/<tag>" \
  --certificate-oidc-issuer "https://token.actions.githubusercontent.com" \
  --bundle genv_<version>_<os>_<arch>.tar.gz.bundle \
  genv_<version>_<os>_<arch>.tar.gz
```

You can also cross-check the downloaded archive against `checksums.txt`:

```bash
sha256sum --check --ignore-missing checksums.txt
```
