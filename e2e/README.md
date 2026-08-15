# genv end-to-end tests

This directory holds integration tests that compile and run the real `genv`
binary against isolated temporary directories. The tests are guarded by the
`integration` build tag so they stay out of the normal unit-test suite.

`TestMain` builds the binary once (`go build github.com/ks1686/genv`) into a
temp dir and points every test at it, so the suite always exercises the current
source.

## Prerequisites

- A Go toolchain compatible with the version declared in `go.mod`.
- No external package managers are needed for the files scenarios in
  `files_test.go`; each one creates its own temp HOME and uses checked-in
  fixtures under `testdata/`.
- The package-manager scenarios in `e2e_test.go` auto-skip unless the matching
  binary (`brew`, `paru`, `yay`, or `snap`) is on `PATH`. When present, they
  install and remove a small real package (`tree`, or `hello` for snap), so run
  them only where that side effect is acceptable.

## Running the tests

Run the whole suite:

```bash
go test ./e2e/... -tags integration
```

Run just the files-block scenarios (hermetic, no package manager required):

```bash
go test ./e2e/... -tags integration -run 'TestFiles_' -v
```

Run a single scenario:

```bash
go test ./e2e/... -tags integration -run TestFiles_S2_ApplyClean -v
```

Run one adapter suite with the race detector:

```bash
go test -tags integration -race -v -run TestE2EBrew ./e2e/
```

Latest local run of the eight files scenarios on macOS (arm64, Go 1.26):

```
--- PASS: TestFiles_S1_FreshEmptyHome (0.28s)
--- PASS: TestFiles_S2_ApplyClean (0.01s)
--- PASS: TestFiles_S3_MismatchNoForce (0.00s)
--- PASS: TestFiles_S4_MismatchForceBackup (0.01s)
--- PASS: TestFiles_S5_CodexTemplatedDrift (0.02s)
--- PASS: TestFiles_S6_DryRun (0.00s)
--- PASS: TestFiles_S7_AdoptFilesRegistersRenderedConfig (0.01s)
--- PASS: TestFiles_S8_AdoptFilesKeepsSpecRepoClean (0.05s)
ok  	github.com/ks1686/genv/e2e	0.668s
```

## Test suites

### Package-manager command suite (`e2e_test.go`)

`runE2ESuite` drives the full M1/M2 command set against a live package manager
using `t.Run` subtests, so `-run` filtering works down to the subtest. Entry
points: `TestE2EParu`, `TestE2EYay`, `TestE2EBrew`, `TestE2ESnap`. Each one
skips itself when its adapter binary is absent, so the same test binary works
across hosts.

It covers `add`, `remove` (and the `rm` alias), `list` (and `ls`), `adopt`,
`disown`, `scan`, `apply`, `apply --dry-run`, `apply --yes`, `apply --strict`,
`status`, and `clean`, plus the JSON envelope on `apply`, `status`, and `scan`.
It also checks lock-file integrity after each mutation and the error paths:
duplicate add, remove/adopt/disown of an untracked package, and apply or status
with no `genv.json`.

`TestE2EServiceLifecycle` exercises the schema-v4 services block:
`service add/remove/list/start/stop/status`, apply starting and stopping
declared services, and lock-file tracking of service state.

Every command is invoked with `--file` (spec path) and `--lock-file` (lock
path) pinned into the test's temp dir, and with `GENV_NO_INTERACTIVE=1` so the
binary never blocks on a prompt.

### Files-block scenarios (`files_test.go`)

`TestFiles_S1` through `TestFiles_S8` cover the schema-v5 `files` block shipped
in v2.2.0 and v2.3.0. Each scenario runs in an isolated temp HOME, writes a v5
`genv.json` at runtime whose `repo.url` points at `testdata/files-v5/repo`, and
drives the real binary. All eight currently pass.

| Scenario | Command exercised | What it asserts |
| --- | --- | --- |
| S1 FreshEmptyHome | `status --files` | Empty HOME: exits 4 and reports each target `missing`. |
| S2 ApplyClean | `apply --yes` | Creates the link, then `status --files` exits 0 (`ok` / `up to date`). |
| S3 MismatchNoForce | `apply --yes` | A real file blocking a link target: exits 4 and leaves that file byte-for-byte untouched (still a regular file). Packages/services still apply when present. |
| S4 MismatchForceBackup | `apply --force --yes` | Backs up the blocking file to one `target.backup.<timestamp>`, installs the symlink, then `status --files` exits 0. |
| S5 CodexTemplatedDrift | `status --files`, `apply --force --yes` | Stale rendered `copy-template` target is flagged `mismatch` (exit 4); force re-renders it with the real HOME (no literal `__HOME__`), writes one backup, and status then exits 0. |
| S6 DryRun | `apply --dry-run --force --yes` | Reports the planned change but writes nothing to disk and creates no backup. |
| S7 AdoptFilesRegistersRenderedConfig | `adopt --files` | Records an already-rendered `copy-template` target into the lock as mode `copy` without touching the file or writing a backup; `status --files` then exits 0. |
| S8 AdoptFilesKeepsSpecRepoClean | `adopt --files` | Against a git-backed spec repo, leaves the working tree clean (`git status --porcelain` empty). Skips when `git` is unavailable. |

## Files-block behavior reference

The `files` block has three sub-lists, each with a per-record `host` selector
and optional `backup` flag.

`links[]` supports three modes:

- `link`: a plain symlink. A wrong or dangling symlink, or any real file or
  directory sitting at the target, is a `mismatch` unless `--force` is passed.
- `managed-link`: self-heals. A wrong or dangling symlink at the target is
  silently relinked without `--force`. A hand-authored real file there still
  needs `--force`.
- `merge-dir`: symlinks every file under the source directory individually into
  the target directory, each with managed-link semantics. Multiple records can
  target the same directory and layer, so a later record's same-named file wins
  over an earlier one without `--force`. `status --files` reports one entry per
  merged file. (The end-to-end scenarios cover `link` and `copy-template`
  directly; `merge-dir` and `dirs` behavior is covered by the unit tests in
  `internal/files`.)

`templates[]` (`copy` / `copy-template`) render placeholder tokens and write the
result atomically. Supported tokens are `__HOME__`, `__USER__`, `__HOST__`,
`__OS__`, and `__ARCH__`; any unknown `__*__` token is left literal.

`dirs[]` ensure a directory exists at the target.

Shared behavior across modes:

- `--force` overwrites a mismatched target.
- `backup` (per record, or `--backup` on apply) preserves the old target as
  `target.backup.<timestamp>` instead of deleting it.
- `--dry-run` reports the plan and writes nothing.
- Legacy `host` filters (schema v1–v7) select records by classified host; v4.0.0+ prefers schema v8 `targets.*` (`macos`, `windows`, `arch`, `ubuntu`, `wsl-arch`). Classification no longer returns `wsl2`. Ubuntu-like WSL2 maps to `ubuntu`; Arch-like WSL2 maps to `wsl-arch` and does not inherit native `arch`.

## Commands and flags exercised

- `genv status --files`: live-filesystem parity check for the `files` block,
  separate from the spec-vs-lock `genv status`. Exits 4 when it finds drift
  (`missing`, `mismatch`, `wrong-type`) and 0 when everything is in sync.
- `genv apply [--yes] [--force] [--dry-run]`: reconciles the `files` block along
  with packages and services.
- `genv adopt --files`: registers already-managed files into the lock without
  rewriting the targets; template targets are rendered before comparison.
- `--file <path>`: spec location, injected by the test runners.
- `--lock-file <path>`: lock location. The default is
  `~/.config/genv/genv.lock.json` under HOME. The package-manager suite passes
  `--lock-file` to keep the lock inside its temp dir; the files suite relies on
  the HOME-based default inside its isolated temp HOME.

Exit codes used across the suite: `0` success, `1` usage error, `2` filesystem
or serialization error, `3` schema validation failure, `4` semantic error or
drift.

## Fixture layout

```
e2e/testdata/files-v5/
├── genv.json              # sample v5 spec, reference only
└── repo/
    ├── simple.txt         # source for link / managed-link scenarios
    └── codex-config.toml  # source for copy-template scenarios (contains __HOME__ / __USER__)
```

The scenarios do not load the checked-in `genv.json`. They generate a real
`genv.json` at runtime and set its `repo` field to `testdata/files-v5/repo`, so
source paths stay hermetic and portable. The checked-in `genv.json` is kept only
as a readable example of the v5 shape.

## Interpreting failures

- A files scenario that fails on a wrong exit code usually means the CLI drift
  contract moved: S1/S3/S5 expect exit `4` on drift and the rest expect `0`.
- A missing-symlink or wrong-type failure points at the apply path in
  `internal/files` (`links.go`, `applier.go`, `dirs.go`), not the test harness.
- A template scenario that leaves a literal `__HOME__` in the output points at
  the renderer in `internal/files/template.go`.
- Package-manager subtests that report "skipping" simply did not find the
  adapter binary on `PATH`; that is expected off the target distro.
- On Windows, symlink creation needs Developer Mode or an elevated process;
  `internal/files/links.go` adds that hint to the underlying error.
