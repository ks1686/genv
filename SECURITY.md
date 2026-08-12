# Security Policy

## Supported Versions

Only the latest stable release receives security fixes.

| Version | Supported |
| ------- | --------- |
| latest (`v4.x`) | Yes |
| older majors / previous minors | No |

## Reporting a Vulnerability

Do **not** open a public GitHub issue for security vulnerabilities.

Use GitHub's [private vulnerability reporting](https://github.com/ks1686/genv/security/advisories/new). Include:

- Description and potential impact
- Steps to reproduce or a minimal proof of concept
- Output of `genv version`
- OS and package-manager combination

Expect acknowledgement within **72 hours** and a status update at least every **7 days** while investigating. Fixes ship with a patched release and public advisory when warranted.

## Verifying Release Integrity

### Cosign (all platforms)

Every release ships `checksums.txt` signed with [cosign](https://docs.sigstore.dev/cosign/overview/) using keyless (OIDC) signing. Sigstore bundles attach to each GitHub release.

```bash
# Example for the linux/amd64 archive of tag v4.0.0
cosign verify-blob \
  --certificate-identity "https://github.com/ks1686/genv/.github/workflows/release.yml@refs/tags/v4.0.0" \
  --certificate-oidc-issuer "https://token.actions.githubusercontent.com" \
  --bundle checksums.txt.sigstore.json \
  checksums.txt

sha256sum --check --ignore-missing checksums.txt
```

Adjust the tag and archive names for the asset you downloaded. See the release assets list for exact filenames (`genv_<version>_<os>_<arch>.tar.gz` or `.zip`).

### Apple Developer ID (macOS)

Darwin release binaries are signed with a **Developer ID Application** certificate and notarized by Apple when the `MACOS_*` GitHub Actions secrets are configured (see [RELEASING.md](RELEASING.md)). Verify a downloaded binary:

```bash
codesign -dv --verbose=4 ./genv
# Expect: Authority=Developer ID Application: … and TeamIdentifier=<TEAMID>

spctl -a -vv ./genv
# Expect: accepted / source=Notarized Developer ID
```

Cosign checksum verification and Apple notarization are complementary: cosign proves the GitHub Actions build provenance; Gatekeeper/notarization proves Apple accepted the Darwin binary.

## Trust boundaries

- `genv.json` is trusted configuration. Lifecycle `hooks` and shell fragments run as the current user with the same privileges as the `genv` process.
- Lock files and secrets must stay machine-local; export/pull omit locks and sensitive env values by design.
- Package-manager subprocesses inherit your user privileges — treat manager availability and foreign locks carefully when applying a portable spec.
