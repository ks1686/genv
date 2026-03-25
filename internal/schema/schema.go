// Package schema defines the genv.json v1/v2 data model and validation logic.
package schema

// Version is the accepted value for genv.json v1 (packages only).
const Version = "1"

// Version2 is the accepted value for genv.json v2 (packages + env block).
const Version2 = "2"

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
type GenvFile struct {
	SchemaVersion string              `json:"schemaVersion"`
	Packages      []Package           `json:"packages"`
	Env           map[string]EnvVar   `json:"env,omitempty"`
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
