# Repo-Package Tab Completions Design

Date: 2026-08-01
Status: Approved

## Goal

Give `genv add` and `genv adopt` Homebrew-like tab completion for **repository package names** across available managers, so users can discover names like `openjdk` without searching first. Completions stay **ambiguous bare names**; the existing interactive `add` picker still chooses the manager and persists `prefer`.

## Problem

Today shell completions for genv only complete:

- Tracked package IDs (`genv __complete packages`) for `remove` / `disown` / `upgrade --only`
- Available manager names (`genv __complete managers`) for `--prefer`

`genv add` / `adopt` do **not** complete positional package ids. Users get brew/pacman Tab help for native CLIs, but not when installing through genv. Cross-manager search already exists (`adapter.Searchable` + `internal/search`) for the interactive picker; nothing feeds Tab.

## Decisions (locked)

| Topic | Choice |
|-------|--------|
| Latency model | Hybrid: fast local name dumps when available; live `Search` only as fallback |
| Tab result shape | Deduped **bare** names (ambiguous); picker sets manager/`prefer` |
| Commands | `add` and `adopt` get repo-name completions; `remove`/`disown`/`upgrade --only` stay on tracked IDs |
| Manager coverage (v1) | All current `Searchable` managers, plus easy high-value listers (e.g. mas, npm/bun) where practical |
| Prefix / delay | Match Homebrew: **no min-prefix gate**; dump full local lists and let the shell filter |
| Approach | Hidden `genv __complete` API + shell scripts (not shell-native reuse of each PM’s completers; not a background index daemon) |
| Output order | Alphabetically sorted unique bare names |

## Required behavior

### CLI contract

- New topic: `genv __complete repo-packages [prefix]`
- Optional positional `prefix` filters candidates with **case-insensitive prefix match** (aligned with shell `compgen`). Empty prefix returns the full dump-backed set (subject to rules below). The shell may still filter further.
- Output: one bare package name per line on stdout; unique; sorted alphabetically.
- Soft failures (missing manager, dump error, timeout): skip that manager; command still exits **0** (same spirit as `__complete packages` with no spec).
- No interactive prompts; no chatter on stdout.

Existing topics (`packages`, `managers`) unchanged.

### Shell wiring

Update `completions/genv.{zsh,bash,fish,ps1}`:

- `add` / `adopt`: complete positional package id via `genv __complete repo-packages <cur>`
- `remove` / `rm` / `disown` / `upgrade --only`: keep `packages`
- Scoped completion when `--prefer` is already on the line is a **nice-to-have**, deferred unless cheap

### Interaction with `genv add`

Unchanged after Tab:

1. User accepts a bare name (e.g. `openjdk`)
2. Interactive search picker (when TTY and no `--prefer` / `--manager` / `--no-search`) still runs and may set `prefer` (+ `managers` if concrete name ≠ id)
3. Resolver soft-locks via persisted `prefer` (then `managers` map / default fallbacks)

Tab must **not** invent `manager:name` ids or auto-set `--prefer`.

## Architecture

```text
shell (add/adopt Tab)
  → genv __complete repo-packages <cur>
      → Detect available managers
      → for each available + AutomaticOnGOOS manager:
          if NameLister → cached ListNames()          (fast path)
          else if Searchable and prefix non-empty
               → Search(prefix) with short timeout   (fallback)
          else skip
      → dedupe bare names → sort → print
  → shell displays / filters
  → user runs add|adopt as today
```

### New adapter extension

```go
// NameLister is an optional extension for managers that can dump installable
// package names quickly enough for shell completion (Homebrew-style).
type NameLister interface {
	ListNames() ([]string, error)
}
```

Separate from `Searchable` so dumps stay fast and search stays keyword-oriented.

### Completer helper

New small package or search-adjacent helper (e.g. `internal/complete` or functions beside `internal/search`) that:

- Builds the candidate set from `NameLister` + `Searchable` fallback
- Applies GOOS automatic-manager policy (same as `search.All`)
- Caps concurrency (reuse ~4 workers)
- Enforces timeouts and overall soft wall clock
- Reads/writes dump cache
- Returns sorted unique strings

Wire from `completeInternalCmd` in `main.go`.

## Manager coverage (v1)

| Manager | Path | Source (intent) |
|---------|------|-----------------|
| brew / linuxbrew | `NameLister` | `HOMEBREW_COMPLETION=1 brew formulae` and `casks` |
| pacman | `NameLister` | `pacman -Slq` |
| paru / yay | `NameLister` if dump is fast; else `Search` | sync DB / `-Slq`-style list |
| snap | `Search` fallback unless a fast local list exists | existing `Search` |
| winget / scoop / choco | Prefer dump when cheap; else `Search` | existing adapters + small dumps where easy |
| mas | Add `NameLister` and/or `Searchable` | practical mas list/search APIs |
| npm / bun | Live `Search` only when prefix **non-empty** | bounded registry search; never full-dump on empty prefix |

Only managers that are **available** and **automatic on this GOOS** participate.

**Empty prefix:** return dump-backed (`NameLister`) names only. Do **not** fire unbounded live searches across huge registries (npm, etc.).

## Caching, latency, and errors

### Cache

- Location: `~/.config/genv/cache/completions/<manager>.txt` plus small sidecar metadata as needed
- Contents: `NameLister` dumps only (never live `Search` results)
- Freshness: valid if age < **14 days** (Homebrew zsh cache policy). Optional cheap invalidation (e.g. brew tap index mtime) when practical; TTL-only is acceptable for v1
- Corrupt/unreadable cache → treat as miss; re-dump

### Latency

- Soft overall wall clock ~**300ms** per Tab invocation: return whatever finished; drop stragglers
- Per-manager timeout ~**150ms** for live `Search` fallback
- Concurrency capped (~4 workers)
- Stdout = names only; diagnostics suppressed or stderr-only

### Errors

- Unavailable / dump failure / timeout → skip manager
- `__complete repo-packages` exits 0 on soft failures

## Testing

- Unit: dump parsing per NameLister; merge/dedupe/sort; timeout skips slow manager; cache hit/miss/TTL; empty prefix does not invoke huge-registry Search
- CLI: `completeInternalCmd` / `__complete repo-packages` with fake adapters
- Completion scripts: ensure `add`/`adopt` invoke `repo-packages` (follow existing completion test patterns if present)

## Documentation

- README / CHANGELOG note that `add`/`adopt` Tab-complete repo names
- No user-facing new flags required

## Non-goals (v1)

- Manager-qualified completion insertion (`brew:openjdk`) or parsing `manager:name` as id
- Background index daemon / scheduled full-repo sync
- Reusing foreign shell completion functions (`_brew`, etc.) from genv scripts
- Changing `prefer` soft-lock semantics or add-before-install ordering
- Completing ecosystem managers that cannot dump or search cheaply

## Implementation sketch (for planning)

1. Add `NameLister` + brew/pacman (and other easy) implementations; tests
2. Completer merge/cache/timeout helper + `__complete repo-packages`
3. Extend high-value Searchable/NameLister (mas, npm/bun) where practical
4. Update all four completion scripts
5. Docs + CHANGELOG
