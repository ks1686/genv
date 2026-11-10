---
name: verify-genv
description: "Drive the genv CLI the way a user does, in an isolated tmux session with a throwaway HOME and XDG config directory. Use when proving init, env, shell aliases, status, or validate, or when a change must not touch the operator's ~/.config/genv."
---

# Verify genv

genv is a short-lived CLI. There is no server to keep up. Launch builds the binary once. Every later command runs in its own isolated tmux session, which stands in for the user's terminal.

The session is the only place a verification command may run `genv`. A shell outside that session uses the operator's `HOME` and can write `~/.config/genv`.

This directory is under the repo gitignore for `.cursor/`. Track the skill with `git add -f .cursor/skills/verify-genv`.

## Launch

From the repo root:

```bash
.cursor/skills/verify-genv/scripts/verify-genv launch
```

Launch is finished when it prints `run_id`, `home`, `spec_dir`, `evidence`, `session`, and `binary`, and the session is sitting at the prompt `GENVVERIFY>`.

What launch does:

- Builds `genv` with `GOTOOLCHAIN=go1.26.10`, matching `go.mod`, and puts the binary on the session `PATH` ahead of anything else.
- Creates a throwaway home under `/tmp/genv-verify-runs/<run-id>/home`.
- Sets `XDG_CONFIG_HOME` to `<home>/.config`, so the default spec is `<home>/.config/genv/genv.json`.
- Starts tmux session `genv-verify-<run-id>` with `env -i`. The tmux server inherits the operator environment. Without a cleared environment the session would read and write the real config directory.
- Forces `SHELL=/bin/bash`. On Linux and macOS, `genv apply` injects its source line into `~/.bashrc` for bash and `~/.zshrc` for zsh. This session always exercises the bash file.
- On Ubuntu, interactive bash sources `/etc/bash.bashrc` before the harness rc file. That file prints a sudo hint when the throwaway home has no `.sudo_as_admin_successful` and the user is in group `sudo` or `admin`. The hint is not genv running sudo. Ignore that banner. Fail the journey if the transcript contains an install or uninstall command.
- Records a fingerprint of the operator's `~/.config/genv` (and `$XDG_CONFIG_HOME/genv` when that variable is set and points somewhere else). Doctor and cleanup fail if that fingerprint changes.
- Writes proof under `$GENV_VERIFY_EVIDENCE/<run-id>` when that variable is set, otherwise `/tmp/genv-verify-evidence/<run-id>`.

Ready means the prompt is up and `verify-genv doctor` passes. A built binary with a dead session is not ready.

Two runs can exist at once. Each launch uses a new home, a new session, and a new evidence directory. Commands after launch talk to the latest run. Set `GENV_VERIFY_RUN` to a printed `run_id` to select an older one.

`go` 1.26.10 and `tmux` 3.2 or newer must be on `PATH`. Older tmux has no literal `send-keys -l`, and the harness will type flags instead of the command. On this VM the harness passes `-f /exec-daemon/tmux.portal.conf` when that file exists. A machine without that file uses the default tmux.

## Doctor

Run this before the first command in a session, and again after any command that did something unexpected.

```bash
.cursor/skills/verify-genv/scripts/verify-genv doctor
```

Doctor exits 0 only when all of these are true.

- The tmux session from launch is still alive.
- Inside the session, `HOME` is the throwaway home, `XDG_CONFIG_HOME` is `<home>/.config`, and `command -v genv` is the binary launch built.
- `GENV_NO_INTERACTIVE`, `GENV_TARGET`, and `GENV_HOST` are unset. `GENV_NO_INTERACTIVE` makes `genv apply` treat the terminal as closed and refuse to apply. `GENV_TARGET` overrides host classification.
- `SHELL` is `/bin/bash`.
- `genv version` prints a line that starts with `genv `.
- The operator config fingerprint is unchanged.

Doctor types two read-only commands into the session. Run it only at the `GENVVERIFY>` prompt. If a command is waiting for `y` or `N`, answer or end that command before doctor, or doctor will type into the prompt.

## Drive

Type commands with `drive`. Answer a prompt with `respond`. Both append the pane to the evidence directory.

```bash
.cursor/skills/verify-genv/scripts/verify-genv drive -- genv version
.cursor/skills/verify-genv/scripts/verify-genv drive --until 'Continue\? \[y/N\]' -- "genv apply --skip-packages"
.cursor/skills/verify-genv/scripts/verify-genv respond -- y
```

`drive` joins the words after `--` with spaces and types that string, then Enter. Quote the whole command when it contains quotes or shell operators, so the outer shell does not eat them:

```bash
.cursor/skills/verify-genv/scripts/verify-genv drive -- '. "$HOME/.bashrc" && printf "EDITOR=%s\n" "$EDITOR"'
```

`--until` is an extended regular expression. The default waits for a new line `GENV_DRIVE_EXIT:<code>`. That line is the session's `PROMPT_COMMAND`, not genv output. `drive` and `respond` then exit with `<code>`. Pass `--until` when the command stops for input before it exits. The apply confirmation line contains `Continue? [y/N]`.

`--timeout` is seconds. The default is 90.

`respond` with no text sends only Enter. That is how `genv init` finishes its package list.

```bash
.cursor/skills/verify-genv/scripts/verify-genv respond --until 'package id \(or Enter to finish\)' -- git
.cursor/skills/verify-genv/scripts/verify-genv respond --
```

The recipes for each user journey live in `features/`. Follow the file for the journey. Do not swap in a different command and call the journey verified.

`genv env set` and `genv shell alias set` write the active target (`--target`, else `GENV_TARGET`, else host classification). They do not write `defaults`. On Ubuntu the bucket is `targets.ubuntu`. A new spec from these commands, or from `genv init`, is schemaVersion 8 and already has a bucket for every known target.

`genv apply` without `--yes` asks `Continue? [y/N]` when stdin is a terminal. Only `y` or `Y` accepts. Enter on this session declines and prints `Aborted.` with exit 0. A closed stdin exits 4 and applies nothing. `--json` without `--yes` is plan-only.

`genv apply --skip-packages` still writes env and shell fragments and still asks before it writes. The plan header starts with `Apply plan` and names files, env, and services. Use that flag for env and shell journeys. A plain `genv apply`, and `genv add`, `genv remove`, `genv upgrade`, `genv scan`, and `genv clean`, call the host package managers. Those managers are outside `HOME`. Do not drive them on a shared machine.

After a bash apply, the fragment is `<spec dir>/env.sh` or `shell.sh`, and `~/.bashrc` gains a `# genv env` source line. That injection happens only when the state directory is the default config directory. A `--file` or `--state-dir` outside that directory writes the fragment and skips the rc file. Verification uses the default spec path so the rc file changes.

## Evidence

Proof is the command, the exit code, and the files genv wrote. The final screen is not enough.

Each `drive` and `respond` appends the pane to `<evidence>/drives/NNN.txt` and a line to `<evidence>/commands.log`. Launch also writes `<evidence>/build.log`, `<evidence>/run.env`, `<evidence>/operator-watch.tsv`, and `<evidence>/pane.log`.

Record the feature id before cleanup:

```bash
printf '%s\n' env > "$(.cursor/skills/verify-genv/scripts/verify-genv paths evidence)/feature.txt"
```

Copy files out of the throwaway home before cleanup. `capture` refuses a path outside that home, and it refuses a name with a slash.

```bash
.cursor/skills/verify-genv/scripts/verify-genv capture env.sh "$(.cursor/skills/verify-genv/scripts/verify-genv paths spec-dir)/env.sh"
.cursor/skills/verify-genv/scripts/verify-genv capture bashrc "$(.cursor/skills/verify-genv/scripts/verify-genv paths home)/.bashrc"
```

Paths:

```bash
.cursor/skills/verify-genv/scripts/verify-genv paths home
.cursor/skills/verify-genv/scripts/verify-genv paths spec-dir
.cursor/skills/verify-genv/scripts/verify-genv paths spec
.cursor/skills/verify-genv/scripts/verify-genv paths evidence
.cursor/skills/verify-genv/scripts/verify-genv paths binary
.cursor/skills/verify-genv/scripts/verify-genv paths session
.cursor/skills/verify-genv/scripts/verify-genv paths run-id
```

Set `GENV_VERIFY_EVIDENCE` before launch when the proof must land in a review directory. Cleanup never deletes that tree.

A dry-run is not proof of an apply. `genv apply --dry-run` prints the plan and leaves `env.sh`, the lock, and `~/.bashrc` untouched. When a journey uses `--skip-packages`, prove the skip by the plan header and by the absence of a package install command in the transcript, then prove the env or shell files were written.

## Cleanup

```bash
.cursor/skills/verify-genv/scripts/verify-genv cleanup
```

Cleanup kills tmux session `genv-verify-<run-id>` and deletes `/tmp/genv-verify-runs/<run-id>`. It does not delete the evidence directory. It writes `<evidence>/cleanup-ok.txt` when the operator fingerprint still matches. A mismatch still removes the session and the throwaway home, writes `<evidence>/cleanup-failed.txt`, and exits non-zero.

Run cleanup after a failed launch or a failed drive so a session is not left behind. Then check the evidence path still exists:

```bash
test -f "$(.cursor/skills/verify-genv/scripts/verify-genv paths evidence)/run.env"
```

That check has to happen before cleanup if you still need `paths`. After cleanup, use the evidence path launch printed. `paths` will not resolve a removed run.

`cleanup` with no recorded run exits 0.

## Helpers

The harness is `.cursor/skills/verify-genv/scripts/verify-genv`. It must be executable. Every invocation above is a complete command from the repo root.

| Command | Role |
| --- | --- |
| `launch` | Build, isolate, start tmux, print paths |
| `doctor` | Session, binary, env, operator fingerprint |
| `paths <name>` | Print one path. No name prints every path |
| `drive -- <command>` | Type a command and wait for it to finish |
| `respond -- <text>` | Type an answer and Enter |
| `capture <name> <path>` | Copy one throwaway-home file into evidence |
| `cleanup` | Kill the session and delete the throwaway home |

## Platform limits

Proved on Linux with bash, the harness covers default-path env and shell writes, `~/.bashrc`, and host class `ubuntu`.

Not proved here:

- macOS and zsh, where the same apply writes `~/.zshrc` instead of `~/.bashrc`.
- Windows, where the fragment is `env.ps1` or `shell.ps1` and the profile backend is PowerShell.
- `genv add`, `genv remove`, wet `genv apply` of packages, `genv upgrade`, `genv scan`, and `genv clean`. Those change package databases that a throwaway `HOME` does not contain.

Do not mark those as verified because a Linux bash env apply passed.

## Maintenance

When genv's commands or prompts change, run `/maintain-verification-skill` on this skill. Update the feature file and re-drive the journey in a throwaway session. A doc-only edit that was not driven is not a correction.
