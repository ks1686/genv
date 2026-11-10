# Set a shell alias

`genv shell alias set` records an alias in the spec. `genv apply` writes it to `shell.sh` and sources that file from the shell rc.

## Sub-features

- `alias-set` records `ll` and creates the spec when it is missing.
- `alias-apply` asks for confirmation, then writes `shell.sh` and the rc source line.

## How to get to it (user POV)

- Run `genv shell alias set ll 'ls -lah'`.
- Run `genv apply --skip-packages` and answer `y`.
- Read `shell.sh`.

## Driving it with verify-genv

Preconditions:

- `verify-genv doctor` printed `doctor ok` for this run.
- No spec exists yet at `verify-genv paths spec`.

- **Set the alias.** Run `verify-genv drive -- "genv shell alias set ll 'ls -lah'"`. Exit code `0`. The pane contains `created`, `set alias ll="ls -lah"`, and `Run 'genv apply' to apply it to your shell.`
- **Apply.** Run `verify-genv drive --until 'Continue\? \[y/N\]' -- "genv apply --skip-packages"`. The pane contains `This will apply 1 shell config entry/entries. Continue? [y/N]`. The header starts with `Apply plan`. The pane does not show an install command.
- **Confirm.** Run `verify-genv respond -- y`. Exit code `0`. The pane contains `shell: set ll` and `shell fragment written to`.
- **Read the fragment.** Run `verify-genv drive -- 'sed -n "1,40p" "$XDG_CONFIG_HOME/genv/shell.sh"'`. Exit code `0`. The pane contains `alias ll='ls -lah'`.
- **Read the rc file.** Run `verify-genv drive -- 'sed -n "1,40p" "$HOME/.bashrc"'`. Exit code `0`. The pane contains `# genv env` and `shell.sh`.
- **Proof.**

```bash
printf '%s\n' shell-alias > "$(verify-genv paths evidence)/feature.txt"
verify-genv capture shell.sh "$(verify-genv paths spec-dir)/shell.sh"
verify-genv capture bashrc "$(verify-genv paths home)/.bashrc"
verify-genv capture genv.json "$(verify-genv paths spec)"
```

The captured `shell.sh` contains `alias ll='ls -lah'`. The alias is under the active target in the spec.

## Gotchas

- The value `ls -lah` must be one argument. If the quotes are dropped, genv sees the value `ls` and a leftover `-lah`.
- The success line uses double quotes (`ll="ls -lah"`). The fragment uses single quotes (`alias ll='ls -lah'`). Check the fragment, not only the success line.
- `--shell bash` wraps the alias in a bash guard. This journey omits `--shell`, so the fragment has a plain `alias` line.
- Fish aliases are not written as fish syntax. The apply output says to source them from `config.fish` by hand. This journey does not pass `--shell fish`.
- The same `--skip-packages`, confirmation, rc-path, and Windows limits as the env journey apply here. The fragment name is `shell.sh`, or `shell.ps1` on Windows.
