# Outdated-Aware Upgrade Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make `genv upgrade` default to planning only actually-outdated packages (with `--all` for the old brute-force path), batch mas upgrades, and add `OutdatedLister` coverage for the common managers listed in the design spec.

**Architecture:** Reuse `upgrade.BuildUpgradePlan` + `resolver.FilterOutdated`. Flip `DetectOutdated` on in `upgradeCmd` unless `--all`. Extend adapters with `ListOutdated` using native outdated commands or installed-vs-registry compares (shared helpers for npm-family and PyPI/crates). Mas gains `BatchUpgrader`.

**Tech Stack:** Go (existing genv modules), `go test`, fake binaries via `installFakeBinary`, httptest seams for registries.

## Global Constraints

- Managers without `OutdatedLister` stay on conservative keep-all (never silently skip).
- `ListOutdated` failure → keep that manager's packages + warning (existing `FilterOutdated` contract).
- Do not run `brew update` (or equivalent refresh) before outdated queries.
- Successful upgrade output stays visible; only the plan is filtered.
- Deferred adapters (asdf/mise/deno/go modules/etc.) get no lister in this plan.
- Spec: `docs/superpowers/specs/2026-07-21-outdated-aware-upgrade-design.md`

---

## File map

| File | Responsibility |
|------|----------------|
| `main.go` | `--all` flag; pass `DetectOutdated: !*all` |
| `internal/upgrade/planner.go` | Update `DetectOutdated` comment |
| `internal/output/json.go` | Optional `All bool` on `UpgradeFilters` for JSON |
| `internal/adapter/mas.go` | `PlanUpgradeBatch` |
| `internal/adapter/adapter_test.go` | Expect mas in `BatchUpgrader` set |
| `internal/adapter/js_outdated.go` (new) | Shared JS global outdated compare |
| `internal/adapter/{npm,pnpm,yarn}.go` | `ListOutdated` via shared helper |
| `internal/adapter/registry.go` (+ new pypi/crates seams) | Registry latest helpers |
| `internal/adapter/{uv,pipx,cargo}.go` | `ListOutdated` |
| `internal/adapter/{winget,scoop,choco,pacman,paru,yay,snap}.go` | Native `ListOutdated` |
| `internal/adapter/outdated_test.go` | Tests for all new listers |
| `README.md`, `CHANGELOG.md` | User-facing docs |

---

### Task 1: Default outdated upgrade + `--all`

**Files:**
- Modify: `main.go` (`upgradeCmd`)
- Modify: `internal/upgrade/planner.go` (comment on `DetectOutdated`)
- Modify: `internal/output/json.go` (`UpgradeFilters.All`)
- Modify: `internal/upgrade/planner_test.go` (optional; CLI covered in `main_test.go`)
- Test: `main_test.go`
- Modify: `README.md` (upgrade flags + updates-check wording)

**Interfaces:**
- Consumes: `upgrade.BuildUpgradePlan`, `upgrade.UpgradeOptions.DetectOutdated`
- Produces: CLI flag `--all`; `DetectOutdated` true by default

- [ ] **Step 1: Write failing CLI test for default DetectOutdated**

Add a test (or extend an existing upgrade dry-run test) in `main_test.go` that builds a lock with brew-tracked packages and stubs outdated detection such that only one package is outdated. Assert default `genv upgrade --dry-run` plans only that package. Prefer testing through `upgrade.BuildUpgradePlan` from a thin CLI integration if full PATH stubbing is hard — minimum: assert the options path by extracting a small helper, OR add:

```go
func TestUpgradeCmd_defaultsToDetectOutdated(t *testing.T) {
	// Prefer unit-level: call BuildUpgradePlan with the same options upgradeCmd will pass.
	// After implementation, upgradeCmd must pass DetectOutdated: true unless --all.
	opts := upgrade.UpgradeOptions{DetectOutdated: true}
	if !opts.DetectOutdated {
		t.Fatal("default upgrade must detect outdated")
	}
}
```

Better concrete test — extend planner usage from CLI by factoring options:

In `main.go`, after parsing flags, the call must become:

```go
planResult, err := upgrade.BuildUpgradePlan(upgrade.UpgradeOptions{
	Spec:           f,
	Lock:           lf,
	Filters:        filters,
	DetectOutdated: !*all,
})
```

Add `main_test.go` coverage that runs `upgrade --dry-run --json` against a fixture where a fake brew outdated returns one package — follow existing upgrade CLI test patterns in `main_test.go` (search `upgrade --dry-run`). If no easy brew stub exists at CLI level, add:

```go
// internal/upgrade/planner_test.go
func TestBuildUpgradePlan_DetectOutdated_default_path_documented(t *testing.T) {
	// Existing TestBuildUpgradePlan_DetectOutdated_filters_to_outdated already covers filtering.
	// This task only requires CLI wiring; keep planner tests green.
}
```

And add a focused wiring test that parses flags:

```go
func TestUpgradeFlagAll_disablesDetectOutdated(t *testing.T) {
	// Document expected semantics in a table test if you extract
	// upgradeDetectOutdated(all bool) bool { return !all }
	if upgradeDetectOutdated(false) != true {
		t.Fatal("want DetectOutdated true when --all unset")
	}
	if upgradeDetectOutdated(true) != false {
		t.Fatal("want DetectOutdated false when --all set")
	}
}
```

Add helper next to `upgradeCmd`:

```go
func upgradeDetectOutdated(all bool) bool { return !all }
```

- [ ] **Step 2: Run test to verify RED**

```bash
go test . -run 'UpgradeFlagAll|UpgradeCmd.*[Oo]utdated' -count=1
```

Expected: FAIL (helper/flag missing) or compile error.

- [ ] **Step 3: Implement CLI wiring**

In `upgradeCmd` flag set:

```go
all := fs.Bool("all", false, "upgrade every unconstrained tracked package (skip outdated detection)")
```

Set filters:

```go
filters := output.UpgradeFilters{
	Only:         only,
	Skip:         skip,
	OnlyManager:  onlyManager,
	SkipManager:  skipManager,
	HooksSkipped: *noHooks,
	All:          *all,
}
```

In `internal/output/json.go`:

```go
type UpgradeFilters struct {
	Only         []string `json:"only,omitempty"`
	Skip         []string `json:"skip,omitempty"`
	OnlyManager  []string `json:"onlyManager,omitempty"`
	SkipManager  []string `json:"skipManager,omitempty"`
	HooksSkipped bool     `json:"hooksSkipped,omitempty"`
	All          bool     `json:"all,omitempty"`
}
```

Wire plan build:

```go
planResult, err := upgrade.BuildUpgradePlan(upgrade.UpgradeOptions{
	Spec:           f,
	Lock:           lf,
	Filters:        filters,
	DetectOutdated: upgradeDetectOutdated(*all),
})
```

Update planner comment:

```go
// DetectOutdated narrows the plan to packages that actually have an update
// available (via each manager's OutdatedLister). Default for `genv upgrade`
// and for updates check / background worker. `genv upgrade --all` sets this
// false to restore brute-force planning of every unconstrained tracked package.
```

Keep empty-plan message as existing: `no upgradeable packages found.`

README: under `genv upgrade` flags add `--all`; under updates check, replace "same shared dry-run planner as `genv upgrade --dry-run`" with wording that both share the planner and default to outdated filtering; `upgrade --all` skips that filter.

- [ ] **Step 4: Run tests GREEN**

```bash
go test . ./internal/upgrade ./internal/output -count=1
```

Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add main.go internal/upgrade/planner.go internal/output/json.go internal/upgrade/planner_test.go main_test.go README.md
git commit -m "$(cat <<'EOF'
feat(upgrade): default to outdated-only plans with --all escape hatch

EOF
)"
```

---

### Task 2: Mas `BatchUpgrader`

**Files:**
- Modify: `internal/adapter/mas.go`
- Modify: `internal/adapter/adapter_test.go` (`TestBatchUpgrader_ExpectedAdapters`, `TestPlanUpgradeBatch_ExpectedBinaries`)
- Test: `internal/adapter/mas_test.go` or `outdated_test.go` / new cases in `adapter_test.go`

**Interfaces:**
- Consumes: `BatchUpgrader`
- Produces: `func (Mas) PlanUpgradeBatch(pkgNames []string) []string`

- [ ] **Step 1: Write failing tests**

```go
func TestMas_PlanUpgradeBatch(t *testing.T) {
	args := Mas{}.PlanUpgradeBatch([]string{"497799835", "409201541"})
	want := []string{"mas", "upgrade", "497799835", "409201541"}
	if !slices.Equal(args, want) {
		t.Fatalf("PlanUpgradeBatch = %v, want %v", args, want)
	}
}
```

Update `TestBatchUpgrader_ExpectedAdapters` want map to include `"mas": true`.
Update `TestPlanUpgradeBatch_ExpectedBinaries` with `{"mas", "mas"}`.

- [ ] **Step 2: Run RED**

```bash
go test ./internal/adapter -run 'BatchUpgrader_Expected|Mas_PlanUpgradeBatch|PlanUpgradeBatch_ExpectedBinaries' -count=1
```

Expected: FAIL — mas not BatchUpgrader / method missing.

- [ ] **Step 3: Implement**

In `mas.go`:

```go
// PlanUpgradeBatch upgrades multiple App Store apps in one mas invocation.
func (Mas) PlanUpgradeBatch(pkgNames []string) []string {
	args := []string{"mas", "upgrade"}
	return append(args, pkgNames...)
}
```

- [ ] **Step 4: Run GREEN**

```bash
go test ./internal/adapter ./internal/resolver -run 'Batch|Mas_|PlanUpgrade' -count=1
```

- [ ] **Step 5: Commit**

```bash
git add internal/adapter/mas.go internal/adapter/adapter_test.go internal/adapter/mas_test.go
git commit -m "$(cat <<'EOF'
feat(mas): batch App Store upgrades into one mas upgrade invocation

EOF
)"
```

---

### Task 3: Shared JS outdated helper + npm/pnpm/yarn

**Files:**
- Create: `internal/adapter/js_outdated.go`
- Modify: `internal/adapter/bun.go` (refactor to call shared helper; behavior unchanged)
- Modify: `internal/adapter/npm.go`, `pnpm.go`, `yarn.go`
- Modify: `internal/adapter/outdated_test.go`

**Interfaces:**
- Consumes: `jsPackageEntry`, `npmLatestVersion`, `jsBasePackageName`
- Produces:

```go
func listJSOutdated(installed map[string]string, pkgNames []string) (map[string]string, error)
```

- [ ] **Step 1: Write failing npm ListOutdated test**

```go
func TestNpm_ListOutdated_ReportsOnlyDiffering(t *testing.T) {
	installFakeBinary(t, "npm",
		`if [ "$1" = "list" ]; then
	cat <<'JSON'
{"dependencies":{"cf":{"version":"1.2.0"},"typescript":{"version":"5.0.0"}}}
JSON
	exit 0
fi
echo "unexpected: $*" >&2; exit 1`)

	restore := swapNpmLatest(t, map[string]string{"cf": "1.3.0", "typescript": "5.0.0"}, nil)
	defer restore()

	got, err := Npm{}.ListOutdated([]string{"cf", "typescript"})
	if err != nil {
		t.Fatalf("ListOutdated: %v", err)
	}
	want := map[string]string{"cf": "1.3.0"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}
```

Mirror for pnpm and yarn (same JSON shape via their existing `listEntries`).

- [ ] **Step 2: Run RED**

```bash
go test ./internal/adapter -run 'Npm_ListOutdated|Pnpm_ListOutdated|Yarn_ListOutdated' -count=1
```

Expected: FAIL — method undefined.

- [ ] **Step 3: Implement shared helper + adapters**

`js_outdated.go`:

```go
package adapter

// listJSOutdated compares installed name->version against npm registry latest.
// Missing installs are skipped. Transport errors flag the package conservatively
// (map value = installed version). 404/empty latest => up to date.
func listJSOutdated(installed map[string]string, pkgNames []string) (map[string]string, error) {
	names := pkgNames
	if len(names) == 0 {
		names = make([]string, 0, len(installed))
		for n := range installed {
			names = append(names, n)
		}
	}
	outdated := make(map[string]string)
	for _, raw := range names {
		base := jsBasePackageName(raw)
		cur, ok := installed[base]
		if !ok {
			continue
		}
		latest, err := npmLatestVersion(base)
		if err != nil {
			outdated[base] = cur
			continue
		}
		if latest != "" && latest != cur {
			outdated[base] = latest
		}
	}
	if len(outdated) == 0 {
		return nil, nil
	}
	return outdated, nil
}
```

Refactor `Bun.ListOutdated` to build `installed` then `return listJSOutdated(installed, pkgNames)`.

For npm/pnpm/yarn:

```go
func (n Npm) ListOutdated(pkgNames []string) (map[string]string, error) {
	entries, err := n.listEntries()
	if err != nil {
		return nil, err
	}
	return listJSOutdated(entriesVersions(entries), pkgNames)
}
```

Same pattern for `Pnpm` and `Yarn`.

- [ ] **Step 4: Run GREEN**

```bash
go test ./internal/adapter -run 'ListOutdated|Bun_ListOutdated|Npm_|Pnpm_|Yarn_' -count=1
```

- [ ] **Step 5: Commit**

```bash
git add internal/adapter/js_outdated.go internal/adapter/bun.go internal/adapter/npm.go internal/adapter/pnpm.go internal/adapter/yarn.go internal/adapter/outdated_test.go
git commit -m "$(cat <<'EOF'
feat(js): add OutdatedLister for npm, pnpm, and yarn globals

EOF
)"
```

---

### Task 4: PyPI latest seam + uv/pipx OutdatedLister

**Files:**
- Modify: `internal/adapter/registry.go` (add `pypiLatestVersion` / `fetchPypiLatest`)
- Modify: `internal/adapter/uv.go`, `pipx.go`
- Modify: `internal/adapter/outdated_test.go`

**Interfaces:**
- Produces:

```go
var pypiLatestVersion = fetchPypiLatest
func fetchPypiLatest(name string) (string, error)
func listRegistryOutdated(installed map[string]string, pkgNames []string, baseName func(string) string, latest func(string) (string, error)) (map[string]string, error)
```

Prefer extracting `listRegistryOutdated` used by JS + PyPI + crates to avoid three copies — if Task 3's `listJSOutdated` can become a thin wrapper around a generic helper, refactor in this task.

- [ ] **Step 1: Write failing tests**

```go
func TestUv_ListOutdated_ReportsOnlyDiffering(t *testing.T) {
	installFakeBinary(t, "uv",
		`if [ "$1" = "tool" ] && [ "$2" = "list" ]; then
	echo "ruff v0.6.0"
	echo "black v24.0.0"
	exit 0
fi
exit 1`)

	orig := pypiLatestVersion
	pypiLatestVersion = func(name string) (string, error) {
		return map[string]string{"ruff": "0.7.0", "black": "24.0.0"}[name], nil
	}
	defer func() { pypiLatestVersion = orig }()

	got, err := Uv{}.ListOutdated([]string{"ruff", "black"})
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{"ruff": "0.7.0"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v want %v", got, want)
	}
}
```

Similar for pipx with fake `pipx list --json` payload matching `parsePipxListJSON`.

Also test `fetchPypiLatest` with httptest:

```go
// GET {base}/pypi/ruff/json -> {"info":{"version":"0.7.0"}}
```

- [ ] **Step 2: Run RED**

```bash
go test ./internal/adapter -run 'Uv_ListOutdated|Pipx_ListOutdated|FetchPypiLatest' -count=1
```

- [ ] **Step 3: Implement**

In `registry.go`:

```go
var pypiRegistryBase = "https://pypi.org"
var pypiLatestVersion = fetchPypiLatest

func fetchPypiLatest(name string) (string, error) {
	endpoint := pypiRegistryBase + "/pypi/" + url.PathEscape(name) + "/json"
	resp, err := npmHTTPClient.Get(endpoint) // reuse short-timeout client
	// ... 404 => "", nil; 200 => decode info.version
}
```

Uv:

```go
func (Uv) ListOutdated(pkgNames []string) (map[string]string, error) {
	entries, err := Uv{}.listEntries()
	if err != nil {
		return nil, err
	}
	installed := make(map[string]string, len(entries))
	for _, e := range entries {
		installed[e.name] = e.version
	}
	return listRegistryOutdated(installed, pkgNames, uvToolName, pypiLatestVersion)
}
```

Pipx: same with `PythonBasePackageName` and pipx `listEntries`.

Generic helper (put in `js_outdated.go` renamed or `outdated_compare.go`):

```go
func listRegistryOutdated(
	installed map[string]string,
	pkgNames []string,
	baseName func(string) string,
	latestFn func(string) (string, error),
) (map[string]string, error) {
	// same loop as listJSOutdated; transport error => outdated[base]=cur
}
```

- [ ] **Step 4: Run GREEN**

```bash
go test ./internal/adapter -run 'Uv_|Pipx_|Pypi|ListOutdated' -count=1
```

- [ ] **Step 5: Commit**

```bash
git add internal/adapter/registry.go internal/adapter/uv.go internal/adapter/pipx.go internal/adapter/outdated_compare.go internal/adapter/js_outdated.go internal/adapter/outdated_test.go
git commit -m "$(cat <<'EOF'
feat(python): add OutdatedLister for uv and pipx tools

EOF
)"
```

---

### Task 5: Cargo OutdatedLister

**Files:**
- Modify: `internal/adapter/registry.go` (`cratesLatestVersion` / `fetchCratesLatest`)
- Modify: `internal/adapter/cargo.go`
- Modify: `internal/adapter/outdated_test.go`

**Interfaces:**
- Produces: `func (Cargo) ListOutdated(pkgNames []string) (map[string]string, error)`

- [ ] **Step 1: Failing test**

Fake `cargo install --list`:

```
ripgrep v14.0.0:
    rg
fd-find v9.0.0:
    fd
```

Stub crates.io latest: ripgrep→14.1.0, fd-find→9.0.0. Expect only ripgrep outdated.

httptest for `GET /api/v1/crates/ripgrep` → `{"crate":{"max_version":"14.1.0"}}` (or `max_stable_version` if preferred — use `max_version`).

- [ ] **Step 2: RED**

```bash
go test ./internal/adapter -run 'Cargo_ListOutdated|FetchCratesLatest' -count=1
```

- [ ] **Step 3: Implement**

```go
func (c Cargo) ListOutdated(pkgNames []string) (map[string]string, error) {
	entries, err := c.listEntries()
	if err != nil {
		return nil, err
	}
	installed := make(map[string]string, len(entries))
	for _, e := range entries {
		installed[e.name] = e.version
	}
	return listRegistryOutdated(installed, pkgNames, cargoBaseName, cratesLatestVersion)
}
```

User-Agent: crates.io requires a User-Agent — set on requests:

```go
req.Header.Set("User-Agent", "genv (https://github.com/ks1686/genv)")
```

- [ ] **Step 4: GREEN + commit**

```bash
go test ./internal/adapter -run 'Cargo_' -count=1
git add internal/adapter/cargo.go internal/adapter/registry.go internal/adapter/outdated_test.go
git commit -m "$(cat <<'EOF'
feat(cargo): add OutdatedLister via crates.io latest

EOF
)"
```

---

### Task 6: Winget / Scoop / Choco OutdatedLister

**Files:**
- Modify: `internal/adapter/winget.go`, `scoop.go`, `choco.go`
- Modify: `internal/adapter/outdated_test.go`

**Native commands (parse + intersect):**

| Manager | Command | Parse |
|---------|---------|-------|
| winget | `winget upgrade` (table) or `winget list --upgrade-available` | reuse/extend `parseWingetTable`; Id column |
| scoop | `scoop status` | first field app name; skip headers |
| choco | `choco outdated -r` | `name|current|available|pinned` pipe-separated |

- [ ] **Step 1: Failing tests with fake binaries** for each manager (success, intersect, empty, command failure → error).

Example choco:

```go
installFakeBinary(t, "choco",
	`if [ "$1" = "outdated" ]; then
	echo "git|2.0.0|2.1.0|false"
	echo "nodejs|20.0.0|20.0.0|false"
	exit 0
fi
exit 1`)
// Only include packages where available != current; here git only if versions differ.
```

For choco `-r` output, flag when available != current.

- [ ] **Step 2: RED**

```bash
go test ./internal/adapter -run 'Winget_ListOutdated|Scoop_ListOutdated|Choco_ListOutdated' -count=1
```

- [ ] **Step 3: Implement each `ListOutdated`**, intersecting with `pkgNames` when non-empty. Return `nil, nil` if none. Return error on command failure (non-empty stderr / exit ≠ 0) so FilterOutdated keeps packages.

Helper:

```go
func intersectNameMap(all map[string]string, pkgNames []string) map[string]string {
	if len(pkgNames) == 0 {
		if len(all) == 0 {
			return nil
		}
		return all
	}
	want := map[string]bool{}
	for _, n := range pkgNames {
		want[n] = true
	}
	out := map[string]string{}
	for n, v := range all {
		if want[n] {
			out[n] = v
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
```

- [ ] **Step 4: GREEN + commit**

```bash
go test ./internal/adapter -run 'Winget_|Scoop_|Choco_' -count=1
git add internal/adapter/winget.go internal/adapter/scoop.go internal/adapter/choco.go internal/adapter/outdated_test.go
git commit -m "$(cat <<'EOF'
feat(windows): add OutdatedLister for winget, scoop, and choco

EOF
)"
```

---

### Task 7: Pacman / Paru / Yay OutdatedLister

**Files:**
- Modify: `internal/adapter/pacman.go`, `paru.go`, `yay.go`
- Modify: `internal/adapter/outdated_test.go`

**Command:** `pacman -Qu` / `paru -Qu` / `yay -Qu`  
Output: `name old -> new` or `name version` — parse first field as name; second token or arrow target as latest version string for the map value.

Shared parser:

```go
func parsePacmanQuLines(lines []string) map[string]string {
	out := map[string]string{}
	for _, line := range lines {
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		name := fields[0]
		latest := "outdated"
		if len(fields) >= 2 {
			latest = fields[len(fields)-1] // after "->"
		}
		out[name] = latest
	}
	return out
}
```

- [ ] **Step 1–4:** TDD each adapter with fake binary; commit once all three green.

```bash
go test ./internal/adapter -run 'Pacman_ListOutdated|Paru_ListOutdated|Yay_ListOutdated' -count=1
git commit -m "$(cat <<'EOF'
feat(arch): add OutdatedLister for pacman, paru, and yay

EOF
)"
```

---

### Task 8: Snap OutdatedLister

**Files:**
- Modify: `internal/adapter/snap.go`
- Modify: `internal/adapter/outdated_test.go`

**Command:** `snap refresh --list`  
Skip header line; first field = snap name; version column = latest.

- [ ] **Step 1: Failing test** with fake `snap refresh --list` table.
- [ ] **Step 2: RED**
- [ ] **Step 3: Implement `ListOutdated`**
- [ ] **Step 4: GREEN + commit**

```bash
git commit -m "$(cat <<'EOF'
feat(snap): add OutdatedLister via snap refresh --list

EOF
)"
```

---

### Task 9: Docs + CHANGELOG + full verification

**Files:**
- Modify: `CHANGELOG.md` (new Unreleased or next version section)
- Modify: `README.md` (finish any remaining wording from Task 1)
- Modify: `internal/upgrade/planner.go` comment if still stale

- [ ] **Step 1: CHANGELOG entry**

```markdown
## Unreleased

### Changed

- `genv upgrade` now plans only packages with a detected update by default (same outdated filtering as `genv updates check`). Pass `--all` to restore the previous brute-force plan of every unconstrained tracked package. Outdated detection now also covers npm/pnpm/yarn, uv/pipx, cargo, winget/scoop/choco, pacman/paru/yay, and snap. Multiple Mac App Store upgrades are batched into one `mas upgrade` invocation.
```

- [ ] **Step 2: Full test suite for touched packages**

```bash
go test ./internal/adapter ./internal/upgrade ./internal/resolver ./internal/output . -count=1
```

Expected: PASS

- [ ] **Step 3: Manual smoke (on this machine)**

```bash
go build -o /tmp/genv .
/tmp/genv upgrade --dry-run 2>&1 | head -40
/tmp/genv upgrade --all --dry-run 2>&1 | head -20
```

Expected: default dry-run lists only outdated packages (small); `--all` lists the large tracked set.

- [ ] **Step 4: Commit**

```bash
git add CHANGELOG.md README.md
git commit -m "$(cat <<'EOF'
docs: document outdated-default upgrade and expanded listers

EOF
)"
```

---

## Spec coverage checklist

| Spec requirement | Task |
|------------------|------|
| Default `DetectOutdated: true` | 1 |
| `--all` escape hatch | 1 |
| Empty plan → exit 0, short message | 1 (existing path) |
| Keep successful upgrade output | 1 (no change to execute path) |
| Updates check/worker unchanged flag | 1 |
| Conservative keep-all without lister | unchanged FilterOutdated |
| Lister failure keeps packages | unchanged FilterOutdated |
| npm/pnpm/yarn listers | 3 |
| uv/pipx listers | 4 |
| cargo lister | 5 |
| winget/scoop/choco | 6 |
| pacman/paru/yay | 7 |
| snap | 8 |
| mas BatchUpgrader | 2 |
| README / CHANGELOG / planner comment | 1, 9 |
| Deferred adapters | non-goal |

## Placeholder / consistency self-review

- No TBD steps; registry helpers named `pypiLatestVersion` / `cratesLatestVersion` consistently.
- `listRegistryOutdated` introduced in Task 4; Task 3 may land `listJSOutdated` first then refactor — implementer should not leave both duplicated long-term.
- Mas added to `TestBatchUpgrader_ExpectedAdapters` want map in Task 2.
