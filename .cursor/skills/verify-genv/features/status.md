# Check status

`genv status` compares the spec with the lock. A variable that is set and not applied is listed as missing. After apply, the same variable is listed as ok.

## Sub-features

- `status-missing` shows a variable that is in the spec and not in the lock.
- `status-ok` shows a variable whose spec value matches the lock.

## How to get to it (user POV)

- Run `genv env set EDITOR nvim`.
- Run `genv status`.
- Run `genv apply --skip-packages` and answer `y`.
- Run `genv status` again.

## Driving it with verify-genv

Preconditions:

- `verify-genv doctor` printed `doctor ok` for this run.
- No spec exists yet at `verify-genv paths spec`.

- **Set a variable.** Run `verify-genv drive -- genv env set EDITOR nvim`. Exit code `0`.
- **Status before apply.** Run `verify-genv drive -- genv status`. Exit code `0`. The pane has a row that starts with `missing` and names `EDITOR`. The row tells the user to run `genv apply`.
- **Apply.** Run `verify-genv drive --until 'Continue\? \[y/N\]' -- "genv apply --skip-packages"`, then `verify-genv respond -- y`. The respond exit code is `0`.
- **Status after apply.** Run `verify-genv drive -- genv status`. Exit code `0`. The pane has a row that starts with `ok` and names `EDITOR` and `nvim`. It does not have a `missing` row for `EDITOR`.
- **Proof.** Copy the status panes from `<evidence>/drives/` and write the feature id.

```bash
printf '%s\n' status > "$(verify-genv paths evidence)/feature.txt"
```

## Gotchas

- Exit code `0` with a `missing` row means the command ran. It does not mean the variable is applied. `modified` and `extra` exit `4`. A `missing` variable exits `0`.
- Default `genv status` may query package managers when the spec tracks packages. This journey tracks none, and the package count in the header is `0`. `--offline` compares the spec and the lock without that query. Use the default command here so the journey matches what a user types.
- `--verify` asks each tracked package's manager whether it is installed. It is a different entry point. Do not substitute it for a plain `genv status` and call the plain command verified.
- Status reads the merged spec. The `EDITOR` row does not by itself show that the value lives on the active target.
