package genvfile

import (
	"bytes"
	"encoding/json"
	"reflect"
	"sort"
	"strings"

	"github.com/ks1686/genv/internal/schema"
)

// rewriteServicesInPlace patches only changed services objects in original.
// A service add would otherwise remarshal the whole spec and rewrite hook
// commands the user did not edit.
func rewriteServicesInPlace(original []byte, f *schema.GenvFile) ([]byte, bool) {
	before, valErrs, parseErr := schema.ParseAndValidate(original)
	if parseErr != nil || len(valErrs) > 0 || before == nil || f == nil {
		return nil, false
	}
	if !sameExceptServices(before, f) {
		return nil, false
	}
	paths := changedServicePaths(before, f)
	if len(paths) == 0 {
		return original, true
	}
	data := original
	for _, path := range paths {
		spans := locateJSONSpans(data)
		if spans == nil {
			return nil, false
		}
		want, ok := servicesWanted(f, path)
		if !ok {
			return nil, false
		}
		next, ok := applyServiceObjectChange(data, spans, path, want)
		if !ok {
			return nil, false
		}
		data = next
	}
	after, valErrs, parseErr := schema.ParseAndValidate(data)
	if parseErr != nil || len(valErrs) > 0 || after == nil {
		return nil, false
	}
	if !sameExceptServices(after, f) || !servicesMatch(after, f) {
		return nil, false
	}
	return data, true
}

// sameExceptServices is the service-edit counterpart of sameNonPackageState.
// Adapters are included: missing one would report success while dropping the edit.
func sameExceptServices(a, b *schema.GenvFile) bool {
	if a.SchemaVersion != b.SchemaVersion {
		return false
	}
	if !reflect.DeepEqual(a.Env, b.Env) ||
		!reflect.DeepEqual(a.Shell, b.Shell) ||
		!reflect.DeepEqual(a.Files, b.Files) ||
		!reflect.DeepEqual(a.Hooks, b.Hooks) ||
		!reflect.DeepEqual(a.Repo, b.Repo) ||
		!reflect.DeepEqual(a.Updates, b.Updates) ||
		!reflect.DeepEqual(a.Modules, b.Modules) ||
		!reflect.DeepEqual(a.Adapters, b.Adapters) ||
		!packageSliceEqual(a.Packages, b.Packages) {
		return false
	}
	if !bundleExceptServicesEqual(a.Defaults, b.Defaults) {
		return false
	}
	if len(a.Targets) != len(b.Targets) {
		return false
	}
	for id, bundle := range a.Targets {
		other, ok := b.Targets[id]
		if !ok || !bundleExceptServicesEqual(bundle, other) {
			return false
		}
	}
	return true
}

func bundleExceptServicesEqual(a, b *schema.TargetBundle) bool {
	if a == nil || b == nil {
		return a == b
	}
	return reflect.DeepEqual(a.UseModules, b.UseModules) &&
		packageSliceEqual(a.Packages, b.Packages) &&
		reflect.DeepEqual(a.Env, b.Env) &&
		reflect.DeepEqual(a.Shell, b.Shell) &&
		reflect.DeepEqual(a.Files, b.Files) &&
		reflect.DeepEqual(a.Hooks, b.Hooks)
}

func servicesMatch(a, b *schema.GenvFile) bool {
	if !reflect.DeepEqual(a.Services, b.Services) {
		return false
	}
	if !reflect.DeepEqual(bundleServiceMap(a.Defaults), bundleServiceMap(b.Defaults)) {
		return false
	}
	if len(a.Targets) != len(b.Targets) {
		return false
	}
	for id, bundle := range a.Targets {
		if !reflect.DeepEqual(bundleServiceMap(bundle), bundleServiceMap(b.Targets[id])) {
			return false
		}
	}
	return true
}

func bundleServiceMap(b *schema.TargetBundle) map[string]*schema.Service {
	if b == nil {
		return nil
	}
	return b.Services
}

func changedServicePaths(before, after *schema.GenvFile) []string {
	var paths []string
	if !reflect.DeepEqual(before.Services, after.Services) {
		paths = append(paths, "services")
	}
	if !reflect.DeepEqual(bundleServiceMap(before.Defaults), bundleServiceMap(after.Defaults)) {
		paths = append(paths, "defaults.services")
	}
	ids := make([]string, 0, len(before.Targets)+len(after.Targets))
	seen := map[string]bool{}
	for id := range before.Targets {
		seen[id] = true
		ids = append(ids, id)
	}
	for id := range after.Targets {
		if !seen[id] {
			ids = append(ids, id)
		}
	}
	sort.Strings(ids)
	for _, id := range ids {
		var beforeBundle, afterBundle *schema.TargetBundle
		if before.Targets != nil {
			beforeBundle = before.Targets[id]
		}
		if after.Targets != nil {
			afterBundle = after.Targets[id]
		}
		if !reflect.DeepEqual(bundleServiceMap(beforeBundle), bundleServiceMap(afterBundle)) {
			paths = append(paths, "targets."+id+".services")
		}
	}
	return paths
}

// serviceWant is one services-map value. null is a target tombstone.
type serviceWant struct {
	null bool
	svc  schema.Service
}

func servicesWanted(f *schema.GenvFile, path string) (map[string]serviceWant, bool) {
	switch {
	case path == "services":
		return legacyServiceWants(f.Services), true
	case path == "defaults.services":
		if f.Defaults == nil {
			return map[string]serviceWant{}, true
		}
		return portableServiceWants(f.Defaults.Services), true
	case strings.HasPrefix(path, "targets.") && strings.HasSuffix(path, ".services"):
		id := strings.TrimSuffix(strings.TrimPrefix(path, "targets."), ".services")
		bundle := f.Targets[id]
		if bundle == nil {
			return map[string]serviceWant{}, true
		}
		return portableServiceWants(bundle.Services), true
	default:
		return nil, false
	}
}

func legacyServiceWants(m map[string]schema.Service) map[string]serviceWant {
	out := make(map[string]serviceWant, len(m))
	for k, v := range m {
		out[k] = serviceWant{svc: v}
	}
	return out
}

func portableServiceWants(m map[string]*schema.Service) map[string]serviceWant {
	out := make(map[string]serviceWant, len(m))
	for k, v := range m {
		if v == nil {
			out[k] = serviceWant{null: true}
			continue
		}
		out[k] = serviceWant{svc: *v}
	}
	return out
}

func applyServiceObjectChange(data []byte, spans map[string]jsonSpan, path string, want map[string]serviceWant) ([]byte, bool) {
	objSpan, has := spans[path]
	if !has {
		if len(want) == 0 {
			return data, true
		}
		raw, ok := newServicesObject(data, spans, path, want)
		if !ok {
			return nil, false
		}
		return insertRawMember(data, spans, path, raw)
	}
	nl := detectNewline(data)
	pretty, keyIndent, closeIndent := objectStyle(data, objSpan)
	existing, ok := existingServiceEntries(data, spans, path, objSpan)
	if !ok {
		return nil, false
	}
	raw, err := renderServicesObject(existing, want, pretty, keyIndent, closeIndent, nl)
	if err != nil {
		return nil, false
	}
	return concat(data[:objSpan.start], raw, data[objSpan.end:]), true
}

func newServicesObject(data []byte, spans map[string]jsonSpan, path string, want map[string]serviceWant) ([]byte, bool) {
	parentSpan, ok := spans[parentPath(path)]
	if !ok {
		return nil, false
	}
	nl := detectNewline(data)
	pretty, childIndent, _ := objectStyle(data, parentSpan)
	keyIndent := ""
	closeIndent := ""
	if pretty {
		keyIndent = childIndent + "  "
		closeIndent = childIndent
	}
	raw, err := renderServicesObject(nil, want, pretty, keyIndent, closeIndent, nl)
	if err != nil {
		return nil, false
	}
	return raw, true
}

type svcEntry struct {
	key string
	raw []byte
}

func existingServiceEntries(data []byte, spans map[string]jsonSpan, path string, obj jsonSpan) ([]svcEntry, bool) {
	keys, ok := objectKeyOrder(data, obj)
	if !ok {
		return nil, false
	}
	out := make([]svcEntry, 0, len(keys))
	for _, key := range keys {
		span, ok := spans[path+"."+key]
		if !ok {
			return nil, false
		}
		out = append(out, svcEntry{key: key, raw: append([]byte(nil), data[span.start:span.end]...)})
	}
	return out, true
}

func objectKeyOrder(data []byte, span jsonSpan) ([]string, bool) {
	dec := json.NewDecoder(bytes.NewReader(data[span.start:span.end]))
	tok, err := dec.Token()
	if err != nil {
		return nil, false
	}
	if d, ok := tok.(json.Delim); !ok || d != '{' {
		return nil, false
	}
	var keys []string
	for dec.More() {
		keyTok, err := dec.Token()
		if err != nil {
			return nil, false
		}
		key, ok := keyTok.(string)
		if !ok {
			return nil, false
		}
		keys = append(keys, key)
		var skip json.RawMessage
		if err := dec.Decode(&skip); err != nil {
			return nil, false
		}
	}
	return keys, true
}

func renderServicesObject(existing []svcEntry, want map[string]serviceWant, pretty bool, keyIndent, closeIndent, nl string) ([]byte, error) {
	used := map[string]bool{}
	ordered := make([]svcEntry, 0, len(want))
	for _, e := range existing {
		w, ok := want[e.key]
		if !ok {
			continue
		}
		used[e.key] = true
		raw := e.raw
		if !serviceRawMatches(e.raw, w) {
			var err error
			raw, err = w.marshal(pretty, keyIndent, nl)
			if err != nil {
				return nil, err
			}
		}
		ordered = append(ordered, svcEntry{key: e.key, raw: raw})
	}
	extra := make([]string, 0, len(want))
	for k := range want {
		if !used[k] {
			extra = append(extra, k)
		}
	}
	sort.Strings(extra)
	for _, k := range extra {
		raw, err := want[k].marshal(pretty, keyIndent, nl)
		if err != nil {
			return nil, err
		}
		ordered = append(ordered, svcEntry{key: k, raw: raw})
	}
	return formatServicesObject(ordered, pretty, keyIndent, closeIndent, nl)
}

func (w serviceWant) marshal(pretty bool, contIndent, nl string) ([]byte, error) {
	if w.null {
		return []byte("null"), nil
	}
	return marshalInline(w.svc, pretty, contIndent, nl)
}

func serviceRawMatches(raw []byte, want serviceWant) bool {
	if want.null {
		return bytes.Equal(bytes.TrimSpace(raw), []byte("null"))
	}
	var got schema.Service
	if err := json.Unmarshal(raw, &got); err != nil {
		return false
	}
	return reflect.DeepEqual(got, want.svc)
}

func marshalInline(v any, pretty bool, contIndent, nl string) ([]byte, error) {
	indent := ""
	if pretty {
		indent = "  "
	}
	b, err := schema.EncodeJSON(v, indent)
	if err != nil {
		return nil, err
	}
	if !pretty {
		return b, nil
	}
	if nl == "\r\n" {
		b = bytes.ReplaceAll(b, []byte("\n"), []byte("\r\n"))
	}
	lines := bytes.Split(b, []byte(nl))
	var out bytes.Buffer
	for i, line := range lines {
		if i > 0 {
			out.WriteString(nl)
			out.WriteString(contIndent)
		}
		out.Write(line)
	}
	return out.Bytes(), nil
}

func formatServicesObject(entries []svcEntry, pretty bool, keyIndent, closeIndent, nl string) ([]byte, error) {
	type rendered struct {
		key []byte
		raw []byte
	}
	parts := make([]rendered, len(entries))
	for i, e := range entries {
		keyJSON, err := schema.EncodeJSON(e.key, "")
		if err != nil {
			return nil, err
		}
		parts[i] = rendered{key: keyJSON, raw: e.raw}
	}
	var b bytes.Buffer
	if !pretty {
		b.WriteByte('{')
		for i, part := range parts {
			if i > 0 {
				b.WriteByte(',')
			}
			b.Write(part.key)
			b.WriteByte(':')
			b.Write(part.raw)
		}
		b.WriteByte('}')
		return b.Bytes(), nil
	}
	b.WriteByte('{')
	if len(parts) == 0 {
		b.WriteByte('}')
		return b.Bytes(), nil
	}
	for i, part := range parts {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteString(nl)
		b.WriteString(keyIndent)
		b.Write(part.key)
		b.WriteString(": ")
		b.Write(part.raw)
	}
	b.WriteString(nl)
	b.WriteString(closeIndent)
	b.WriteByte('}')
	return b.Bytes(), nil
}

func insertRawMember(data []byte, spans map[string]jsonSpan, path string, value []byte) ([]byte, bool) {
	parentSpan, ok := spans[parentPath(path)]
	if !ok || parentSpan.end <= parentSpan.start || data[parentSpan.end-1] != '}' {
		return nil, false
	}
	key := path
	if i := strings.LastIndex(path, "."); i >= 0 {
		key = path[i+1:]
	}
	keyJSON, err := schema.EncodeJSON(key, "")
	if err != nil {
		return nil, false
	}
	nl := detectNewline(data)
	pretty, childIndent, closeIndent := objectStyle(data, parentSpan)
	member := append(append([]byte{}, keyJSON...), ':')
	if pretty {
		member = append(member, ' ')
	}
	member = append(member, value...)
	closeIdx := parentSpan.end - 1
	if isEmptyObject(data[parentSpan.start:parentSpan.end]) {
		var insert []byte
		if pretty {
			insert = []byte(nl + childIndent)
			insert = append(insert, member...)
			insert = append(insert, nl...)
			insert = append(insert, closeIndent...)
		} else {
			insert = member
		}
		return concat(data[:parentSpan.start+1], insert, data[closeIdx:]), true
	}
	last := closeIdx - 1
	for last > parentSpan.start && isWS(data[last]) {
		last--
	}
	var insert []byte
	if pretty {
		insert = []byte("," + nl + childIndent)
		insert = append(insert, member...)
		insert = append(insert, nl...)
		insert = append(insert, closeIndent...)
	} else {
		insert = append([]byte{','}, member...)
	}
	return concat(data[:last+1], insert, data[closeIdx:]), true
}
