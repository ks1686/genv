package schema

import (
	"bytes"
	"encoding/json"
	"fmt"
	"unicode"
)

func validateUnknownKeys(raw map[string]json.RawMessage, positions map[string]Position) []ValidationError {
	var errs []ValidationError
	errs = append(errs, rejectUnknown(raw, "", rootFields, positions)...)
	errs = append(errs, walkPackages(raw["packages"], "packages", positions)...)
	errs = append(errs, walkEnvMap(raw["env"], "env", positions)...)
	errs = append(errs, walkShell(raw["shell"], "shell", positions)...)
	errs = append(errs, walkServiceMap(raw["services"], "services", positions)...)
	errs = append(errs, walkFiles(raw["files"], "files", positions)...)
	errs = append(errs, walkHooks(raw["hooks"], "hooks", positions)...)
	errs = append(errs, walkObject(raw["repo"], "repo", repoFields, positions)...)
	errs = append(errs, walkObject(raw["updates"], "updates", updatesFields, positions)...)
	errs = append(errs, walkBundle(raw["defaults"], "defaults", positions)...)
	if targets, ok := asObject(raw["targets"]); ok {
		for name, body := range targets {
			errs = append(errs, walkBundle(body, "targets."+name, positions)...)
		}
	}
	return errs
}

var (
	rootFields = strSet(
		"$schema", "schemaVersion", "packages", "env", "shell", "services",
		"files", "hooks", "repo", "updates", "defaults", "targets",
	)
	packageFields = strSet("id", "version", "prefer", "managers", "host")
	envVarFields  = strSet("value", "sensitive")
	shellFields   = strSet("aliases", "functions", "source")
	aliasFields   = strSet("value", "shell")
	funcFields    = strSet("body", "shell")
	serviceFields = strSet("start", "stop", "restart", "status", "brew_formula", "host")
	filesFields   = strSet("links", "templates", "dirs")
	linkFields    = strSet("source", "target", "mode", "host", "backup", "perm")
	tmplFields    = strSet("source", "target", "host", "backup", "perm")
	dirFields     = strSet("target", "host", "perm")
	hooksFields   = strSet("preApply", "postApply", "preAdd", "postAdd", "preRemove", "postRemove", "preUpgrade", "postUpgrade")
	hookFields    = strSet("command", "file", "host", "name", "continueOnError")
	repoFields    = strSet("url", "ref")
	updatesFields = strSet("enabled", "interval", "autoApply", "notify", "onlyManagers", "skipManagers", "only", "skip")
	bundleFields  = strSet("packages", "env", "shell", "services", "files", "hooks")
)

func strSet(keys ...string) map[string]bool {
	m := make(map[string]bool, len(keys))
	for _, k := range keys {
		m[k] = true
	}
	return m
}

func rejectUnknown(obj map[string]json.RawMessage, path string, allowed map[string]bool, positions map[string]Position) []ValidationError {
	if obj == nil {
		return nil
	}
	var errs []ValidationError
	for key := range obj {
		if allowed[key] {
			continue
		}
		field := key
		if path != "" {
			field = path + "." + key
		}
		errs = append(errs, ValidationError{
			Position: positions[field],
			Field:    field,
			Message:  fmt.Sprintf("unknown field %q", key),
		})
	}
	return errs
}

func walkObject(raw json.RawMessage, path string, allowed map[string]bool, positions map[string]Position) []ValidationError {
	obj, ok := asObject(raw)
	if !ok {
		return nil
	}
	return rejectUnknown(obj, path, allowed, positions)
}

func walkBundle(raw json.RawMessage, path string, positions map[string]Position) []ValidationError {
	obj, ok := asObject(raw)
	if !ok {
		return nil
	}
	var errs []ValidationError
	errs = append(errs, rejectUnknown(obj, path, bundleFields, positions)...)
	errs = append(errs, walkPackages(obj["packages"], path+".packages", positions)...)
	errs = append(errs, walkEnvMap(obj["env"], path+".env", positions)...)
	errs = append(errs, walkShell(obj["shell"], path+".shell", positions)...)
	errs = append(errs, walkServiceMap(obj["services"], path+".services", positions)...)
	errs = append(errs, walkFiles(obj["files"], path+".files", positions)...)
	errs = append(errs, walkHooks(obj["hooks"], path+".hooks", positions)...)
	return errs
}

func walkPackages(raw json.RawMessage, path string, positions map[string]Position) []ValidationError {
	items, ok := asArray(raw)
	if !ok {
		return nil
	}
	var errs []ValidationError
	for i, item := range items {
		obj, ok := asObject(item)
		if !ok {
			continue
		}
		pkgPath := fmt.Sprintf("%s[%d]", path, i)
		errs = append(errs, rejectUnknown(obj, pkgPath, packageFields, positions)...)
	}
	return errs
}

func walkEnvMap(raw json.RawMessage, path string, positions map[string]Position) []ValidationError {
	return walkKeyedObjects(raw, path, envVarFields, positions)
}

func walkServiceMap(raw json.RawMessage, path string, positions map[string]Position) []ValidationError {
	return walkKeyedObjects(raw, path, serviceFields, positions)
}

func walkShell(raw json.RawMessage, path string, positions map[string]Position) []ValidationError {
	obj, ok := asObject(raw)
	if !ok {
		return nil
	}
	var errs []ValidationError
	errs = append(errs, rejectUnknown(obj, path, shellFields, positions)...)
	errs = append(errs, walkKeyedObjects(obj["aliases"], path+".aliases", aliasFields, positions)...)
	errs = append(errs, walkKeyedObjects(obj["functions"], path+".functions", funcFields, positions)...)
	return errs
}

func walkFiles(raw json.RawMessage, path string, positions map[string]Position) []ValidationError {
	obj, ok := asObject(raw)
	if !ok {
		return nil
	}
	var errs []ValidationError
	errs = append(errs, rejectUnknown(obj, path, filesFields, positions)...)
	errs = append(errs, walkObjectArray(obj["links"], path+".links", linkFields, positions)...)
	errs = append(errs, walkObjectArray(obj["templates"], path+".templates", tmplFields, positions)...)
	errs = append(errs, walkObjectArray(obj["dirs"], path+".dirs", dirFields, positions)...)
	return errs
}

func walkHooks(raw json.RawMessage, path string, positions map[string]Position) []ValidationError {
	obj, ok := asObject(raw)
	if !ok {
		return nil
	}
	var errs []ValidationError
	errs = append(errs, rejectUnknown(obj, path, hooksFields, positions)...)
	for _, phase := range []string{"preApply", "postApply", "preAdd", "postAdd", "preRemove", "postRemove", "preUpgrade", "postUpgrade"} {
		errs = append(errs, walkObjectArray(obj[phase], path+"."+phase, hookFields, positions)...)
	}
	return errs
}

func walkKeyedObjects(raw json.RawMessage, path string, allowed map[string]bool, positions map[string]Position) []ValidationError {
	obj, ok := asObject(raw)
	if !ok {
		return nil
	}
	var errs []ValidationError
	for key, val := range obj {
		if isJSONNull(val) {
			continue
		}
		errs = append(errs, walkObject(val, path+"."+key, allowed, positions)...)
	}
	return errs
}

func walkObjectArray(raw json.RawMessage, path string, allowed map[string]bool, positions map[string]Position) []ValidationError {
	items, ok := asArray(raw)
	if !ok {
		return nil
	}
	var errs []ValidationError
	for i, item := range items {
		errs = append(errs, walkObject(item, fmt.Sprintf("%s[%d]", path, i), allowed, positions)...)
	}
	return errs
}

func asObject(raw json.RawMessage) (map[string]json.RawMessage, bool) {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || raw[0] != '{' {
		return nil, false
	}
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(raw, &obj); err != nil {
		return nil, false
	}
	return obj, true
}

func asArray(raw json.RawMessage) ([]json.RawMessage, bool) {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || raw[0] != '[' {
		return nil, false
	}
	var items []json.RawMessage
	if err := json.Unmarshal(raw, &items); err != nil {
		return nil, false
	}
	return items, true
}

func isJSONNull(raw json.RawMessage) bool {
	return bytes.Equal(bytes.TrimSpace(raw), []byte("null"))
}

// ValidPackageName reports whether name is safe to pass as a package-manager
// operand: non-empty, no leading dash, and no whitespace or control characters.
func ValidPackageName(name string) bool {
	if name == "" || name[0] == '-' {
		return false
	}
	for _, r := range name {
		if r <= 0x1f || r == 0x7f || unicode.IsSpace(r) {
			return false
		}
	}
	return true
}
