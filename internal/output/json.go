// Package output provides stable JSON envelope types for machine-readable
// gpm output. The Envelope type is the top-level wrapper emitted to stdout
// when a command is run with --json. Inner Data types are stable across
// patch releases; new optional fields may be added in minor releases.
package output

import (
	"encoding/json"
	"io"
)

// Envelope is the top-level JSON wrapper emitted for every --json response.
type Envelope struct {
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

// PlanResult is the Data payload for `gpm apply [--dry-run] --json`.
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

// StatusResult is the Data payload for `gpm status --json`.
type StatusResult struct {
	Entries []StatusEntry `json:"entries"`
}

// ScanResult is the Data payload for `gpm scan --json`.
type ScanResult struct {
	Added   int `json:"added"`
	Skipped int `json:"skipped"`
}

// ApplyResult is the Data payload for `gpm apply --json` (non-dry-run).
type ApplyResult struct {
	Installed   []string `json:"installed"`
	Uninstalled []string `json:"uninstalled"`
}

// Write serialises env to w as a single JSON line followed by a newline.
func Write(w io.Writer, env Envelope) error {
	return json.NewEncoder(w).Encode(env)
}
