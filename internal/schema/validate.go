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
	f, raw, valErrs, err := unmarshalGenvFile(data)
	if valErrs != nil || err != nil {
		return f, valErrs, err
	}

	// Index field positions only after the document is known-valid JSON.
	// Walking an invalid token stream can spin in Decoder.More() on some
	// inputs (for example Windows paths interpolated into JSON unescaped).
	positions := make(map[string]Position)
	locateFields(data, positions)

	var errs []ValidationError

	errs = append(errs, validateUnknownKeys(raw, positions)...)
	errs = append(errs, validateSchemaVersion(f, raw, positions)...)
	errs = append(errs, validatePackages(f, raw, positions)...)
	errs = append(errs, validateEnv(f, raw, positions)...)
	errs = append(errs, validateShell(f, raw, positions)...)
	errs = append(errs, validateServices(f, raw, positions)...)
	errs = append(errs, validateFiles(f, raw, positions)...)
	errs = append(errs, validateHooks(f, raw, positions)...)
	errs = append(errs, validateRepo(f, raw, positions)...)
	errs = append(errs, validateUpdates(f, raw, positions)...)
	errs = append(errs, validateV8(f, positions)...)

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

func walkValue(dec *json.Decoder, data []byte, path string, pos map[string]Position) bool {
	tok, err := dec.Token()
	if err != nil {
		return false
	}
	offset := dec.InputOffset()
	switch v := tok.(type) {
	case json.Delim:
		switch v {
		case '{':
			pos[path] = offsetToPosition(data, offset)
			return walkObjectBody(dec, data, path, pos)
		case '[':
			pos[path] = offsetToPosition(data, offset)
			return walkArrayBody(dec, data, path, pos)
		}
	default:
		pos[path] = offsetToPosition(data, offset)
	}
	return true
}

func walkObjectBody(dec *json.Decoder, data []byte, path string, pos map[string]Position) bool {
	// Bound iterations so a decoder that keeps reporting More() after a
	// syntax error cannot hang ParseAndValidate (seen on Windows CI).
	for n := 0; n <= len(data) && dec.More(); n++ {
		keyTok, err := dec.Token()
		if err != nil {
			return false
		}
		key, ok := keyTok.(string)
		if !ok {
			return false
		}
		childPath := key
		if path != "" {
			childPath = path + "." + key
		}
		if !walkValue(dec, data, childPath, pos) {
			return false
		}
	}
	_, _ = dec.Token() // consume closing }
	return true
}

func walkArrayBody(dec *json.Decoder, data []byte, path string, pos map[string]Position) bool {
	for i := 0; i <= len(data) && dec.More(); i++ {
		childPath := fmt.Sprintf("%s[%d]", path, i)
		if !walkValue(dec, data, childPath, pos) {
			return false
		}
	}
	_, _ = dec.Token() // consume closing ]
	return true
}

func validateSchemaVersion(f *GenvFile, raw map[string]json.RawMessage, positions map[string]Position) []ValidationError {
	var errs []ValidationError
	if _, ok := raw["schemaVersion"]; !ok {
		errs = append(errs, ValidationError{
			Field:   "schemaVersion",
			Message: "required field is missing",
		})
	} else if f.SchemaVersion != Version && f.SchemaVersion != Version2 && f.SchemaVersion != Version3 && f.SchemaVersion != Version4 && f.SchemaVersion != Version5 && f.SchemaVersion != Version6 && f.SchemaVersion != Version7 && f.SchemaVersion != Version8 {
		errs = append(errs, ValidationError{
			Position: positions["schemaVersion"],
			Field:    "schemaVersion",
			Message:  fmt.Sprintf("unsupported version %q; expected %q, %q, %q, %q, %q, %q, %q, or %q", f.SchemaVersion, Version, Version2, Version3, Version4, Version5, Version6, Version7, Version8),
		})
	}
	return errs
}

func validatePackages(f *GenvFile, raw map[string]json.RawMessage, positions map[string]Position) []ValidationError {
	var errs []ValidationError
	if _, ok := raw["packages"]; !ok {
		// Schema v5 adds files/hooks/repo blocks and v6 adds updates; a spec may
		// legitimately contain only those blocks, so packages is optional there.
		if f.SchemaVersion != Version5 && f.SchemaVersion != Version6 && f.SchemaVersion != Version7 && f.SchemaVersion != Version8 {
			errs = append(errs, ValidationError{
				Field:   "packages",
				Message: "required field is missing",
			})
		}
	} else {
		errs = append(errs, validatePackageList(f.Packages, "packages", positions)...)
	}
	return errs
}

func validatePackageList(packages []Package, fieldPrefix string, positions map[string]Position) []ValidationError {
	var errs []ValidationError
	seen := make(map[string]int) // id → first index
	for i, pkg := range packages {
		pkgPath := fmt.Sprintf("%s[%d]", fieldPrefix, i)

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
				Message:  fmt.Sprintf("duplicate id %q (first seen at %s[%d])", pkg.ID, fieldPrefix, prev),
			})
		} else if !ValidPackageName(pkg.ID) {
			errs = append(errs, ValidationError{
				Position: positions[pkgPath+".id"],
				Field:    pkgPath + ".id",
				Message:  fmt.Sprintf("invalid package id %q: must not start with '-' or contain whitespace", pkg.ID),
			})
			seen[pkg.ID] = i
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

		for mgr, pkgName := range pkg.Managers {
			field := fmt.Sprintf("%s.managers.%s", pkgPath, mgr)
			if !KnownManagers[mgr] {
				errs = append(errs, ValidationError{
					Position: positions[field],
					Field:    field,
					Message:  fmt.Sprintf("unknown manager %q", mgr),
				})
			}
			if !ValidPackageName(pkgName) {
				errs = append(errs, ValidationError{
					Position: positions[field],
					Field:    field,
					Message:  fmt.Sprintf("invalid package name %q: must not be empty, start with '-', or contain whitespace", pkgName),
				})
			}
		}
	}
	return errs
}

func validateEnv(f *GenvFile, raw map[string]json.RawMessage, positions map[string]Position) []ValidationError {
	var errs []ValidationError
	if _, hasEnv := raw["env"]; hasEnv {
		if f.SchemaVersion != Version2 && f.SchemaVersion != Version3 && f.SchemaVersion != Version4 && f.SchemaVersion != Version5 && f.SchemaVersion != Version6 && f.SchemaVersion != Version7 && f.SchemaVersion != Version8 {
			errs = append(errs, ValidationError{
				Position: positions["env"],
				Field:    "env",
				Message:  fmt.Sprintf("env block requires schemaVersion %q or newer (current: %q); run 'genv env set' to upgrade", Version2, f.SchemaVersion),
			})
		}
		errs = append(errs, validateEnvMap(f.Env, "env")...)
	}
	return errs
}

func validateEnvMap(env map[string]EnvVar, fieldPrefix string) []ValidationError {
	var errs []ValidationError
	for name := range env {
		if !ValidEnvName(name) {
			errs = append(errs, ValidationError{
				Field:   fieldPrefix + "." + name,
				Message: fmt.Sprintf("invalid variable name %q: must match [A-Za-z_][A-Za-z0-9_]*", name),
			})
		}
	}
	return errs
}

func validateTargetEnvMap(env map[string]*EnvVar, fieldPrefix string, allowTombstones bool) []ValidationError {
	var errs []ValidationError
	for name, v := range env {
		if !ValidEnvName(name) {
			errs = append(errs, ValidationError{
				Field:   fieldPrefix + "." + name,
				Message: fmt.Sprintf("invalid variable name %q: must match [A-Za-z_][A-Za-z0-9_]*", name),
			})
		}
		if v == nil && !allowTombstones {
			errs = append(errs, ValidationError{
				Field:   fieldPrefix + "." + name,
				Message: "tombstone null entries are only valid under targets",
			})
		}
	}
	return errs
}

func validateShell(f *GenvFile, raw map[string]json.RawMessage, positions map[string]Position) []ValidationError {
	var errs []ValidationError
	if _, hasShell := raw["shell"]; hasShell {
		if f.SchemaVersion != Version3 && f.SchemaVersion != Version4 && f.SchemaVersion != Version5 && f.SchemaVersion != Version6 && f.SchemaVersion != Version7 && f.SchemaVersion != Version8 {
			errs = append(errs, ValidationError{
				Position: positions["shell"],
				Field:    "shell",
				Message:  fmt.Sprintf("shell block requires schemaVersion %q or newer (current: %q); run 'genv shell alias set' to upgrade", Version3, f.SchemaVersion),
			})
		}
		errs = append(errs, validateShellConfig(f, f.Shell, "shell")...)
	}
	return errs
}

func validateShellConfig(f *GenvFile, shell *ShellConfig, fieldPrefix string) []ValidationError {
	if shell == nil {
		return nil
	}
	var errs []ValidationError
	aliasShells := make(map[string]string, len(shell.Aliases))
	for k, v := range shell.Aliases {
		aliasShells[k] = v.Shell
	}
	errs = append(errs, validateShellEntries(aliasShells, fieldPrefix+".aliases", "alias")...)

	funcShells := make(map[string]string, len(shell.Functions))
	for k, v := range shell.Functions {
		funcShells[k] = v.Shell
		if containsShellMeta(v.Body) {
			errs = append(errs, ValidationError{
				Field:   fmt.Sprintf("%s.functions.%s.body", fieldPrefix, k),
				Message: "contains shell metacharacters; function body must be plain text without separators or substitutions",
			})
		}
	}
	errs = append(errs, validateShellEntries(funcShells, fieldPrefix+".functions", "function")...)
	errs = append(errs, requirePowerShellV7(f, fieldPrefix, aliasShells, funcShells)...)

	for i, src := range shell.Source {
		if src == "" {
			errs = append(errs, ValidationError{
				Field:   fmt.Sprintf("%s.source[%d]", fieldPrefix, i),
				Message: "source path must not be empty",
			})
			continue
		}
		if containsShellMeta(src) {
			errs = append(errs, ValidationError{
				Field:   fmt.Sprintf("%s.source[%d]", fieldPrefix, i),
				Message: "contains shell metacharacters",
			})
		}
	}
	return errs
}

func validateTargetShellConfig(f *GenvFile, shell *TargetShellConfig, fieldPrefix string, allowTombstones bool) []ValidationError {
	if shell == nil {
		return nil
	}
	var errs []ValidationError
	aliasShells := make(map[string]string, len(shell.Aliases))
	for k, v := range shell.Aliases {
		if v == nil {
			if k == "" {
				errs = append(errs, ValidationError{
					Field:   fieldPrefix + ".aliases",
					Message: "alias name must not be empty",
				})
			}
			if !allowTombstones {
				errs = append(errs, ValidationError{
					Field:   fmt.Sprintf("%s.aliases.%s", fieldPrefix, k),
					Message: "tombstone null entries are only valid under targets",
				})
			}
			continue
		}
		aliasShells[k] = v.Shell
	}
	errs = append(errs, validateShellEntries(aliasShells, fieldPrefix+".aliases", "alias")...)

	funcShells := make(map[string]string, len(shell.Functions))
	for k, v := range shell.Functions {
		if v == nil {
			if k == "" {
				errs = append(errs, ValidationError{
					Field:   fieldPrefix + ".functions",
					Message: "function name must not be empty",
				})
			}
			if !allowTombstones {
				errs = append(errs, ValidationError{
					Field:   fmt.Sprintf("%s.functions.%s", fieldPrefix, k),
					Message: "tombstone null entries are only valid under targets",
				})
			}
			continue
		}
		funcShells[k] = v.Shell
		if containsShellMeta(v.Body) {
			errs = append(errs, ValidationError{
				Field:   fmt.Sprintf("%s.functions.%s.body", fieldPrefix, k),
				Message: "contains shell metacharacters; function body must be plain text without separators or substitutions",
			})
		}
	}
	errs = append(errs, validateShellEntries(funcShells, fieldPrefix+".functions", "function")...)
	errs = append(errs, requirePowerShellV7(f, fieldPrefix, aliasShells, funcShells)...)

	for i, src := range shell.Source {
		if src == "" {
			errs = append(errs, ValidationError{
				Field:   fmt.Sprintf("%s.source[%d]", fieldPrefix, i),
				Message: "source path must not be empty",
			})
			continue
		}
		if containsShellMeta(src) {
			errs = append(errs, ValidationError{
				Field:   fmt.Sprintf("%s.source[%d]", fieldPrefix, i),
				Message: "contains shell metacharacters",
			})
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
		} else if !validShellIdent(name) {
			errs = append(errs, ValidationError{
				Field:   fmt.Sprintf("%s.%s", fieldPrefix, name),
				Message: fmt.Sprintf("invalid %s name %q: must match [A-Za-z_][A-Za-z0-9_.-]*", singularName, name),
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
func requirePowerShellV7(f *GenvFile, fieldPrefix string, aliasShells, funcShells map[string]string) []ValidationError {
	if versionRank(f.SchemaVersion) >= versionRank(Version7) {
		return nil
	}
	var errs []ValidationError
	for name, sh := range aliasShells {
		if sh != "powershell" {
			continue
		}
		errs = append(errs, ValidationError{
			Field:   fmt.Sprintf("%s.aliases.%s.shell", fieldPrefix, name),
			Message: fmt.Sprintf(`shell target "powershell" requires schemaVersion %q or newer (current: %q)`, Version7, f.SchemaVersion),
		})
	}
	for name, sh := range funcShells {
		if sh != "powershell" {
			continue
		}
		errs = append(errs, ValidationError{
			Field:   fmt.Sprintf("%s.functions.%s.shell", fieldPrefix, name),
			Message: fmt.Sprintf(`shell target "powershell" requires schemaVersion %q or newer (current: %q)`, Version7, f.SchemaVersion),
		})
	}
	return errs
}

func validateServices(f *GenvFile, raw map[string]json.RawMessage, positions map[string]Position) []ValidationError {
	var errs []ValidationError
	if _, hasServices := raw["services"]; hasServices {
		if f.SchemaVersion != Version4 && f.SchemaVersion != Version5 && f.SchemaVersion != Version6 && f.SchemaVersion != Version7 && f.SchemaVersion != Version8 {
			errs = append(errs, ValidationError{
				Position: positions["services"],
				Field:    "services",
				Message:  fmt.Sprintf("services block requires schemaVersion %q or newer (current: %q); run 'genv service add' to upgrade", Version4, f.SchemaVersion),
			})
		}
		errs = append(errs, validateServiceMap(f.Services, "services")...)
	}
	return errs
}

func validateServiceMap(services map[string]Service, fieldPrefix string) []ValidationError {
	var errs []ValidationError
	for name, svc := range services {
		errs = append(errs, validateServiceName(name, fieldPrefix)...)
		errs = append(errs, validateService(name, svc, fieldPrefix)...)
	}
	return errs
}

func validateTargetServiceMap(services map[string]*Service, fieldPrefix string, allowTombstones bool) []ValidationError {
	var errs []ValidationError
	for name, svc := range services {
		errs = append(errs, validateServiceName(name, fieldPrefix)...)
		if svc == nil {
			if !allowTombstones {
				errs = append(errs, ValidationError{
					Field:   fmt.Sprintf("%s.%s", fieldPrefix, name),
					Message: "tombstone null entries are only valid under targets",
				})
			}
			continue
		}
		errs = append(errs, validateService(name, *svc, fieldPrefix)...)
	}
	return errs
}

func validateServiceName(name, fieldPrefix string) []ValidationError {
	var errs []ValidationError
	if name == "" {
		errs = append(errs, ValidationError{
			Field:   fieldPrefix,
			Message: "service name must not be empty",
		})
	}
	if strings.ContainsAny(name, "\r\n") {
		errs = append(errs, ValidationError{
			Field:   fmt.Sprintf("%s.%s", fieldPrefix, name),
			Message: "service name must not contain newlines",
		})
	}
	return errs
}

func validateService(name string, svc Service, fieldPrefix string) []ValidationError {
	var errs []ValidationError
	if len(svc.Start) == 0 && svc.BrewFormula == "" {
		errs = append(errs, ValidationError{
			Field:   fmt.Sprintf("%s.%s.start", fieldPrefix, name),
			Message: "start command is required (or set brew_formula for brew-managed services)",
		})
	}
	if svc.BrewFormula != "" && len(svc.Start) > 0 {
		errs = append(errs, ValidationError{
			Field:   fmt.Sprintf("%s.%s", fieldPrefix, name),
			Message: "brew_formula and start are mutually exclusive; use one or the other",
		})
	}
	if strings.ContainsAny(svc.BrewFormula, "\r\n") {
		errs = append(errs, ValidationError{
			Field:   fmt.Sprintf("%s.%s.brew_formula", fieldPrefix, name),
			Message: "brew_formula must not contain newlines",
		})
	}
	errs = append(errs, validateServiceCommand(fieldPrefix, name, "start", svc.Start)...)
	errs = append(errs, validateServiceCommand(fieldPrefix, name, "stop", svc.Stop)...)
	errs = append(errs, validateServiceCommand(fieldPrefix, name, "restart", svc.Restart)...)
	errs = append(errs, validateServiceCommand(fieldPrefix, name, "status", svc.Status)...)
	return errs
}

func validateServiceCommand(fieldPrefix, name, field string, args []string) []ValidationError {
	var errs []ValidationError
	for i, arg := range args {
		if arg == "" {
			errs = append(errs, ValidationError{
				Field:   fmt.Sprintf("%s.%s.%s[%d]", fieldPrefix, name, field, i),
				Message: "command arguments must not be empty",
			})
		}
		if strings.ContainsAny(arg, "\r\n") {
			errs = append(errs, ValidationError{
				Field:   fmt.Sprintf("%s.%s.%s[%d]", fieldPrefix, name, field, i),
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
		if f.SchemaVersion != Version5 && f.SchemaVersion != Version6 && f.SchemaVersion != Version7 && f.SchemaVersion != Version8 {
			errs = append(errs, ValidationError{
				Position: positions["files"],
				Field:    "files",
				Message:  fmt.Sprintf("files block requires schemaVersion %q or newer (current: %q)", Version5, f.SchemaVersion),
			})
		}
		errs = append(errs, validateFilesConfig(f.Files, "files")...)
	}
	return errs
}

func validateFilesConfig(files *FilesConfig, fieldPrefix string) []ValidationError {
	if files == nil {
		return nil
	}
	var errs []ValidationError
	for i, l := range files.Links {
		field := fmt.Sprintf("%s.links[%d]", fieldPrefix, i)
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
	for i, tpl := range files.Templates {
		field := fmt.Sprintf("%s.templates[%d]", fieldPrefix, i)
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
	for i, d := range files.Dirs {
		field := fmt.Sprintf("%s.dirs[%d]", fieldPrefix, i)
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
	return errs
}

func validateHooks(f *GenvFile, raw map[string]json.RawMessage, positions map[string]Position) []ValidationError {
	var errs []ValidationError
	if _, hasHooks := raw["hooks"]; hasHooks {
		if f.SchemaVersion != Version5 && f.SchemaVersion != Version6 && f.SchemaVersion != Version7 && f.SchemaVersion != Version8 {
			errs = append(errs, ValidationError{
				Position: positions["hooks"],
				Field:    "hooks",
				Message:  fmt.Sprintf("hooks block requires schemaVersion %q or newer (current: %q)", Version5, f.SchemaVersion),
			})
		}
		errs = append(errs, validateHooksConfig(f, f.Hooks, "hooks", positions)...)
	}
	return errs
}

func validateHooksConfig(f *GenvFile, hooks *HooksConfig, fieldPrefix string, positions map[string]Position) []ValidationError {
	if hooks == nil {
		return nil
	}
	var errs []ValidationError
	err := validateHookPhase(fieldPrefix, "preUpgrade", hooks.PreUpgrade)
	errs = append(errs, err...)
	err = validateHookPhase(fieldPrefix, "postApply", hooks.PostApply)
	errs = append(errs, err...)
	err = validateHookPhase(fieldPrefix, "postUpgrade", hooks.PostUpgrade)
	errs = append(errs, err...)
	if f.SchemaVersion != Version6 && f.SchemaVersion != Version7 && f.SchemaVersion != Version8 {
		errs = append(errs, validateNoV6Hooks(hooks, fieldPrefix, positions)...)
		return errs
	}
	err = validateHookPhase(fieldPrefix, "preApply", hooks.PreApply)
	errs = append(errs, err...)
	err = validateHookPhase(fieldPrefix, "preAdd", hooks.PreAdd)
	errs = append(errs, err...)
	err = validateHookPhase(fieldPrefix, "postAdd", hooks.PostAdd)
	errs = append(errs, err...)
	err = validateHookPhase(fieldPrefix, "preRemove", hooks.PreRemove)
	errs = append(errs, err...)
	err = validateHookPhase(fieldPrefix, "postRemove", hooks.PostRemove)
	errs = append(errs, err...)
	return errs
}

func validateHookPhase(fieldPrefix, phase string, hooks []Hook) []ValidationError {
	var errs []ValidationError
	for i, h := range hooks {
		field := fmt.Sprintf("%s.%s[%d]", fieldPrefix, phase, i)
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

func validateNoV6Hooks(h *HooksConfig, fieldPrefix string, positions map[string]Position) []ValidationError {
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
		field := fieldPrefix + "." + phase
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
			field := fmt.Sprintf("%s.%s[%d].file", fieldPrefix, phase, i)
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
		if f.SchemaVersion != Version5 && f.SchemaVersion != Version6 && f.SchemaVersion != Version7 && f.SchemaVersion != Version8 {
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
		} else if err := ValidRepoURL(f.Repo.URL); err != nil {
			errs = append(errs, ValidationError{
				Position: positions["repo.url"],
				Field:    "repo.url",
				Message:  err.Error(),
			})
		}
		if f.Repo != nil && f.Repo.Ref != "" {
			if err := ValidGitRef(f.Repo.Ref); err != nil {
				errs = append(errs, ValidationError{
					Position: positions["repo.ref"],
					Field:    "repo.ref",
					Message:  err.Error(),
				})
			}
		}
	}
	return errs
}

func validateUpdates(f *GenvFile, raw map[string]json.RawMessage, positions map[string]Position) []ValidationError {
	var errs []ValidationError
	if _, hasUpdates := raw["updates"]; !hasUpdates {
		return errs
	}
	if f.SchemaVersion != Version6 && f.SchemaVersion != Version7 && f.SchemaVersion != Version8 {
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

func validateV8(f *GenvFile, positions map[string]Position) []ValidationError {
	if f.SchemaVersion != Version8 {
		return nil
	}

	var errs []ValidationError
	if len(f.Packages) > 0 {
		errs = append(errs, ValidationError{
			Position: positions["packages"],
			Field:    "packages",
			Message:  "top-level packages are not allowed in schemaVersion \"8\"; use targets.<target>.packages",
		})
	}
	if f.Env != nil {
		errs = append(errs, ValidationError{
			Position: positions["env"],
			Field:    "env",
			Message:  "top-level env is not allowed in schemaVersion \"8\"; use defaults.env or targets.<target>.env",
		})
	}
	if f.Shell != nil {
		errs = append(errs, ValidationError{
			Position: positions["shell"],
			Field:    "shell",
			Message:  "top-level shell is not allowed in schemaVersion \"8\"; use defaults.shell or targets.<target>.shell",
		})
	}
	if f.Files != nil {
		errs = append(errs, ValidationError{
			Position: positions["files"],
			Field:    "files",
			Message:  "top-level files are not allowed in schemaVersion \"8\"; use defaults.files or targets.<target>.files",
		})
	}
	if f.Services != nil {
		errs = append(errs, ValidationError{
			Position: positions["services"],
			Field:    "services",
			Message:  "top-level services are not allowed in schemaVersion \"8\"; use defaults.services or targets.<target>.services",
		})
	}
	if f.Hooks != nil {
		errs = append(errs, ValidationError{
			Position: positions["hooks"],
			Field:    "hooks",
			Message:  "top-level hooks are not allowed in schemaVersion \"8\"; use defaults.hooks or targets.<target>.hooks",
		})
	}

	if len(f.Targets) == 0 {
		errs = append(errs, ValidationError{
			Position: positions["targets"],
			Field:    "targets",
			Message:  "at least one target is required",
		})
	}

	if f.Defaults != nil {
		errs = append(errs, validateTargetBundle(f, f.Defaults, "defaults", false, positions)...)
	}
	for target, bundle := range f.Targets {
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
		errs = append(errs, validateTargetBundle(f, bundle, targetPath, true, positions)...)
		errs = append(errs, validateKnownTombstones(bundle, f.Defaults, targetPath, positions)...)
	}
	return errs
}

func validateKnownTombstones(target, defaults *TargetBundle, fieldPrefix string, positions map[string]Position) []ValidationError {
	var errs []ValidationError
	for name, v := range target.Env {
		if v != nil || hasDefaultEnv(defaults, name) {
			continue
		}
		field := fmt.Sprintf("%s.env.%s", fieldPrefix, name)
		errs = append(errs, ValidationError{
			Position: positions[field],
			Field:    field,
			Message:  fmt.Sprintf("unknown tombstone %q: no non-null defaults.env entry exists to delete", name),
		})
	}
	if target.Shell != nil {
		for name, v := range target.Shell.Aliases {
			if v != nil || hasDefaultAlias(defaults, name) {
				continue
			}
			field := fmt.Sprintf("%s.shell.aliases.%s", fieldPrefix, name)
			errs = append(errs, ValidationError{
				Position: positions[field],
				Field:    field,
				Message:  fmt.Sprintf("unknown tombstone %q: no non-null defaults.shell.aliases entry exists to delete", name),
			})
		}
		for name, v := range target.Shell.Functions {
			if v != nil || hasDefaultFunction(defaults, name) {
				continue
			}
			field := fmt.Sprintf("%s.shell.functions.%s", fieldPrefix, name)
			errs = append(errs, ValidationError{
				Position: positions[field],
				Field:    field,
				Message:  fmt.Sprintf("unknown tombstone %q: no non-null defaults.shell.functions entry exists to delete", name),
			})
		}
	}
	for name, svc := range target.Services {
		if svc != nil || hasDefaultService(defaults, name) {
			continue
		}
		field := fmt.Sprintf("%s.services.%s", fieldPrefix, name)
		errs = append(errs, ValidationError{
			Position: positions[field],
			Field:    field,
			Message:  fmt.Sprintf("unknown tombstone %q: no non-null defaults.services entry exists to delete", name),
		})
	}
	return errs
}

func hasDefaultEnv(defaults *TargetBundle, name string) bool {
	if defaults == nil {
		return false
	}
	return defaults.Env[name] != nil
}

func hasDefaultAlias(defaults *TargetBundle, name string) bool {
	if defaults == nil || defaults.Shell == nil {
		return false
	}
	return defaults.Shell.Aliases[name] != nil
}

func hasDefaultFunction(defaults *TargetBundle, name string) bool {
	if defaults == nil || defaults.Shell == nil {
		return false
	}
	return defaults.Shell.Functions[name] != nil
}

func hasDefaultService(defaults *TargetBundle, name string) bool {
	if defaults == nil {
		return false
	}
	return defaults.Services[name] != nil
}

func validateTargetBundle(f *GenvFile, bundle *TargetBundle, fieldPrefix string, allowTombstones bool, positions map[string]Position) []ValidationError {
	var errs []ValidationError
	errs = append(errs, validatePackageList(bundle.Packages, fieldPrefix+".packages", positions)...)
	errs = append(errs, validateNoPackageHosts(bundle.Packages, fieldPrefix+".packages", positions)...)
	errs = append(errs, validateTargetEnvMap(bundle.Env, fieldPrefix+".env", allowTombstones)...)
	errs = append(errs, validateTargetShellConfig(f, bundle.Shell, fieldPrefix+".shell", allowTombstones)...)
	errs = append(errs, validateTargetServiceMap(bundle.Services, fieldPrefix+".services", allowTombstones)...)
	errs = append(errs, validateNoServiceHosts(bundle.Services, fieldPrefix+".services", positions)...)
	errs = append(errs, validateFilesConfig(bundle.Files, fieldPrefix+".files")...)
	errs = append(errs, validateNoFileHosts(bundle.Files, fieldPrefix+".files", positions)...)
	errs = append(errs, validateHooksConfig(f, bundle.Hooks, fieldPrefix+".hooks", positions)...)
	errs = append(errs, validateNoHookHosts(bundle.Hooks, fieldPrefix+".hooks", positions)...)
	return errs
}

func validateNoPackageHosts(packages []Package, fieldPrefix string, positions map[string]Position) []ValidationError {
	var errs []ValidationError
	for i, pkg := range packages {
		if len(pkg.Host) == 0 {
			continue
		}
		field := fmt.Sprintf("%s[%d].host", fieldPrefix, i)
		errs = append(errs, ValidationError{
			Position: positions[field],
			Field:    field,
			Message:  "host predicates are not allowed in schemaVersion \"8\"; use target buckets",
		})
	}
	return errs
}

func validateNoServiceHosts(services map[string]*Service, fieldPrefix string, positions map[string]Position) []ValidationError {
	var errs []ValidationError
	for name, svc := range services {
		if svc == nil || len(svc.Host) == 0 {
			continue
		}
		field := fmt.Sprintf("%s.%s.host", fieldPrefix, name)
		errs = append(errs, ValidationError{
			Position: positions[field],
			Field:    field,
			Message:  "host predicates are not allowed in schemaVersion \"8\"; use target buckets",
		})
	}
	return errs
}

func validateNoFileHosts(files *FilesConfig, fieldPrefix string, positions map[string]Position) []ValidationError {
	if files == nil {
		return nil
	}
	var errs []ValidationError
	for i, link := range files.Links {
		if len(link.Host) == 0 {
			continue
		}
		field := fmt.Sprintf("%s.links[%d].host", fieldPrefix, i)
		errs = append(errs, ValidationError{
			Position: positions[field],
			Field:    field,
			Message:  "host predicates are not allowed in schemaVersion \"8\"; use target buckets",
		})
	}
	for i, tpl := range files.Templates {
		if len(tpl.Host) == 0 {
			continue
		}
		field := fmt.Sprintf("%s.templates[%d].host", fieldPrefix, i)
		errs = append(errs, ValidationError{
			Position: positions[field],
			Field:    field,
			Message:  "host predicates are not allowed in schemaVersion \"8\"; use target buckets",
		})
	}
	for i, dir := range files.Dirs {
		if len(dir.Host) == 0 {
			continue
		}
		field := fmt.Sprintf("%s.dirs[%d].host", fieldPrefix, i)
		errs = append(errs, ValidationError{
			Position: positions[field],
			Field:    field,
			Message:  "host predicates are not allowed in schemaVersion \"8\"; use target buckets",
		})
	}
	return errs
}

func validateNoHookHosts(hooks *HooksConfig, fieldPrefix string, positions map[string]Position) []ValidationError {
	if hooks == nil {
		return nil
	}
	phases := map[string][]Hook{
		"preApply":    hooks.PreApply,
		"postApply":   hooks.PostApply,
		"preAdd":      hooks.PreAdd,
		"postAdd":     hooks.PostAdd,
		"preRemove":   hooks.PreRemove,
		"postRemove":  hooks.PostRemove,
		"preUpgrade":  hooks.PreUpgrade,
		"postUpgrade": hooks.PostUpgrade,
	}
	var errs []ValidationError
	for phase, entries := range phases {
		for i, hook := range entries {
			if len(hook.Host) == 0 {
				continue
			}
			field := fmt.Sprintf("%s.%s[%d].host", fieldPrefix, phase, i)
			errs = append(errs, ValidationError{
				Position: positions[field],
				Field:    field,
				Message:  "host predicates are not allowed in schemaVersion \"8\"; use target buckets",
			})
		}
	}
	return errs
}

func containsShellMeta(s string) bool {
	return strings.ContainsAny(s, "\r\n;&|`$<>()")
}

func validShellIdent(name string) bool {
	if name == "" {
		return false
	}
	for i, r := range name {
		switch {
		case r >= 'A' && r <= 'Z', r >= 'a' && r <= 'z', r == '_':
		case i > 0 && ((r >= '0' && r <= '9') || r == '.' || r == '-'):
		default:
			return false
		}
	}
	return true
}
