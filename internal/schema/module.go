package schema

import (
	"encoding/json"
	"errors"
	"fmt"
	"path"
	"strings"
)

// ModuleDoc is a v10 module document: one named, reusable slice of a portable
// environment. A module carries defaults and/or per-target bundles and nothing
// else — module selection, the registry, repo, updates, and spec adapters all
// stay owned by the root spec so composition stays predictable.
type ModuleDoc struct {
	SchemaVersion   string                   `json:"schemaVersion"`
	RequiresModules []string                 `json:"requiresModules,omitempty"`
	Defaults        *TargetBundle            `json:"defaults,omitempty"`
	Targets         map[string]*TargetBundle `json:"targets,omitempty"`
}

// moduleFields is the closed set of keys a module document may carry.
func moduleFields() map[string]bool {
	return strSet("schemaVersion", "requiresModules", "defaults", "targets")
}

// ValidateModuleName reports whether name is an acceptable module id:
// kebab-case, bounded length, and not a built-in manager id (a module that
// shares a manager name would make `prefer` ambiguous).
func ValidateModuleName(name string) bool {
	if name == "" || len(name) > 64 {
		return false
	}
	if KnownManagers[name] {
		return false
	}
	for i := 0; i < len(name); i++ {
		c := name[i]
		switch {
		case c >= 'a' && c <= 'z', c >= '0' && c <= '9':
		case c == '-' && i > 0 && i < len(name)-1:
		default:
			return false
		}
	}
	return true
}

// ValidateModulePath reports whether rel is a safe repository-relative module
// document path. Module documents are read during composition, so the syntax
// check rejects anything that could escape the spec root or depend on
// environment expansion: absolute paths, traversal segments, ~, and $VAR.
func ValidateModulePath(rel string) bool {
	if rel == "" || strings.HasPrefix(rel, "/") || strings.HasPrefix(rel, "~") || strings.ContainsRune(rel, '$') {
		return false
	}
	if strings.ContainsRune(rel, '\\') || strings.HasSuffix(rel, "/") {
		return false
	}
	cleaned := path.Clean(rel)
	if cleaned != rel || cleaned == "." || cleaned == ".." || strings.HasPrefix(cleaned, "../") {
		return false
	}
	return true
}

// ParseAndValidateModule parses data as a v10 module document. It follows the
// same contract as ParseAndValidate: a non-nil error is a fatal parse failure,
// while semantic problems come back as ValidationError values with positions.
func ParseAndValidateModule(data []byte) (*ModuleDoc, []ValidationError, error) {
	doc, raw, valErrs, parseErr := unmarshalModule(data)
	if valErrs != nil || parseErr != nil {
		return doc, valErrs, parseErr
	}

	positions := make(map[string]Position)
	locateFields(data, positions)

	var errs []ValidationError
	errs = append(errs, rejectUnknown(raw, "", moduleFields(), positions)...)

	if _, ok := raw["schemaVersion"]; !ok {
		errs = append(errs, ValidationError{
			Position: positions["schemaVersion"],
			Field:    "schemaVersion",
			Message:  "module documents require a schemaVersion field",
		})
	} else if doc.SchemaVersion != Version10 {
		errs = append(errs, ValidationError{
			Position: positions["schemaVersion"],
			Field:    "schemaVersion",
			Message:  fmt.Sprintf("module documents require schemaVersion %q (current: %q)", Version10, doc.SchemaVersion),
		})
	}

	seenRequire := map[string]bool{}
	for i, name := range doc.RequiresModules {
		field := fmt.Sprintf("requiresModules[%d]", i)
		if !ValidateModuleName(name) {
			errs = append(errs, ValidationError{
				Position: positions[field],
				Field:    field,
				Message:  fmt.Sprintf("invalid module name %q; expected kebab-case, at most 64 characters, and not a built-in manager", name),
			})
			continue
		}
		if seenRequire[name] {
			errs = append(errs, ValidationError{
				Position: positions[field],
				Field:    field,
				Message:  fmt.Sprintf("duplicate module requirement %q", name),
			})
			continue
		}
		seenRequire[name] = true
	}

	if doc.Defaults == nil && len(doc.Targets) == 0 {
		errs = append(errs, ValidationError{
			Position: Position{Line: 1, Column: 1},
			Field:    "targets",
			Message:  "module must declare at least one of defaults or targets",
		})
	}
	if doc.Defaults != nil {
		errs = append(errs, validateTargetBundle(&GenvFile{SchemaVersion: Version10}, doc.Defaults, "defaults", false, positions)...)
		errs = append(errs, rejectUseModules(doc.Defaults, "defaults", positions)...)
	}
	for target, bundle := range doc.Targets {
		targetPath := "targets." + target
		if !KnownTargets[target] {
			errs = append(errs, ValidationError{
				Position: positions[targetPath],
				Field:    targetPath,
				Message:  fmt.Sprintf("unknown target %q", target),
			})
		}
		if bundle == nil {
			errs = append(errs, ValidationError{
				Position: positions[targetPath],
				Field:    targetPath,
				Message:  "target must be an object",
			})
			continue
		}
		errs = append(errs, validateTargetBundle(&GenvFile{SchemaVersion: Version10}, bundle, targetPath, true, positions)...)
		errs = append(errs, rejectUseModules(bundle, targetPath, positions)...)
	}

	return doc, errs, nil
}

// rejectUseModules keeps selection out of module documents: dependency edges
// belong in requiresModules so a module never pulls in an unrelated selection.
func rejectUseModules(bundle *TargetBundle, prefix string, positions map[string]Position) []ValidationError {
	if len(bundle.UseModules) == 0 {
		return nil
	}
	field := prefix + ".useModules"
	return []ValidationError{{
		Position: positions[field],
		Field:    field,
		Message:  "useModules is not allowed inside a module; declare depends-on modules with requiresModules",
	}}
}

// unmarshalModule decodes a module document, splitting a type mismatch (a
// validation problem) from a syntax failure (a fatal error) exactly the way the
// root spec parser does.
func unmarshalModule(data []byte) (*ModuleDoc, map[string]json.RawMessage, []ValidationError, error) {
	var doc ModuleDoc
	if err := json.Unmarshal(data, &doc); err != nil {
		var syntaxErr *json.SyntaxError
		if errors.As(err, &syntaxErr) {
			pos := offsetToPosition(data, syntaxErr.Offset)
			return nil, nil, nil, fmt.Errorf("line %d:%d: JSON syntax error: %s", pos.Line, pos.Column, syntaxErr.Error())
		}
		var typeErr *json.UnmarshalTypeError
		if errors.As(err, &typeErr) {
			return nil, nil, []ValidationError{{
				Position: offsetToPosition(data, typeErr.Offset),
				Field:    typeErr.Field,
				Message:  fmt.Sprintf("expected %s, got %s", typeErr.Type, typeErr.Value),
			}}, nil
		}
		return nil, nil, nil, err
	}

	// A raw map distinguishes an absent key from a zero value. The JSON already
	// parsed into &doc, so this second decode cannot fail.
	var raw map[string]json.RawMessage
	_ = json.Unmarshal(data, &raw)
	return &doc, raw, nil, nil
}
