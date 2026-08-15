# genv Security Audit Report

**Date:** 2026-07-05  
**Auditor:** Sisyphus (OhMyOpenCode)  
**Scope:** Full repository — all Go source files, JSON schema, documentation  
**Methodology:** Static analysis of every line in `internal/`, `main.go`, `e2e/`, and `schema/`; verification against documented security claims.

---

## Remediation Status (2026-07-25)

**Current verdict: HISTORICAL / PARTIALLY REMEDIATED.** The original BLOCK verdict below is retained as the audit record. Several critical and high findings were fixed after 2026-07-05; remaining items are tracked here rather than as an open release blocker.

| # | Status | Notes |
|---|---|---|
| 1 | FIXED | Function bodies reject shell metacharacters; alias/function names must match `[A-Za-z_][A-Za-z0-9_.-]*` |
| 2 | FIXED | Shell `source` entries are single-quoted; schema rejects metacharacters |
| 3 | SUPERSEDED | `shellQuote`/`singleQuote` use POSIX `'\''` escaping; original quote-injection premise was incorrect |
| 4 | FIXED | `InjectSourceLine` quotes `fragmentPath` via `shellQuote` |
| 5 | FIXED | `genvfile.Write` re-validates via `schema.ParseAndValidate` before persist |
| 6 | FIXED | Service CLI uses `parseCommandWords` instead of `strings.Fields` |
| 7 | OPEN (by design) | `--file` accepts an explicit filesystem path; not a hidden traversal bypass |
| 8 | PARTIAL | `schema/v8/genv.json` exists (Go `ParseAndValidate` is authoritative); `schema/v1/genv.json` remains a v1 mirror |
| 9 | OPEN | Package IDs: empty/duplicate checks only |
| 10 | OPEN | Manager map *values* not content-validated (keys checked against `KnownManagers`) |
| 11 | OPEN | Version constraint strings lack format/length validation |
| 12 | FIXED | `runSubcmd` rejects empty argv |
| 13 | OPEN | No collection-size bounds in `ParseAndValidate` |
| 14 | FIXED | Alias and function names must match `[A-Za-z_][A-Za-z0-9_.-]*` |
| 15 | SUPERSEDED | Code accepts schema versions `"1"`–`"8"`; published mirrors exist for v1 and v8 |

Follow-ups that would close residual risk: collection bounds, published JSON Schema enum sync for v1, and package-ID charset/length checks.

---

## Executive Summary

**VERDICT (2026-07-05): BLOCK — Multiple critical and high-severity findings require fixes before this codebase is safe for production use.**

The project claims *"All user input goes through `schema.Validate()` before touching the filesystem or subprocesses"* (AGENTS.md line 74). This claim is **demonstrably false**. `schema.ParseAndValidate()` validates structural correctness (schema version, required fields, duplicate IDs, known manager names) but **does not validate the semantic content** of strings that are later:

- Executed as subprocess commands (`Service.Start`, `Service.Stop`)
- Written to shell fragments as raw code (`ShellFunction.Body`, `Shell.Source`)
- Injected into shell syntax via quoting flaws (`EnvVar.Value` containing `'`)
- Passed to package managers without content sanitization (`Package.ID`, `Package.Managers` values)

This creates multiple trust boundary violations where a malicious `genv.json` — or even a single CLI invocation — can achieve arbitrary code execution.

**No security-focused regression tests exist** in the test suite. All 559 tests pass, but none exercise adversarial inputs.

---

## Critical Findings (Immediate Fix Required)

### 1. CWE-78: Arbitrary Code Execution via Unvalidated Shell Function Bodies

| | |
|---|---|
| **File** | `internal/shellcfg/shellcfg.go` |
| **Line** | 70 |
| **Exact Code** | `body := fmt.Sprintf("%s() {\n%s\n}", name, indent(fn.Body))` |

`fn.Body` is inserted **raw and unescaped** into the generated POSIX shell fragment (`~/.config/genv/shell.sh`). When the user's shell sources this file on startup, the body is parsed as shell code.

**Attack scenario:** A malicious `genv.json` (e.g., from a compromised dotfiles repo) contains:
```json
{
  "shell": {
    "functions": {
      "pwn": {
        "body": "echo hi; } rm -rf /; false || {"
      }
    }
  }
}
```

Generated fragment:
```sh
pwn() {
    echo hi; } rm -rf /; false || {
}
```

Bash parses this as:
1. Define function `pwn` with body `echo hi`
2. `}` ends the function
3. `rm -rf /` executes immediately
4. `false || {` starts a new block; `}` ends it

**Impact:** Any shell session startup executes the injected payload.

**Fix:** Validate function bodies against a strict whitelist of safe constructs, or escape/reject shell metacharacters (`;`, `|`, `&`, `$()`, `` ` ``, `$(`, `${`, newlines). Consider deprecating raw function bodies in favor of safer alternatives.

---

### 2. CWE-78: Arbitrary Code Execution via Unvalidated Shell Source Entries

| | |
|---|---|
| **File** | `internal/shellcfg/shellcfg.go` |
| **Line** | 80 |
| **Exact Code** | `sb.WriteString(fmt.Sprintf(". %s\n", s))` |

Shell `source` entries from `genv.json` are inserted into the generated fragment **without quoting**. The `.` (source) builtin receives the path as a raw word; bash word-splits it and executes any shell metacharacters.

**Attack scenario:**
```json
{
  "shell": {
    "source": ["/dev/null; rm -rf /"]
  }
}
```

Generated fragment:
```sh
. /dev/null; rm -rf /
```

This sources `/dev/null` (no-op) and then executes `rm -rf /`.

**Impact:** Same as Finding #1 — arbitrary code execution on every shell startup.

**Fix:** Quote the source path with `shellQuote()` or reject paths containing shell metacharacters.

---

### 3. CWE-78: Command Injection via `shellQuote` when Value Starts with `'`

| | |
|---|---|
| **File** | `internal/env/env.go` (line 281), `internal/shellcfg/shellcfg.go` (line 261) |
| **Exact Code** | `return "'" + strings.ReplaceAll(v, "'", `'\''`) + "'"` |

`shellQuote()` and `singleQuote()` attempt to escape single quotes using the POSIX idiom `\'\'\'`. However, when the **value starts with a single quote**, the escaping produces a broken shell string that escapes the outer quotes and injects commands.

**Attack scenario:**
```bash
genv env set FOO "'; rm -rf /; '"
```

Generated fragment (`env.sh`):
```sh
export FOO=''\''; rm -rf /; '\'''
```

Bash parses this as:
1. `export FOO=''` — assigns empty string
2. `\'` — literal `'` outside quotes
3. `;` — command separator
4. `rm -rf /` — executes
5. `;` — command separator
6. `\'` — literal `'`
7. `''` — empty quoted string

The same bug affects alias values in `shellcfg.go`:
```json
{
  "shell": {
    "aliases": {
      "foo": {
        "value": "'; rm -rf /; '"
      }
    }
  }
}
```

Generated:
```sh
alias foo=''\''; rm -rf /; '\'''
```

Which also executes `rm -rf /`.

**Impact:** Arbitrary code execution when the shell fragment is sourced.

**Fix:** The escaping logic must handle the edge case where the value starts or ends with `'`. A safer approach is to reject values containing single quotes entirely, or use a more robust escaping function that does not produce command separators.

---

### 4. CWE-78: Arbitrary Code Execution via `XDG_CONFIG_HOME` Manipulation

| | |
|---|---|
| **File** | `internal/env/env.go` (line 87), `internal/shellcfg/shellcfg.go` (implied) |
| **Exact Code** | `sourceLine := ". " + fragmentPath` |

`InjectSourceLine()` constructs a shell `source` command by concatenating `. ` with the fragment path. The fragment path comes from `FragmentPath()` which uses `filepath.Join(DefaultDir(), "env.sh")`. `DefaultDir()` respects `$XDG_CONFIG_HOME`. If an attacker controls `XDG_CONFIG_HOME`, they can inject shell metacharacters into the source line written to `.bashrc`/`.zshrc`.

**Attack scenario:**
```bash
export XDG_CONFIG_HOME="/dev/null; rm -rf / #"
genv env set FOO bar
```

`FragmentPath()` returns `/dev/null; rm -rf / #/genv/env.sh`. `InjectSourceLine()` appends to `.bashrc`:
```sh
. /dev/null; rm -rf / #/genv/env.sh
```

On the next shell startup, `. /dev/null` is a no-op, then `rm -rf /` executes.

**Impact:** Arbitrary code execution on every new shell session.

**Fix:** Quote `fragmentPath` with `shellQuote()` when constructing `sourceLine`, or escape/reject shell metacharacters in the path. Alternatively, resolve the absolute path and verify it contains no metacharacters before writing the source line.

---

## High Findings (Fix Within One Sprint)

### 5. CWE-20: Trust Boundary Violation — CLI Commands Bypass Schema Validation

| | |
|---|---|
| **File** | `main.go` (multiple locations: 351, 547, 1151, 1352, 2560) |

The `add`, `adopt`, `env set`, `shell alias set`, and `service add` CLI commands construct `schema` structs in-memory and write them directly to `genv.json` via `genvfile.Write()`. **No validation is performed before writing.** The claim that "all user input goes through schema.Validate()" is violated because these commands never call `ParseAndValidate()` on the mutated struct.

**Attack scenario:** A user (or a malicious script) runs:
```bash
genv env set FOO "'; rm -rf /; '"
```

The value is written to `genv.json` without validation. On the next `genv apply`, it is written to `env.sh` and sourced.

**Fix:** After every in-memory mutation in CLI commands, re-validate the resulting `GenvFile` by serializing and calling `schema.ParseAndValidate()` before writing. Alternatively, make `genvfile.Write()` validate before persisting.

---

### 6. CWE-78: `strings.Fields` Breaks Shell Quoting in Service CLI

| | |
|---|---|
| **File** | `main.go` (lines 2560–2572) |
| **Exact Code** | `startCmd = strings.Fields(*start)` (and stopCmd, restartCmd, statusCmd) |

`strings.Fields` splits on any whitespace and **does not respect shell quoting**. A user who wants to pass a command with spaces or quotes cannot do so safely via the CLI.

**Attack scenario:**
```bash
genv service add foo --start 'sh -c "echo hello"'
```

`strings.Fields` produces `["sh", "-c", "\"echo", "hello\""]`. The quotes become literal characters. The `-c` argument to `sh` receives `"echo` (with a literal double-quote), which is not the intended command. Users must bypass the CLI and edit `genv.json` directly to inject raw arrays — which is exactly what a malicious actor would do.

**Fix:** Replace `strings.Fields` with a proper shell-aware parser (e.g., `github.com/kballard/go-shellquote` or a simple state machine that respects single and double quotes).

---

### 7. CWE-22: Path Traversal via `--file` Flag

| | |
|---|---|
| **File** | `main.go` (widespread), `internal/genvfile/genvfile.go` |

The `--file` flag accepts arbitrary paths with no validation. genv reads and writes `genv.json` and `genv.lock.json` (derived as `strings.TrimSuffix(path, ".json") + ".lock.json"`) at the specified path without checking for directory traversal.

**Attack scenario:**
```bash
genv apply --file /etc/cron.d/genv
```

This writes `/etc/cron.d/genv.lock.json`. If the user has permissions, it overwrites system files.

**Fix:** Validate that `--file` resolves to a path within an allowed directory (e.g., `$HOME/.config/genv/` or a user-specified dotfiles directory). Reject paths containing `..` or absolute paths outside the allowed directory.

---

## Medium Findings

### 8. CWE-1104: JSON Schema File is Outdated and Mismatched

| | |
|---|---|
| **File** | `schema/v1/genv.json` |

The JSON schema only validates version `"1"`, but the code accepts `"1"`, `"2"`, `"3"`, `"4"`. The `prefer` enum includes managers (`apt`, `dnf`, `pacman`, `flatpak`) that are **not** in `schema.KnownManagers` (`paru`, `yay`, `snap`, `brew`, `linuxbrew`). `additionalProperties: false` on the root means v2/v3/v4 features would be rejected by strict validators.

**Fix:** Update the JSON schema to cover v4, or create separate schema files per version. Sync the `prefer` enum and `propertyNames` with `schema.KnownManagers`.

---

### 9. CWE-20: Package ID Format Not Validated

| | |
|---|---|
| **File** | `internal/schema/validate.go` (line 231) |

`validatePackages()` only checks `pkg.ID != ""`. IDs containing path traversal (`../etc/passwd`), shell metacharacters, or control characters are accepted. These IDs flow into error messages, lock files, subprocess arguments, and file paths.

**Fix:** Validate `pkg.ID` against a strict whitelist (e.g., `^[a-zA-Z0-9._-]+$`) with a reasonable maximum length (e.g., 128 characters).

---

### 10. CWE-20: Manager-Specific Package Names Not Validated

| | |
|---|---|
| **File** | `internal/schema/validate.go` (lines 255–264) |

Validation checks that manager **keys** are known, but never validates the **values** (the actual package names passed to `NormalizeID()` and `PlanInstall()`). A malicious `genv.json` can set `"managers": {"brew": "foo; rm -rf /"}`. While `brew` itself uses `exec.Command` with argv (safe from shell injection), this is unvalidated user input flowing into subprocesses.

**Fix:** Validate manager map values against a safe character set (alphanumeric, hyphens, dots, underscores, plus signs) with a max length.

---

### 11. CWE-20 / CWE-400: Version String Not Validated

| | |
|---|---|
| **File** | `internal/schema/validate.go` (line 228) |

`pkg.Version` can be any string of arbitrary length. It is passed to `version.Satisfies()` in the resolver. A crafted version string could cause excessive CPU consumption.

**Fix:** Validate `version` against a known safe pattern (e.g., `^[0-9.*~^>=<+\-a-zA-Z]+$`) with a maximum length of 64 characters.

---

### 12. CWE-391: Potential Panic on Empty Command Slice

| | |
|---|---|
| **File** | `internal/resolver/resolver.go` (line 148) |
| **Exact Code** | `cmd := exec.CommandContext(ctx, args[0], args[1:]...)` |

`runSubcmd` assumes `args` has at least one element. If an adapter's `PlanInstall()`, `PlanUninstall()`, `PlanUpgrade()`, or `PlanClean()` returns an empty slice, `args[0]` causes a runtime panic.

**Fix:** Add a guard: `if len(args) == 0 { return fmt.Errorf("empty command") }`.

---

## Low Findings

### 13. CWE-400: No Bounds Checking on Collection Sizes

| | |
|---|---|
| **File** | `internal/schema/validate.go` (lines 228, 270, 292, 338) |

No maximum limits on `Packages`, `Env`, `Services`, `Shell.Aliases`, or `Shell.Functions`. A malicious `genv.json` with millions of entries can cause memory exhaustion.

**Fix:** Add reasonable maximums (e.g., `Packages` ≤ 10,000, `Env` ≤ 1,000, `Services` ≤ 500, shell entries ≤ 1,000).

---

### 14. CWE-78: Alias Names Inserted Raw into Shell Script

| | |
|---|---|
| **File** | `internal/shellcfg/shellcfg.go` (line 58) |
| **Exact Code** | `line := fmt.Sprintf("alias %s=%s", name, singleQuote(a.Value))` |

While `a.Value` is single-quoted, `name` is inserted **raw**. A malicious alias name like `foo='bar'; rm -rf /; alias baz` breaks the alias syntax.

**Fix:** Validate alias and function names against POSIX identifier rules (`[a-zA-Z_][a-zA-Z0-9_]*`).

---

## Info Findings

### 15. Schema Version Documentation Discrepancy

| | |
|---|---|
| **File** | `internal/schema/schema.go` (lines 5–14), `schema/v1/genv.json` |

The docs claim the schema version is `"v1"`, but the code uses plain integers `"1"`, `"2"`, `"3"`, `"4"`. The JSON schema file is in `schema/v1/` but only validates version `"1"`. This is confusing.

**Fix:** Align documentation, code constants, and JSON schema files. Use consistent naming.

---

## Summary Table

| # | Severity | File | Line | CWE | Description |
|---|----------|------|------|-----|-------------|
| 1 | **CRITICAL** | `shellcfg.go` | 70 | CWE-78 | Unvalidated shell function bodies execute arbitrary code |
| 2 | **CRITICAL** | `shellcfg.go` | 80 | CWE-78 | Unvalidated shell source entries execute arbitrary commands |
| 3 | **CRITICAL** | `env.go` | 281 | CWE-78 | `shellQuote` fails when value starts with `'`, injecting commands |
| 4 | **CRITICAL** | `env.go` | 87 | CWE-78 | `XDG_CONFIG_HOME` manipulation injects commands into `.bashrc` |
| 5 | **HIGH** | `main.go` | 351, 547, 1151, 1352, 2560 | CWE-20 | CLI commands bypass schema validation before writing |
| 6 | **HIGH** | `main.go` | 2560–2572 | CWE-78 | `strings.Fields` breaks shell quoting in service CLI |
| 7 | **HIGH** | `main.go` / `genvfile.go` | — | CWE-22 | Path traversal via `--file` flag |
| 8 | **MEDIUM** | `schema/v1/genv.json` | 12, 40 | CWE-1104 | JSON schema outdated and mismatched with code |
| 9 | **MEDIUM** | `validate.go` | 231 | CWE-20 | Package ID format not validated |
| 10 | **MEDIUM** | `validate.go` | 255–264 | CWE-20 | Manager map values not validated |
| 11 | **MEDIUM** | `validate.go` | 228 | CWE-20/400 | Version string not validated |
| 12 | **MEDIUM** | `resolver.go` | 148 | CWE-391 | Potential panic on empty command slice |
| 13 | **LOW** | `validate.go` | 228, 270, etc. | CWE-400 | No bounds checking on collection sizes |
| 14 | **LOW** | `shellcfg.go` | 58 | CWE-78 | Alias names inserted raw into shell script |
| 15 | **INFO** | `schema.go` | 5–14 | N/A | Schema version naming discrepancy |

---

## Recommended Priority Actions

### Immediate (This Week)

1. **Fix `shellQuote`/`singleQuote`** to handle values starting/ending with `'`. Reject values containing single quotes, or implement proper POSIX escaping that doesn't produce command separators.
2. **Validate shell function bodies** in `validateShell()` — reject shell metacharacters (`;`, `|`, `&`, `$()`, `` ` ``, `$(`, `${`).
3. **Quote shell source paths** with `shellQuote()` or reject paths containing shell metacharacters.
4. **Quote `fragmentPath`** in `InjectSourceLine()` — use `shellQuote(fragmentPath)` instead of raw concatenation.
5. **Add validation gate** in `genvfile.Write()` that calls `schema.ParseAndValidate()` before persisting.

### Short-Term (Next Sprint)

6. **Replace `strings.Fields`** in `serviceAddCmd` with a shell-quote-aware parser.
7. **Validate `--file`** to prevent path traversal.
8. **Update JSON schema** to v4 or create versioned schema files.
9. **Add bounds checking** on all collections in `ParseAndValidate()`.
10. **Add guard** in `runSubcmd` for empty command slices.

### Medium-Term

11. **Write adversarial regression tests** for all findings above.
12. **Validate package IDs** and manager map values against safe character sets.
13. **Validate alias/function names** against POSIX identifier rules.
14. **Validate env values** to reject newlines and control characters.

---

## Security Testing Gap

The current test suite (559 tests, all passing) contains **zero security-focused tests**. No tests exercise:
- Malicious `genv.json` payloads
- Shell metacharacters in env values, alias values, or function bodies
- Path traversal in `--file` or service names
- Argument injection in service commands
- Quoting edge cases in `shellQuote`/`singleQuote`

**Recommendation:** Add a dedicated `internal/security/` test package with adversarial inputs for every trust boundary.

---

## Verification Log

| Check | Result |
|-------|--------|
| All adapter subprocess calls use `exec.Command` with argv | ✅ Confirmed safe — no shell invocation |
| Service unit file path traversal | ✅ Mitigated by `filepath.Base` + backslash replacement |
| `schema.Validate()` validates structural fields | ✅ Confirmed — checks version, IDs, managers, duplicates |
| `schema.Validate()` validates semantic content | ❌ **FAIL** — does NOT validate command arrays, alias values, function bodies, env values |
| CLI commands validate before writing | ❌ **FAIL** — bypass `ParseAndValidate()` |
| JSON schema matches code | ❌ **FAIL** — schema is v1-only, code supports v4 |
| Tests cover adversarial inputs | ❌ **FAIL** — zero security tests |
