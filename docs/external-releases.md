# Managed external releases

Schema v9 recipes manage software that has release artifacts but no suitable
host package. Keep recipes in `defaults.packages` when all targets share the
same declaration, or use target-specific platform entries.

## GitHub archive example

```json
{
  "id": "tool",
  "prefer": "external",
  "external": {
    "detect": {
      "command": ["tool", "--version"],
      "versionRegex": "tool ([0-9.]+)"
    },
    "source": {
      "type": "githubRelease",
      "repository": "owner/tool",
      "release": "stable",
      "tagRegex": "^v?(.+)$"
    },
    "platforms": [
      {
        "os": ["linux"],
        "arch": ["amd64"],
        "libc": ["glibc"],
        "assetRegex": "^tool_[0-9.]+_linux_amd64\\.tar\\.gz$",
        "install": {
          "type": "archive",
          "stripComponents": 1,
          "files": [
            {"from": "bin/tool", "to": "~/.local/bin/tool", "mode": "0755"}
          ]
        }
      }
    ],
    "verify": [
      {"type": "sha256File", "assetRegex": "^checksums\\.txt$"}
    ]
  }
}
```

Asset and verifier regular expressions must match exactly one release asset.
Archive recipes install only declared regular files and reject traversal,
links, duplicate entries, and device files.

## Structured HTTP and script example

HTTP metadata must be JSON or text, not scraped HTML. Installer scripts are
downloaded to a private directory, verified, and passed to one explicit
interpreter without `eval`, a command string, or a pipe.

```json
{
  "id": "vendor-tool",
  "prefer": "external",
  "external": {
    "detect": {
      "command": ["vendor-tool", "--version"],
      "versionRegex": "([0-9.]+)"
    },
    "source": {
      "type": "httpRelease",
      "versionURL": "https://downloads.example.com/latest.json",
      "format": "json",
      "versionPointer": "/version"
    },
    "platforms": [
      {
        "os": ["windows"],
        "arch": ["amd64", "arm64"],
        "artifactURL": "https://downloads.example.com/{version}/install.ps1",
        "install": {
          "type": "script",
          "interpreter": "pwsh",
          "args": ["-Version", "{version}"],
          "env": {"GENV_EXTERNAL_OS": "{os}"},
          "uninstall": ["vendor-tool", "uninstall", "--yes"]
        }
      }
    ],
    "verify": [
      {"type": "sha256", "valuePointer": "/sha256"}
    ],
    "allowBackgroundExecution": true
  }
}
```

Allowed argv/environment/destination placeholders are `{version}`, `{tag}`,
`{os}`, `{arch}`, `{script}`, and `{destination}`. Unknown or malformed
placeholders fail validation. `sh`, `bash`, `pwsh`, and `powershell` are the only
installer interpreters. Uninstall is an executable plus argv, not a shell
command.

## Verification entries

Every entry in `verify` must succeed:

```json
{"type":"githubDigest"}
{"type":"sha256","value":"<64 lowercase or uppercase hex characters>"}
{"type":"sha256","valuePointer":"/sha256"}
{"type":"sha256File","assetRegex":"^checksums\\.txt$"}
{"type":"sigstore","bundleAssetRegex":"^tool\\.sigstore$","identity":"https://github.com/owner/tool/.github/workflows/release.yml@refs/heads/main","issuer":"https://token.actions.githubusercontent.com"}
{"type":"minisign","signatureAssetRegex":"^tool\\.minisig$","publicKeyFile":"keys/tool.minisign.pub"}
{"type":"openpgp","signatureAssetRegex":"^tool\\.asc$","publicKeyFile":"keys/tool.asc","fingerprint":"0123456789ABCDEF0123456789ABCDEF01234567"}
```

Signature/checksum material can use `url` instead of a GitHub asset expression.
Minisign and OpenPGP public keys may be inline with `publicKey` or read from a
local `publicKeyFile`; exactly one key source is required. Export bundles relative
key files and rewrites their paths. Sigstore requires an exact certificate
identity and OIDC issuer and validates the transparency evidence in the bundle.

## Execution policy

- HTTPS is required unless `allowInsecureHTTP` is explicitly true.
- Plain HTTP is never eligible for scheduler execution.
- An empty verifier list requires `allowUnverified: true`; each install or
  upgrade then needs its own interactive acknowledgement, even with `--yes`.
- Verified direct/archive installs may run unattended.
- Verified scripts additionally require `allowBackgroundExecution: true` for
  `--yes` or `updates.autoApply` execution.
- User-scope direct/archive destinations must stay under the user's home.
  Destinations elsewhere require `scope: "system"` and appropriate elevation.
- Script-created files are never inferred. Removal requires the declared
  uninstall argv; adopted external installations remain unowned.

The lock file records sanitized release metadata, artifact and path digests,
verification summaries, recipe identity, and ownership. Do not commit lock files.
