// Package schema defines the genv.json v1-v5 data model and validation logic.
package schema

import (
	"encoding/json"
	"fmt"
)

// Version is the accepted value for genv.json v1 (packages only).
const Version = "1"

// Version2 is the accepted value for genv.json v2 (packages + env block).
const Version2 = "2"

// Version3 is the accepted value for genv.json v3 (packages + env + shell block).
const Version3 = "3"

// Version4 is the accepted value for genv.json v4 (packages + env + shell + services block).
const Version4 = "4"

// Version5 is the accepted value for genv.json v5 (adds files, hooks, host, and repo).
const Version5 = "5"

// KnownShellTargets is the set of valid per-shell targeting values for alias
// and function entries. An empty string means "all supported shells".
var KnownShellTargets = map[string]bool{
	"bash": true,
	"zsh":  true,
	"fish": true,
}

// ValidShellTargetsMsg is the user-facing string describing valid shell target values.
const ValidShellTargetsMsg = `"bash", "zsh", "fish", or omit for all`

// KnownManagers is the set of package-manager IDs recognized in schema v1.
var KnownManagers = map[string]bool{
	"paru":      true,
	"yay":       true,
	"snap":      true,
	"brew":      true,
	"linuxbrew": true,
	"mas":       true,
	"pacman":    true,
	"bun":       true,
	"uv":        true,
	"winget":    true,
	"scoop":     true,
	"choco":     true,
}

// HostPredicate selects which host(s) a record applies to. It unmarshals from
// either a single string ("macos") or a JSON array (["arch","wsl2"]). An empty
// predicate matches every host.
type HostPredicate []string

// UnmarshalJSON accepts a string or a string array for the host field.
func (h *HostPredicate) UnmarshalJSON(data []byte) error {
	if len(data) == 0 {
		*h = nil
		return nil
	}
	if data[0] == '"' {
		var s string
		if err := json.Unmarshal(data, &s); err != nil {
			return fmt.Errorf("host must be a string or array of strings: %w", err)
		}
		*h = HostPredicate{s}
		return nil
	}
	var arr []string
	if err := json.Unmarshal(data, &arr); err != nil {
		return fmt.Errorf("host must be a string or array of strings: %w", err)
	}
	*h = HostPredicate(arr)
	return nil
}

// MarshalJSON always emits host as a JSON array.
func (h HostPredicate) MarshalJSON() ([]byte, error) {
	return json.Marshal([]string(h))
}

// GenvFile is the top-level structure of a genv.json file.
// v1: schemaVersion "1", packages only.
// v2: schemaVersion "2", packages + optional env block.
// v3: schemaVersion "3", packages + optional env + optional shell block.
// v4: schemaVersion "4", packages + optional env + optional shell + optional services block.
// v5: schemaVersion "5", adds optional files, hooks, host selectors, and repo fields.
type GenvFile struct {
	SchemaVersion string             `json:"schemaVersion"`
	Packages      []Package          `json:"packages"`
	Env           map[string]EnvVar  `json:"env,omitempty"`
	Shell         *ShellConfig       `json:"shell,omitempty"`
	Services      map[string]Service `json:"services,omitempty"`
	Files         *FilesConfig       `json:"files,omitempty"`
	Hooks         *HooksConfig       `json:"hooks,omitempty"`
	Repo          *Repo              `json:"repo,omitempty"`
}

// Repo points to the spec repository used by `genv pull`.
type Repo struct {
	URL string `json:"url"`
	Ref string `json:"ref,omitempty"`
}

// FilesConfig declares filesystem entries that genv should reconcile.
type FilesConfig struct {
	Links     []FileLink     `json:"links,omitempty"`
	Templates []FileTemplate `json:"templates,omitempty"`
	Dirs      []FileDir      `json:"dirs,omitempty"`
}

// FileLink declares a symbolic link from Source to Target.
// Mode is "link" (default) or "managed-link".
type FileLink struct {
	Source string        `json:"source"`
	Target string        `json:"target"`
	Mode   string        `json:"mode,omitempty"`
	Host   HostPredicate `json:"host,omitempty"`
	Backup bool          `json:"backup,omitempty"`
}

// FileTemplate declares a file that should be copied from Source to Target
// after running the v5 placeholder renderer.
type FileTemplate struct {
	Source string        `json:"source"`
	Target string        `json:"target"`
	Host   HostPredicate `json:"host,omitempty"`
	Backup bool          `json:"backup,omitempty"`
}

// FileDir declares a directory that should exist.
type FileDir struct {
	Target string        `json:"target"`
	Host   HostPredicate `json:"host,omitempty"`
}

// HooksConfig declares lifecycle shell commands.
type HooksConfig struct {
	PreUpgrade  []Hook `json:"preUpgrade,omitempty"`
	PostApply   []Hook `json:"postApply,omitempty"`
	PostUpgrade []Hook `json:"postUpgrade,omitempty"`
}

// Hook is a single lifecycle shell command.
type Hook struct {
	Command string        `json:"command"`
	Host    HostPredicate `json:"host,omitempty"`
}

// Service is a single user-space service declaration.
// Either Start or BrewFormula must be provided.
// When BrewFormula is set, genv manages the service via `brew services` on macOS.
type Service struct {
	Start       []string      `json:"start,omitempty"`
	Stop        []string      `json:"stop,omitempty"`
	Restart     []string      `json:"restart,omitempty"`
	Status      []string      `json:"status,omitempty"`
	BrewFormula string        `json:"brew_formula,omitempty"`
	Host        HostPredicate `json:"host,omitempty"`
}

// ShellConfig is the shell configuration block in genv.json.
type ShellConfig struct {
	Aliases   map[string]ShellAlias    `json:"aliases,omitempty"`
	Functions map[string]ShellFunction `json:"functions,omitempty"`
	Source    []string                 `json:"source,omitempty"`
}

// ShellAlias is a single shell alias declaration.
// Shell may be "bash", "zsh", "fish", or empty (applied to all supported shells).
type ShellAlias struct {
	Value string `json:"value"`
	Shell string `json:"shell,omitempty"`
}

// ShellFunction is a single shell function declaration.
// Shell may be "bash", "zsh", "fish", or empty (applied to all supported shells).
type ShellFunction struct {
	Body  string `json:"body"`
	Shell string `json:"shell,omitempty"`
}

// EnvVar is a declared environment variable in the genv.json env block.
type EnvVar struct {
	Value     string `json:"value"`
	Sensitive bool   `json:"sensitive,omitempty"`
}

// Package is a single entry in the packages array.
type Package struct {
	ID       string            `json:"id"`
	Version  string            `json:"version,omitempty"`
	Prefer   string            `json:"prefer,omitempty"`
	Managers map[string]string `json:"managers,omitempty"`
	Host     HostPredicate     `json:"host,omitempty"`
}
