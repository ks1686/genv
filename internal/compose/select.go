package compose

import (
	"fmt"

	"github.com/ks1686/genv/internal/schema"
)

// Select resolves the module selection closure for a target.
//
// extra selections (machine/role picks, which land in M3) are appended after the
// selections already declared by the spec's defaults and active target, and the
// result is deduplicated by first occurrence.
//
// Ordering rules, chosen so composition output is reproducible regardless of
// filesystem or map iteration order:
//   - a module's dependencies come before the module itself,
//   - selections keep their declared order,
//   - requiresModules keep their declared order,
//   - a cycle is an error naming the full path.
func Select(registered map[string]string, docs map[string]*Module, selections []string) ([]string, error) {
	ordered := make([]string, 0, len(selections))
	inOrder := map[string]bool{}
	for _, name := range selections {
		if inOrder[name] {
			continue
		}
		if _, ok := registered[name]; !ok {
			return nil, fmt.Errorf("%w: module %q is selected but not registered", ErrMissingModule, name)
		}
		inOrder[name] = true
		ordered = append(ordered, name)
	}

	state := make(map[string]int, len(ordered)) // 0 unvisited, 1 visiting, 2 done
	out := make([]string, 0, len(ordered))
	var visit func(name string, stack []string, depth int) error
	visit = func(name string, stack []string, depth int) error {
		switch state[name] {
		case 2:
			return nil
		case 1:
			return &CycleError{Path: append(stack, name)}
		}
		if depth >= maxModuleDepth {
			return fmt.Errorf("%w: module dependency chain longer than %d modules (from %q)", ErrLimit, maxModuleDepth, name)
		}
		mod, ok := docs[name]
		if !ok {
			return fmt.Errorf("%w: module %q has no loaded document", ErrMissingModule, name)
		}
		state[name] = 1
		for _, dep := range mod.Doc.RequiresModules {
			if _, ok := registered[dep]; !ok {
				return fmt.Errorf("%w: module %q requires unregistered module %q", ErrMissingModule, name, dep)
			}
			if err := visit(dep, append(stack, name), depth+1); err != nil {
				return err
			}
		}
		state[name] = 2
		out = append(out, name)
		return nil
	}

	for _, name := range ordered {
		if err := visit(name, nil, 0); err != nil {
			return nil, err
		}
	}
	return out, nil
}

// bundleSelections collects module selections contributed by a bundle,
// preserving declared order and dropping duplicates.
func bundleSelections(bundle *schema.TargetBundle, into []string) []string {
	if bundle == nil {
		return into
	}
	seen := map[string]bool{}
	for _, existing := range into {
		seen[existing] = true
	}
	for _, name := range bundle.UseModules {
		if seen[name] {
			continue
		}
		seen[name] = true
		into = append(into, name)
	}
	return into
}
