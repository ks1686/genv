package schema

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"regexp"
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
	errs = append(errs, validateAdapters(f, raw, positions)...)
	errs = append(errs, validatePortable(f, positions)...)

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
	} else if versionRank(f.SchemaVersion) < 0 {
		errs = append(errs, ValidationError{
			Position: positions["schemaVersion"],
			Field:    "schemaVersion",
			Message:  fmt.Sprintf("unsupported version %q; expected one of %s", f.SchemaVersion, strings.Join(versionOrder, ", ")),
		})
	}
	return errs
}

func validatePackages(f *GenvFile, raw map[string]json.RawMessage, positions map[string]Position) []ValidationError {
	var errs []ValidationError
	if _, ok := raw["packages"]; !ok {
		// Schema v5 adds files/hooks/repo blocks and v6 adds updates; a spec may
		// legitimately contain only those blocks, so packages is optional there.
		if versionRank(f.SchemaVersion) < versionRank(Version5) {
			errs = append(errs, ValidationError{
				Field:   "packages",
				Message: "required field is missing",
			})
		}
	} else {
		errs = append(errs, validatePackageList(f, f.Packages, "packages", positions)...)
	}
	return errs
}

func validatePackageList(f *GenvFile, packages []Package, fieldPrefix string, positions map[string]Position) []ValidationError {
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

		if pkg.Prefer != "" && !KnownManager(f, pkg.Prefer) {
			errs = append(errs, ValidationError{
				Position: positions[pkgPath+".prefer"],
				Field:    pkgPath + ".prefer",
				Message:  fmt.Sprintf("unknown manager %q", pkg.Prefer),
			})
		}

		for mgr, pkgName := range pkg.Managers {
			field := fmt.Sprintf("%s.managers.%s", pkgPath, mgr)
			if !KnownManager(f, mgr) {
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
		if pkg.External != nil {
			errs = append(errs, validateExternalRecipe(pkg, pkgPath, f.SchemaVersion, positions)...)
		}
	}
	return errs
}

func validateExternalRecipe(pkg Package, pkgPath, schemaVersion string, positions map[string]Position) []ValidationError {
	field := pkgPath + ".external"
	if schemaVersion != Version9 {
		return []ValidationError{{Position: positions[field], Field: field, Message: "managed external recipe requires schemaVersion \"9\""}}
	}
	var errs []ValidationError
	if pkg.Prefer != "external" {
		errs = append(errs, ValidationError{Position: positions[field], Field: field, Message: "managed external recipe requires prefer \"external\""})
	}
	r := pkg.External
	if len(r.Detect.Command) == 0 || r.Detect.Command[0] == "" {
		errs = append(errs, externalValidation(field+".detect.command", "at least one non-empty argv element is required", positions))
	}
	errs = append(errs, validateCaptureRegex(r.Detect.VersionRegex, field+".detect.versionRegex", true, positions)...)
	errs = append(errs, validateExternalSource(r.Source, field+".source", r.AllowInsecureHTTP, positions)...)
	if len(r.Platforms) == 0 {
		errs = append(errs, externalValidation(field+".platforms", "at least one platform is required", positions))
	}
	for i, platform := range r.Platforms {
		errs = append(errs, validateExternalPlatform(platform, fmt.Sprintf("%s.platforms[%d]", field, i), r.Source.Type, r.AllowInsecureHTTP, positions)...)
	}
	if len(r.Verify) == 0 && !r.AllowUnverified {
		errs = append(errs, externalValidation(field+".verify", "verify is required unless allowUnverified is true", positions))
	}
	for i, verify := range r.Verify {
		errs = append(errs, validateExternalVerification(verify, fmt.Sprintf("%s.verify[%d]", field, i), positions)...)
	}
	return errs
}

func validateExternalSource(source ExternalSource, field string, allowInsecure bool, positions map[string]Position) []ValidationError {
	var errs []ValidationError
	switch source.Type {
	case "githubRelease":
		parts := strings.Split(source.Repository, "/")
		if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
			errs = append(errs, externalValidation(field+".repository", "repository must be owner/name", positions))
		}
		if source.Release != "" && source.Release != "stable" && source.Release != "prerelease" && source.Release != "any" {
			errs = append(errs, externalValidation(field+".release", "release must be stable, prerelease, or any", positions))
		}
		if source.VersionURL != "" || source.Format != "" || source.VersionPointer != "" || source.VersionRegex != "" {
			errs = append(errs, externalValidation(field, "githubRelease source cannot contain HTTP release fields", positions))
		}
		errs = append(errs, validateCaptureRegex(source.TagRegex, field+".tagRegex", false, positions)...)
		if source.APIBase != "" {
			errs = append(errs, validateExternalURL(source.APIBase, field+".apiBase", allowInsecure, positions)...)
		}
	case "httpRelease":
		errs = append(errs, validateExternalURL(source.VersionURL, field+".versionURL", allowInsecure, positions)...)
		if source.Repository != "" || source.Release != "" || source.TagRegex != "" || source.APIBase != "" {
			errs = append(errs, externalValidation(field, "httpRelease source cannot contain GitHub release fields", positions))
		}
		switch source.Format {
		case "json":
			if source.VersionPointer == "" || source.VersionRegex != "" {
				errs = append(errs, externalValidation(field, "json source requires versionPointer and forbids versionRegex", positions))
			}
		case "text":
			if source.VersionPointer != "" {
				errs = append(errs, externalValidation(field, "text source forbids versionPointer", positions))
			}
			errs = append(errs, validateCaptureRegex(source.VersionRegex, field+".versionRegex", true, positions)...)
		default:
			errs = append(errs, externalValidation(field+".format", "format must be json or text", positions))
		}
	default:
		errs = append(errs, externalValidation(field+".type", "source type must be githubRelease or httpRelease", positions))
	}
	return errs
}

func validateExternalPlatform(platform ExternalPlatform, field, sourceType string, allowInsecure bool, positions map[string]Position) []ValidationError {
	var errs []ValidationError
	if len(platform.OS) == 0 {
		errs = append(errs, externalValidation(field+".os", "at least one os is required", positions))
	}
	for _, value := range platform.OS {
		if value != "linux" && value != "darwin" && value != "windows" {
			errs = append(errs, externalValidation(field+".os", fmt.Sprintf("unknown os %q", value), positions))
		}
	}
	if len(platform.Arch) == 0 {
		errs = append(errs, externalValidation(field+".arch", "at least one arch is required", positions))
	}
	for _, value := range platform.Arch {
		if value != "amd64" && value != "arm64" {
			errs = append(errs, externalValidation(field+".arch", fmt.Sprintf("unknown arch %q", value), positions))
		}
	}
	for _, value := range platform.Libc {
		if value != "glibc" && value != "musl" {
			errs = append(errs, externalValidation(field+".libc", fmt.Sprintf("unknown libc %q", value), positions))
		}
	}
	if sourceType == "githubRelease" {
		if platform.AssetRegex == "" || platform.ArtifactURL != "" {
			errs = append(errs, externalValidation(field, "GitHub platform requires assetRegex and forbids artifactURL", positions))
		} else {
			errs = append(errs, validateRegex(platform.AssetRegex, field+".assetRegex", positions)...)
		}
	} else if sourceType == "httpRelease" {
		if platform.ArtifactURL == "" || platform.AssetRegex != "" {
			errs = append(errs, externalValidation(field, "HTTP platform requires artifactURL and forbids assetRegex", positions))
		} else {
			errs = append(errs, validateExternalURL(platform.ArtifactURL, field+".artifactURL", allowInsecure, positions)...)
		}
	}
	errs = append(errs, validateExternalInstall(platform.Install, field+".install", positions)...)
	return errs
}

func validateExternalInstall(install ExternalInstall, field string, positions map[string]Position) []ValidationError {
	var errs []ValidationError
	if install.Scope != "" && install.Scope != "user" && install.Scope != "system" {
		errs = append(errs, externalValidation(field+".scope", "scope must be user or system", positions))
	}
	switch install.Type {
	case "direct":
		if install.Destination == "" {
			errs = append(errs, externalValidation(field+".destination", "destination is required for direct install", positions))
		}
		if len(install.Files) > 0 || install.StripComponents != 0 || install.Interpreter != "" || len(install.Args) > 0 || len(install.Env) > 0 || len(install.Uninstall) > 0 {
			errs = append(errs, externalValidation(field, "direct install contains fields for another install type", positions))
		}
	case "archive":
		if len(install.Files) == 0 {
			errs = append(errs, externalValidation(field+".files", "at least one file is required for archive install", positions))
		}
		for i, file := range install.Files {
			if file.From == "" || file.To == "" {
				errs = append(errs, externalValidation(fmt.Sprintf("%s.files[%d]", field, i), "from and to are required", positions))
			}
			errs = append(errs, validateExternalTemplate(file.To, fmt.Sprintf("%s.files[%d].to", field, i), positions)...)
		}
		if install.Destination != "" || install.Interpreter != "" || len(install.Args) > 0 || len(install.Env) > 0 || len(install.Uninstall) > 0 {
			errs = append(errs, externalValidation(field, "archive install contains fields for another install type", positions))
		}
	case "script":
		if install.Interpreter != "sh" && install.Interpreter != "bash" && install.Interpreter != "pwsh" && install.Interpreter != "powershell" {
			errs = append(errs, externalValidation(field+".interpreter", "interpreter must be sh, bash, pwsh, or powershell", positions))
		}
		if len(install.Files) > 0 || install.StripComponents != 0 {
			errs = append(errs, externalValidation(field, "script install contains archive fields", positions))
		}
		if len(install.Uninstall) > 0 && (install.Uninstall[0] == "sh" || install.Uninstall[0] == "bash" || install.Uninstall[0] == "pwsh" || install.Uninstall[0] == "powershell") {
			errs = append(errs, externalValidation(field+".uninstall", "uninstall must be explicit argv, not a shell interpreter", positions))
		}
		for i, value := range install.Args {
			errs = append(errs, validateExternalTemplate(value, fmt.Sprintf("%s.args[%d]", field, i), positions)...)
		}
		for key, value := range install.Env {
			if matched, _ := regexp.MatchString(`^[A-Za-z_][A-Za-z0-9_]*$`, key); !matched {
				errs = append(errs, externalValidation(field+".env", fmt.Sprintf("invalid environment variable name %q", key), positions))
			}
			errs = append(errs, validateExternalTemplate(value, field+".env."+key, positions)...)
		}
		for i, value := range install.Uninstall {
			if value == "" {
				errs = append(errs, externalValidation(fmt.Sprintf("%s.uninstall[%d]", field, i), "uninstall argv entries cannot be empty", positions))
			}
			errs = append(errs, validateExternalTemplate(value, fmt.Sprintf("%s.uninstall[%d]", field, i), positions)...)
		}
	default:
		errs = append(errs, externalValidation(field+".type", "install type must be direct, archive, or script", positions))
	}
	if install.Destination != "" {
		errs = append(errs, validateExternalTemplate(install.Destination, field+".destination", positions)...)
	}
	return errs
}

func validateExternalTemplate(value, field string, positions map[string]Position) []ValidationError {
	re := regexp.MustCompile(`\{([A-Za-z][A-Za-z0-9]*)\}`)
	allowed := map[string]bool{"version": true, "tag": true, "os": true, "arch": true, "script": true, "destination": true}
	var errs []ValidationError
	for _, match := range re.FindAllStringSubmatch(value, -1) {
		if !allowed[match[1]] {
			errs = append(errs, externalValidation(field, fmt.Sprintf("unknown template placeholder %q", match[1]), positions))
		}
	}
	withoutPlaceholders := re.ReplaceAllString(value, "")
	if strings.ContainsAny(withoutPlaceholders, "{}") {
		errs = append(errs, externalValidation(field, "invalid template syntax", positions))
	}
	return errs
}

func validateExternalVerification(verify ExternalVerification, field string, positions map[string]Position) []ValidationError {
	var errs []ValidationError
	switch verify.Type {
	case "githubDigest":
	case "sha256":
		if verify.Value == "" && verify.ValuePointer == "" {
			errs = append(errs, externalValidation(field, "sha256 requires value or valuePointer", positions))
		}
	case "sha256File":
		if verify.AssetRegex == "" && verify.URL == "" {
			errs = append(errs, externalValidation(field, "sha256File requires assetRegex or url", positions))
		}
	case "sigstore":
		if verify.Identity == "" || verify.Issuer == "" || (verify.BundleAssetRegex == "" && verify.URL == "") {
			errs = append(errs, externalValidation(field, "sigstore requires identity, issuer, and bundle asset or URL", positions))
		}
	case "minisign":
		if (verify.PublicKey == "") == (verify.PublicKeyFile == "") || (verify.SignatureAssetRegex == "" && verify.URL == "") {
			errs = append(errs, externalValidation(field, "minisign requires exactly one public key source and a signature asset", positions))
		}
	case "openpgp":
		if (verify.PublicKey == "") == (verify.PublicKeyFile == "") || (verify.SignatureAssetRegex == "" && verify.URL == "") || len(verify.Fingerprint) < 40 {
			errs = append(errs, externalValidation(field, "openpgp requires exactly one public key source, a signature asset, and full fingerprint", positions))
		}
	default:
		errs = append(errs, externalValidation(field+".type", "unknown verification type", positions))
	}
	return errs
}

func validateCaptureRegex(expr, field string, required bool, positions map[string]Position) []ValidationError {
	if expr == "" {
		if required {
			return []ValidationError{externalValidation(field, "required field is missing or empty", positions)}
		}
		return nil
	}
	re, err := regexp.Compile(expr)
	if err != nil {
		return []ValidationError{externalValidation(field, fmt.Sprintf("invalid regex: %v", err), positions)}
	}
	if re.NumSubexp() != 1 {
		return []ValidationError{externalValidation(field, "regex must contain exactly one capture group", positions)}
	}
	return nil
}

func validateRegex(expr, field string, positions map[string]Position) []ValidationError {
	if _, err := regexp.Compile(expr); err != nil {
		return []ValidationError{externalValidation(field, fmt.Sprintf("invalid regex: %v", err), positions)}
	}
	return nil
}

func validateExternalURL(value, field string, allowInsecure bool, positions map[string]Position) []ValidationError {
	u, err := url.Parse(value)
	if err != nil || u.Host == "" || (u.Scheme != "https" && !(allowInsecure && u.Scheme == "http")) {
		return []ValidationError{externalValidation(field, "must be an absolute HTTPS URL", positions)}
	}
	return nil
}

func externalValidation(field, message string, positions map[string]Position) ValidationError {
	return ValidationError{Position: positions[field], Field: field, Message: message}
}

func validateEnv(f *GenvFile, raw map[string]json.RawMessage, positions map[string]Position) []ValidationError {
	var errs []ValidationError
	if _, hasEnv := raw["env"]; hasEnv {
		if versionRank(f.SchemaVersion) < versionRank(Version2) {
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
		if versionRank(f.SchemaVersion) < versionRank(Version3) {
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
		if versionRank(f.SchemaVersion) < versionRank(Version4) {
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
	hasSupervisor := svc.DeclaresLaunchd() || svc.DeclaresSystemd()
	if len(svc.Start) == 0 && svc.BrewFormula == "" && !hasSupervisor {
		errs = append(errs, ValidationError{
			Field:   fmt.Sprintf("%s.%s.start", fieldPrefix, name),
			Message: "start command is required (or set brew_formula, launchd.plist, or systemd.unit)",
		})
	}
	if svc.BrewFormula != "" && len(svc.Start) > 0 {
		errs = append(errs, ValidationError{
			Field:   fmt.Sprintf("%s.%s", fieldPrefix, name),
			Message: "brew_formula and start are mutually exclusive; use one or the other",
		})
	}
	if hasSupervisor && (len(svc.Start) > 0 || svc.BrewFormula != "") {
		errs = append(errs, ValidationError{
			Field:   fmt.Sprintf("%s.%s", fieldPrefix, name),
			Message: "launchd/systemd templates are mutually exclusive with start and brew_formula",
		})
	}
	if strings.ContainsAny(svc.BrewFormula, "\r\n") {
		errs = append(errs, ValidationError{
			Field:   fmt.Sprintf("%s.%s.brew_formula", fieldPrefix, name),
			Message: "brew_formula must not contain newlines",
		})
	}
	if svc.Launchd != nil {
		errs = append(errs, validateServicePath(fieldPrefix, name, "launchd.plist", svc.Launchd.Plist, true)...)
	}
	if svc.Systemd != nil {
		errs = append(errs, validateServicePath(fieldPrefix, name, "systemd.unit", svc.Systemd.Unit, true)...)
	}
	errs = append(errs, validateServiceCommand(fieldPrefix, name, "start", svc.Start)...)
	errs = append(errs, validateServiceCommand(fieldPrefix, name, "stop", svc.Stop)...)
	errs = append(errs, validateServiceCommand(fieldPrefix, name, "restart", svc.Restart)...)
	errs = append(errs, validateServiceCommand(fieldPrefix, name, "status", svc.Status)...)
	return errs
}

func validateServicePath(fieldPrefix, name, field, path string, required bool) []ValidationError {
	var errs []ValidationError
	loc := fmt.Sprintf("%s.%s.%s", fieldPrefix, name, field)
	if path == "" {
		if required {
			errs = append(errs, ValidationError{Field: loc, Message: "path must not be empty"})
		}
		return errs
	}
	if strings.ContainsAny(path, "\r\n") {
		errs = append(errs, ValidationError{Field: loc, Message: "path must not contain newlines"})
	}
	if expanded, err := expandPath(path); err != nil || expanded == "" {
		msg := "cannot expand path"
		if err != nil {
			msg = fmt.Sprintf("%s: %v", msg, err)
		}
		errs = append(errs, ValidationError{Field: loc, Message: msg})
	}
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
		if versionRank(f.SchemaVersion) < versionRank(Version5) {
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
		errs = append(errs, validateFilePerm(l.Perm, field)...)
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
		errs = append(errs, validateFilePerm(tpl.Perm, field)...)
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
		errs = append(errs, validateFilePerm(d.Perm, field)...)
	}
	return errs
}

func validateHooks(f *GenvFile, raw map[string]json.RawMessage, positions map[string]Position) []ValidationError {
	var errs []ValidationError
	if _, hasHooks := raw["hooks"]; hasHooks {
		if versionRank(f.SchemaVersion) < versionRank(Version5) {
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
	if versionRank(f.SchemaVersion) < versionRank(Version6) {
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
		if versionRank(f.SchemaVersion) < versionRank(Version5) {
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
	if versionRank(f.SchemaVersion) < versionRank(Version6) {
		errs = append(errs, ValidationError{
			Position: positions["updates"],
			Field:    "updates",
			Message:  fmt.Sprintf("updates block requires schemaVersion %q or newer (current: %q); bump schemaVersion to %q to use the updates config", Version6, f.SchemaVersion, Version6),
		})
	}
	if f.Updates == nil {
		return errs
	}
	errs = append(errs, validateUpdatesManagers(f, "updates.onlyManagers", f.Updates.OnlyManagers, positions)...)
	errs = append(errs, validateUpdatesManagers(f, "updates.skipManagers", f.Updates.SkipManagers, positions)...)
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

func validateUpdatesManagers(f *GenvFile, field string, managers []string, positions map[string]Position) []ValidationError {
	var errs []ValidationError
	for i, mgr := range managers {
		if KnownManager(f, mgr) {
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

func validateAdapters(f *GenvFile, raw map[string]json.RawMessage, positions map[string]Position) []ValidationError {
	var errs []ValidationError
	if _, hasAdapters := raw["adapters"]; !hasAdapters {
		return errs
	}
	if !IsPortableVersion(f.SchemaVersion) {
		errs = append(errs, ValidationError{
			Position: positions["adapters"],
			Field:    "adapters",
			Message:  fmt.Sprintf("adapters block requires schemaVersion %q or %q (current: %q)", Version8, Version9, f.SchemaVersion),
		})
	}
	if len(f.Adapters) == 0 {
		return errs
	}
	for name, def := range f.Adapters {
		path := "adapters." + name
		if !ValidAdapterName(name) {
			errs = append(errs, ValidationError{
				Position: positions[path],
				Field:    path,
				Message:  fmt.Sprintf("invalid adapter name %q: must be lowercase letters, digits, and hyphens, starting with a letter", name),
			})
		}
		if KnownManagers[name] {
			errs = append(errs, ValidationError{
				Position: positions[path],
				Field:    path,
				Message:  fmt.Sprintf("adapter name %q collides with a built-in manager", name),
			})
		}
		if strings.TrimSpace(def.List) == "" {
			errs = append(errs, ValidationError{
				Position: positions[path+".list"],
				Field:    path + ".list",
				Message:  "required field is missing or empty",
			})
		}
		if strings.TrimSpace(def.Install) == "" {
			errs = append(errs, ValidationError{
				Position: positions[path+".install"],
				Field:    path + ".install",
				Message:  "required field is missing or empty",
			})
		}
		if strings.TrimSpace(def.Remove) == "" {
			errs = append(errs, ValidationError{
				Position: positions[path+".remove"],
				Field:    path + ".remove",
				Message:  "required field is missing or empty",
			})
		}
		if def.ListMatch != "" {
			if _, err := regexp.Compile(def.ListMatch); err != nil {
				errs = append(errs, ValidationError{
					Position: positions[path+".listMatch"],
					Field:    path + ".listMatch",
					Message:  fmt.Sprintf("invalid listMatch regexp: %v", err),
				})
			}
		}
	}
	return errs
}

func validatePortable(f *GenvFile, positions map[string]Position) []ValidationError {
	if !IsPortableVersion(f.SchemaVersion) {
		return nil
	}

	var errs []ValidationError
	if len(f.Packages) > 0 {
		errs = append(errs, ValidationError{
			Position: positions["packages"],
			Field:    "packages",
			Message:  fmt.Sprintf("top-level packages are not allowed in schemaVersion %q; use targets.<target>.packages", f.SchemaVersion),
		})
	}
	if f.Env != nil {
		errs = append(errs, ValidationError{
			Position: positions["env"],
			Field:    "env",
			Message:  fmt.Sprintf("top-level env is not allowed in schemaVersion %q; use defaults.env or targets.<target>.env", f.SchemaVersion),
		})
	}
	if f.Shell != nil {
		errs = append(errs, ValidationError{
			Position: positions["shell"],
			Field:    "shell",
			Message:  fmt.Sprintf("top-level shell is not allowed in schemaVersion %q; use defaults.shell or targets.<target>.shell", f.SchemaVersion),
		})
	}
	if f.Files != nil {
		errs = append(errs, ValidationError{
			Position: positions["files"],
			Field:    "files",
			Message:  fmt.Sprintf("top-level files are not allowed in schemaVersion %q; use defaults.files or targets.<target>.files", f.SchemaVersion),
		})
	}
	if f.Services != nil {
		errs = append(errs, ValidationError{
			Position: positions["services"],
			Field:    "services",
			Message:  fmt.Sprintf("top-level services are not allowed in schemaVersion %q; use defaults.services or targets.<target>.services", f.SchemaVersion),
		})
	}
	if f.Hooks != nil {
		errs = append(errs, ValidationError{
			Position: positions["hooks"],
			Field:    "hooks",
			Message:  fmt.Sprintf("top-level hooks are not allowed in schemaVersion %q; use defaults.hooks or targets.<target>.hooks", f.SchemaVersion),
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
	errs = append(errs, validatePackageList(f, bundle.Packages, fieldPrefix+".packages", positions)...)
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
