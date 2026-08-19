package schema

import (
	"fmt"
	"path/filepath"
	"strings"
	"unicode"
)

// ValidRepoURL reports whether raw is a git remote genv will clone or fetch.
// Allowed: https://, ssh://, file://, git@scp-syntax, absolute paths, and ~/ paths.
func ValidRepoURL(raw string) error {
	if raw == "" {
		return fmt.Errorf("repository URL is empty")
	}
	if strings.HasPrefix(raw, "-") {
		return fmt.Errorf("repository URL must not start with '-'")
	}
	if strings.HasPrefix(strings.ToLower(raw), "ext::") {
		return fmt.Errorf("ext:: repository URLs are not allowed")
	}
	switch {
	case strings.HasPrefix(strings.ToLower(raw), "https://"),
		strings.HasPrefix(strings.ToLower(raw), "ssh://"),
		strings.HasPrefix(strings.ToLower(raw), "file://"),
		strings.HasPrefix(raw, "git@"),
		strings.HasPrefix(raw, "~/"),
		strings.HasPrefix(raw, `~\`),
		filepath.IsAbs(raw),
		isWindowsAbsPath(raw):
		return nil
	default:
		return fmt.Errorf("unsupported repository URL %q", raw)
	}
}

// ValidGitRef reports whether ref is safe to pass as a git checkout/merge operand.
func ValidGitRef(ref string) error {
	if ref == "" {
		return fmt.Errorf("git ref is empty")
	}
	if strings.HasPrefix(ref, "-") {
		return fmt.Errorf("git ref must not start with '-'")
	}
	for _, r := range ref {
		if r <= 0x1f || r == 0x7f || unicode.IsSpace(r) {
			return fmt.Errorf("git ref contains invalid characters")
		}
	}
	return nil
}

func isWindowsAbsPath(raw string) bool {
	if len(raw) < 3 {
		return false
	}
	drive := raw[0]
	if (drive < 'A' || drive > 'Z') && (drive < 'a' || drive > 'z') {
		return false
	}
	if raw[1] != ':' {
		return false
	}
	return raw[2] == '\\' || raw[2] == '/'
}
