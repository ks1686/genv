# Repo-Package Tab Completions Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Give `genv add` / `genv adopt` Homebrew-like Tab completion of repository package names via `genv __complete repo-packages`, using cached `NameLister` dumps plus live `Search` fallback, while leaving manager selection to the existing interactive picker.

**Architecture:** Add optional `adapter.NameLister` (and `CompletionNamer` only for mas). New `internal/complete` merges available automatic managers with dump cache + timeouts, prints sorted unique bare names. Wire `__complete repo-packages` and update all four shell completion scripts.

**Tech Stack:** Go 1.24.3, existing `internal/adapter` + `internal/search` patterns, `go test`, fake binaries via `installFakeBinary`, shell scripts under `completions/`.

## Global Constraints

- Spec: `docs/superpowers/specs/2026-08-01-repo-package-tab-completions-design.md`
- Tab results are **ambiguous bare names only**; do not insert `manager:name` or auto-set `--prefer`
- No min-prefix gate; empty prefix = dump-backed names only (no live Search on huge registries)
- Soft overall wall clock ~300ms; per-manager Search timeout ~150ms; concurrency ≤4
- `__complete repo-packages` exits 0 on soft failures; stdout = names only
- Same automatic-manager policy as `search.All` (`available` ∩ `AutomaticOnGOOS`)
- Cache under `~/.config/genv/cache/completions/` (via `genvfile.DefaultDir`), TTL 14 days, dumps only
- Output: unique bare names, sorted alphabetically
- Do not change add-before-install ordering or prefer soft-lock semantics

---

## File map

| File | Responsibility |
|------|----------------|
| `internal/adapter/adapter.go` | `NameLister`, `CompletionNamer` interfaces |
| `internal/adapter/brew.go` | `ListNames` for brew/linuxbrew (`formulae` + `casks`) |
| `internal/adapter/pacman.go` | `ListNames` via `pacman -Slq` |
| `internal/adapter/paru.go`, `yay.go` | Prefer `ListNames` if same dump works; else keep Search-only |
| `internal/adapter/mas.go` | `Search` + `CompletionNames` (IDs vs name labels) |
| `internal/adapter/npm.go`, `bun.go` | `Search` via registry search CLIs |
| `internal/adapter/*_test.go` | Adapter unit tests with fakes |
| `internal/complete/cache.go` | Dump cache read/write/TTL |
| `internal/complete/repo.go` | Merge NameLister + Search + CompletionNamer |
| `internal/complete/*_test.go` | Merge/cache/timeout tests with fake adapters |
| `main.go` | `completeInternalCmd` topic `repo-packages` |
| `main_helpers_coverage_test.go` / `main_test.go` | CLI complete tests |
| `completions/genv.{zsh,bash,fish,ps1}` | `add`/`adopt` positional → `repo-packages` |
| `README.md`, `CHANGELOG.md` | User-facing note |

---

### Task 1: `NameLister` + Homebrew `ListNames`

**Files:**
- Modify: `internal/adapter/adapter.go` (add interfaces after `Searchable`)
- Modify: `internal/adapter/brew.go`
- Create: `internal/adapter/namelist_test.go` (or extend `brew_test.go`)
- Test: `internal/adapter/namelist_test.go`

**Interfaces:**
- Consumes: `runListOutput`, brew env patterns
- Produces:
  ```go
  type NameLister interface {
      ListNames() ([]string, error)
  }
  // CompletionNamer is optional; used when Tab labels differ from install PkgName (mas).
  type CompletionNamer interface {
      CompletionNames(prefix string) ([]string, error)
  }
  ```
  `brewBase.ListNames() ([]string, error)` — formulae ∪ casks, deduped

- [ ] **Step 1: Write failing test for brew ListNames**

```go
func TestBrew_ListNames_formulaeAndCasks(t *testing.T) {
	installFakeBinary(t, "brew", `#!/bin/sh
case "$1" in
  formulae) echo "openjdk"; echo "wget" ;;
  casks) echo "docker"; echo "openjdk" ;;
  *) exit 1 ;;
esac
`)
	names, err := Brew{}.ListNames()
	if err != nil {
		t.Fatalf("ListNames: %v", err)
	}
	// Expect both sources; duplicates OK at adapter layer (completer dedupes) OR dedupe here — prefer dedupe in ListNames.
	want := []string{"docker", "openjdk", "wget"}
	if !sameStringSet(names, want) {
		t.Fatalf("ListNames = %v, want %v", names, want)
	}
}
```

Add a small `sameStringSet` helper in the test file.

- [ ] **Step 2: Run test — expect fail**

Run: `go test ./internal/adapter -run TestBrew_ListNames -count=1`
Expected: FAIL (method undefined)

- [ ] **Step 3: Add interfaces + brew ListNames**

In `adapter.go`, after `Searchable`:

```go
// NameLister is an optional extension for managers that can dump installable
// package names quickly enough for shell completion (Homebrew-style).
type NameLister interface {
	ListNames() ([]string, error)
}

// CompletionNamer is an optional extension for managers whose Tab labels should
// differ from Search/install package names (e.g. mas app names vs product IDs).
type CompletionNamer interface {
	CompletionNames(prefix string) ([]string, error)
}
```

In `brew.go` on `brewBase`:

```go
func (brewBase) ListNames() ([]string, error) {
	cmd := exec.Command("brew", "formulae")
	cmd.Env = append(os.Environ(), "HOMEBREW_COMPLETION=1")
	out, err := cmd.Output()
	// On failure, return nil, err (completer will skip)
	// Also run brew casks with same env; merge unique preserving order
}
```

Prefer a private helper `brewCompletionList(arg string) ([]string, error)` used for `formulae` and `casks`. Linuxbrew shares `brewBase` — same method applies.

- [ ] **Step 4: Run test — expect pass**

Run: `go test ./internal/adapter -run TestBrew_ListNames -count=1`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/adapter/adapter.go internal/adapter/brew.go internal/adapter/namelist_test.go
git commit -m "$(cat <<'EOF'
Add NameLister and Homebrew ListNames for completions.

EOF
)"
```

---

### Task 2: Pacman-family `ListNames`

**Files:**
- Modify: `internal/adapter/pacman.go`
- Modify: `internal/adapter/paru.go`, `internal/adapter/yay.go` (if they can share `pacman -Slq` or their own `-Slq`)
- Modify: `internal/adapter/namelist_test.go`
- Test: `internal/adapter/namelist_test.go`

**Interfaces:**
- Consumes: `NameLister`, `runListOutput`
- Produces: `Pacman.ListNames`, and Paru/Yay `ListNames` when dump is equivalent

- [ ] **Step 1: Write failing tests**

```go
func TestPacman_ListNames(t *testing.T) {
	installFakeBinary(t, "pacman", `#!/bin/sh
[ "$1" = "-Slq" ] || exit 1
echo "ripgrep"
echo "git"
`)
	names, err := Pacman{}.ListNames()
	if err != nil {
		t.Fatalf("ListNames: %v", err)
	}
	if !sameStringSet(names, []string{"git", "ripgrep"}) {
		t.Fatalf("got %v", names)
	}
}
```

For paru/yay: if `paru -Slq` / `yay -Slq` works the same, implement; otherwise leave Search-only and note in test that they are not NameLister.

- [ ] **Step 2: Run — expect fail**

Run: `go test ./internal/adapter -run 'TestPacman_ListNames|TestParu_ListNames|TestYay_ListNames' -count=1`

- [ ] **Step 3: Implement**

```go
func (Pacman) ListNames() ([]string, error) {
	return runListOutput("pacman", "-Slq")
}
```

Mirror for paru/yay if applicable (`paru -Slq`, `yay -Slq`).

- [ ] **Step 4: Run — expect pass**

- [ ] **Step 5: Commit**

```bash
git add internal/adapter/pacman.go internal/adapter/paru.go internal/adapter/yay.go internal/adapter/namelist_test.go
git commit -m "$(cat <<'EOF'
Add pacman-family ListNames dumps for tab completions.

EOF
)"
```

---

### Task 3: Dump cache helper

**Files:**
- Create: `internal/complete/cache.go`
- Create: `internal/complete/cache_test.go`
- Test: `internal/complete/cache_test.go`

**Interfaces:**
- Consumes: `genvfile.DefaultDir`, `os`, `time`
- Produces:
  ```go
  const CacheTTL = 14 * 24 * time.Hour

  func CacheDir() (string, error) // DefaultDir()/cache/completions
  func ReadDump(manager string) ([]string, bool) // names, hit
  func WriteDump(manager string, names []string) error
  ```

- [ ] **Step 1: Write failing cache tests**

```go
func TestReadWriteDump_roundTrip(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	names := []string{"git", "wget"}
	if err := WriteDump("brew", names); err != nil {
		t.Fatal(err)
	}
	got, hit := ReadDump("brew")
	if !hit {
		t.Fatal("expected hit")
	}
	if !slices.Equal(got, names) {
		t.Fatalf("got %v want %v", got, names)
	}
}

func TestReadDump_expired(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	_ = WriteDump("brew", []string{"git"})
	// backdate mtime beyond CacheTTL
	path := dumpPath("brew") // export for test or duplicate path join in test
	past := time.Now().Add(-CacheTTL - time.Hour)
	_ = os.Chtimes(path, past, past)
	_, hit := ReadDump("brew")
	if hit {
		t.Fatal("expected miss after TTL")
	}
}

func TestReadDump_corrupt(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	dir, _ := CacheDir()
	_ = os.MkdirAll(dir, 0o700)
	_ = os.WriteFile(filepath.Join(dir, "brew.txt"), []byte{0xff, 0xfe}, 0o600)
	// If we only store plain text lines, "corrupt" = unreadable file → miss
	_, hit := ReadDump("brew")
	// readable bytes still parse as lines; instead test missing file:
	_, hit = ReadDump("missing-manager")
	if hit {
		t.Fatal("expected miss")
	}
}
```

Use one name per line in `*.txt`. Missing file → miss. Age via `os.Stat` `ModTime`.

- [ ] **Step 2: Run — expect fail**

Run: `go test ./internal/complete -count=1`

- [ ] **Step 3: Implement cache.go**

```go
package complete

func CacheDir() (string, error) {
	base, err := genvfile.DefaultDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, "cache", "completions"), nil
}

func ReadDump(manager string) ([]string, bool) { /* stat TTL; read lines */ }
func WriteDump(manager string, names []string) error { /* mkdir; write */ }
```

Sanitize `manager` to a single path segment (reject `/` and `..`).

- [ ] **Step 4: Run — expect pass**

- [ ] **Step 5: Commit**

```bash
git add internal/complete/cache.go internal/complete/cache_test.go
git commit -m "$(cat <<'EOF'
Add completion dump cache with 14-day TTL.

EOF
)"
```

---

### Task 4: `RepoPackages` merge helper

**Files:**
- Create: `internal/complete/repo.go`
- Create: `internal/complete/repo_test.go`
- Test: `internal/complete/repo_test.go`

**Interfaces:**
- Consumes: `adapter.All`, `NameLister`, `Searchable`, `CompletionNamer`, `AutomaticOnGOOS`, cache helpers
- Produces:
  ```go
  const (
      OverallTimeout  = 300 * time.Millisecond
      SearchTimeout   = 150 * time.Millisecond
      MaxWorkers      = 4
  )

  // RepoPackages returns sorted unique bare names for Tab completion.
  func RepoPackages(prefix string, available map[string]bool) []string
  func repoPackagesOnGOOS(prefix string, available map[string]bool, goos string, now time.Time) []string
  ```

Behavior:
1. For each adapter in `adapter.All` where `available[name] && AutomaticOnGOOS(name, goos)`:
   - If `CompletionNamer` and `prefix != ""`: call `CompletionNames(prefix)` under SearchTimeout (mas path)
   - Else if `NameLister`: `ReadDump` or `ListNames` + `WriteDump`; filter by case-insensitive prefix if prefix non-empty
   - Else if `Searchable` and `prefix != ""`: `Search(prefix)` under SearchTimeout
   - Else skip
2. Soft overall deadline: drop unfinished work after OverallTimeout
3. Dedupe (case-sensitive unique strings as returned), sort with `sort.Strings`
4. Errors → skip manager

- [ ] **Step 1: Write failing tests with fake adapters**

Do **not** mutate global `adapter.All` if avoidable — prefer injecting via an internal function:

```go
func collectRepoNames(prefix string, jobs []repoJob, overall, search time.Duration) []string
```

```go
type fakeLister struct{ name string; names []string; delay time.Duration }
// implement Name() via job struct, not full Adapter

func TestCollectRepoNames_prefersListerAndSorts(t *testing.T) {
	got := collectRepoNames("o", []repoJob{
		{manager: "brew", list: func() ([]string, error) { return []string{"wget", "openjdk"}, nil }},
		{manager: "snap", search: func(q string) ([]string, error) { return []string{"opera"}, nil }},
	}, time.Second, time.Second)
	want := []string{"openjdk", "opera"} // prefix "o"
	if !slices.Equal(got, want) {
		t.Fatalf("got %v want %v", got, want)
	}
}

func TestCollectRepoNames_emptyPrefixSkipsSearch(t *testing.T) {
	called := false
	got := collectRepoNames("", []repoJob{
		{manager: "brew", list: func() ([]string, error) { return []string{"git"}, nil }},
		{manager: "npm", search: func(q string) ([]string, error) { called = true; return []string{"git-foo"}, nil }},
	}, time.Second, time.Second)
	if called {
		t.Fatal("Search must not run on empty prefix")
	}
	if !slices.Equal(got, []string{"git"}) {
		t.Fatalf("got %v", got)
	}
}

func TestCollectRepoNames_searchTimeoutSkipped(t *testing.T) {
	got := collectRepoNames("g", []repoJob{
		{manager: "slow", search: func(q string) ([]string, error) {
			time.Sleep(200 * time.Millisecond)
			return []string{"git"}, nil
		}},
		{manager: "fast", list: func() ([]string, error) { return []string{"gdb"}, nil }},
	}, 300*time.Millisecond, 50*time.Millisecond)
	if !slices.Equal(got, []string{"gdb"}) {
		t.Fatalf("got %v", got)
	}
}
```

- [ ] **Step 2: Run — expect fail**

Run: `go test ./internal/complete -run TestCollectRepoNames -count=1`

- [ ] **Step 3: Implement `repo.go`**

Wire `RepoPackages` to build `repoJob`s from `adapter.All` + cache. Use `context.WithTimeout` per search job and an overall context.

For NameLister cache integration:

```go
if names, hit := ReadDump(manager); hit {
    // use names
} else {
    names, err = lister.ListNames()
    if err == nil {
        _ = WriteDump(manager, names)
    }
}
```

Prefix filter:

```go
func filterPrefix(names []string, prefix string) []string {
	if prefix == "" {
		return names
	}
	var out []string
	for _, n := range names {
		if strings.HasPrefix(strings.ToLower(n), strings.ToLower(prefix)) {
			out = append(out, n)
		}
	}
	return out
}
```

- [ ] **Step 4: Run — expect pass**

Run: `go test ./internal/complete -count=1`

- [ ] **Step 5: Commit**

```bash
git add internal/complete/repo.go internal/complete/repo_test.go
git commit -m "$(cat <<'EOF'
Add RepoPackages merge helper with timeouts and prefix rules.

EOF
)"
```

---

### Task 5: Wire `genv __complete repo-packages`

**Files:**
- Modify: `main.go` (`completeInternalCmd` ~3338)
- Modify: `main_helpers_coverage_test.go` (extend `TestXDGHelpersAndCompleteInternal`)
- Test: `main_helpers_coverage_test.go`

**Interfaces:**
- Consumes: `complete.RepoPackages`, `resolver.Detect`
- Produces: topic `repo-packages` printing one name per line

- [ ] **Step 1: Write failing CLI test**

```go
func TestCompleteRepoPackages_topicExists(t *testing.T) {
	// Even with no managers, topic must be recognized and exit 0
	code := completeInternalCmd([]string{"repo-packages", "zzz-nonexistent-prefix-genv"})
	if code != exitOK {
		t.Fatalf("code = %d, want %d", code, exitOK)
	}
}
```

Optionally stub PATH with a fake brew that lists names and assert stdout contains them (heavier; keep at least topic smoke + unit coverage in `internal/complete`).

- [ ] **Step 2: Run — expect fail** (unknown topic → `exitUsage`)

Run: `go test . -run TestCompleteRepoPackages -count=1`

- [ ] **Step 3: Implement**

In `completeInternalCmd` switch:

```go
case "repo-packages":
	prefix := ""
	if len(args) > 1 {
		prefix = args[1]
	}
	available := resolver.Detect()
	for _, name := range complete.RepoPackages(prefix, available) {
		fPrintln(os.Stdout, name)
	}
```

Update the doc comment on `completeInternalCmd` to list the new topic.

- [ ] **Step 4: Run — expect pass**

Run: `go test . -run 'TestCompleteRepoPackages|TestXDGHelpersAndCompleteInternal' -count=1`

- [ ] **Step 5: Commit**

```bash
git add main.go main_helpers_coverage_test.go
git commit -m "$(cat <<'EOF'
Wire genv __complete repo-packages for shell Tab completion.

EOF
)"
```

---

### Task 6: High-value Search — npm, bun, mas

**Files:**
- Modify: `internal/adapter/npm.go`, `bun.go`, `mas.go`
- Create/modify: `internal/adapter/npm_search_test.go`, `bun_search_test.go`, `mas_search_test.go`
- Test: those files

**Interfaces:**
- Consumes: `Searchable`, `CompletionNamer`
- Produces:
  - `Npm.Search` / `Bun.Search` — package names from registry search
  - `Mas.Search` — product IDs for picker/`search.All`
  - `Mas.CompletionNames` — lowercase name tokens for Tab

- [ ] **Step 1: Write failing tests**

```go
func TestNpm_Search_parseable(t *testing.T) {
	installFakeBinary(t, "npm", `#!/bin/sh
# npm search --parseable QUERY
echo "lodash	desc	date	ver	keywords"
echo "@types/lodash	desc	date	ver	"
`)
	got, err := Npm{}.Search("lodash")
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(got, "lodash") {
		t.Fatalf("got %v", got)
	}
}

func TestMas_Search_returnsIDs(t *testing.T) {
	installFakeBinary(t, "mas", `#!/bin/sh
echo "497799835  Xcode (16.0)"
echo "123  Other App (1.0)"
`)
	got, err := Mas{}.Search("xcode")
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(got, []string{"497799835"}) {
		t.Fatalf("got %v", got)
	}
}

func TestMas_CompletionNames_returnsSlugs(t *testing.T) {
	installFakeBinary(t, "mas", `#!/bin/sh
echo "497799835  Xcode (16.0)"
`)
	got, err := Mas{}.CompletionNames("xco")
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(got, []string{"xcode"}) {
		t.Fatalf("got %v", got)
	}
}
```

Bun: there is no stable bun-native registry search comparable to `npm search --parseable`. Implement `Bun.Search` by shelling to `npm search --parseable` when `npm` is on PATH; if `npm` is missing, return `nil, nil` (completer skips). Minimum bar for this task: Npm.Search + Mas Search/CompletionNames + Bun.Search as above.

- [ ] **Step 2: Run — expect fail**

- [ ] **Step 3: Implement**

Npm:

```go
func (Npm) Search(query string) ([]string, error) {
	lines, err := runListOutput("npm", "search", "--parseable", query)
	// first column before tab
}
```

Mas Search: filter lines where name contains query (case-insensitive); return ID field.
Mas CompletionNames: same filter; return `strings.ToLower(strings.ReplaceAll(name, " ", "-"))` (strip version parentheses).

Ensure `internal/complete` jobs prefer `CompletionNamer` over raw `Search` for that manager when both exist (already specified in Task 4).

- [ ] **Step 4: Run adapter + complete tests**

Run: `go test ./internal/adapter ./internal/complete -count=1`

- [ ] **Step 5: Commit**

```bash
git add internal/adapter/npm.go internal/adapter/bun.go internal/adapter/mas.go internal/adapter/*search*test.go
git commit -m "$(cat <<'EOF'
Add npm/bun/mas search hooks for completion and add picker.

EOF
)"
```

---

### Task 7: Shell completion scripts

**Files:**
- Modify: `completions/genv.zsh`
- Modify: `completions/genv.bash`
- Modify: `completions/genv.fish`
- Modify: `completions/genv.ps1`

**Interfaces:**
- Consumes: `genv __complete repo-packages`
- Produces: positional completion on `add` / `adopt`

- [ ] **Step 1: Update zsh `add` / `adopt`**

For `add`, extend `_arguments` with a positional package id state (mirror `remove`):

```zsh
add)
  _arguments \
    '--file=[Path to genv.json]:path:_files' \
    # ... existing flags ...
    '1: :->pkgid'
  if [[ $state == pkgid ]]; then
    local -a pkgs
    pkgs=(${(f)"$(genv __complete repo-packages ${words[CURRENT]} 2>/dev/null)"})
    _describe -t packages 'package' pkgs
  fi
  ;;
```

Same for `adopt`. Keep `--prefer` manager completion.

- [ ] **Step 2: Update bash**

```bash
add)
  if [[ "${prev}" == "--prefer" ]]; then
    mapfile -t COMPREPLY < <(compgen -W "$(genv __complete managers 2>/dev/null)" -- "${cur}")
    return 0
  fi
  if [[ "${cur}" != -* ]]; then
    mapfile -t COMPREPLY < <(compgen -W "$(genv __complete repo-packages "${cur}" 2>/dev/null)" -- "${cur}")
    return 0
  fi
  opts="--file --lock-file --version --prefer --manager --no-search --no-hooks --hook-timeout --host --target"
  ;;
```

Mirror for `adopt`.

- [ ] **Step 3: Update fish**

```fish
function __fish_genv_repo_packages
    set -l cur (commandline -ct)
    genv __complete repo-packages $cur 2>/dev/null
end

complete -c genv -n '__fish_genv_using_command add' -n 'not __fish_seen_argument -l prefer' \
    -a '(__fish_genv_repo_packages)' -d 'Package'
# simpler: always offer repo packages as positional completions
complete -c genv -n '__fish_genv_using_command add' -a '(__fish_genv_repo_packages)'
complete -c genv -n '__fish_genv_using_command adopt' -a '(__fish_genv_repo_packages)'
```

Keep existing flag completions.

- [ ] **Step 4: Update PowerShell**

```powershell
$completeRepoPackages = {
  param($prefix)
  try {
    & genv __complete repo-packages $prefix 2>$null
  } catch { @() }
}

{ $_ -in 'add' } {
  if ($WordToComplete -like '-*') {
    $flags = @(...)
    return (& $completeCandidates -Candidates $flags)
  }
  if ($previous is --prefer) { managers... }
  return (& $completeCandidates -Candidates @(& $completeRepoPackages $WordToComplete) -ResultType 'ParameterValue')
}
```

Mirror for `adopt`.

- [ ] **Step 5: Smoke-check scripts mention repo-packages**

Run: `rg -n 'repo-packages' completions/`
Expected: hits in all four files for add/adopt paths

- [ ] **Step 6: Commit**

```bash
git add completions/
git commit -m "$(cat <<'EOF'
Complete add/adopt package ids from repo-packages.

EOF
)"
```

---

### Task 8: Docs + CHANGELOG

**Files:**
- Modify: `README.md` (completion / add section)
- Modify: `CHANGELOG.md` (Unreleased)
- Test: n/a (docs)

**Interfaces:**
- Consumes: shipped behavior from Tasks 1–7
- Produces: user-facing documentation

- [ ] **Step 1: CHANGELOG entry under Unreleased**

```markdown
### Added
- Shell completions for `genv add` / `genv adopt` suggest repository package names via
  `genv __complete repo-packages` (cached manager dumps + live search fallback).
```

- [ ] **Step 2: README note**

Near completion install docs, add one short paragraph:

> Tab completion on `add` / `adopt` suggests package names from available managers (Homebrew-style local dumps when possible). After you accept a name, interactive `genv add` still asks which manager to use when multiple match.

- [ ] **Step 3: Commit**

```bash
git add README.md CHANGELOG.md
git commit -m "$(cat <<'EOF'
Document repo-package tab completions for add and adopt.

EOF
)"
```

- [ ] **Step 4: Final verification**

Run: `make ci`  
Expected: pass (or fix coverage if new packages dip below gate)

Manual (optional on macOS): `genv __complete repo-packages open | head` should list `openjdk` etc. when brew is available.

---

## Spec coverage checklist

| Spec requirement | Task |
|------------------|------|
| `NameLister` + brew dumps | 1 |
| pacman-family dumps | 2 |
| Cache TTL 14d under config dir | 3 |
| Merge + timeouts + empty-prefix rule | 4 |
| `__complete repo-packages` | 5 |
| Searchable expansion (npm/bun/mas) | 6 |
| Shell scripts add/adopt | 7 |
| README/CHANGELOG | 8 |
| Bare names; picker unchanged | 4–7 (no prefer injection) |
| Alphabetical unique | 4 |
| snap/winget/scoop/choco via existing Search | 4 (automatic once Searchable + non-empty prefix) |

## Deferred (explicit non-goals)

- `--prefer`-scoped completion
- Brew tap-index cache invalidation beyond TTL
- Background index daemon
- Manager-qualified insertion
`)
