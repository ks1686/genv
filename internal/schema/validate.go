package schema

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
)

// Position is a 1-based line and column in a source file.
type Position struct {
	Line   int
	Column int
}

// ValidationError is a single schema violation with optional source location.
type ValidationError struct {
	Position
	Field   string
	Message string
}

func (e ValidationError) Error() string {
	if e.Line > 0 {
		return fmt.Sprintf("line %d:%d: %s: %s", e.Line, e.Column, e.Field, e.Message)
	}
	return fmt.Sprintf("%s: %s", e.Field, e.Message)
}

func unmarshalGenvFile(data []byte) (*GenvFile, map[string]json.RawMessage, []ValidationError, error) {
	var f GenvFile
	if err := json.Unmarshal(data, &f); err != nil {
		var syntaxErr *json.SyntaxError
		var typeErr *json.UnmarshalTypeError
		switch {
		case errors.As(err, &syntaxErr):
			pos := offsetToPosition(data, syntaxErr.Offset)
			return nil, nil, nil, fmt.Errorf("line %d:%d: JSON syntax error: %s", pos.Line, pos.Column, syntaxErr.Error())
		case errors.As(err, &typeErr):
			pos := offsetToPosition(data, typeErr.Offset)
			return nil, nil, []ValidationError{{
				Position: pos,
				Field:    typeErr.Field,
				Message:  fmt.Sprintf("expected %s, got %s", typeErr.Type, typeErr.Value),
			}}, nil
		default:
			return nil, nil, nil, err
		}
	}

	// Use a raw map to distinguish "key absent" from "key set to zero value".
	// Error is intentionally ignored: the JSON was already successfully parsed
	// above into &f, so this second unmarshal into a plain map cannot fail.
	var raw map[string]json.RawMessage
	_ = json.Unmarshal(data, &raw)

	return &f, raw, nil, nil
}

// ParseAndValidate parses data as a genv.json file and validates it against
// schema v1 rules.
//
// A non-nil error indicates a fatal parse failure (e.g. malformed JSON).
// Semantic validation problems are returned as a []ValidationError slice
// alongside a best-effort *GenvFile.  Both can be non-nil at the same time.
func ParseAndValidate(data []byte) (*GenvFile, []ValidationError, error) {
	// Build a (path → position) index from the raw token stream.
	// Errors here are non-fatal; the index may be partial on malformed input.
	positions := make(map[string]Position)
	locateFields(data, positions)

	f, raw, valErrs, err := unmarshalGenvFile(data)
	if valErrs != nil || err != nil {
		return f, valErrs, err
	}

	var errs []ValidationError

	errs = append(errs, validateSchemaVersion(f, raw, positions)...)
	errs = append(errs, validatePackages(f, raw, positions)...)
	errs = append(errs, validateEnv(f, raw, positions)...)
	errs = append(errs, validateShell(f, raw, positions)...)
	errs = append(errs, validateServices(f, raw, positions)...)

	return f, errs, nil
}

// ValidEnvName reports whether name is a valid POSIX shell environment variable
// name: starts with a letter or underscore, followed by letters, digits, or underscores.
func ValidEnvName(name string) bool {
	if len(name) == 0 {
		return false
	}
	for i, r := range name {
		switch {
		case r >= 'A' && r <= 'Z', r >= 'a' && r <= 'z', r == '_':
			// always valid
		case r >= '0' && r <= '9':
			if i == 0 {
				return false // no leading digit
			}
		default:
			return false
		}
	}
	return true
}

// offsetToPosition converts a byte offset (as returned by json.Decoder.InputOffset)
// into a 1-based line and column.  The offset is treated as the position of the
// character AFTER the token, which is always on the same line as the token for
// single-line tokens.
func offsetToPosition(data []byte, offset int64) Position {
	if offset <= 0 {
		return Position{Line: 1, Column: 1}
	}
	limit := offset
	if limit > int64(len(data)) {
		limit = int64(len(data))
	}
	line, col := 1, 1
	for i := int64(0); i < limit; i++ {
		if data[i] == '\n' {
			line++
			col = 1
		} else {
			col++
		}
	}
	return Position{Line: line, Column: col}
}

// locateFields walks the JSON token stream and populates pos with the position
// of each field's value.  Paths use dot-notation with bracket indices:
//
//	"schemaVersion"             top-level scalar
//	"packages[0]"               first array element (an object)
//	"packages[0].id"            field inside first element
//	"packages[0].managers.apt"  nested map entry
//
// Positions are the end-of-token offsets returned by json.Decoder.InputOffset,
// which are always on the same line as the token for typical JSON values.
func locateFields(data []byte, pos map[string]Position) {
	dec := json.NewDecoder(bytes.NewReader(data))
	tok, err := dec.Token()
	if err != nil {
		return
	}
	if d, ok := tok.(json.Delim); !ok || d != '{' {
		return
	}
	walkObjectBody(dec, data, "", pos)
}

func walkValue(dec *json.Decoder, data []byte, path string, pos map[string]Position) {
	tok, err := dec.Token()
	if err != nil {
		return
	}
	offset := dec.InputOffset()
	switch v := tok.(type) {
	case json.Delim:
		switch v {
		case '{':
			pos[path] = offsetToPosition(data, offset)
			walkObjectBody(dec, data, path, pos)
		case '[':
			pos[path] = offsetToPosition(data, offset)
			walkArrayBody(dec, data, path, pos)
		}
	default:
		pos[path] = offsetToPosition(data, offset)
	}
}

func walkObjectBody(dec *json.Decoder, data []byte, path string, pos map[string]Position) {
	for dec.More() {
		keyTok, err := dec.Token()
		if err != nil {
			return
		}
		key, ok := keyTok.(string)
		if !ok {
			return
		}
		childPath := key
		if path != "" {
			childPath = path + "." + key
		}
		walkValue(dec, data, childPath, pos)
	}
	_, _ = dec.Token() // consume closing }
}

func walkArrayBody(dec *json.Decoder, data []byte, path string, pos map[string]Position) {
	for i := 0; dec.More(); i++ {
		childPath := fmt.Sprintf("%s[%d]", path, i)
		walkValue(dec, data, childPath, pos)
	}
	_, _ = dec.Token() // consume closing ]
}

func validateSchemaVersion(f *GenvFile, raw map[string]json.RawMessage, positions map[string]Position) []ValidationError {
	var errs []ValidationError
	if _, ok := raw["schemaVersion"]; !ok {
		errs = append(errs, ValidationError{
			Field:   "schemaVersion",
			Message: "required field is missing",
		})
	} else if f.SchemaVersion != Version && f.SchemaVersion != Version2 && f.SchemaVersion != Version3 && f.SchemaVersion != Version4 {
		errs = append(errs, ValidationError{
			Position: positions["schemaVersion"],
			Field:    "schemaVersion",
			Message:  fmt.Sprintf("unsupported version %q; expected %q, %q, %q, or %q", f.SchemaVersion, Version, Version2, Version3, Version4),
		})
	}
	return errs
}

func validatePackages(f *GenvFile, raw map[string]json.RawMessage, positions map[string]Position) []ValidationError {
	var errs []ValidationError
	if _, ok := raw["packages"]; !ok {
		errs = append(errs, ValidationError{
			Field:   "packages",
			Message: "required field is missing",
		})
	} else {
		seen := make(map[string]int) // id → first index
		for i, pkg := range f.Packages {
			pkgPath := fmt.Sprintf("packages[%d]", i)

			if pkg.ID == "" {
				errs = append(errs, ValidationError{
					Position: positions[pkgPath],
					Field:    pkgPath + ".id",
					Message:  "required field is missing or empty",
				})
			} else if prev, dup := seen[pkg.ID]; dup {
				errs = append(errs, ValidationError{
					Position: positions[pkgPath+".id"],
					Field:    pkgPath + ".id",
					Message:  fmt.Sprintf("duplicate id %q (first seen at packages[%d])", pkg.ID, prev),
				})
			} else {
				seen[pkg.ID] = i
			}

			if pkg.Prefer != "" && !KnownManagers[pkg.Prefer] {
				errs = append(errs, ValidationError{
					Position: positions[pkgPath+".prefer"],
					Field:    pkgPath + ".prefer",
					Message:  fmt.Sprintf("unknown manager %q", pkg.Prefer),
				})
			}

			for mgr := range pkg.Managers {
				if !KnownManagers[mgr] {
					field := fmt.Sprintf("%s.managers.%s", pkgPath, mgr)
					errs = append(errs, ValidationError{
						Position: positions[field],
						Field:    field,
						Message:  fmt.Sprintf("unknown manager %q", mgr),
					})
				}
			}
		}
	}
	return errs
}

func validateEnv(f *GenvFile, raw map[string]json.RawMessage, positions map[string]Position) []ValidationError {
	var errs []ValidationError
	if _, hasEnv := raw["env"]; hasEnv {
		if f.SchemaVersion != Version2 && f.SchemaVersion != Version3 && f.SchemaVersion != Version4 {
			errs = append(errs, ValidationError{
				Position: positions["env"],
				Field:    "env",
				Message:  fmt.Sprintf("env block requires schemaVersion %q, %q, or %q (current: %q); run 'genv env set' to upgrade", Version2, Version3, Version4, f.SchemaVersion),
			})
		}
		for name, ev := range f.Env {
			if !ValidEnvName(name) {
				errs = append(errs, ValidationError{
					Field:   "env." + name,
					Message: fmt.Sprintf("invalid variable name %q: must match [A-Za-z_][A-Za-z0-9_]*", name),
				})
			}
			_ = ev
		}
	}
	return errs
}

func validateShell(f *GenvFile, raw map[string]json.RawMessage, positions map[string]Position) []ValidationError {
	var errs []ValidationError
	if _, hasShell := raw["shell"]; hasShell {
		if f.SchemaVersion != Version3 && f.SchemaVersion != Version4 {
			errs = append(errs, ValidationError{
				Position: positions["shell"],
				Field:    "shell",
				Message:  fmt.Sprintf("shell block requires schemaVersion %q or %q (current: %q); run 'genv shell alias set' to upgrade", Version3, Version4, f.SchemaVersion),
			})
		}
		if f.Shell != nil {
			for name := range f.Shell.Aliases {
				if name == "" {
					errs = append(errs, ValidationError{
						Field:   "shell.aliases",
						Message: "alias name must not be empty",
					})
				}
				if sh := f.Shell.Aliases[name].Shell; sh != "" && !KnownShellTargets[sh] {
					errs = append(errs, ValidationError{
						Field:   fmt.Sprintf("shell.aliases.%s.shell", name),
						Message: fmt.Sprintf("unknown shell %q; expected %s", sh, ValidShellTargetsMsg),
					})
				}
			}
			for name := range f.Shell.Functions {
				if name == "" {
					errs = append(errs, ValidationError{
						Field:   "shell.functions",
						Message: "function name must not be empty",
					})
				}
				if sh := f.Shell.Functions[name].Shell; sh != "" && !KnownShellTargets[sh] {
					errs = append(errs, ValidationError{
						Field:   fmt.Sprintf("shell.functions.%s.shell", name),
						Message: fmt.Sprintf("unknown shell %q; expected %s", sh, ValidShellTargetsMsg),
					})
				}
			}
		}
	}
	return errs
}

func validateServices(f *GenvFile, raw map[string]json.RawMessage, positions map[string]Position) []ValidationError {
	var errs []ValidationError
	if _, hasServices := raw["services"]; hasServices {
		if f.SchemaVersion != Version4 {
			errs = append(errs, ValidationError{
				Position: positions["services"],
				Field:    "services",
				Message:  fmt.Sprintf("services block requires schemaVersion %q (current: %q); run 'genv service add' to upgrade", Version4, f.SchemaVersion),
			})
		}
		if f.Services != nil {
			for name, svc := range f.Services {
				if name == "" {
					errs = append(errs, ValidationError{
						Field:   "services",
						Message: "service name must not be empty",
					})
				}
				if len(svc.Start) == 0 {
					errs = append(errs, ValidationError{
						Field:   fmt.Sprintf("services.%s.start", name),
						Message: "start command is required and must not be empty",
					})
				}
			}
		}
	}
	return errs
}
