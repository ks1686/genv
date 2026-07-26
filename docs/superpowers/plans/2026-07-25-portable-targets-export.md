# Portable Targets + Export Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Ship schema v8 multi-target portability (`defaults` + `targets.*`), safe apply with classify/merge/foreign-lock gating, assist-only `export`/`map`/`migrate`, and pull asset copy—without debloat/benchmark work.

**Architecture:** Keep a multi-target git source of truth. Apply resolves one active target (`--target` → `GENV_TARGET` → `Classify()`), merges optional `defaults` with tombstones, refuses foreign locks, then runs the existing reconcile pipeline on a flat effective `GenvFile`. Export reuses the same merge and writes a single-target snapshot + report. v1–v7 remain readable for migrate/`export --from-v7`; v8 bans top-level packages/env/shell/files/services/hooks and per-record `host`.

**Tech Stack:** Go 1.24, existing `internal/schema`, `internal/host`, `internal/genvfile`, `internal/resolver`, `main.go` flag dispatch. New packages: `internal/target` (resolve/merge helpers if not kept under schema), `internal/migrate`, `internal/export`.

## Global Constraints

- Spec: `docs/superpowers/specs/2026-07-25-portable-targets-export-design.md` (Status may still say Proposed; treat as approved)
- Schema clean break **v8**; no silent auto-translation of foreign managers at apply time
- Locks and secrets never in export/pull bundles
- Snap in scope for Ubuntu; **apt/dnf/apk deferred** (document only)
- Remove WSL→arch blanket inherit; target IDs: `macos`, `windows`, `arch`, `ubuntu`, `wsl-arch`, optional `linux`
- Tombstones: JSON `null` on env/alias/function keys in a target overlay
- `--force-new-lock` is the override name
- Keep `COVER_MIN=80` and `make ci` / bench-gate green
- **Out of scope:** codebase debloat, cold-start redesign, `genv add` install-before-write (separate track), publishing winget/scoop/choco

---

## File map

| File | Responsibility |
|------|----------------|
| `internal/schema/schema.go` | `Version8`, `TargetBundle`, `Defaults`/`Targets` on `GenvFile`, pointer env/alias maps for tombstones, `KnownTargets` |
| `internal/schema/merge.go` | `MergeTarget(f, id) (*GenvFile, error)` defaults+target+tombstones → flat effective spec |
| `internal/schema/validate.go` | Accept v8; reject top-level flat fields + `host` on v8; validate target IDs / tombstones |
| `internal/host/host.go` | Classify → new IDs; drop `GENV_HOST` class override and wsl2→arch inherit |
| `internal/host/filter.go` | Keep for v1–v7 apply only; document v8 does not use it |
| `internal/target/resolve.go` | `Resolve(flag string) (string, error)` order: flag, `GENV_TARGET`, `Classify()` |
| `internal/genvfile/lockfile.go` | `Target`, `GOOS` (and optional `Classified`) metadata |
| `internal/lockgate/gate.go` | Foreign-lock check vs active target + available managers |
| `internal/migrate/migrate.go` | v1–v7 → v8 bucketing |
| `internal/export/export.go` | Snapshot + `report.json` / `report.md` + asset copy |
| `internal/export/map.go` | Assist-only mapping suggestions |
| `internal/commands/*.go` | Mutate into `targets.<active>` when schema is v8 |
| `main.go` / `main_*.go` | `--target`, `--force-new-lock`, `migrate`/`export`/`map` dispatch; apply path |
| `main_pull.go` | Copy relative `files` assets with the spec |
| `schema/v8/genv.json` | JSON Schema for v8 (Go validator remains source of truth) |
| Docs | `docs/multi-machine.md`, `docs/wsl2-install.md`, `README.md`, `CHANGELOG.md`, `SCHEMA.md`, completions |

---

### Task 1: Schema v8 types + validation

**Files:**
- Modify: `internal/schema/schema.go`
- Create: `internal/schema/merge.go` (types only if merge deferred—prefer types in `schema.go`, merge in Task 2)
- Modify: `internal/schema/validate.go`
- Modify: `internal/schema/validate_test.go`
- Create: `schema/v8/genv.json` (minimal mirror of Go rules)

**Interfaces:**
- Produces:
  - `const Version8 = "8"`
  - `var KnownTargets = map[string]bool{"macos":true,"windows":true,"arch":true,"ubuntu":true,"wsl-arch":true,"linux":true}`
  - `type TargetBundle struct { Packages []Package; Env map[string]*EnvVar; Shell *ShellConfig; Services map[string]*Service; Files *FilesConfig; Hooks *HooksConfig }`
  - `GenvFile` gains `Defaults *TargetBundle \`json:"defaults,omitempty"\`` and `Targets map[string]*TargetBundle \`json:"targets,omitempty"\``
  - Nil map values in target `Env` / shell alias&function maps / services mean tombstone when overlaying defaults

**Notes on JSON null:** Use pointer map values (`map[string]*EnvVar`, `map[string]*ShellAlias`, `map[string]*ShellFunction`, `map[string]*Service`) inside `TargetBundle` and `ShellConfig` used under targets/defaults so `"KEY": null` unmarshals to a present key with nil pointer. Defaults must not contain nil pointer entries (validation error). Flat v1–v7 `GenvFile.Env map[string]EnvVar` stays value-typed for backward compatibility.

- [ ] **Step 1: Write failing validation tests**

```go
func TestParseAndValidate_V8RejectsTopLevelPackages(t *testing.T) {
	raw := `{"schemaVersion":"8","packages":[{"id":"git"}],"targets":{"arch":{"packages":[]}}}`
	_, errs, err := ParseAndValidate([]byte(raw))
	if err != nil {
		t.Fatal(err)
	}
	if len(errs) == 0 {
		t.Fatal("expected validation error for top-level packages on v8")
	}
}

func TestParseAndValidate_V8AcceptsDefaultsAndTargets(t *testing.T) {
	raw := `{
	  "schemaVersion":"8",
	  "defaults":{"env":{"EDITOR":{"value":"nvim"}}},
	  "targets":{
	    "arch":{"packages":[{"id":"git","prefer":"pacman"}],"env":{"EDITOR":null}},
	    "macos":{"packages":[{"id":"git","prefer":"brew"}]}
	  }
	}`
	f, errs, err := ParseAndValidate([]byte(raw))
	if err != nil || len(errs) > 0 {
		t.Fatalf("unexpected: err=%v errs=%v", err, errs)
	}
	if f.Targets["arch"].Env["EDITOR"] != nil {
		t.Fatal("expected tombstone nil pointer for EDITOR on arch")
	}
}

func TestParseAndValidate_V8RejectsHostOnPackage(t *testing.T) {
	raw := `{"schemaVersion":"8","targets":{"arch":{"packages":[{"id":"git","host":["arch"]}]}}}`
	_, errs, err := ParseAndValidate([]byte(raw))
	if err != nil {
		t.Fatal(err)
	}
	if len(errs) == 0 {
		t.Fatal("expected error for host on v8 package")
	}
}
```

- [ ] **Step 2: Run tests — expect FAIL** (Version8 unknown / types missing)

```bash
go test ./internal/schema -count=1 -run 'TestParseAndValidate_V8'
```

Expected: FAIL (unsupported version or compile error).

- [ ] **Step 3: Implement minimal schema + validate**

In `schema.go`:
- Add `Version8` to `versionOrder`
- Add `KnownTargets`
- Add `TargetBundle` and fields on `GenvFile`
- Keep flat fields for v1–v7 unmarshal

In `validate.go`:
- Accept `"8"` in `validateSchemaVersion`
- If version is v8:
  - Error if any top-level `packages`/`env`/`shell`/`files`/`services`/`hooks` are non-empty (treat omitted/empty as OK only if truly absent—prefer: any presence of non-zero top-level packages slice or non-nil env/shell/files/services/hooks → error)
  - Require `targets` non-empty
  - Each target key must be in `KnownTargets` (unknown key → error)
  - Reject any `Host` non-empty on packages/services/files/hooks inside defaults/targets
  - Reject nil pointer entries inside `defaults` maps (tombstones only valid under `targets.*`)
- Keep v1–v7 rules unchanged (still accept those versions)

- [ ] **Step 4: Run tests — expect PASS**

```bash
go test ./internal/schema -count=1
```

- [ ] **Step 5: Commit**

```bash
git add internal/schema schema/v8/genv.json
git commit -m "feat(schema): add v8 defaults/targets types and validation"
```

---

### Task 2: Merge defaults + target + tombstones

**Files:**
- Create: `internal/schema/merge.go`
- Create: `internal/schema/merge_test.go`

**Interfaces:**
- Consumes: `GenvFile` with `Defaults`/`Targets` from Task 1
- Produces: `func MergeTarget(f *GenvFile, targetID string) (*GenvFile, error)`
  - Error if `targets[targetID]` missing
  - Returns a **new** flat `*GenvFile` with `SchemaVersion: Version8`, `Repo`/`Updates` copied from source, and top-level `Packages`/`Env`/`Shell`/`Files`/`Services`/`Hooks` filled from merge
  - `Defaults`/`Targets` on the returned value are nil (flat effective doc for apply)
  - Array fields (`packages`, hook lists, file lists): target replaces defaults when target defines the field; else keep defaults
  - Map fields: defaults then target overlay; nil pointer in target deletes key
  - Returned `Env` should be `map[string]EnvVar` (dereference non-nil pointers; omit tombstoned keys)

- [ ] **Step 1: Failing merge tests**

```go
func TestMergeTarget_TargetWinsAndTombstone(t *testing.T) {
	f := &GenvFile{
		SchemaVersion: Version8,
		Defaults: &TargetBundle{
			Env: map[string]*EnvVar{
				"EDITOR": {Value: "nvim"},
				"LANG":   {Value: "en_US.UTF-8"},
			},
			Shell: &ShellConfig{Aliases: map[string]*ShellAlias{
				"ll": {Value: "ls -la"},
			}},
		},
		Targets: map[string]*TargetBundle{
			"macos": {
				Packages: []Package{{ID: "git", Prefer: "brew"}},
				Env: map[string]*EnvVar{
					"EDITOR": nil, // tombstone
					"HOMEBREW_NO_ANALYTICS": {Value: "1"},
				},
			},
		},
	}
	got, err := MergeTarget(f, "macos")
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := got.Env["EDITOR"]; ok {
		t.Fatal("EDITOR should be tombstoned away")
	}
	if got.Env["LANG"].Value != "en_US.UTF-8" {
		t.Fatalf("LANG=%q", got.Env["LANG"].Value)
	}
	if got.Env["HOMEBREW_NO_ANALYTICS"].Value != "1" {
		t.Fatal("missing target env")
	}
	if len(got.Packages) != 1 || got.Packages[0].Prefer != "brew" {
		t.Fatalf("packages=%v", got.Packages)
	}
	if got.Targets != nil || got.Defaults != nil {
		t.Fatal("effective doc must be flat")
	}
}

func TestMergeTarget_MissingTarget(t *testing.T) {
	f := &GenvFile{SchemaVersion: Version8, Targets: map[string]*TargetBundle{"arch": {}}}
	if _, err := MergeTarget(f, "ubuntu"); err == nil {
		t.Fatal("expected error")
	}
}
```

Adjust `ShellConfig.Aliases` type to `map[string]*ShellAlias` under bundles (Task 1). If flat v1–v7 shell still uses value maps, keep a conversion helper in merge output that emits the type `apply` already consumes—**prefer one ShellConfig shape with pointer maps project-wide**, updating existing shell tests/commands in this task if compile breaks.

- [ ] **Step 2: Run — expect FAIL**

```bash
go test ./internal/schema -count=1 -run TestMergeTarget
```

- [ ] **Step 3: Implement `MergeTarget`** deep-copying defaults then overlaying target; never mutate input.

- [ ] **Step 4: Run — expect PASS**

```bash
go test ./internal/schema -count=1
```

- [ ] **Step 5: Commit**

```bash
git add internal/schema/merge.go internal/schema/merge_test.go internal/schema/schema.go
git commit -m "feat(schema): merge defaults/targets with tombstones"
```

---

### Task 3: Classify target IDs + resolve

**Files:**
- Modify: `internal/host/host.go`
- Modify: `internal/host/host_test.go`
- Modify: `internal/host/filter_test.go` (update WSL inherit expectations)
- Create: `internal/target/resolve.go`
- Create: `internal/target/resolve_test.go`

**Interfaces:**
- Produces: `func Classify() (string, error)` returns `macos`|`windows`|`arch`|`ubuntu`|`wsl-arch` (or error)
- Produces: `func Match(predicate schema.HostPredicate, host string) bool` — **no** wsl→arch inherit
- Produces: `func Resolve(flag string) (string, error)` in `internal/target`:
  1. non-empty `flag`
  2. `os.Getenv("GENV_TARGET")`
  3. `host.Classify()`
  4. error with guidance text

**Classify rules:**
- Do **not** read `GENV_HOST` inside `Classify` (hostname override stays on `Current()` / explicit `--host` for legacy filter only)
- darwin → `macos`; windows → `windows`
- linux + WSL + Arch-like → `wsl-arch`
- linux + WSL + Ubuntu-like (`ID=ubuntu` or `ID_LIKE` contains ubuntu) → `ubuntu`
- linux + WSL + neither → error (set `GENV_TARGET`)
- linux + not WSL + Arch-like → `arch`
- linux + not WSL + Ubuntu-like → `ubuntu`
- else → error

- [ ] **Step 1: Rewrite/add tests**

```go
func TestMatch_WslArchDoesNotInheritBareArch(t *testing.T) {
	if Match(schema.HostPredicate{"arch"}, "wsl-arch") {
		t.Fatal("wsl-arch must not match bare arch after portability change")
	}
	if !Match(schema.HostPredicate{"wsl-arch"}, "wsl-arch") {
		t.Fatal("exact match")
	}
}

func TestResolve_PrefersFlagThenEnv(t *testing.T) {
	t.Setenv("GENV_TARGET", "ubuntu")
	got, err := Resolve("arch")
	if err != nil || got != "arch" {
		t.Fatalf("got %q err %v", got, err)
	}
	got, err = Resolve("")
	if err != nil || got != "ubuntu" {
		t.Fatalf("got %q err %v", got, err)
	}
}
```

For Classify unit tests that need `/etc/os-release` / `/proc/version`, keep existing pattern of env injection or extract `classifyLinux(osRelease, procVersion string) (string, error)` for pure testing.

- [ ] **Step 2: Run — expect FAIL** (inherit still true / resolve missing)

```bash
go test ./internal/host ./internal/target -count=1
```

- [ ] **Step 3: Implement classify + resolve; delete wsl2 inherit branch in `Match`; update any test named `TestMatch_WslInheritsArch` to the new contract; map legacy fixture strings carefully.**

- [ ] **Step 4: Run — expect PASS**

```bash
go test ./internal/host ./internal/target -count=1
```

- [ ] **Step 5: Commit**

```bash
git add internal/host internal/target
git commit -m "feat(host): classify ubuntu/wsl-arch; add target.Resolve"
```

---

### Task 4: Lock metadata + foreign-lock gate

**Files:**
- Modify: `internal/genvfile/lockfile.go`
- Modify: `internal/genvfile/genvfile_test.go` (or lock tests)
- Create: `internal/lockgate/gate.go`
- Create: `internal/lockgate/gate_test.go`

**Interfaces:**
- `LockFile` gains:
  - `Target string \`json:"target,omitempty"\``
  - `GOOS string \`json:"goos,omitempty"\``
- Produces: `type Decision struct { Foreign bool; Reason string }`
- Produces: `func Check(lf *genvfile.LockFile, activeTarget, goos string, available map[string]adapter.Adapter) Decision`
  - Empty lock (`len(Packages)==0` && Target=="" && GOOS=="") → not foreign
  - If `lf.Target != "" && lf.Target != activeTarget` → foreign
  - If `lf.GOOS != "" && lf.GOOS != goos` → foreign
  - If any `lf.Packages[i].Manager` is non-empty and not present in `available` → foreign
  - Else OK

- [ ] **Step 1: Failing tests**

```go
func TestCheck_MacOSLockOnArchIsForeign(t *testing.T) {
	lf := &genvfile.LockFile{
		Target: "macos",
		GOOS:   "darwin",
		Packages: []genvfile.LockedPackage{
			{ID: "foo", Manager: "mas", PkgName: "1"},
			{ID: "bar", Manager: "brew", PkgName: "bar"},
		},
	}
	available := map[string]adapter.Adapter{"pacman": stubAdapter{name: "pacman"}}
	d := Check(lf, "arch", "linux", available)
	if !d.Foreign {
		t.Fatal("expected foreign")
	}
}

func TestCheck_MatchingTargetOK(t *testing.T) {
	lf := &genvfile.LockFile{Target: "arch", GOOS: "linux", Packages: []genvfile.LockedPackage{{ID: "git", Manager: "pacman", PkgName: "git"}}}
	available := map[string]adapter.Adapter{"pacman": stubAdapter{name: "pacman"}}
	if Check(lf, "arch", "linux", available).Foreign {
		t.Fatal("expected OK")
	}
}
```

Use a tiny stub implementing `adapter.Adapter` in the test file (or reuse existing test doubles from `internal/resolver` tests).

- [ ] **Step 2: Run — expect FAIL**

```bash
go test ./internal/lockgate ./internal/genvfile -count=1
```

- [ ] **Step 3: Implement fields + `Check`**

- [ ] **Step 4: Run — expect PASS**

```bash
go test ./internal/lockgate ./internal/genvfile -count=1
```

- [ ] **Step 5: Commit**

```bash
git add internal/genvfile internal/lockgate
git commit -m "feat(lock): target/goos metadata and foreign-lock gate"
```

---

### Task 5: Wire apply — select, merge, gate, lock write

**Files:**
- Modify: `main.go` (`applyOptions`, `applyCmd`, `runApply`, `runApplyWithSpecAndLock`, `writeLockAfterApply`)
- Create or modify: `main_apply_portability_test.go` (new preferred)
- Modify: completions that list apply flags if present

**Interfaces:**
- `applyOptions` gains `Target string`, `ForceNewLock bool`
- Flags: `--target`, `--force-new-lock`
- Behavior in `runApplyWithSpecAndLock`:
  1. `available := resolver.Detect()`
  2. If `f.SchemaVersion == schema.Version8`:
     - `id, err := target.Resolve(opts.Target)`; on err → exit validation/usage with message
     - If `f.Targets[id]==nil` → fail (“no matching targets.<id>”)
     - `effective, err := schema.MergeTarget(f, id)`
     - Replace `f` with `effective` for reconcile/env/shell/files/hooks
     - `dec := lockgate.Check(lf, id, runtime.GOOS, available)`
     - If foreign && !opts.ForceNewLock → print reason + suggest backup/`--force-new-lock`; return nonzero (`exitIO` or dedicated)
     - If foreign && ForceNewLock → rename `lockPath` to `lockPath+".bak-<utc>"` (best effort), set `lf = &genvfile.LockFile{SchemaVersion: schema.Version8}`
  3. Else (v1–v7): keep `host.FilterForHost(f, hostForCommand(opts.Host))` path (legacy)
  4. On successful lock write: set `lf.Target` / `lf.GOOS` from active target + `runtime.GOOS` when v8 (and optionally when legacy if Classify succeeds)

- [ ] **Step 1: Failing integration-style test** with temp spec/lock, stubbing detect if needed via test manager registration already used in `main_test.go`:

```go
func TestApply_ForeignLockRefused(t *testing.T) {
	dir := t.TempDir()
	spec := filepath.Join(dir, "genv.json")
	lock := filepath.Join(dir, "genv.lock.json")
	writeFile(t, spec, `{"schemaVersion":"8","targets":{"arch":{"packages":[{"id":"git","prefer":"test-hook-manager"}]}}}`)
	writeFile(t, lock, `{"schemaVersion":"1","target":"macos","goos":"darwin","packages":[{"id":"x","manager":"mas","pkgName":"1"}]}`)
	code := run([]string{"apply", "--file", spec, "--lock-file", lock, "--target", "arch", "--yes", "--dry-run"})
	if code == exitOK {
		t.Fatalf("expected nonzero, got %d", code)
	}
}
```

Add a second test: `--force-new-lock` allows dry-run to proceed and does not plan mas uninstall (plan remove list must not include foreign ids that require unavailable managers—empty prior lock after force).

- [ ] **Step 2: Run — expect FAIL**

```bash
go test . -count=1 -run 'TestApply_ForeignLock|TestApply_ForceNewLock'
```

- [ ] **Step 3: Implement wiring** (keep dry-run still going through the gate so foreign locks fail before planning uninstalls).

- [ ] **Step 4: Run broader apply tests**

```bash
go test . -count=1 -run 'TestApply'
go test ./internal/schema ./internal/host ./internal/target ./internal/lockgate -count=1
```

- [ ] **Step 5: Commit**

```bash
git add main.go main_apply_portability_test.go completions/
git commit -m "feat(apply): target merge and foreign-lock refusal"
```

---

### Task 6: `genv migrate` (v7 host → v8 targets)

**Files:**
- Create: `internal/migrate/migrate.go`
- Create: `internal/migrate/migrate_test.go`
- Create: `main_migrate.go`
- Modify: `main.go` dispatch (`case "migrate":`)

**Interfaces:**
- Produces: `func ToV8(in *schema.GenvFile) (*schema.GenvFile, []string, error)` → out + warnings
- Rules (from design §2):
  - Empty-host env/shell keys → `defaults`
  - Empty-host packages/files/services/hooks → every concrete target inferred from observed host predicates; if none observed, create `macos`/`linux`/`windows` stubs only as needed—**minimum:** create buckets for every distinct host class seen; if none seen, put empty-host packages into `targets.linux` and also document warning
  - `host: ["macos"]` → `targets.macos`
  - `host` containing `arch` → `targets.arch`; if also/`only` `wsl2` → `targets.wsl-arch`
  - Strip all `Host` fields; set `schemaVersion` to `"8"`
  - Preserve `repo` / `updates` at top level

CLI: `genv migrate [--file] [--write]` — default print migrated JSON to stdout; `--write` overwrites file after validate.

- [ ] **Step 1: Failing fixture test** with a v7 JSON containing macos/arch/wsl2 packages + shared EDITOR env.

- [ ] **Step 2: Implement `ToV8` + `migrateCmd`**

- [ ] **Step 3:**

```bash
go test ./internal/migrate -count=1
go test . -count=1 -run TestMigrate
```

- [ ] **Step 4: Commit**

```bash
git add internal/migrate main_migrate.go main.go
git commit -m "feat: add genv migrate for v7 host specs to v8 targets"
```

---

### Task 7: `genv export` snapshot + report

**Files:**
- Create: `internal/export/export.go`
- Create: `internal/export/report.go`
- Create: `internal/export/export_test.go`
- Create: `main_export.go`
- Modify: `main.go` dispatch
- Create: `testdata/export/multi-target/genv.json` golden input
- Create: `testdata/export/multi-target/golden/arch/` (snapshot + report.json)

**Interfaces:**
- `type ReportItem struct { Class string; Code string; Message string; PackageID string }` with Class in `error|warning|suggestion`
- `func Build(f *schema.GenvFile, targetID string, outDir string) (Report, error)`:
  - `MergeTarget`
  - Write `outDir/genv.json` as v8 with single `targets.<id>` bucket containing merged content (no sibling targets); omit locks/secrets
  - Copy relative file sources into `outDir/` rewriting paths as needed
  - Build report: error if package has no usable manager for destination (prefer/`managers` keys intersect a static allowlist per target: macos → brew/mas/…; arch → pacman/paru/yay/…; ubuntu → snap/linuxbrew/…; windows → winget/scoop/choco/…); warning for absolute paths; suggestions for deferred apt/dnf text when prefer empty
- CLI: `genv export --target <id> --out <dir> [--strict] [--from-v7]`
  - `--strict` → nonzero if any error-class items
  - `--from-v7` → run migrate in memory first

- [ ] **Step 1: Golden test** — load fixture, export to temp dir, compare `genv.json` + `report.json` to golden (normalize path separators).

- [ ] **Step 2: Implement export + CLI**

- [ ] **Step 3:**

```bash
go test ./internal/export -count=1
go test . -count=1 -run TestExport
```

- [ ] **Step 4: Commit**

```bash
git add internal/export main_export.go main.go testdata/export
git commit -m "feat: add genv export --target snapshot and report"
```

---

### Task 8: `genv map` assist-only

**Files:**
- Create: `internal/export/map.go` (or `internal/mapassist/map.go`)
- Create: matching `*_test.go`
- Create: `main_map.go`
- Modify: `main.go` dispatch

**Interfaces:**
- `func Suggest(f *schema.GenvFile, destTarget string) []ReportItem` — read-only
- CLI: `genv map --target <dest> [--file]` prints suggestions
- Optional `--apply-suggestions` **must** require an extra `--yes` and rewrite only `managers`/`prefer` fields inside `targets.<dest>` (never during apply/export). If too risky for v1 of this work, ship print-only and leave `--apply-suggestions` unimplemented with a clear “not yet supported” message—**prefer print-only in this task** to stay YAGNI; document confirm-write as follow-up.

- [ ] **Step 1: Test** that a macos-only `mas` package produces a suggestion for `ubuntu` without mutating input.

- [ ] **Step 2: Implement + wire CLI**

- [ ] **Step 3:**

```bash
go test ./internal/export -count=1 -run Map
go test . -count=1 -run TestMap
```

- [ ] **Step 4: Commit**

```bash
git add internal/export main_map.go main.go
git commit -m "feat: add genv map assist-only package suggestions"
```

---

### Task 9: Pull copies file assets (SourceRoot)

**Files:**
- Modify: `main_pull.go`
- Create: helper in `internal/files/bundle.go` or `internal/pull/assets.go`
- Modify: `main_test.go` pull tests / add `TestPull_CopiesRelativeFileAssets`

**Behavior:**
- After fetching repo cache, read remote `genv.json` (v7 or v8)
- Collect relative `files.links[].source` and `files.templates[].source` from the effective set of paths (for v8: union across all targets+defaults; for v7: top-level files)
- Copy those relative paths from cache into the destination directory beside the written `genv.json` (destination dir = `filepath.Dir(--file)`)
- Never copy lockfiles or `**/secrets/**`
- Preserve `--dry-run` messaging listing assets that would copy

- [ ] **Step 1: Failing test** with a fake git repo or by factoring `copyPullBundle(cacheDir, destSpec string) error` and calling it directly with a temp cache tree.

- [ ] **Step 2: Implement copy; keep JSON write**

- [ ] **Step 3:**

```bash
go test . -count=1 -run TestPull
```

- [ ] **Step 4: Commit**

```bash
git add main_pull.go internal/pull main_test.go
git commit -m "fix(pull): copy relative files assets with spec bundle"
```

---

### Task 10: Command mutations write into active target (v8)

**Files:**
- Modify: `internal/commands/add.go`, `remove.go`, `env.go`, `shell.go`, `service.go`
- Modify: corresponding `*_test.go`
- Modify: `main.go` `addCmd`/`removeCmd`/env/shell/service to pass resolved target when spec is v8

**Behavior:**
- When `f.SchemaVersion == Version8`, `commands.Add` appends to `f.Targets[active].Packages` (create bundle if needed), not top-level
- Same for remove/env/shell/service
- `active` from `--target` / `GENV_TARGET` / `Classify()`; fail if unresolved or missing bucket (do not auto-create unknown target IDs; may create empty bundle only for KnownTargets already present—**require existing target key**)
- v1–v7 path unchanged (top-level)

- [ ] **Step 1: Failing unit tests** for `Add` into `targets.arch`

- [ ] **Step 2: Implement target-aware mutators (signature may gain `targetID string` or operate via helper `func ActiveBundle(f *GenvFile, id string) (*TargetBundle, error)`)

- [ ] **Step 3:**

```bash
go test ./internal/commands -count=1
go test . -count=1 -run 'TestAdd|TestRemove|TestEnv|TestShell'
```

- [ ] **Step 4: Commit**

```bash
git add internal/commands main.go
git commit -m "feat(commands): mutate v8 specs inside targets.<active>"
```

---

### Task 11: Docs, completions, CHANGELOG, CI gate

**Files:**
- Create: `docs/multi-machine.md`
- Modify: `docs/wsl2-install.md` (no blanket arch inheritance; `wsl-arch` vs `ubuntu`)
- Modify: `README.md` (target table; Linux channels: Arch-first + snap + linuxbrew; apt/dnf deferred)
- Modify: `SCHEMA.md`, `CHANGELOG.md`, `AGENTS.md` (CLI list + schema v8 fact)
- Modify: `completions/*` for `migrate`/`export`/`map`/`--target`/`--force-new-lock`

- [ ] **Step 1: Write multi-machine guide** covering git source of truth, never commit locks, export/migrate/apply flow, foreign-lock recovery.

- [ ] **Step 2: Update WSL + README tables**

- [ ] **Step 3: CHANGELOG Unreleased** entry for v8 portability

- [ ] **Step 4: Run full CI**

```bash
make ci
```

Expected: PASS (format, race tests, cover-gate ≥80, bench-gate).

- [ ] **Step 5: Commit**

```bash
git add docs README.md SCHEMA.md CHANGELOG.md AGENTS.md completions
git commit -m "docs: multi-machine portability and v8 target rollout"
```

---

## Container regression checklist (manual / CI follow-up)

Not blocking unit CI, but required before calling the suite done:

- [ ] Arch container + foreign macOS lock → refuse; `--force-new-lock` → no mas/brew uninstall plan
- [ ] Ubuntu fixture with snap prefer documents/resolves when snap present
- [ ] Migrate fixture from v7 host-scoped → v8 buckets inspected by hand once

Reuse ideas from the v4 readiness audit W7/W8 harnesses when adding workflow jobs (optional extra task if time; do not block docs commit).

---

## Done when

- [ ] Schema v8 validate + merge/tombstones
- [ ] Classify/resolve with ubuntu + wsl-arch; no WSL→arch inherit
- [ ] Foreign-lock gate + `--force-new-lock`; lock target/goos metadata
- [ ] Apply select→merge→gate wired
- [ ] `migrate`, `export`, `map` CLIs
- [ ] Pull asset copy
- [ ] v8 command mutations target-aware
- [ ] Docs/completions/CHANGELOG; `make ci` green
- [ ] Debloat **not** started

## Spec coverage (self-review)

| Spec item | Task |
|-----------|------|
| Approach 1 multi-target + export snapshot | 5, 7 |
| Everything under `targets.*`; optional `defaults` | 1, 2 |
| Tombstones null | 1, 2 |
| Assist-only translation | 7, 8 |
| Snap in scope; apt/dnf deferred | 7 report + 11 docs |
| Locks/secrets never in bundles | 7, 9 |
| No matching target → fail | 2, 5 |
| Foreign lock refuse + `--force-new-lock` | 4, 5 |
| `export --strict` | 7 |
| Classify IDs + drop inherit | 3 |
| Migrate v7→v8 | 6 |
| Pull SourceRoot assets | 9 |
| Docs / rollout | 11 |
| Debloat out of scope | Global Constraints |

## Explicitly not in this plan

- Performance debloat / cold-start redesign
- apt/dnf/apk adapters
- `genv add` install-before-spec-write ordering fix
- Auto-apply map suggestions without confirm (print-only in Task 8)
)
