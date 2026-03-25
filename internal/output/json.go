// Package output provides stable JSON envelope types for machine-readable
// genv output. The Envelope type is the top-level wrapper emitted to stdout
// when a command is run with --json. Inner Data types are stable across
// patch releases; new optional fields may be added in minor releases.
package output

import (
	"encoding/json"
	"io"
)

// SchemaVersion is the version of the JSON output envelope schema.
// Consumers should check this field to detect incompatible schema changes.
// The schema version follows the same major-version policy as genv itself:
// breaking changes bump the major version; new optional fields are additive.
const SchemaVersion = "1"

// Envelope is the top-level JSON wrapper emitted for every --json response.
type Envelope struct {
	Version string      `json:"version"`
	Command string      `json:"command"`
	OK      bool        `json:"ok"`
	Data    interface{} `json:"data,omitempty"`
	Errors  []string    `json:"errors,omitempty"`
}

// PlanPackage is a single entry in a PlanResult list.
type PlanPackage struct {
	ID      string `json:"id"`
	Manager string `json:"manager,omitempty"`
	Cmd     string `json:"cmd,omitempty"`
}

// PlanResult is the Data payload for `genv apply [--dry-run] --json`.
type PlanResult struct {
	ToInstall  []PlanPackage `json:"toInstall"`
	ToRemove   []PlanPackage `json:"toRemove"`
	Unchanged  []PlanPackage `json:"unchanged"`
	Unresolved int           `json:"unresolved"`
}

// StatusEntry is a single package entry in a StatusResult.
type StatusEntry struct {
	ID               string `json:"id"`
	Manager          string `json:"manager,omitempty"`
	Kind             string `json:"kind"` // "ok" | "drift" | "missing" | "extra"
	SpecVersion      string `json:"specVersion,omitempty"`
	InstalledVersion string `json:"installedVersion,omitempty"`
}

// StatusResult is the Data payload for `genv status --json`.
type StatusResult struct {
	Entries      []StatusEntry      `json:"entries"`
	EnvEntries   []EnvStatusEntry   `json:"envEntries,omitempty"`
	ShellEntries []ShellStatusEntry `json:"shellEntries,omitempty"`
}

// ScanResult is the Data payload for `genv scan --json`.
type ScanResult struct {
	Added   int `json:"added"`
	Skipped int `json:"skipped"`
}

// ApplyResult is the Data payload for `genv apply --json` (non-dry-run).
type ApplyResult struct {
	Installed    []string `json:"installed"`
	Uninstalled  []string `json:"uninstalled"`
	EnvApplied   []string `json:"envApplied,omitempty"`
	EnvRemoved   []string `json:"envRemoved,omitempty"`
	ShellApplied []string `json:"shellApplied,omitempty"`
	ShellRemoved []string `json:"shellRemoved,omitempty"`
}

// EnvStatusEntry is a single env variable entry in an EnvStatusResult.
type EnvStatusEntry struct {
	Name      string `json:"name"`
	Kind      string `json:"kind"` // "ok" | "modified" | "missing" | "extra"
	SpecValue string `json:"specValue,omitempty"`
	LockValue string `json:"lockValue,omitempty"`
	Sensitive bool   `json:"sensitive,omitempty"`
}

// EnvStatusResult is the Data payload for `genv env list --json` and the env
// section of `genv status --json`.
type EnvStatusResult struct {
	Entries []EnvStatusEntry `json:"entries"`
}

// ShellStatusEntry is a single shell config entry in a ShellStatusResult.
type ShellStatusEntry struct {
	Kind      string `json:"kind"`      // "ok" | "modified" | "missing" | "extra"
	EntryType string `json:"entryType"` // "alias" | "function" | "source"
	Name      string `json:"name"`
	SpecValue string `json:"specValue,omitempty"`
	LockValue string `json:"lockValue,omitempty"`
}

// ShellStatusResult is the Data payload for `genv shell status --json`.
type ShellStatusResult struct {
	Entries []ShellStatusEntry `json:"entries"`
}

// Write serializes env to w as a single JSON line followed by a newline.
func Write(w io.Writer, env Envelope) error {
	return json.NewEncoder(w).Encode(env)
}
