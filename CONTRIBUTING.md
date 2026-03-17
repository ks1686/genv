# Contributing to gpm

Thank you for your interest in contributing to `gpm`!

This project is in active early development. The core team is working through the
milestones defined in [ROADMAP.md](ROADMAP.md). To keep things moving efficiently,
we ask that community contributions stay focused on bugs and performance for now.

---

## Where to focus

**Bug reports and bug fixes** are always welcome.
If something crashes, produces wrong output, or behaves unexpectedly, please open
an issue or submit a fix.

**Performance optimizations** are also a great way to help.
If you spot an inefficiency in the resolver, planner, or any adapter, feel free to
open an issue or send a PR with benchmarks.

**Feature work** is currently handled by the development team.
Unless a milestone in [ROADMAP.md](ROADMAP.md) is explicitly tagged **"help needed"**,
please hold off on feature PRs. We want to keep the scope tight during early
milestones so the spec and core CLI stay stable.

---

## How to contribute

1. **Check existing issues** — search [Issues](../../issues) before opening a new one
   to avoid duplicates.
2. **Open an issue first** — for anything beyond a trivial fix, open an issue to
   discuss the problem before sending a PR.
3. **Fork and branch** — work in a feature branch off `main`:
   ```bash
   git checkout -b fix/short-description
   ```
4. **Write or update tests** — all changes must include tests. Run the existing suite
   with:
   ```bash
   go test ./...
   ```
5. **Keep the diff small** — one logical change per PR makes review faster.
6. **Describe your change** — explain what broke and how you fixed it in the PR
   description. Link the related issue.
7. **Submit a pull request** — a maintainer will review and may ask for changes.

---

## Bug reports

A good bug report includes:

- `gpm` version (`go version` output and the binary version if applicable)
- Operating system and package manager(s) in use
- The `gpm.json` content (or a minimal reproduction)
- The exact command you ran
- The actual output vs. what you expected

---

## Code style

- Follow standard Go conventions (`gofmt`, `go vet`).
- Keep functions focused; prefer small, testable units.
- Match the naming and structure of existing files.

---

## Questions

If you are unsure whether something is a bug or a feature, open a discussion or an
issue and ask. We are happy to clarify.
