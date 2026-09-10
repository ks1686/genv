# External Release Management Design

## Status

Approved design for a full-featured, cross-platform replacement for manual
external installation tracking. This feature does not restore genv's own Snap
package. It makes software distributed through GitHub Releases and structured
vendor endpoints installable, upgradable, removable, and observable by genv.

## Goals

- Extend the existing `external` manager without breaking presence-only specs.
- Support Linux, macOS, native Windows, and WSL2.
- Discover releases from GitHub Releases and structured HTTP JSON or text
  endpoints.
- Install direct executables, archives, and downloaded installer scripts.
- Detect installed versions and report newer remote releases.
- Integrate with `apply`, `status`, `upgrade`, and `updates check`.
- Verify payloads with SHA-256, GitHub asset digests, Sigstore, minisign, or
  OpenPGP.
- Record enough machine-local receipt data for safe upgrades and removals.
- Keep installation policy local and git-tracked. Remote systems provide release
  metadata and artifacts, not recipes.

## Non-Goals

- HTML scraping.
- A centralized or community recipe registry.
- Fetching executable recipes from remote URLs.
- Inferring arbitrary vendor installation behavior.
- Treating a checksum file from the same release as stronger publisher identity
  than it provides.
- Removing files created internally by opaque installer scripts unless the
  recipe declares an uninstall action.

## Compatibility And Schema

The feature introduces schema version 9. Version 9 keeps the portable
`defaults`/`targets` structure introduced by version 8 and adds managed external
recipes. Portable command paths must use a shared `schema.IsPortableVersion`
helper rather than checking only `schemaVersion == "8"`. Versions 1 through 8
remain accepted. An existing package using `prefer: "external"` without an
`external` object keeps today's presence-only behavior: genv checks that the
mapped executable is on `PATH` but does not install, upgrade, or remove it.

A managed package adds an `external` object directly to its package record. This
keeps source and execution policy in the same target-specific package list and
avoids a second merge system for shared recipe definitions.

```json
{
  "id": "tool",
  "prefer": "external",
  "external": {
    "detect": {
      "command": ["tool", "--version"],
      "versionRegex": "tool ([0-9]+\\.[0-9]+\\.[0-9]+)"
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
        "assetRegex": "^tool_.+_linux_amd64\\.tar\\.gz$",
        "install": {
          "type": "archive",
          "stripComponents": 1,
          "files": [
            {"from": "tool", "to": "~/.local/bin/tool", "mode": "0755"}
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

### Detection

`detect.command` is an argv array and never a shell string.
`detect.versionRegex` is a bounded RE2 expression with exactly one capture group.
The command must exit zero and produce a non-empty captured version. Output is
size-limited and may be read from stdout or stderr because many tools print
versions to stderr.

Detection determines local presence and version. A failed command means absent;
a successful command whose output cannot be parsed is an explicit drift/error,
not an absent installation.

### Release Sources

`githubRelease` supports:

- `repository`: required `owner/name` identifier.
- `release`: `stable`, `prerelease`, or `any`; default `stable`.
- `tagRegex`: optional single-capture version normalization.
- `apiBase`: optional GitHub Enterprise API base URL.

The provider uses release records, not a floating browser download URL. It keeps
the release ID, tag, publication state, and asset metadata. `GITHUB_TOKEN` is
optional for github.com and supported for private repositories and rate limits.

`httpRelease` supports:

- `versionURL`: required HTTPS endpoint.
- `format`: `json` or `text`.
- `versionPointer`: RFC 6901 JSON Pointer for JSON responses.
- `versionRegex`: bounded single-capture RE2 expression for text responses.
- Platform artifact and verification URLs containing `{version}`, `{os}`, and
  `{arch}` placeholders.

JSON uses `versionPointer`; text uses `versionRegex`. HTML content types are
rejected. Responses, redirects, and elapsed time are bounded.

The selected source record is authoritative for "latest" within its configured
channel. genv compares the normalized detected version with that selected
version. It does not attempt to sort arbitrary opaque vendor version strings.
Existing exact and prefix-wildcard package constraints still gate upgrades.

### Platform Selection

Each recipe contains one or more platform entries. Selectors use Go-style OS and
architecture names (`linux`, `darwin`, `windows`, `amd64`, `arm64`) and may also
select `glibc` or `musl`. WSL selects Linux assets. Exactly one entry must match;
zero or multiple matches are validation/runtime errors.

GitHub entries select a single release asset through `assetRegex`. HTTP entries
use an `artifactURL` template. Asset matches must be unique. Regexes and URL
templates may use the normalized release fields but cannot execute code.

### Installation Types

`direct` installs one downloaded executable at a declared destination.

`archive` supports ZIP, tar, tar.gz, tar.xz, and tar.zst. It can strip a declared
number of leading path components and install an explicit list of files. Every
source path must resolve to exactly one regular file. Archive traversal,
absolute paths, links escaping the staging directory, duplicate destinations,
and special device files are rejected.

`script` downloads a script as the verified artifact and executes it from a
private temporary directory through an explicit interpreter: `sh`, `bash`,
`pwsh`, or `powershell`. Arguments and environment variables are separate arrays
and maps; shell command strings and `curl | sh` pipelines are not accepted.
Template substitution is restricted to documented release and platform values.

Direct and archive installs default to user scope. Destinations outside the
user's home require `scope: "system"`, a visible plan entry, and the existing
interactive/elevation path. Recipes never gain elevation merely because an
installer exits with a permissions error.

### Removal

Direct and archive installs record every destination owned by genv. Removal
deletes only paths in that receipt after confirming that each path still matches
the installed receipt digest. Modified files are reported as drift and retained
unless the user uses the existing force/backup controls where applicable.

Script recipes must declare an `uninstall` argv action if removal is expected.
Without one, `genv remove` refuses to claim successful uninstallation and leaves
the spec unchanged. genv does not guess which files a script created.

## Verification

`verify` is an array. Every configured verifier must succeed. Supported entries:

- `githubDigest`: require and verify the digest returned for the selected GitHub
  asset.
- `sha256`: compare with a literal or structured-endpoint digest.
- `sha256File`: download a uniquely matched checksum asset or URL and find the
  exact selected artifact name.
- `sigstore`: verify a bundle/signature and require configured certificate
  identity and OIDC issuer values.
- `minisign`: verify with a pinned inline public key or local public-key file.
- `openpgp`: verify with a pinned inline public key or local key file and require
  the configured full fingerprint.

Public keys and identity policy are local recipe data. A recipe cannot fetch a
new trusted key from the same release it is trying to authenticate. Key files
participate in existing export asset bundling.

When `verify` is empty, schema validation requires `allowUnverified: true`.
Unverified installation or upgrade always requires an interactive confirmation,
even when `--yes` is present. It is forbidden in `updates.autoApply`.

Scripts are eligible for background execution only when verification succeeds
and the recipe also sets `allowBackgroundExecution: true`. This opt-in is
specific to the risk that a verified script can download additional payloads
outside genv's verification boundary. Direct and archive payloads need successful
verification but no additional script-risk opt-in.

## Download And Execution Safety

- HTTPS is required by default. HTTP requires a separate explicit insecure
  transport acknowledgement and is never background-eligible.
- Redirect count is bounded; HTTPS-to-HTTP redirects are rejected.
- Metadata, artifact, checksum, and signature downloads have separate size and
  time limits.
- Temporary files and directories use owner-only permissions.
- Payload verification occurs before extraction or execution.
- Existing destination files are staged and replaced atomically when the host
  filesystem permits it. Failure restores the previous file.
- Secrets from environment variables are never persisted in spec, lock, logs,
  plans, or JSON output.
- Plans show source host, release version, verification method, install type,
  destination scope, and whether script background execution is enabled.

## Lifecycle Integration

`apply` handles recipe-backed external packages as package actions. Missing
packages are resolved, downloaded, verified, installed, detected again, and only
then written to the lock. A failure leaves the spec and prior receipt unchanged.

`status` remains local by default. It runs detection, compares the detected
version and owned-file digests with the lock, and reports absent, version drift,
modified owned files, or recipe drift. It does not contact remote providers.

`updates check` contacts the configured provider, applies the existing manager
and package filters, and reports an update only when the selected remote version
differs from the successfully detected local version and satisfies the package
constraint. Provider failures retain the existing conservative behavior: report
the check failure and do not silently label the package current.

`upgrade` uses the same external update planner, then performs the same verified
transaction as apply. Script upgrades rerun the installer unless a distinct
upgrade action is declared.

`updates.autoApply` enforces verification and transport policy again at execution
time. A stale plan cannot bypass a changed recipe or signer policy.

`adopt` can record a detected existing installation and its version, but marks
it as not owned by genv. A later successful genv-managed upgrade creates a new
receipt for paths installed by that upgrade. Removal never deletes an adopted
path without ownership evidence.

## Lock Receipt

`LockedPackage` gains an optional external receipt containing:

- normalized source type, release ID/tag, and installed version
- resolved artifact URL without credentials
- artifact SHA-256 regardless of the configured verifier
- verification method and pinned signer identity/fingerprint summary
- recipe SHA-256 computed from canonical non-secret recipe JSON
- install type and ownership state
- installed destination paths and post-install file SHA-256 values

The lock remains machine-local. It does not store downloaded scripts, signatures,
public-key file contents, tokens, or installer environment secrets.

## Architecture

The existing `adapter.External` remains the legacy presence-only adapter.
Recipe-backed operations live in a new `internal/external` package because the
current adapter interface only plans static argv and cannot safely represent
release discovery, verification, staging, receipts, and rollback.

The package is divided into focused units:

- provider: GitHub and structured HTTP metadata resolution
- selector: platform and unique asset selection
- downloader: bounded transport and private staging
- verifier: SHA-256, Sigstore, minisign, and OpenPGP implementations
- installer: direct, archive, and script execution
- receipt: canonical recipe digest, ownership, drift, and rollback metadata
- policy: interactive, `--yes`, scheduled, verified, and transport decisions

Resolver actions already retain `schema.Package`, so reconciliation can detect a
recipe-backed external package and delegate execution without changing every
adapter. Dependencies such as HTTP clients, command runners, clock, filesystem,
and host facts are injected into the engine for deterministic tests.

## CLI And Output

No remote recipe registry or large flag-based recipe builder is introduced.
Users author local JSON and validate it with `genv validate`.

Human and JSON plans include remote version, selected asset, verification state,
install type, and destination scope. `--dry-run` resolves metadata and selection
but does not download payloads or execute installers. Secret-bearing URL query
parameters and headers are redacted.

The existing `--target`, `--only`, `--skip`, `--only-manager`, `--skip-manager`,
`--timeout`, `--yes`, and background update settings apply consistently.
`migrate` continues to produce version 8 when no managed external recipe is
needed. `export` preserves version 9 when its selected package set contains a
managed recipe; otherwise its existing version 8 output remains valid.

## Testing

- Schema table tests for every union, invalid combination, unknown field, and
  v1-v8 compatibility.
- Golden JSON Schema tests for the v9 mirror.
- Provider tests with `httptest` for GitHub pagination/channel selection,
  structured HTTP parsing, authentication, limits, redirects, and errors.
- Platform tests across Linux/macOS/Windows, amd64/arm64, and glibc/musl.
- Verifier tests using fixed local fixtures for every supported method, including
  wrong keys, identities, fingerprints, and modified payloads.
- Archive adversarial tests for traversal, links, devices, duplicate paths,
  oversized expansion, and partial writes.
- Installer tests for atomic replacement, rollback, modes, script argv/env,
  PowerShell selection, elevation policy, and uninstall receipts.
- Resolver tests for apply, adopt, status, remove, upgrade, dry-run, lock writes,
  recipe drift, constraints, and failures.
- Update-policy tests proving unverified recipes and non-opted-in scripts cannot
  run unattended.
- Linux and Windows integration tests using local HTTP fixtures. macOS behavior
  is covered by unit tests plus a release smoke test because CI host mutation is
  inappropriate.

## Delivery Slices

1. Schema v9, lock receipt, provider resolution, and platform selection.
2. Downloading, all verification methods, direct/archive installation, receipts,
   rollback, and removal.
3. Script installers, uninstall actions, elevation, and unattended policy.
4. Resolver, status, upgrade, scheduled updates, output, documentation, and
   cross-platform integration coverage.

Each slice must preserve the legacy presence-only external behavior and pass the
full repository CI gate before the next slice begins.
