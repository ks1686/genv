# Unattended updates + system-scope elevation Implementation Plan

> **For agentic workers:** Execute inline in this worktree. TDD. Do not push.

**Goal:** Scheduled `updates __run-once` never prompts for admin; interactive `apply`/`upgrade` elevate system-scope external installs (issue #173); include the grpc 1.83.2 bump.

**Architecture:** `__run-once` sets `Unattended` on refresh and execute. Unix sudo argv gains `-n`; Windows/unelevated mutating winget/choco/paru/yay/sudo commands are skipped before spawn. Interactive system-scope file installs use sudo when the destination is not writable; unattended refuses elevation instead.

**Tech Stack:** Go 1.24+ (module already on 1.26.8 in v4.4.0), `golang.org/x/sys`

**Spec:** Conversation design — approach 3. Interactive may prompt; only `updates __run-once` is non-interactive.

## Global Constraints

- Interactive `genv apply` / `genv upgrade` may still sudo/UAC.
- `updates __run-once` (check refresh and autoApply) never prompts and never ShellExecutes runas.
- Passwordless `sudo -n` may still run on Unix; Windows unelevated skips mutating managers.
- Do not attempt an unprivileged write under a system destination when elevation is required (#173).
- Elevation skips are logged, not hard errors; the worker exits 0 when only skips remain.
- Include Dependabot grpc 1.82.1 → 1.83.2 in this branch.
- Authorship stays Karim; no AI trailers.
