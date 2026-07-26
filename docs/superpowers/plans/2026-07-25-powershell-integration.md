# PowerShell Integration Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Native Windows PowerShell parity for env/shell fragments, profile injection, completions, and hooks (prefer `pwsh`, else `powershell.exe`), without requiring PowerShell on non-Windows hosts.

**Architecture:** Introduce `internal/profilebackend` with `POSIXBackend` and `PowerShellBackend`. Apply selects backends per GOOS/engine. Schema v7 adds `shell: "powershell"`. Hooks on Windows use the detected PS engine. Completions embed `genv.ps1`.

**Tech Stack:** Go 1.24, existing `internal/env`, `internal/shellcfg`, `internal/hooks`, `internal/schema`.

## Global Constraints

- Spec: `docs/superpowers/specs/2026-07-25-powershell-integration-design.md`
- Prefer `pwsh`, fall back to `powershell` / `powershell.exe`
- Omitted `shell` target = POSIX-only (never auto-apply to PowerShell)
- Missing PS engine on Windows: warn + skip PS backend, do not fail apply solely for that
- Non-Windows: never write `.ps1` profiles
- Keep `COVER_MIN=80` and existing CI gates green
- Schema additive v7; do not break v1–v6

---

## File map

| File | Responsibility |
|------|----------------|
| `internal/profilebackend/engine.go` | Detect PowerShell engine |
| `internal/profilebackend/backend.go` | Backend interface + SelectBackends |
| `internal/profilebackend/posix.go` | Wrap existing env/shellcfg apply |
| `internal/profilebackend/powershell.go` | `.ps1` render + profile inject |
| `internal/profilebackend/*_test.go` | Unit tests with temp HOME / fake PATH |
| `internal/schema/schema.go` | Version7, KnownShellTargets |
| `internal/schema/validate.go` | v7 / powershell messages |
| `internal/hooks/executor.go` | Windows PS runner |
| `main.go` | Wire apply backends; completion powershell |
| `completions/genv.ps1` | PowerShell completion script |
| Docs / CHANGELOG | Windows install + README |

---

### Task 1: Schema v7 + `powershell` shell target

**Files:**
- Modify: `internal/schema/schema.go`
- Modify: `internal/schema/validate.go`
- Modify: `internal/schema/validate_test.go`
- Modify: `internal/commands/shell_test.go` if it asserts invalid powershell

- [ ] **Step 1:** Add `Version7 = "7"` to `versionOrder`; add `"powershell"` to `KnownShellTargets`; update `ValidShellTargetsMsg`.
- [ ] **Step 2:** Update validate messages that list schema versions through v6 to include v7 where shell/env blocks are allowed.
- [ ] **Step 3:** Flip `TestParseAndValidate` / v7 rejection test to accept v7; add case for `shell: "powershell"`.
- [ ] **Step 4:** `go test ./internal/schema ./internal/commands -count=1`

---

### Task 2: Engine detection + backend selection

**Files:**
- Create: `internal/profilebackend/engine.go`
- Create: `internal/profilebackend/backend.go`
- Create: `internal/profilebackend/engine_test.go`
- Create: `internal/profilebackend/backend_test.go`

**Interfaces:**
- Produces: `type Engine struct { Bin string }`; `func DetectEngine() (Engine, bool)`
- Produces: `type Backend interface { Name() string; ApplyEnv(...); ApplyShell(...) }`
- Produces: `func SelectBackends(goos string) []Backend`

- [ ] **Step 1:** Failing tests for prefer-pwsh, fallback powershell, neither.
- [ ] **Step 2:** Implement DetectEngine via `exec.LookPath`.
- [ ] **Step 3:** SelectBackends: non-windows → posix only; windows+engine → ps (+ posix if `posixRelevant()`); windows no engine → posix if relevant else empty/warn path at call site.
- [ ] **Step 4:** `go test ./internal/profilebackend -count=1`

---

### Task 3: PowerShellBackend render + inject

**Files:**
- Create: `internal/profilebackend/powershell.go`
- Create: `internal/profilebackend/powershell_test.go`
- Modify: `internal/shellcfg/shellcfg.go` — skip `shell=="powershell"` in POSIX WriteFragment

- [ ] **Step 1:** Tests for env.ps1 / shell.ps1 content and idempotent profile inject.
- [ ] **Step 2:** Implement WriteEnvPS1, WriteShellPS1, InjectProfileLine (marked block).
- [ ] **Step 3:** Filter powershell-targeted aliases/functions into PS fragment only; POSIX WriteFragment ignores `shell=="powershell"`.
- [ ] **Step 4:** `go test ./internal/profilebackend ./internal/shellcfg -count=1`

---

### Task 4: Wire apply in main

**Files:**
- Modify: `main.go` (`applyEnvVars`, `applyShellCfg`)

- [ ] **Step 1:** Call `profilebackend.SelectBackends(runtime.GOOS)` and apply each; keep lock updates identical.
- [ ] **Step 2:** On Windows with no engine and PS-only intent, print warning once.
- [ ] **Step 3:** Extend `main_helpers_coverage_test.go` / env-shell tests if needed.
- [ ] **Step 4:** `go test . -count=1 -run 'TestApplyEnv|TestEnv|TestShell'`

---

### Task 5: Hooks PowerShell runner

**Files:**
- Modify: `internal/hooks/executor.go`
- Modify: `internal/hooks/*_test.go`

- [ ] **Step 1:** Failing test: on goos=windows with fake pwsh on PATH, shellFor/scriptRunner uses pwsh.
- [ ] **Step 2:** Implement Windows runner via DetectEngine; flags `-NoProfile -Command` / `-File`.
- [ ] **Step 3:** If no engine, fall back to `cmd /C` (compat) or fail closed with clear error—prefer **cmd fallback** with warning to avoid breaking existing Windows hooks mid-upgrade.
- [ ] **Step 4:** `go test ./internal/hooks -count=1`

---

### Task 6: Completions

**Files:**
- Create: `completions/genv.ps1`
- Modify: `main.go` embed + `completionScriptFor`
- Modify: `main_coverage_extra_test.go`

- [ ] **Step 1:** Minimal Register-ArgumentCompleter style script covering top-level commands.
- [ ] **Step 2:** Wire `completion powershell`; install default under genv config `completions/`.
- [ ] **Step 3:** Update tests that expect powershell as unknown shell.
- [ ] **Step 4:** `go test . -count=1 -run TestCompletion`

---

### Task 7: Docs + CHANGELOG + CI

**Files:**
- Modify: `docs/windows-install.md`, `README.md`, `CHANGELOG.md`, `AGENTS.md` CLI notes if needed

- [ ] **Step 1:** Document PS fragments, engine preference, completion, hooks.
- [ ] **Step 2:** CHANGELOG Unreleased entry.
- [ ] **Step 3:** `make ci` (or cover-gate + tests).
- [ ] **Step 4:** Commit.

---

## Done when

- [x] Design spec committed
- [ ] Schema v7 + powershell target
- [ ] Profile backends select and apply correctly
- [ ] Windows hooks prefer pwsh
- [ ] `genv completion powershell` works
- [ ] Docs updated; `make ci` green
