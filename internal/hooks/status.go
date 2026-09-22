package hooks

import (
	"bytes"
	"strings"
)

// Hook status values printed in the per-phase summary. Hooks report these by
// printing a line `GENV_HOOK_STATUS=changed|skipped` on stdout or stderr.
const (
	StatusChanged = "changed"
	StatusSkipped = "skipped"
	StatusError   = "error"
)

const hookStatusPrefix = "GENV_HOOK_STATUS="

// classifyHookStatus maps a hook's exit code and captured output to a status.
// Non-zero exit is always error. Exit 0 uses the last well-formed
// GENV_HOOK_STATUS line, defaulting to changed so legacy exit-only hooks stay
// visible as work rather than a silent no-op.
func classifyHookStatus(exitCode int, output []byte) string {
	if exitCode != 0 {
		return StatusError
	}
	switch last := lastHookStatusLine(output); last {
	case StatusChanged, StatusSkipped:
		return last
	default:
		return StatusChanged
	}
}

func lastHookStatusLine(output []byte) string {
	var last string
	for _, raw := range bytes.Split(output, []byte("\n")) {
		line := strings.TrimSpace(string(raw))
		status, ok := strings.CutPrefix(line, hookStatusPrefix)
		if !ok {
			continue
		}
		switch status {
		case StatusChanged, StatusSkipped, StatusError:
			last = status
		}
	}
	return last
}

func formatHookStatus(status string) string {
	if status == StatusSkipped {
		return "skipped (no-op)"
	}
	return status
}
