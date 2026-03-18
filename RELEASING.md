# Releasing gpm

This repository is set up to publish GitHub releases from annotated tags.

## Versioning

Use full semantic versions for tags and releases:

- `v0.1.0-beta.1` — first public prerelease (shipped)
- `v0.1.0` — first stable release; M1 and M2 complete and validated on Linux
- `v0.2.0` — M3 complete (scan, version pinning, status)
- `v0.3.0` — M4 complete (reliability and automation)
- `v0.4.0` — M5 complete (macOS and WSL2 support)

## One-time setup

1. Push the new [release workflow](.github/workflows/release.yml) and [.goreleaser.yml](.goreleaser.yml) to `main`.
2. In the GitHub repository, confirm Actions are enabled and that the default `GITHUB_TOKEN` has permission to create releases.

## Release checklist

1. Make sure `main` is the exact commit you want to ship.
2. Run the local verification step:

   ```bash
   make ci
   ```

3. Create an annotated tag for the release:

   ```bash
   git checkout main
   git pull --ff-only origin main
   git tag -a v0.1.0-beta.1 -m "gpm v0.1.0-beta.1"
   git push origin v0.1.0-beta.1
   ```

4. Watch the `Release` workflow in GitHub Actions.
5. When it completes, GitHub will contain a release with:
   - Linux, macOS, and Windows archives
   - a `checksums.txt` file
   - binaries stamped with version metadata visible via `gpm version`

## Release note framing

For each release, the notes should cover:

- what milestone is complete
- any known limitations or partially-validated surfaces (e.g., adapters not tested in CI)
- any breaking changes to `gpm.json` schema or lock format

## Verifying the published release

Download one artifact from the GitHub release page and verify:

```bash
./gpm version
```

Expected output should include the tag, for example `gpm v0.1.0`.
For this prerelease, expect `gpm v0.1.0-beta.1`.

## If you want to dry-run packaging locally

Install GoReleaser, then run:

```bash
goreleaser release --clean --snapshot
```

This builds the release artifacts locally without publishing a GitHub release.
