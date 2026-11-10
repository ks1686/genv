# Initialize a spec

`genv init` asks for package ids and writes a new schemaVersion 8 spec. It does not install those packages. A second run refuses to overwrite the file.

## Sub-features

- `init-add` records one package id from the prompt.
- `init-empty` writes a spec when the user finishes without an id.
- `init-refuse` leaves an existing spec unchanged.

## How to get to it (user POV)

- Run `genv init`.
- Type a package id, then press Enter.
- Press Enter on a blank line to finish.
- Run `genv init` again and read the refusal.

## Driving it with verify-genv

Preconditions:

- `verify-genv doctor` printed `doctor ok` for this run.
- No spec exists yet at `verify-genv paths spec`.

- **Open the wizard.** Run `verify-genv drive --until 'package id \(or Enter to finish\)' -- genv init`. The pane contains `Creating`, the spec path, and `Enter package IDs to track, one per line.`
- **Add an id.** Run `verify-genv respond --until 'package id \(or Enter to finish\)' -- git`. The pane contains `added git`.
- **Finish.** Run `verify-genv respond --`. Exit code `0`. The pane contains `created` and `1 package`.
- **Read the spec.** Run `verify-genv drive -- 'sed -n "1,80p" "$XDG_CONFIG_HOME/genv/genv.json"'`. Exit code `0`. The pane contains `"schemaVersion": "8"` and `"id": "git"`.
- **Refuse a second init.** Run `verify-genv drive -- genv init`. Exit code `4`. The pane contains `already exists`.
- **Proof.** The spec file is unchanged by the second init. Copy it.

```bash
printf '%s\n' init > "$(verify-genv paths evidence)/feature.txt"
verify-genv capture genv.json "$(verify-genv paths spec)"
```

The captured spec contains one `"id": "git"` and no lock file is required. `genv init` does not write `genv.lock.json`.

For `init-empty`, use a new launch and run `verify-genv drive --until 'package id \(or Enter to finish\)' -- genv init`, then `verify-genv respond --`. Exit code `0`. The pane contains `No packages entered` and `0 package`.

## Gotchas

- `genv init` only writes the spec. The packages are not installed. `genv apply` would install them, and that reaches the host package manager.
- The package id is stored on the active target. It is not stored in `defaults`.
- A blank first answer still creates the spec. The next `genv init` then exits 4 because the file exists.
- Typing the same id twice in one wizard skips the duplicate and continues. The spec contains that id once.
- The wizard reads stdin. Drive it in this tmux session. A closed stdin finishes the list immediately and can write an empty spec.
