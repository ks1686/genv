# Contributing to genv

`genv` is a personal, solo-maintained project. I build it primarily to manage my
own machines, and I'm sharing it in case it's useful to others. There is no broad
call for contributors — but bug reports are genuinely appreciated, and the notes
below explain how to file a good one and what to expect.

---

## Maintainer

`genv` is maintained by [Karim Smires](https://github.com/ks1686).

Because it is a personal tool, I make no promises about response times, feature
requests, or accepting external pull requests. I may decline changes that don't
fit how I use `genv`, even if they're well-made — please don't take it personally.

## Contributors

- [Omar Waseem](https://github.com/OWaseem) — a friend and trusted contributor
  who added the WSL2 host detection and PATH sanitization, the macOS
  Homebrew/cask fixes and integration tests, the `brew services` wrapper for
  macOS-managed services, and the original macOS and WSL2 install guides. Omar
  contributes by arrangement rather than through the open PR queue.

---

## Reporting bugs

Bug reports are the most useful thing you can send. If `genv` crashes, produces
wrong output, or behaves unexpectedly, please [open an issue](https://github.com/ks1686/genv/issues).

Search [existing issues](https://github.com/ks1686/genv/issues) first to avoid
duplicates. A good bug report includes:

- `genv version` output
- Operating system and package manager(s) in use (macOS / Arch / Ubuntu / WSL2 / native Windows, and which of `pacman`/`paru`/`yay`/`apt`/`dnf`/`apk`/`snap`/`brew`/`linuxbrew`/`bun`/`uv`/`winget`/`scoop`/`choco`)
- The `genv.json` content (or a minimal reproduction)
- The exact command you ran
- The actual output vs. what you expected
- If possible, re-run with `--debug` and include the debug output

Clear reproductions get fixed fastest.

---

## Pull requests

Since this is a personal project, please **open an issue before sending a PR** so
we can agree the change is a fit before you spend time on it. Unsolicited PRs may
sit or be declined.

If we've agreed on a change:

- Work in a feature branch off `main`.
- Include tests for any behavior change (`go test ./...`).
- Keep the diff small and focused — one logical change per PR.
- Describe what broke and how you fixed it, and link the related issue.

---

## Development setup

```bash
git clone https://github.com/ks1686/genv.git
cd genv
go build .          # build the binary
go test ./...       # run all unit tests
```

Integration tests (require Docker):

```bash
go test -tags integration ./internal/adapter/

# Full schemaVersion 8 CLI matrix (builds genv in Arch and runs every command):
make integration-v8
```

GitHub Actions (`CI`, `Integration Tests`, `Regression`) run on every push and pull request. `Regression` pins fail-closed `add`, v8 empty specs, path sandbox, and related review-stack tests.

---

## Code style

- Follow standard Go conventions (`gofmt`, `go vet`).
- Keep functions focused; prefer small, testable units.
- Match the naming and structure of existing files.
- New adapter methods must implement the full `Adapter` interface defined in `internal/adapter/adapter.go`.
- Adapters may also implement optional extensions (`Searchable`, `VersionLister`, `BatchUpgrader`) when the underlying package manager supports those capabilities.
- All user-facing errors must include a corrective action or next step.

---

## Questions

If you're unsure whether something is a bug or intended behavior, open an issue
and ask. I'm happy to clarify, time permitting.
