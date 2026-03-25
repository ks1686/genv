// Package schema defines the genv.json v1/v2 data model and validation logic.
package schema

// Version is the accepted value for genv.json v1 (packages only).
const Version = "1"

// Version2 is the accepted value for genv.json v2 (packages + env block).
const Version2 = "2"

// Version3 is the accepted value for genv.json v3 (packages + env + shell block).
const Version3 = "3"

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
	"apt":       true,
	"dnf":       true,
	"pacman":    true,
	"paru":      true,
	"yay":       true,
	"flatpak":   true,
	"snap":      true,
	"brew":      true,
	"macports":  true,
	"linuxbrew": true,
}

// GenvFile is the top-level structure of a genv.json file.
// v1: schemaVersion "1", packages only.
// v2: schemaVersion "2", packages + optional env block.
// v3: schemaVersion "3", packages + optional env + optional shell block.
type GenvFile struct {
	SchemaVersion string            `json:"schemaVersion"`
	Packages      []Package         `json:"packages"`
	Env           map[string]EnvVar `json:"env,omitempty"`
	Shell         *ShellConfig      `json:"shell,omitempty"`
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
}
