# Set an environment variable

`genv env set` records a variable in the spec. `genv apply` exports it into the shell fragment and sources that fragment from the shell rc file. `genv env list` shows the spec value before apply and the applied value after.

## Sub-features

- `env-set` adds `EDITOR` on the active target and creates the spec when it is missing.
- `env-list` prints the variable from the spec before it is applied.
- `env-apply` asks for confirmation, then writes `env.sh` and the rc source line.
- `env-reload` shows the value in a shell that sources `~/.bashrc`.
- `env-list-applied` reports the variable as applied.

## How to get to it (user POV)

- Run `genv env set EDITOR nvim` in a terminal.
- Run `genv env list`.
- Run `genv apply --skip-packages`, then answer `y`.
- Open a shell that reads `~/.bashrc` and print `EDITOR`.
- Run `genv env list --json`.

## Driving it with verify-genv

Preconditions:

- `verify-genv doctor` printed `doctor ok` for this run.
- No spec exists yet at the default path. `verify-genv paths spec` is not a file.
- The operator config fingerprint is unchanged.

- **Set the variable.** Run `verify-genv drive -- genv env set EDITOR nvim`. Exit code `0`. Stdout contains `created`, the spec path from `verify-genv paths spec`, `set EDITOR=nvim`, and `Run 'genv apply' to export it to your shell.`
- **List it.** Run `verify-genv drive -- genv env list`. Exit code `0`. Stdout contains `EDITOR` and `nvim`.
- **Apply.** Run `verify-genv drive --until 'Continue\? \[y/N\]' -- "genv apply --skip-packages"`. The pane shows a header that starts with `Apply plan` and names files, env, and services. It shows `This will apply 1 env variable(s). Continue? [y/N]`. It names the throwaway `env.sh` path from `verify-genv paths spec-dir`. It does not show an install command.
- **Confirm.** Run `verify-genv respond -- y`. Exit code `0`. The pane contains `env: set EDITOR` and `env fragment written`.
- **Read the fragment.** Run `verify-genv drive -- 'sed -n "1,40p" "$XDG_CONFIG_HOME/genv/env.sh"'`. Exit code `0`. The pane contains `export EDITOR='nvim'` and `# BEGIN genv env`.
- **Read the rc file.** Run `verify-genv drive -- 'sed -n "1,40p" "$HOME/.bashrc"'`. Exit code `0`. The pane contains `# genv env` and `env.sh`.
- **Reload.** Run `verify-genv drive -- '. "$HOME/.bashrc" && printf "EDITOR=%s\n" "$EDITOR"'`. Exit code `0`. The pane contains `EDITOR=nvim`.
- **List the applied value.** Run `verify-genv drive -- genv env list --json`. Exit code `0`. Stdout contains `"command":"env list"`, `"ok":true`, `"name":"EDITOR"`, `"kind":"ok"`, and `"specValue":"nvim"`.
- **Proof.** Write the feature id and copy the files.

```bash
printf '%s\n' env > "$(verify-genv paths evidence)/feature.txt"
verify-genv capture env.sh "$(verify-genv paths spec-dir)/env.sh"
verify-genv capture bashrc "$(verify-genv paths home)/.bashrc"
verify-genv capture genv.json "$(verify-genv paths spec)"
verify-genv capture genv.lock.json "$(verify-genv paths spec-dir)/genv.lock.json"
```

The captured `env.sh` contains `export EDITOR='nvim'`. The captured `bashrc` contains `# genv env`. The captured spec contains `"EDITOR"` under the active target, not only under `defaults`. On a host genv classifies, that target is the host class (`ubuntu` on Ubuntu). On a host the harness cannot classify, the harness sets `GENV_TARGET=linux` and the bucket is `targets.linux`.

## Gotchas

- `genv env set` writes `targets.<active>`, not `defaults`. `genv env list` shows the merged value, so a list line alone does not prove which bucket was written. Read the spec.
- `--sensitive` redacts the value in stdout and in `genv env list`. The fragment still contains the real value. This journey uses a normal value so both views show `nvim`.
- `genv apply` without `--skip-packages` also plans package installs and removals. Those run against the host package manager. This journey passes `--skip-packages` and checks that the transcript has no install command.
- `--dry-run` prints the plan and does not write `env.sh` or `~/.bashrc`. Do not use it as the proof of apply.
- Enter at the confirmation prompt declines. The command exits 0 and prints `Aborted.` The fragment is absent.
- `--json` without `--yes` does not apply. The error text includes `wet-run requires --yes`.
- A spec path passed with `--file`, or a `--state-dir` outside the default config directory, skips the rc injection. This journey uses the default path.
- The harness session is bash, so the rc file is `~/.bashrc`. A zsh user gets `~/.zshrc` from the same apply. Do not mark the zsh path verified from this session.
- On Ubuntu the session can print `man sudo_root` before the first prompt. That banner comes from `/etc/bash.bashrc`. It is not an install command and it is not genv calling sudo.
- On Windows the fragment is `env.ps1`. This journey does not cover that file.
