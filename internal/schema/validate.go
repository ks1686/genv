package schema

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"
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
	errs = append(errs, validateFiles(f, raw, positions)...)
	errs = append(errs, validateHooks(f, raw, positions)...)
	errs = append(errs, validateRepo(f, raw, positions)...)
	errs = append(errs, validateUpdates(f, raw, positions)...)

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
	} else if f.SchemaVersion != Version && f.SchemaVersion != Version2 && f.SchemaVersion != Version3 && f.SchemaVersion != Version4 && f.SchemaVersion != Version5 && f.SchemaVersion != Version6 && f.SchemaVersion != Version7 {
		errs = append(errs, ValidationError{
			Position: positions["schemaVersion"],
			Field:    "schemaVersion",
			Message:  fmt.Sprintf("unsupported version %q; expected %q, %q, %q, %q, %q, %q, or %q", f.SchemaVersion, Version, Version2, Version3, Version4, Version5, Version6, Version7),
		})
	}
	return errs
}

func validatePackages(f *GenvFile, raw map[string]json.RawMessage, positions map[string]Position) []ValidationError {
	var errs []ValidationError
	if _, ok := raw["packages"]; !ok {
		// Schema v5 adds files/hooks/repo blocks and v6 adds updates; a spec may
		// legitimately contain only those blocks, so packages is optional there.
		if f.SchemaVersion != Version5 && f.SchemaVersion != Version6 && f.SchemaVersion != Version7 {
			errs = append(errs, ValidationError{
				Field:   "packages",
				Message: "required field is missing",
			})
		}
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
		if f.SchemaVersion != Version2 && f.SchemaVersion != Version3 && f.SchemaVersion != Version4 && f.SchemaVersion != Version5 && f.SchemaVersion != Version6 && f.SchemaVersion != Version7 {
			errs = append(errs, ValidationError{
				Position: positions["env"],
				Field:    "env",
				Message:  fmt.Sprintf("env block requires schemaVersion %q or newer (current: %q); run 'genv env set' to upgrade", Version2, f.SchemaVersion),
			})
		}
		for name := range f.Env {
			if !ValidEnvName(name) {
				errs = append(errs, ValidationError{
					Field:   "env." + name,
					Message: fmt.Sprintf("invalid variable name %q: must match [A-Za-z_][A-Za-z0-9_]*", name),
				})
			}
		}
	}
	return errs
}

func validateShell(f *GenvFile, raw map[string]json.RawMessage, positions map[string]Position) []ValidationError {
	var errs []ValidationError
	if _, hasShell := raw["shell"]; hasShell {
		if f.SchemaVersion != Version3 && f.SchemaVersion != Version4 && f.SchemaVersion != Version5 && f.SchemaVersion != Version6 && f.SchemaVersion != Version7 {
			errs = append(errs, ValidationError{
				Position: positions["shell"],
				Field:    "shell",
				Message:  fmt.Sprintf("shell block requires schemaVersion %q or newer (current: %q); run 'genv shell alias set' to upgrade", Version3, f.SchemaVersion),
			})
		}
		if f.Shell != nil {
			aliasShells := make(map[string]string, len(f.Shell.Aliases))
			for k, v := range f.Shell.Aliases {
				aliasShells[k] = v.Shell
			}
			errs = append(errs, validateShellEntries(aliasShells, "shell.aliases", "alias")...)

			funcShells := make(map[string]string, len(f.Shell.Functions))
			for k, v := range f.Shell.Functions {
				funcShells[k] = v.Shell
				if containsShellMeta(v.Body) {
					errs = append(errs, ValidationError{
						Field:   fmt.Sprintf("shell.functions.%s.body", k),
						Message: "contains shell metacharacters; function body must be plain text without separators or substitutions",
					})
				}
			}
			errs = append(errs, validateShellEntries(funcShells, "shell.functions", "function")...)
			errs = append(errs, requirePowerShellV7(f, aliasShells, funcShells)...)

			for i, src := range f.Shell.Source {
				if src == "" {
					errs = append(errs, ValidationError{
						Field:   fmt.Sprintf("shell.source[%d]", i),
						Message: "source path must not be empty",
					})
					continue
				}
				if containsShellMeta(src) {
					errs = append(errs, ValidationError{
						Field:   fmt.Sprintf("shell.source[%d]", i),
						Message: "contains shell metacharacters",
					})
				}
			}
		}
	}
	return errs
}

func validateShellEntries(shells map[string]string, fieldPrefix, singularName string) []ValidationError {
	var errs []ValidationError
	for name, sh := range shells {
		if name == "" {
			errs = append(errs, ValidationError{
				Field:   fieldPrefix,
				Message: singularName + " name must not be empty",
			})
		}
		if sh != "" && !KnownShellTargets[sh] {
			errs = append(errs, ValidationError{
				Field:   fmt.Sprintf("%s.%s.shell", fieldPrefix, name),
				Message: fmt.Sprintf("unknown shell %q; expected %s", sh, ValidShellTargetsMsg),
			})
		}
	}
	return errs
}

// requirePowerShellV7 rejects shell: "powershell" targets on schema versions
// older than v7 (the version that introduced PowerShell targeting).
func requirePowerShellV7(f *GenvFile, aliasShells, funcShells map[string]string) []ValidationError {
	if versionRank(f.SchemaVersion) >= versionRank(Version7) {
		return nil
	}
	var errs []ValidationError
	for name, sh := range aliasShells {
		if sh != "powershell" {
			continue
		}
		errs = append(errs, ValidationError{
			Field:   fmt.Sprintf("shell.aliases.%s.shell", name),
			Message: fmt.Sprintf(`shell target "powershell" requires schemaVersion %q or newer (current: %q)`, Version7, f.SchemaVersion),
		})
	}
	for name, sh := range funcShells {
		if sh != "powershell" {
			continue
		}
		errs = append(errs, ValidationError{
			Field:   fmt.Sprintf("shell.functions.%s.shell", name),
			Message: fmt.Sprintf(`shell target "powershell" requires schemaVersion %q or newer (current: %q)`, Version7, f.SchemaVersion),
		})
	}
	return errs
}

func validateServices(f *GenvFile, raw map[string]json.RawMessage, positions map[string]Position) []ValidationError {
	var errs []ValidationError
	if _, hasServices := raw["services"]; hasServices {
		if f.SchemaVersion != Version4 && f.SchemaVersion != Version5 && f.SchemaVersion != Version6 && f.SchemaVersion != Version7 {
			errs = append(errs, ValidationError{
				Position: positions["services"],
				Field:    "services",
				Message:  fmt.Sprintf("services block requires schemaVersion %q or newer (current: %q); run 'genv service add' to upgrade", Version4, f.SchemaVersion),
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
				if strings.ContainsAny(name, "\r\n") {
					errs = append(errs, ValidationError{
						Field:   fmt.Sprintf("services.%s", name),
						Message: "service name must not contain newlines",
					})
				}
				if len(svc.Start) == 0 && svc.BrewFormula == "" {
					errs = append(errs, ValidationError{
						Field:   fmt.Sprintf("services.%s.start", name),
						Message: "start command is required (or set brew_formula for brew-managed services)",
					})
				}
				if svc.BrewFormula != "" && len(svc.Start) > 0 {
					errs = append(errs, ValidationError{
						Field:   fmt.Sprintf("services.%s", name),
						Message: "brew_formula and start are mutually exclusive; use one or the other",
					})
				}
				if strings.ContainsAny(svc.BrewFormula, "\r\n") {
					errs = append(errs, ValidationError{
						Field:   fmt.Sprintf("services.%s.brew_formula", name),
						Message: "brew_formula must not contain newlines",
					})
				}
				errs = append(errs, validateServiceCommand(name, "start", svc.Start)...)
				errs = append(errs, validateServiceCommand(name, "stop", svc.Stop)...)
				errs = append(errs, validateServiceCommand(name, "restart", svc.Restart)...)
				errs = append(errs, validateServiceCommand(name, "status", svc.Status)...)
			}
		}
	}
	return errs
}

func validateServiceCommand(name, field string, args []string) []ValidationError {
	var errs []ValidationError
	for i, arg := range args {
		if arg == "" {
			errs = append(errs, ValidationError{
				Field:   fmt.Sprintf("services.%s.%s[%d]", name, field, i),
				Message: "command arguments must not be empty",
			})
		}
		if strings.ContainsAny(arg, "\r\n") {
			errs = append(errs, ValidationError{
				Field:   fmt.Sprintf("services.%s.%s[%d]", name, field, i),
				Message: "command arguments must not contain newlines",
			})
		}
	}
	return errs
}

// expandPath performs the v5 path expansion rules: leading ~ becomes the user
// home directory, and $VAR/${VAR} are replaced with os.Getenv values. It is a
// best-effort helper; callers validate the result rather than the raw string.
func expandPath(s string) (string, error) {
	if strings.HasPrefix(s, "~") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		s = home + s[1:]
	}
	return os.Expand(s, os.Getenv), nil
}

func validateFiles(f *GenvFile, raw map[string]json.RawMessage, positions map[string]Position) []ValidationError {
	var errs []ValidationError
	if _, hasFiles := raw["files"]; hasFiles {
		if f.SchemaVersion != Version5 && f.SchemaVersion != Version6 && f.SchemaVersion != Version7 {
			errs = append(errs, ValidationError{
				Position: positions["files"],
				Field:    "files",
				Message:  fmt.Sprintf("files block requires schemaVersion %q or newer (current: %q)", Version5, f.SchemaVersion),
			})
		}
		if f.Files == nil {
			return errs
		}
		for i, l := range f.Files.Links {
			field := fmt.Sprintf("files.links[%d]", i)
			if l.Source == "" {
				errs = append(errs, ValidationError{Field: field + ".source", Message: "source must not be empty"})
			}
			if l.Target == "" {
				errs = append(errs, ValidationError{Field: field + ".target", Message: "target must not be empty"})
			}
			if l.Mode != "" && l.Mode != "link" && l.Mode != "managed-link" && l.Mode != "merge-dir" {
				errs = append(errs, ValidationError{
					Field:   field + ".mode",
					Message: fmt.Sprintf("invalid link mode %q; expected \"link\", \"managed-link\", or \"merge-dir\"", l.Mode),
				})
			}
			if expanded, err := expandPath(l.Target); err != nil || expanded == "" {
				msg := "cannot expand target path"
				if err != nil {
					msg = fmt.Sprintf("%s: %v", msg, err)
				}
				errs = append(errs, ValidationError{Field: field + ".target", Message: msg})
			}
		}
		for i, tpl := range f.Files.Templates {
			field := fmt.Sprintf("files.templates[%d]", i)
			if tpl.Source == "" {
				errs = append(errs, ValidationError{Field: field + ".source", Message: "source must not be empty"})
			}
			if tpl.Target == "" {
				errs = append(errs, ValidationError{Field: field + ".target", Message: "target must not be empty"})
			}
			if expanded, err := expandPath(tpl.Target); err != nil || expanded == "" {
				msg := "cannot expand target path"
				if err != nil {
					msg = fmt.Sprintf("%s: %v", msg, err)
				}
				errs = append(errs, ValidationError{Field: field + ".target", Message: msg})
			}
		}
		for i, d := range f.Files.Dirs {
			field := fmt.Sprintf("files.dirs[%d]", i)
			if d.Target == "" {
				errs = append(errs, ValidationError{Field: field + ".target", Message: "target must not be empty"})
			}
			if expanded, err := expandPath(d.Target); err != nil || expanded == "" {
				msg := "cannot expand target path"
				if err != nil {
					msg = fmt.Sprintf("%s: %v", msg, err)
				}
				errs = append(errs, ValidationError{Field: field + ".target", Message: msg})
			}
		}
	}
	return errs
}

func validateHooks(f *GenvFile, raw map[string]json.RawMessage, positions map[string]Position) []ValidationError {
	var errs []ValidationError
	if _, hasHooks := raw["hooks"]; hasHooks {
		if f.SchemaVersion != Version5 && f.SchemaVersion != Version6 && f.SchemaVersion != Version7 {
			errs = append(errs, ValidationError{
				Position: positions["hooks"],
				Field:    "hooks",
				Message:  fmt.Sprintf("hooks block requires schemaVersion %q or newer (current: %q)", Version5, f.SchemaVersion),
			})
		}
		if f.Hooks == nil {
			return errs
		}
		err := validateHookPhase("preUpgrade", f.Hooks.PreUpgrade)
		errs = append(errs, err...)
		err = validateHookPhase("postApply", f.Hooks.PostApply)
		errs = append(errs, err...)
		err = validateHookPhase("postUpgrade", f.Hooks.PostUpgrade)
		errs = append(errs, err...)
		if f.SchemaVersion != Version6 && f.SchemaVersion != Version7 {
			errs = append(errs, validateNoV6Hooks(f.Hooks, positions)...)
			return errs
		}
		err = validateHookPhase("preApply", f.Hooks.PreApply)
		errs = append(errs, err...)
		err = validateHookPhase("preAdd", f.Hooks.PreAdd)
		errs = append(errs, err...)
		err = validateHookPhase("postAdd", f.Hooks.PostAdd)
		errs = append(errs, err...)
		err = validateHookPhase("preRemove", f.Hooks.PreRemove)
		errs = append(errs, err...)
		err = validateHookPhase("postRemove", f.Hooks.PostRemove)
		errs = append(errs, err...)
	}
	return errs
}

func validateHookPhase(phase string, hooks []Hook) []ValidationError {
	var errs []ValidationError
	for i, h := range hooks {
		field := fmt.Sprintf("hooks.%s[%d]", phase, i)
		hasCommand := h.Command != ""
		hasFile := h.File != ""
		if hasCommand == hasFile {
			errs = append(errs, ValidationError{
				Field:   field,
				Message: "exactly one of command or file must be set",
			})
		}
		if strings.ContainsAny(h.File, "\r\n") {
			errs = append(errs, ValidationError{Field: field + ".file", Message: "file must not contain newlines"})
		}
	}
	return errs
}

func validateNoV6Hooks(h *HooksConfig, positions map[string]Position) []ValidationError {
	phases := map[string][]Hook{
		"preApply":   h.PreApply,
		"preAdd":     h.PreAdd,
		"postAdd":    h.PostAdd,
		"preRemove":  h.PreRemove,
		"postRemove": h.PostRemove,
	}
	var errs []ValidationError
	for phase, hooks := range phases {
		if len(hooks) == 0 {
			continue
		}
		field := "hooks." + phase
		errs = append(errs, ValidationError{
			Position: positions[field],
			Field:    field,
			Message:  fmt.Sprintf("%s requires schemaVersion %q", phase, Version6),
		})
	}
	for phase, hooks := range map[string][]Hook{"preUpgrade": h.PreUpgrade, "postApply": h.PostApply, "postUpgrade": h.PostUpgrade} {
		for i, hook := range hooks {
			if hook.File == "" {
				continue
			}
			field := fmt.Sprintf("hooks.%s[%d].file", phase, i)
			errs = append(errs, ValidationError{
				Position: positions[field],
				Field:    field,
				Message:  fmt.Sprintf("script file hooks require schemaVersion %q", Version6),
			})
		}
	}
	return errs
}

func validateRepo(f *GenvFile, raw map[string]json.RawMessage, positions map[string]Position) []ValidationError {
	var errs []ValidationError
	if _, hasRepo := raw["repo"]; hasRepo {
		if f.SchemaVersion != Version5 && f.SchemaVersion != Version6 && f.SchemaVersion != Version7 {
			errs = append(errs, ValidationError{
				Position: positions["repo"],
				Field:    "repo",
				Message:  fmt.Sprintf("repo block requires schemaVersion %q or newer (current: %q)", Version5, f.SchemaVersion),
			})
		}
		if f.Repo == nil {
			errs = append(errs, ValidationError{
				Position: positions["repo"],
				Field:    "repo",
				Message:  "repo block must be an object with a url field",
			})
		} else if f.Repo.URL == "" {
			errs = append(errs, ValidationError{
				Position: positions["repo.url"],
				Field:    "repo.url",
				Message:  "required field is missing or empty",
			})
		}
	}
	return errs
}

func validateUpdates(f *GenvFile, raw map[string]json.RawMessage, positions map[string]Position) []ValidationError {
	var errs []ValidationError
	if _, hasUpdates := raw["updates"]; !hasUpdates {
		return errs
	}
	if f.SchemaVersion != Version6 && f.SchemaVersion != Version7 {
		errs = append(errs, ValidationError{
			Position: positions["updates"],
			Field:    "updates",
			Message:  fmt.Sprintf("updates block requires schemaVersion %q or newer (current: %q); bump schemaVersion to %q to use the updates config", Version6, f.SchemaVersion, Version6),
		})
	}
	if f.Updates == nil {
		return errs
	}
	errs = append(errs, validateUpdatesManagers("updates.onlyManagers", f.Updates.OnlyManagers, positions)...)
	errs = append(errs, validateUpdatesManagers("updates.skipManagers", f.Updates.SkipManagers, positions)...)
	if !f.Updates.Enabled {
		return errs
	}
	if f.Updates.Interval == "" {
		errs = append(errs, ValidationError{
			Position: positions["updates.interval"],
			Field:    "updates.interval",
			Message:  `interval is required when updates.enabled is true; set a positive Go duration such as "24h"`,
		})
		return errs
	}
	d, err := time.ParseDuration(f.Updates.Interval)
	if err != nil {
		errs = append(errs, ValidationError{
			Position: positions["updates.interval"],
			Field:    "updates.interval",
			Message:  fmt.Sprintf("invalid duration %q: %v; use a Go duration such as \"24h\", \"90m\", or \"1h30m\"", f.Updates.Interval, err),
		})
		return errs
	}
	if d <= 0 {
		errs = append(errs, ValidationError{
			Position: positions["updates.interval"],
			Field:    "updates.interval",
			Message:  fmt.Sprintf("interval %q must be a positive duration; set a value greater than zero such as \"24h\"", f.Updates.Interval),
		})
	}
	return errs
}

func validateUpdatesManagers(field string, managers []string, positions map[string]Position) []ValidationError {
	var errs []ValidationError
	for i, mgr := range managers {
		if KnownManagers[mgr] {
			continue
		}
		elem := fmt.Sprintf("%s[%d]", field, i)
		errs = append(errs, ValidationError{
			Position: positions[elem],
			Field:    elem,
			Message:  fmt.Sprintf("unknown manager %q", mgr),
		})
	}
	return errs
}

func containsShellMeta(s string) bool {
	return strings.ContainsAny(s, "\r\n;&|`$<>()")
}
