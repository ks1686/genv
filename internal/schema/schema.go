// Package schema defines the gpm.json v1 data model and validation logic.
package schema

// SchemaVersion is the only accepted value for the schemaVersion field.
const SchemaVersion = "1"

// KnownManagers is the set of package-manager IDs recognised in schema v1.
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

// GpmFile is the top-level structure of a gpm.json file.
type GpmFile struct {
	SchemaVersion string    `json:"schemaVersion"`
	Packages      []Package `json:"packages"`
}

// Package is a single entry in the packages array.
type Package struct {
	ID       string            `json:"id"`
	Version  string            `json:"version,omitempty"`
	Prefer   string            `json:"prefer,omitempty"`
	Managers map[string]string `json:"managers,omitempty"`
}
