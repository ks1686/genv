// Package compose turns a root spec plus its registered module documents into
// one effective environment: the flat desired-state spec that commands already
// know how to reconcile, together with the provenance of every resource.
//
// The package performs filesystem reads only. It never executes commands,
// contacts a network, or writes to the machine.
package compose

import (
	"errors"
	"fmt"
	"strings"
)

// Sentinel errors so callers (and tests) can classify composition failures
// without string matching.
var (
	// ErrPathEscape reports a module path or document that leaves the spec root,
	// reaches through a symlink, or is otherwise unsafe to open.
	ErrPathEscape = errors.New("module path escapes the spec root")
	// ErrMissingModule reports a module that is selected or registered but whose
	// document does not exist.
	ErrMissingModule = errors.New("module document not found")
	// ErrCycle reports a dependency cycle among requiresModules edges.
	ErrCycle = errors.New("module dependency cycle")
	// ErrConflict reports two contributors declaring the same resource
	// differently. genv never resolves this silently.
	ErrConflict = errors.New("conflicting declarations")
	// ErrInvalidModule reports a module document that failed schema validation.
	ErrInvalidModule = errors.New("invalid module document")
	// ErrLimit reports an input that exceeds a documented safety limit.
	ErrLimit = errors.New("input exceeds composition limit")
	// ErrServiceGraph reports a composed requires edge that no declared service
	// satisfies, or a cycle among services once every contributor is merged.
	ErrServiceGraph = errors.New("service dependency graph is not resolvable")
)

// ConflictError describes one identity declared differently by two or more
// contributors, naming every origin so the user knows which files to edit.
type ConflictError struct {
	Identity Identity
	Origins  []Origin
	Reason   string
}

func (e *ConflictError) Error() string {
	parts := make([]string, 0, len(e.Origins))
	for _, o := range e.Origins {
		parts = append(parts, o.String())
	}
	return fmt.Sprintf("%s declared differently by %s: %s", e.Identity, strings.Join(parts, ", "), e.Reason)
}

func (e *ConflictError) Unwrap() error { return ErrConflict }

// CycleError reports the module dependency path that closes a cycle.
type CycleError struct {
	Path []string
}

func (e *CycleError) Error() string {
	// Select already closes the path (a -> b -> a). Appending the first node
	// again rendered a -> b -> a -> a.
	if e == nil || len(e.Path) == 0 {
		return ErrCycle.Error()
	}
	return fmt.Sprintf("%s: %s", ErrCycle.Error(), strings.Join(e.Path, " -> "))
}

func (e *CycleError) Unwrap() error { return ErrCycle }

func assertIsError(t interface {
	Helper()
	Errorf(string, ...any)
}, err, target error) {
	t.Helper()
	if !errors.Is(err, target) {
		t.Errorf("error = %v, want errors.Is(_, %v)", err, target)
	}
}
