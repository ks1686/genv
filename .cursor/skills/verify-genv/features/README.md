# genv verification map

This directory is the source for verifying genv's user-facing CLI. Read this index, then follow one feature file. The harness and the isolation rules live in `../SKILL.md`.

## Baseline preconditions

- Run `verify-genv launch`, export `GENV_VERIFY_RUN` to the printed `run_id`, then `verify-genv doctor`, and require `doctor ok`.
- Use the throwaway `HOME` and `XDG_CONFIG_HOME` from that launch. The default spec is `<spec-dir>/genv.json`.
- Do not export `GENV_HOST` or `GENV_NO_INTERACTIVE`. Leave `GENV_TARGET` to the harness. It stays unset when genv can classify the host, and it is `linux` when genv cannot.
- Start each feature from a new launch unless that feature says to continue a session.
- Never drive a session this run did not start.

## Driving conventions

`verify-genv` in these files is `.cursor/skills/verify-genv/scripts/verify-genv`, run from the repo root.

- Type user commands with `verify-genv drive`.
- Answer prompts with `verify-genv respond`.
- Treat every command in a feature file as literal. Keep flags and quotes unchanged.
- Quote a drive command as one string when it contains quotes or `&&`.
- Pass `--until` when a command waits for input before it exits.
- Copy proof files with `verify-genv capture` before `verify-genv cleanup "$GENV_VERIFY_RUN"`.
- The harness joins wrapped pane lines in the saved drive transcript. Search that file for a phrase. A raw 80-column capture can split the phrase across lines.
- Cleanup removes the session and the throwaway home. It leaves the evidence directory.

## Proof and skip reporting

- Capture the command, its exit code, and the files that changed.
- A mutation is proved by a second view of the stored value, not by the success line alone.
- Record the feature id in `<evidence>/feature.txt`.
- Report an unreachable path with the command that was typed and the precondition that failed.
- Do not report a skipped entry point as verified through a different command.

## Feature entry contract

Each feature file has an H1, one opening paragraph, then these H2 sections in order.

1. `Sub-features`
2. `How to get to it (user POV)`
3. `Driving it with verify-genv`
4. `Gotchas`

## Features

- [Initialize a spec](./init.md) covers the interactive `genv init` wizard, an empty finish, a second run that refuses to overwrite, and the spec file that results.
- [Set an environment variable](./env.md) covers `genv env set`, `genv env list`, the apply confirmation, the shell fragment, and a new shell that sees the variable.
- [Check status](./status.md) covers spec-versus-lock status for a variable that is not applied yet and for one that is.
- [Validate the spec](./validate.md) covers a missing spec, a valid spec, and a spec that fails validation.
- [Set a shell alias](./shell-alias.md) covers `genv shell alias set`, the apply confirmation, and `shell.sh`.

Package install and removal (`genv add`, `genv remove`, wet package `genv apply`, `genv upgrade`, `genv scan`, `genv clean`) are not in this map. Those commands change the host package database, which a throwaway home does not isolate. Add a feature for them only on a machine where that change is acceptable, and drive the real install.
