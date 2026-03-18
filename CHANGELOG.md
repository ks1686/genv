# Changelog

All notable changes to this project will be documented in this file.

## Unreleased

- `gpm adopt <id>` — track an already-installed package in gpm.json and the lock file without reinstalling it
- `gpm disown <id>` — stop tracking a package without uninstalling it, leaving it under the native package manager
- E2E Docker integration tests for adopt and disown across all supported Linux distros

## v0.1.0-beta.1 - 2026-03-17

First public pre-release of `gpm`.

Highlights:

- core CLI commands: `add`, `remove`, `list`, `apply`, `edit`, `clean`, and `version`
- `gpm.json` schema v1 and validation
- declarative apply flow backed by `gpm.lock.json`
- resolver and adapter support for Linux, macOS, and WSL2-oriented environments
- GitHub release automation with versioned binaries and checksums

Notes:

- this release reflects the current usable state of the project rather than a strict milestone boundary
- it includes Milestone 2 functionality that is still undergoing final validation on some target environments
