# Changelog

All notable changes to this project will be documented in this file.

## v0.1.0 - 2026-03-18

First stable release of `gpm`. Milestones M1 and M2 are complete and validated on Linux.

Highlights:

- core CLI commands: `add`, `remove`, `adopt`, `disown`, `list`, `apply`, `edit`, `clean`, and `version`
- `gpm.json` schema v1 with line-aware validation errors
- declarative apply flow backed by `gpm.lock.json`
- `gpm adopt` — track an already-installed package without reinstalling it
- `gpm disown` — stop tracking a package without uninstalling it
- resolver and adapter support for `apt`, `dnf`, `pacman`, `paru`, `yay`, `flatpak`, `snap`, `brew`, `macports`, and `linuxbrew`
- Docker-based integration tests validating all Linux adapters in CI
- Homebrew tap and AUR (`gpm-bin`) distribution

Notes:

- macOS (`brew`, `macports`) and WSL2 adapters are implemented but not yet validated in automated CI — tracked in Milestone M5
- `go install github.com/ks1686/gpm@latest` works on any platform with Go installed

## v0.1.0-beta.1 - 2026-03-17

First public pre-release of `gpm`.

Highlights:

- core CLI commands: `add`, `remove`, `list`, `apply`, `edit`, `clean`, and `version`
- `gpm.json` schema v1 and validation
- declarative apply flow backed by `gpm.lock.json`
- resolver and adapter support for Linux, macOS, and WSL2-oriented environments
- GitHub release automation with versioned binaries and checksums
