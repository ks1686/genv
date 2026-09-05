package adapter

import (
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"sync"
	"unicode"
)

// CommandDef is the runtime form of a spec-level adapter. Fields match
// schema.AdapterDef; this package does not import schema so built-in
// adapters stay independent of spec parsing.
type CommandDef struct {
	List         string
	Install      string
	Remove       string
	Upgrade      string
	Version      string
	Outdated     string
	ListMatch    string
	IDField      string
	VersionField string
}

// Command is a spec-defined adapter. Install, remove, upgrade, and inventory
// all run the declared command templates. It is never a default fallback.
type Command struct {
	name string
	def  CommandDef
}

// NewCommand builds a spec adapter. name is the adapters map key (prefer value).
func NewCommand(name string, def CommandDef) Command {
	return Command{name: name, def: def}
}

func (c Command) Name() string { return c.name }

func (c Command) Available() bool {
	tmpl := c.def.List
	if strings.TrimSpace(tmpl) == "" {
		tmpl = c.def.Install
	}
	argv, err := splitCommand(expandTemplate(tmpl, "id"))
	if err != nil || len(argv) == 0 {
		return false
	}
	_, err = lookPath(argv[0])
	return err == nil
}

func (c Command) NormalizeID(id string, managers map[string]string) (string, bool) {
	return normalizeID(c.name, id, managers)
}

func (c Command) PlanInstall(pkgName string) []string {
	return planCommand(c.def.Install, pkgName)
}

func (c Command) PlanUninstall(pkgName string) []string {
	return planCommand(c.def.Remove, pkgName)
}

func (c Command) PlanUpgrade(pkgName string) []string {
	if strings.TrimSpace(c.def.Upgrade) == "" {
		return c.PlanInstall(pkgName)
	}
	return planCommand(c.def.Upgrade, pkgName)
}

func (c Command) PlanClean() [][]string { return nil }

func (c Command) Query(pkgName string) (bool, error) {
	names, err := c.ListInstalled()
	if err != nil {
		return false, err
	}
	for _, name := range names {
		if name == pkgName {
			return true, nil
		}
	}
	return false, nil
}

func (c Command) ListInstalled() ([]string, error) {
	entries, _, err := c.listEntries(c.def.List)
	return entries, err
}

func (c Command) QueryVersion(pkgName string) (string, error) {
	if strings.TrimSpace(c.def.Version) != "" {
		argv, err := splitCommand(expandTemplate(c.def.Version, pkgName))
		if err != nil || len(argv) == 0 {
			return "", err
		}
		return runVersionOutput(argv[0], argv[1:]...)
	}
	_, versions, err := c.listEntries(c.def.List)
	if err != nil {
		return "", err
	}
	return versions[pkgName], nil
}

func (c Command) ListInstalledVersions() (map[string]string, error) {
	names, versions, err := c.listEntries(c.def.List)
	if err != nil {
		return nil, err
	}
	if len(versions) > 0 {
		return versions, nil
	}
	out := make(map[string]string, len(names))
	for _, name := range names {
		out[name] = ""
	}
	return out, nil
}

// commandWithOutdated is a Command that can report outdated packages.
// Command itself does not implement OutdatedLister: without an outdated
// command, FilterOutdated keeps every tracked package (same as go/asdf).
type commandWithOutdated struct {
	Command
}

func (c commandWithOutdated) ListOutdated(pkgNames []string) (map[string]string, error) {
	names, versions, err := c.listEntries(c.def.Outdated)
	if err != nil {
		return nil, err
	}
	want := make(map[string]bool, len(pkgNames))
	for _, name := range pkgNames {
		want[name] = true
	}
	out := make(map[string]string)
	for _, name := range names {
		if len(want) > 0 && !want[name] {
			continue
		}
		out[name] = versions[name]
	}
	return out, nil
}

func (c Command) listEntries(tmpl string) ([]string, map[string]string, error) {
	argv, err := splitCommand(expandTemplate(tmpl, "id"))
	if err != nil {
		return nil, nil, err
	}
	if len(argv) == 0 {
		return nil, nil, nil
	}
	out, err := runProbe(argv[0], argv[1:]...)
	if err != nil {
		return nil, nil, classifyListErr(err)
	}
	return parseListOutput(out, c.def)
}

func classifyListErr(err error) error {
	if err == nil {
		return nil
	}
	type exitCoder interface{ ExitCode() int }
	if _, ok := err.(exitCoder); ok {
		return nil
	}
	return err
}

func planCommand(tmpl, pkgName string) []string {
	argv, err := splitCommand(expandTemplate(tmpl, pkgName))
	if err != nil {
		return nil
	}
	return argv
}

func expandTemplate(tmpl, id string) string {
	s := strings.ReplaceAll(tmpl, "{{id}}", id)
	return strings.ReplaceAll(s, "{{name}}", id)
}

// splitCommand tokenizes an argv template. Quotes are stripped; there is no
// shell expansion, piping, or redirection — wrap in `sh -c` if needed.
func splitCommand(s string) ([]string, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, fmt.Errorf("empty command")
	}
	var parts []string
	var buf strings.Builder
	var quote rune
	escaped := false
	flush := func() {
		if buf.Len() == 0 {
			return
		}
		parts = append(parts, buf.String())
		buf.Reset()
	}
	for _, r := range s {
		switch {
		case escaped:
			buf.WriteRune(r)
			escaped = false
		case r == '\\' && quote != '\'':
			escaped = true
		case quote != 0:
			if r == quote {
				quote = 0
			} else {
				buf.WriteRune(r)
			}
		case r == '"' || r == '\'':
			quote = r
		case unicode.IsSpace(r):
			flush()
		default:
			buf.WriteRune(r)
		}
	}
	if quote != 0 {
		return nil, fmt.Errorf("unclosed quote in command")
	}
	if escaped {
		return nil, fmt.Errorf("trailing backslash in command")
	}
	flush()
	if len(parts) == 0 {
		return nil, fmt.Errorf("empty command")
	}
	return parts, nil
}

func parseListOutput(out []byte, def CommandDef) ([]string, map[string]string, error) {
	trimmed := strings.TrimSpace(string(out))
	if trimmed == "" {
		return nil, nil, nil
	}
	if def.IDField != "" && looksLikeJSON(trimmed) {
		ids, versions, err := extractJSONIDs([]byte(trimmed), def.IDField, def.VersionField)
		if err == nil {
			return ids, versions, nil
		}
		if def.ListMatch == "" {
			return nil, nil, fmt.Errorf("parse list JSON: %w", err)
		}
	}
	if def.ListMatch != "" {
		return extractRegexIDs(trimmed, def.ListMatch)
	}
	return extractLineIDs(trimmed), nil, nil
}

func looksLikeJSON(s string) bool {
	return strings.HasPrefix(s, "{") || strings.HasPrefix(s, "[")
}

func extractLineIDs(s string) []string {
	var ids []string
	for _, line := range strings.Split(s, "\n") {
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		ids = append(ids, fields[0])
	}
	return ids
}

func extractRegexIDs(s, pattern string) ([]string, map[string]string, error) {
	re, err := regexp.Compile(pattern)
	if err != nil {
		return nil, nil, err
	}
	idIdx := 1
	verIdx := 0
	for i, name := range re.SubexpNames() {
		switch name {
		case "id", "name":
			idIdx = i
		case "version":
			verIdx = i
		}
	}
	matches := re.FindAllStringSubmatch(s, -1)
	var ids []string
	versions := make(map[string]string)
	seen := make(map[string]bool)
	for _, m := range matches {
		if idIdx >= len(m) || m[idIdx] == "" {
			continue
		}
		id := m[idIdx]
		if seen[id] {
			continue
		}
		seen[id] = true
		ids = append(ids, id)
		if verIdx > 0 && verIdx < len(m) && m[verIdx] != "" {
			versions[id] = m[verIdx]
		}
	}
	if len(versions) == 0 {
		versions = nil
	}
	return ids, versions, nil
}

func extractJSONIDs(data []byte, idField, versionField string) ([]string, map[string]string, error) {
	var v any
	if err := json.Unmarshal(data, &v); err != nil {
		return nil, nil, err
	}
	path := strings.Split(idField, ".")
	var ids []string
	versions := make(map[string]string)
	walkJSONIDs(v, path, versionField, &ids, versions)
	if len(versions) == 0 {
		versions = nil
	}
	return ids, versions, nil
}

func walkJSONIDs(v any, idPath []string, versionField string, ids *[]string, versions map[string]string) {
	switch x := v.(type) {
	case []any:
		for _, item := range x {
			walkJSONIDs(item, idPath, versionField, ids, versions)
		}
	case map[string]any:
		if len(idPath) > 1 {
			if next, ok := x[idPath[0]]; ok {
				walkJSONIDs(next, idPath[1:], versionField, ids, versions)
				return
			}
		}
		if id, ok := jsonStringField(x, idPath[len(idPath)-1]); ok {
			*ids = append(*ids, id)
			if versionField != "" {
				if ver, ok := jsonStringField(x, versionField); ok {
					versions[id] = ver
				}
			}
			return
		}
		for _, child := range x {
			walkJSONIDs(child, idPath, versionField, ids, versions)
		}
	}
}

func jsonStringField(obj map[string]any, key string) (string, bool) {
	v, ok := obj[key]
	if !ok || v == nil {
		return "", false
	}
	switch t := v.(type) {
	case string:
		if t == "" {
			return "", false
		}
		return t, true
	case json.Number:
		return t.String(), true
	case float64:
		return fmt.Sprintf("%g", t), true
	default:
		return fmt.Sprint(t), true
	}
}

var (
	specMu       sync.RWMutex
	specAdapters []Adapter
)

// SetSpecAdapters replaces the process-wide spec adapter set. Pass nil to clear.
// Commands call this after reading genv.json so Detect/ByName/scan see them.
func SetSpecAdapters(adapters []Adapter) {
	specMu.Lock()
	defer specMu.Unlock()
	if len(adapters) == 0 {
		specAdapters = nil
		return
	}
	specAdapters = append([]Adapter(nil), adapters...)
}

// CommandsFromDefs builds Command adapters from a name→def map, sorted by name.
func CommandsFromDefs(defs map[string]CommandDef) []Adapter {
	if len(defs) == 0 {
		return nil
	}
	names := make([]string, 0, len(defs))
	for name := range defs {
		names = append(names, name)
	}
	sort.Strings(names)
	out := make([]Adapter, 0, len(names))
	for _, name := range names {
		cmd := NewCommand(name, defs[name])
		if strings.TrimSpace(defs[name].Outdated) != "" {
			out = append(out, commandWithOutdated{Command: cmd})
		} else {
			out = append(out, cmd)
		}
	}
	return out
}

// Registered returns built-in adapters followed by the current spec adapters.
func Registered() []Adapter {
	specMu.RLock()
	defer specMu.RUnlock()
	out := make([]Adapter, 0, len(All)+len(specAdapters))
	out = append(out, All...)
	out = append(out, specAdapters...)
	return out
}

// SpecName reports whether name is a currently bound spec adapter.
func SpecName(name string) bool {
	specMu.RLock()
	defer specMu.RUnlock()
	for _, a := range specAdapters {
		if a.Name() == name {
			return true
		}
	}
	return false
}
