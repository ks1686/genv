# Validate the spec

`genv validate` checks the default spec, or the file named by `--file`. A missing file fails. A spec from `genv init` or `genv env set` passes. A file that is not valid JSON fails.

## Sub-features

- `validate-missing` fails when the default spec does not exist.
- `validate-ok` passes a spec the user just created.
- `validate-bad` fails a file that does not parse.

## How to get to it (user POV)

- Run `genv validate` before any spec exists.
- Run `genv init` and finish with a blank line, or run `genv env set EDITOR nvim`.
- Run `genv validate`.
- Point `genv validate --file` at a broken JSON file.

## Driving it with verify-genv

Preconditions:

- `verify-genv doctor` printed `doctor ok` for this run.
- No spec exists yet at `verify-genv paths spec`.

- **Missing spec.** Run `verify-genv drive -- genv validate`. Exit code `3`. The pane contains `not found` and `genv init`.
- **Create a spec.** Run `verify-genv drive -- genv env set EDITOR nvim`. Exit code `0`.
- **Valid spec.** Run `verify-genv drive -- genv validate`. Exit code `0`. The pane contains `is valid.` and the spec path.
- **Broken file.** Run `verify-genv drive -- 'printf "%s\n" "{not json" > "$HOME/bad.json"'`. Exit code `0`. Then run `verify-genv drive -- 'genv validate --file "$HOME/bad.json"'`. Exit code `3`. The pane contains `genv validate` and the bad path. The default spec is still valid. Run `verify-genv drive -- genv validate` and require exit code `0`.
- **Proof.**

```bash
printf '%s\n' validate > "$(verify-genv paths evidence)/feature.txt"
verify-genv capture genv.json "$(verify-genv paths spec)"
verify-genv capture bad.json "$(verify-genv paths home)/bad.json"
```

## Gotchas

- Exit code `3` is a validation failure. Exit code `4` is a different semantic failure. Do not treat them as the same result.
- `genv validate` also checks genv-managed supervisor executables under the session home. A throwaway home has none, so a valid spec exits `0`. A real home with a dangling updates agent can exit `3` even when the spec parses. Read the extra line before calling the spec invalid.
- `--file` validates that path only. A failure there does not rewrite the default spec. Prove the default spec with a second `genv validate` that does not pass `--file`.
- Validation does not write `env.sh` and does not install packages.
