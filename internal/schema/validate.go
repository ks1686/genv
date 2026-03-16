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

// ParseAndValidate parses data as a gpm.json file and validates it against
// schema v1 rules.
//
// A non-nil error indicates a fatal parse failure (e.g. malformed JSON).
// Semantic validation problems are returned as a []ValidationError slice
// alongside a best-effort *GpmFile.  Both can be non-nil at the same time.
func ParseAndValidate(data []byte) (*GpmFile, []ValidationError, error) {
	// Build a (path → position) index from the raw token stream.
	// Errors here are non-fatal; the index may be partial on malformed input.
	positions := make(map[string]Position)
	locateFields(data, positions)

	// Unmarshal into the typed struct, turning JSON errors into user messages.
	var f GpmFile
	if err := json.Unmarshal(data, &f); err != nil {
		var syntaxErr *json.SyntaxError
		var typeErr *json.UnmarshalTypeError
		switch {
		case errors.As(err, &syntaxErr):
			pos := offsetToPosition(data, syntaxErr.Offset)
			return nil, nil, fmt.Errorf("line %d:%d: JSON syntax error: %s", pos.Line, pos.Column, syntaxErr.Error())
		case errors.As(err, &typeErr):
			pos := offsetToPosition(data, typeErr.Offset)
			return nil, []ValidationError{{
				Position: pos,
				Field:    typeErr.Field,
				Message:  fmt.Sprintf("expected %s, got %s", typeErr.Type, typeErr.Value),
			}}, nil
		default:
			return nil, nil, err
		}
	}

	// Use a raw map to distinguish "key absent" from "key set to zero value".
	// Error is intentionally ignored: the JSON was already successfully parsed
	// above into &f, so this second unmarshal into a plain map cannot fail.
	var raw map[string]json.RawMessage
	_ = json.Unmarshal(data, &raw)

	var errs []ValidationError

	// --- schemaVersion ---
	if _, ok := raw["schemaVersion"]; !ok {
		errs = append(errs, ValidationError{
			Field:   "schemaVersion",
			Message: "required field is missing",
		})
	} else if f.SchemaVersion != SchemaVersion {
		errs = append(errs, ValidationError{
			Position: positions["schemaVersion"],
			Field:    "schemaVersion",
			Message:  fmt.Sprintf("unsupported version %q; expected %q", f.SchemaVersion, SchemaVersion),
		})
	}

	// --- packages ---
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

	return &f, errs, nil
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
	dec.Token() // consume closing }
}

func walkArrayBody(dec *json.Decoder, data []byte, path string, pos map[string]Position) {
	for i := 0; dec.More(); i++ {
		childPath := fmt.Sprintf("%s[%d]", path, i)
		walkValue(dec, data, childPath, pos)
	}
	dec.Token() // consume closing ]
}
