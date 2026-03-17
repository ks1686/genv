# Releasing gpm

This repository is set up to publish GitHub releases from annotated tags.

## Versioning

Use full semantic versions for tags and releases:

- `v0.1.0-beta.1` for the first public prerelease
- `v0.1.0` for the first stable release
- `v0.2.0` for the release where Milestone 2 is considered fully validated and complete

For this repository, `v0.1.0-beta.1` is the first public, current-state prerelease. It may include work from Milestone 2 that is functional but not yet fully validated across every target environment.

If you ship partially tested M2 functionality in `v0.1.0-beta.1`, say that plainly in the release notes.
Keep `v0.1.0` for the first stable release and `v0.2.0` for the point where Milestone 2 is complete and you want to signal stronger confidence in that surface area.

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

## Recommended release note framing for `v0.1.0-beta.1`

Use wording close to this:

- first public release of `gpm`
- explicitly a prerelease build
- stable core CLI and spec workflow from Milestone 1
- includes current resolver and adapter work from Milestone 2
- some cross-platform M2 paths are still in final validation

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
