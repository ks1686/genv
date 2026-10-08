package compose

import (
	"fmt"
	"path/filepath"
	"strings"
)

// Resource kinds used for identity, conflict detection, and explanations. A
// resource is addressed by a structured (Kind, Key) pair; the "kind:key" string
// form exists only for display and `genv explain` arguments, and is never
// parsed back into a pair.
const (
	KindPackage = "package"
	KindService = "service"
	KindEnv     = "env"
	KindAlias   = "alias"
	KindFunc    = "function"
	KindFile    = "file"
	KindDir     = "dir"
	KindHook    = "hook"
)

// Identity is one composed resource. Two contributors that declare the same
// identity must agree byte-for-byte on the declaration, otherwise composition
// fails with a conflict rather than silently preferring one of them.
type Identity struct {
	Kind string
	Key  string
}

func (i Identity) String() string {
	if i.Key == "" {
		return i.Kind
	}
	return i.Kind + ":" + i.Key
}

// IdentityFromDisplay parses a "kind:key" selector as accepted by
// `genv explain`. Everything after the first colon is the key, so a file
// destination such as "file:/home/me/.zshrc" keeps its path intact.
func IdentityFromDisplay(s string) (Identity, bool) {
	kind, key, found := strings.Cut(s, ":")
	if !found {
		return Identity{}, false
	}
	if kind == "" || key == "" {
		return Identity{}, false
	}
	return Identity{Kind: kind, Key: key}, true
}

func packageIdentity(id string) Identity   { return Identity{Kind: KindPackage, Key: id} }
func serviceIdentity(name string) Identity { return Identity{Kind: KindService, Key: name} }
func envIdentity(name string) Identity     { return Identity{Kind: KindEnv, Key: name} }
func aliasIdentity(name string) Identity   { return Identity{Kind: KindAlias, Key: name} }
func funcIdentity(name string) Identity    { return Identity{Kind: KindFunc, Key: name} }
func dirIdentity(target string) Identity   { return Identity{Kind: KindDir, Key: target} }

// fileIdentity normalizes a destination so two contributors that name the same
// file the same way (relative vs absolute, /./ segments, repeated separators)
// share one identity instead of producing a false conflict.
func fileIdentity(target string) Identity {
	return Identity{Kind: KindFile, Key: CleanPath(target)}
}

// hookIdentity is positional: two hooks with the same command text in different
// phases, or two anonymous hooks in one phase, are distinct resources. The key
// therefore carries the owning document and the index within its phase.
func hookIdentity(document, phase string, index int) Identity {
	return Identity{Kind: KindHook, Key: fmt.Sprintf("%s#%s[%d]", document, phase, index)}
}

// CleanPath normalizes a path for identity comparison without resolving
// symlinks or touching the filesystem. It keeps the host's own separator
// semantics so a Windows-style destination is not rewritten into POSIX form
// merely because composition happened to run on a Unix planning host.
func CleanPath(p string) string {
	if p == "" {
		return ""
	}
	winStyle := strings.ContainsRune(p, '\\')
	slashed := p
	if winStyle {
		slashed = strings.ReplaceAll(slashed, "\\", "/")
	}
	cleaned := filepath.ToSlash(filepath.Clean(filepath.FromSlash(slashed)))
	if winStyle {
		// filepath.Clean on a Unix host leaves a drive-relative "C:/x" alone;
		// normalize separators again so the key is host-independent.
		cleaned = strings.ReplaceAll(cleaned, "/", "\\")
	}
	return cleaned
}

// KnownKinds lists every resource kind composition tracks, in the order they
// are reported. Commands that accept a kind from the user validate against this
// so a typo produces a list of valid values instead of an empty result.
func KnownKinds() []string {
	return []string{KindPackage, KindService, KindEnv, KindAlias, KindFunc, KindFile, KindDir, KindHook}
}

// IsKnownKind reports whether kind is one composition tracks.
func IsKnownKind(kind string) bool {
	for _, k := range KnownKinds() {
		if k == kind {
			return true
		}
	}
	return false
}
