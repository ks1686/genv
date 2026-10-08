package compose

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/ks1686/genv/internal/genvfile"
	"github.com/ks1686/genv/internal/schema"
)

// Safety limits. They exist to keep a pathological repository from turning one
// `genv apply` into an unbounded read, and each one is documented in SCHEMA.md.
// They are variables so tests can tighten them.
var (
	// maxModuleBytes caps a single module document.
	maxModuleBytes = 1 << 20 // 1 MiB
	// maxRegisteredModules caps how many modules one root spec may register.
	maxRegisteredModules = 256
	// maxModuleDepth caps how many modules may appear in one dependency chain.
	maxModuleDepth = 64
	// maxTotalModuleBytes caps the bytes read across every module document in a
	// single composition. Without it, 256 modules at the per-file limit would let
	// one interactive command read a quarter of a gigabyte.
	maxTotalModuleBytes = 32 << 20 // 32 MiB
)

// budget tracks cumulative module bytes read during one composition.
type budget struct {
	read int
}

func (b *budget) charge(n int, relPath string) error {
	b.read += n
	if b.read > maxTotalModuleBytes {
		return fmt.Errorf("%w: module documents total more than %d bytes (at %s)", ErrLimit, maxTotalModuleBytes, relPath)
	}
	return nil
}

// Module is one loaded module document plus the paths it was read from.
type Module struct {
	Name    string
	RelPath string
	AbsPath string
	Doc     *schema.ModuleDoc
}

// LoadDocument reads and validates one module document registered at relPath
// under root.
//
// Path safety is enforced here rather than by validation alone: a module
// document is opened during composition, so every component must stay inside
// root and must not traverse a symlink. That keeps a cloned repository from
// redirecting genv to a file outside the tree it was pointed at.
func LoadDocument(root, relPath string) (*Module, error) {
	return loadDocumentBudget(root, relPath, &budget{})
}

func loadDocumentBudget(root, relPath string, charged *budget) (*Module, error) {
	if !schema.ValidateModulePath(relPath) {
		return nil, fmt.Errorf("%w: %q must be a repository-relative path without traversal, ~, or $VAR", ErrPathEscape, relPath)
	}
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("%w: resolving spec root %s: %v", ErrPathEscape, root, err)
	}

	rel := filepath.FromSlash(relPath)
	abs := filepath.Join(absRoot, rel)
	if !genvfile.WithinDir(absRoot, abs) {
		return nil, fmt.Errorf("%w: %q resolves outside %s", ErrPathEscape, relPath, absRoot)
	}
	if err := rejectSymlinkComponents(absRoot, abs); err != nil {
		return nil, err
	}

	info, err := os.Lstat(abs)
	if errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("%w: %s (%s)", ErrMissingModule, relPath, abs)
	}
	if err != nil {
		return nil, fmt.Errorf("reading module %s: %w", relPath, err)
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("%w: %q is not a regular file (mode %s)", ErrPathEscape, relPath, info.Mode())
	}
	if info.Size() > int64(maxModuleBytes) {
		return nil, fmt.Errorf("%w: %s is %d bytes (limit %d)", ErrLimit, relPath, info.Size(), maxModuleBytes)
	}

	// Read through a handle opened with O_NOFOLLOW so the file cannot be swapped
	// for a symlink between the checks above and the read.
	file, err := openNoFollow(abs)
	if err != nil {
		return nil, fmt.Errorf("opening module %s: %w", relPath, err)
	}
	defer func() { _ = file.Close() }()

	// Re-check the opened handle, not the path: this is the object we read.
	handleInfo, err := file.Stat()
	if err != nil {
		return nil, fmt.Errorf("inspecting module %s: %w", relPath, err)
	}
	if !handleInfo.Mode().IsRegular() {
		return nil, fmt.Errorf("%w: %q is not a regular file (mode %s)", ErrPathEscape, relPath, handleInfo.Mode())
	}
	if handleInfo.Size() > int64(maxModuleBytes) {
		return nil, fmt.Errorf("%w: %s is %d bytes (limit %d)", ErrLimit, relPath, handleInfo.Size(), maxModuleBytes)
	}
	data, err := io.ReadAll(io.LimitReader(file, int64(maxModuleBytes)+1))
	if err != nil {
		return nil, fmt.Errorf("reading module %s: %w", relPath, err)
	}
	if len(data) > maxModuleBytes {
		return nil, fmt.Errorf("%w: %s exceeds %d bytes", ErrLimit, relPath, maxModuleBytes)
	}
	if err := charged.charge(len(data), relPath); err != nil {
		return nil, err
	}

	doc, valErrs, parseErr := schema.ParseAndValidateModule(data)
	if parseErr != nil {
		return nil, fmt.Errorf("%w: %s: %v", ErrInvalidModule, relPath, parseErr)
	}
	if len(valErrs) > 0 {
		msgs := make([]string, len(valErrs))
		for i, e := range valErrs {
			msgs[i] = e.Error()
		}
		return nil, fmt.Errorf("%w: %s:\n  %s", ErrInvalidModule, relPath, strings.Join(msgs, "\n  "))
	}

	return &Module{RelPath: relPath, AbsPath: abs, Doc: doc}, nil
}

// LoadAllRegistered loads every module registered in a root spec. Every
// registered module is read (not only the selected ones) so `genv validate`
// reports a broken or missing module even when the active target does not
// select it.
//
// Results are keyed by module name and returned in sorted name order for
// deterministic diagnostics.
func LoadAllRegistered(root string, registered map[string]string) (map[string]*Module, error) {
	names := make([]string, 0, len(registered))
	for name := range registered {
		names = append(names, name)
	}
	sort.Strings(names)
	return loadClosure(root, registered, names, nil)
}

// LoadSelected loads only the modules reachable from selections through
// requiresModules edges. Reconciliation commands use this so a repository with
// many large modules does not pay for modules the active target does not use;
// `genv validate` uses LoadAllRegistered to still catch a broken module nobody
// selects.
func LoadSelected(root string, registered map[string]string, selections []string) (map[string]*Module, error) {
	if len(selections) == 0 {
		return map[string]*Module{}, nil
	}
	return loadClosure(root, registered, selections, registered)
}

// loadClosure reads the documents named by roots, following requiresModules.
// When membership is non-nil, every visited name must be registered, which is
// how a module's dependency on an unregistered name is caught during the read
// rather than later.
func loadClosure(root string, registered map[string]string, roots []string, membership map[string]string) (map[string]*Module, error) {
	if len(registered) > maxRegisteredModules {
		return nil, fmt.Errorf("%w: %d modules registered (limit %d)", ErrLimit, len(registered), maxRegisteredModules)
	}

	charged := &budget{}
	out := make(map[string]*Module)
	queue := append([]string(nil), roots...)
	for i := range roots {
		if !schema.ValidateModuleName(roots[i]) {
			return nil, fmt.Errorf("%w: invalid module name %q (expected kebab-case, at most 64 characters, and not a built-in manager)", ErrPathEscape, roots[i])
		}
		if membership != nil {
			if _, ok := membership[roots[i]]; !ok {
				return nil, fmt.Errorf("%w: module %q is selected but not registered", ErrMissingModule, roots[i])
			}
		}
	}

	// Deterministic order: breadth-first over sorted names, so a diagnostics
	// error names the same module on every run.
	for len(queue) > 0 {
		name := queue[0]
		queue = queue[1:]
		if _, done := out[name]; done {
			continue
		}
		rel, ok := registered[name]
		if !ok {
			return nil, fmt.Errorf("%w: module %q is not registered", ErrMissingModule, name)
		}
		mod, err := loadDocumentBudget(root, rel, charged)
		if err != nil {
			return nil, fmt.Errorf("module %q: %w", name, err)
		}
		mod.Name = name
		out[name] = mod

		deps := append([]string(nil), mod.Doc.RequiresModules...)
		sort.Strings(deps)
		for _, dep := range deps {
			if membership != nil {
				if _, ok := membership[dep]; !ok {
					return nil, fmt.Errorf("%w: module %q requires unregistered module %q", ErrMissingModule, name, dep)
				}
			}
			if _, done := out[dep]; !done {
				queue = append(queue, dep)
			}
		}
	}
	return out, nil
}

// rejectSymlinkComponents fails when any component between root and the module
// document is a symlink. root itself is trusted (the user chose the spec path);
// everything below it is repository content.
func rejectSymlinkComponents(root, abs string) error {
	rel, err := filepath.Rel(root, abs)
	if err != nil {
		return fmt.Errorf("%w: %s is not under the spec root", ErrPathEscape, abs)
	}
	current := root
	for _, seg := range strings.Split(rel, string(filepath.Separator)) {
		if seg == "" || seg == "." {
			continue
		}
		current = filepath.Join(current, seg)
		info, err := os.Lstat(current)
		if errors.Is(err, os.ErrNotExist) {
			// A missing path is reported as ErrMissingModule by the caller, which
			// has the friendlier message.
			return nil
		}
		if err != nil {
			return fmt.Errorf("inspecting %s: %w", current, err)
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("%w: %s is a symlink", ErrPathEscape, relPathForMessage(root, current))
		}
	}
	return nil
}

func relPathForMessage(root, abs string) string {
	rel, err := filepath.Rel(root, abs)
	if err != nil {
		return abs
	}
	return filepath.ToSlash(rel)
}
