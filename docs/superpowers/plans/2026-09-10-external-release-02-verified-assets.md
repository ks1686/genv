# External Release Management: Verified Assets Plan

## Goal

Download and verify external release artifacts, install direct executables and
archives transactionally, write ownership receipts, and safely upgrade or remove
owned files.

## Architecture, Tools, And Constraints

- Depends on plan 01 provider, selector, schema, and receipt types.
- Payloads are verified before extraction or execution.
- Support SHA-256, GitHub digest, Sigstore, minisign, and OpenPGP in this slice.
- Prefer maintained pure-Go verifier libraries so behavior works on every target
  without requiring `cosign`, `minisign`, or `gpg` executables.
- Pin signer identity, issuer, key, and full fingerprint according to the recipe.
- Never remove an adopted or modified path without ownership evidence.
- Consult current upstream documentation for each verifier library before adding
  dependencies; record selected modules and rationale in the PR description.

## File Map

- `go.mod`, `go.sum`: approved verification and tar.zst dependencies.
- `internal/external/download.go`: bounded HTTPS transport and private staging.
- `internal/external/verify.go`: verifier interface and policy dispatch.
- `internal/external/verify_sha256.go`: literal, structured, checksum-file, and
  GitHub digest verification.
- `internal/external/verify_sigstore.go`: bundle and identity verification.
- `internal/external/verify_minisign.go`: pinned minisign key verification.
- `internal/external/verify_openpgp.go`: pinned key and fingerprint verification.
- `internal/external/archive.go`: safe ZIP/tar extraction.
- `internal/external/install.go`: direct/archive staging, atomic replacement,
  rollback, and modes.
- `internal/external/receipt.go`: recipe hashing and owned-file drift checks.
- `internal/external/engine.go`: resolve/download/verify/install transaction.
- `internal/external/testdata/`: fixed signed payload fixtures and public keys.
- `internal/resolver/resolver.go`: delegate recipe-backed asset actions and retain
  old locks on failed transactions.
- `internal/resolver/*_test.go`: apply, upgrade, remove, and rollback integration.

## Task 1: Bounded Downloader

1. Add tests for HTTPS success, timeout, cancellation, maximum metadata/artifact
   sizes, redirect limits, HTTPS-to-HTTP rejection, owner-only temporary modes,
   cleanup, and redacted credential-bearing errors.
2. Run `go test ./internal/external -run Download -count=1` and confirm failure.
3. Implement the downloader with injected client, limits, and temporary root.
4. Run `go test ./internal/external -run Download -count=1`.

## Task 2: Verification Matrix

1. Add deterministic fixtures and tests for every verifier's success path plus
   modified payload, wrong digest, malformed checksum file, duplicate checksum,
   wrong Sigstore identity/issuer, wrong minisign key, wrong OpenPGP key, short
   fingerprint rejection, expired/invalid signatures, and cancellation.
2. Run `go test ./internal/external -run Verify -count=1` and confirm failure.
3. Implement all verifiers. Require every configured method to pass and always
   calculate the final payload SHA-256 for the receipt.
4. Run `go test ./internal/external -run Verify -count=1`.

## Task 3: Safe Archive Handling

1. Add adversarial ZIP and tar fixtures covering `..`, absolute paths, escaping
   symlinks/hardlinks, devices, duplicate names, expansion limits, wrong strip
   counts, missing files, and ambiguous files.
2. Add positive tests for ZIP, tar, tar.gz, tar.xz, and tar.zst.
3. Run `go test ./internal/external -run Archive -count=1` and confirm failure.
4. Implement extraction into a private staging directory and copy only declared
   regular files.
5. Run `go test ./internal/external -run Archive -count=1`.

## Task 4: Transactional Direct And Archive Installation

1. Add tests for user destinations, explicit system scope, executable modes,
   parent creation, replacement, cross-filesystem fallback, rollback after each
   failure point, and unchanged unrelated paths.
2. Run `go test ./internal/external -run 'Install|Rollback' -count=1` and confirm
   failure.
3. Implement stage, backup, atomic replace, post-install digest, and rollback.
4. Run `go test ./internal/external -run 'Install|Rollback' -count=1`.

## Task 5: Resolver And Receipt Lifecycle

1. Add failing resolver tests for missing install, successful post-detection,
   failed detection rollback, adopted non-ownership, recipe drift, verified
   upgrade, safe removal, modified-file refusal, dry-run without download, and
   lock preservation on failure.
2. Run `go test ./internal/resolver -run External -count=1`.
3. Delegate managed `external` actions to the engine without changing legacy
   track-only handling or other adapters.
4. Store the receipt only after installation and detection succeed. Remove only
   receipt-owned paths whose digests still match.
5. Run `go test ./internal/resolver -run External -count=1`.

## Verification And Review Gate

Run:

```bash
go test ./internal/external ./internal/resolver
go test -race ./internal/external ./internal/resolver
go test ./...
make bench
go vet ./...
git diff --check
```

Run the repository security scan, inspect new dependency licenses and advisories,
and manually test a locally served signed direct asset and archive on Ubuntu and
Windows. Do not start plan 03 until rollback and malicious-archive tests pass.
