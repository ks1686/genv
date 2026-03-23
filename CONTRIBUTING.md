# Contributing to genv

Thank you for your interest in contributing to `genv`!

The project has reached v0.2.0 with all core milestones (M1–M5) complete. We are now working toward v1.0.0, which focuses on API stability (M6) and developer experience (M7). Contributions in both areas are welcome.

---

## Where to focus

**Bug reports and bug fixes** are always welcome.
If something crashes, produces wrong output, or behaves unexpectedly, please open an issue or submit a fix.

**Performance optimizations** are a great way to help, especially for the resolver and adapter detection path (target: <200ms cold start).

**M6 and M7 feature work** is now open for community contributions. See [ROADMAP.md](ROADMAP.md) for the unchecked items. Good first targets:

- Shell completions (`genv completion <shell>`) — self-contained, no core changes needed.
- `genv validate` — validate `genv.json` without installing anything; straightforward CLI addition.
- Test coverage improvements — adding table-driven tests for uncovered code paths.
- Fuzz tests for the version constraint logic in `internal/version`.

**Adapter improvements** — if a package manager's install/uninstall/query behavior is wrong on your distro, fix the adapter in `internal/adapter/` and add a test.

---

## How to contribute

1. **Check existing issues** — search [Issues](../../issues) before opening a new one to avoid duplicates.
2. **Open an issue first** — for anything beyond a trivial fix, open an issue to discuss the problem before sending a PR.
3. **Fork and branch** — work in a feature branch off `main`:
   ```bash
   git checkout -b fix/short-description
   ```
4. **Write or update tests** — all changes must include tests. Run the existing suite with:
   ```bash
   go test ./...
   ```
5. **Keep the diff small** — one logical change per PR makes review faster.
6. **Describe your change** — explain what broke and how you fixed it in the PR description. Link the related issue.
7. **Update the roadmap checklist** — if your PR completes a checklist item in ROADMAP.md, check it off in the same PR.
8. **Submit a pull request** — a maintainer will review and may ask for changes.

---

## Bug reports

A good bug report includes:

- `genv version` output
- Operating system and package manager(s) in use
- The `genv.json` content (or a minimal reproduction)
- The exact command you ran
- The actual output vs. what you expected
- If possible, re-run with `--debug` and include the debug output

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
```

---

## Code style

- Follow standard Go conventions (`gofmt`, `go vet`).
- Keep functions focused; prefer small, testable units.
- Match the naming and structure of existing files.
- New adapter methods must implement the full `Adapter` interface defined in `internal/adapter/adapter.go`.
- All user-facing errors must include a corrective action or next step.

---

## Questions

If you are unsure whether something is a bug or a feature, open a discussion or an issue and ask. We are happy to clarify.
