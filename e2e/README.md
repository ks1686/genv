# genv end-to-end tests

This directory holds integration tests that compile and run the real `genv`
binary against a temporary HOME directory. The tests are guarded by the
`integration` build tag so they do not run with the normal unit-test suite.

## Running the tests

Build the binary once and run the integration suite:

```bash
go test ./e2e/... -tags integration
```

Run a single scenario:

```bash
go test ./e2e/... -tags integration -run TestFiles_S1_FreshEmptyHome -v
```

The tests require no external package managers; each scenario creates its own
temporary HOME directory and uses checked-in fixtures under `testdata/`.

## Expected RED state

The S1-S6 scenarios in `files_test.go` encode the schema-v5 `files` block
behavior (link, copy-template, managed-link, `--force`, `--dry-run`, and
`status --files`). These features are not yet implemented, so the tests
**compile** but are expected to be **RED** until the following work lands:

- Todo 1 — schema v5 (`files`, `hooks`, `repo`, `HostPredicate`, lock-path
  override)
- Todo 6 — files-block apply path (link/copy/templated-copy, `--force` gate,
  `Backup`, dry-run)
- Todo 7 — placeholder/template renderer (`__HOME__`, `__USER__`, etc.)
- Todo 9 — wiring `status --files`, `apply`, and `--lock-file` into `main.go`

Until then, `go test ./e2e/... -tags integration` will show failures such as
unknown flags (`--files`), rejected schema version `"5"`, or missing target
links. Those failures are intentional and provide a failing-first proof of the
S1-S6 contract.

## Fixture layout

```
e2e/testdata/files-v5/
├── genv.json              # sample v5 spec for reference
└── repo/
    ├── simple.txt         # source for link/copy tests
    └── codex-config.toml  # source for copy-template tests (contains __HOME__)
```

The tests generate the real `genv.json` at runtime and point its `repo` field
at `testdata/files-v5/repo` so that source paths stay hermetic and portable.
