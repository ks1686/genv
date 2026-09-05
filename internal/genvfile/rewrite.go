package genvfile

import (
	"bytes"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"

	"github.com/ks1686/genv/internal/schema"
)

type jsonSpan struct {
	start int
	end   int
}

// rewritePackagesInPlace patches only changed packages arrays in original.
// Unchanged JSON, including empty blocks and key order, is left intact.
func rewritePackagesInPlace(original []byte, f *schema.GenvFile) ([]byte, bool) {
	before, valErrs, parseErr := schema.ParseAndValidate(original)
	if parseErr != nil || len(valErrs) > 0 || before == nil || f == nil {
		return nil, false
	}
	if !sameNonPackageState(before, f) {
		return nil, false
	}
	paths := changedPackagePaths(before, f)
	if len(paths) == 0 {
		return original, true
	}
	data := original
	for _, path := range paths {
		spans := locateJSONSpans(data)
		if spans == nil {
			return nil, false
		}
		next, ok := applyPackageArrayChange(data, spans, path, packageListAt(f, path))
		if !ok {
			return nil, false
		}
		data = next
	}
	after, valErrs, parseErr := schema.ParseAndValidate(data)
	if parseErr != nil || len(valErrs) > 0 || after == nil {
		return nil, false
	}
	if !sameNonPackageState(after, f) || !packagesMatch(after, f) {
		return nil, false
	}
	return data, true
}

func sameNonPackageState(a, b *schema.GenvFile) bool {
	if a.SchemaVersion != b.SchemaVersion {
		return false
	}
	if !reflect.DeepEqual(a.Env, b.Env) ||
		!reflect.DeepEqual(a.Shell, b.Shell) ||
		!reflect.DeepEqual(a.Services, b.Services) ||
		!reflect.DeepEqual(a.Files, b.Files) ||
		!reflect.DeepEqual(a.Hooks, b.Hooks) ||
		!reflect.DeepEqual(a.Repo, b.Repo) ||
		!reflect.DeepEqual(a.Updates, b.Updates) {
		return false
	}
	if !bundleMetaEqual(a.Defaults, b.Defaults) {
		return false
	}
	if len(a.Targets) != len(b.Targets) {
		return false
	}
	for id, bundle := range a.Targets {
		other, ok := b.Targets[id]
		if !ok || !bundleMetaEqual(bundle, other) {
			return false
		}
	}
	return true
}

func bundleMetaEqual(a, b *schema.TargetBundle) bool {
	if a == nil || b == nil {
		return a == b
	}
	return reflect.DeepEqual(a.Env, b.Env) &&
		reflect.DeepEqual(a.Shell, b.Shell) &&
		reflect.DeepEqual(a.Services, b.Services) &&
		reflect.DeepEqual(a.Files, b.Files) &&
		reflect.DeepEqual(a.Hooks, b.Hooks)
}

func changedPackagePaths(before, after *schema.GenvFile) []string {
	var paths []string
	if !packageSliceEqual(before.Packages, after.Packages) {
		paths = append(paths, "packages")
	}
	if !packageSliceEqual(bundlePackages(before.Defaults), bundlePackages(after.Defaults)) {
		paths = append(paths, "defaults.packages")
	}
	seen := map[string]bool{}
	for id := range before.Targets {
		seen[id] = true
	}
	for id := range after.Targets {
		seen[id] = true
	}
	for id := range seen {
		if !packageSliceEqual(bundlePackages(before.Targets[id]), bundlePackages(after.Targets[id])) {
			paths = append(paths, "targets."+id+".packages")
		}
	}
	return paths
}

func bundlePackages(b *schema.TargetBundle) []schema.Package {
	if b == nil {
		return nil
	}
	return b.Packages
}

func packageListAt(f *schema.GenvFile, path string) []schema.Package {
	switch {
	case path == "packages":
		return f.Packages
	case path == "defaults.packages":
		return bundlePackages(f.Defaults)
	case strings.HasPrefix(path, "targets.") && strings.HasSuffix(path, ".packages"):
		id := strings.TrimSuffix(strings.TrimPrefix(path, "targets."), ".packages")
		return bundlePackages(f.Targets[id])
	default:
		return nil
	}
}

func packageSliceEqual(a, b []schema.Package) bool {
	return reflect.DeepEqual(a, b)
}

func packagesMatch(a, b *schema.GenvFile) bool {
	if !packageSliceEqual(a.Packages, b.Packages) {
		return false
	}
	if !packageSliceEqual(bundlePackages(a.Defaults), bundlePackages(b.Defaults)) {
		return false
	}
	if len(a.Targets) != len(b.Targets) {
		return false
	}
	for id, bundle := range a.Targets {
		if !packageSliceEqual(bundlePackages(bundle), bundlePackages(b.Targets[id])) {
			return false
		}
	}
	return true
}

func applyPackageArrayChange(data []byte, spans map[string]jsonSpan, path string, want []schema.Package) ([]byte, bool) {
	arraySpan, hasArray := spans[path]
	if !hasArray {
		if len(want) == 0 {
			return data, true
		}
		return insertPackagesKey(data, spans, path, want)
	}
	nl := detectNewline(data)
	pretty, elemIndent, closeIndent := arrayStyle(data, arraySpan, packageElemSpans(spans, path))
	existingIDs, existingRaw := packageElements(data, spans, path)
	if len(existingRaw) == 0 {
		if parent, ok := spans[parentPath(path)]; ok {
			if parentPretty, _, _ := objectStyle(data, parent); parentPretty {
				pretty = true
				closeIndent = lineIndent(data, arraySpan.start)
				elemIndent = closeIndent + "  "
			}
		}
	}
	array := formatPackageArray(existingIDs, existingRaw, want, pretty, elemIndent, closeIndent, nl)
	if array == nil {
		return nil, false
	}
	return concat(data[:arraySpan.start], array, data[arraySpan.end:]), true
}

func insertPackagesKey(data []byte, spans map[string]jsonSpan, path string, want []schema.Package) ([]byte, bool) {
	parent := parentPath(path)
	parentSpan, ok := spans[parent]
	if !ok || parentSpan.end <= parentSpan.start || data[parentSpan.end-1] != '}' {
		return nil, false
	}
	nl := detectNewline(data)
	pretty, childIndent, closeIndent := objectStyle(data, parentSpan)
	elemIndent := childIndent
	if pretty {
		elemIndent = childIndent + "  "
	}
	array := formatPackageArray(nil, nil, want, pretty, elemIndent, childIndent, nl)
	if array == nil {
		return nil, false
	}
	member := []byte(`"packages":`)
	if pretty {
		member = []byte(`"packages": `)
	}
	member = append(member, array...)
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

func formatPackageArray(existingIDs []string, existingRaw [][]byte, want []schema.Package, pretty bool, elemIndent, closeIndent, nl string) []byte {
	used := make([]bool, len(existingRaw))
	parts := make([][]byte, 0, len(want))
	for _, pkg := range want {
		if i := indexUnusedID(existingIDs, used, pkg.ID); i >= 0 {
			used[i] = true
			parts = append(parts, existingRaw[i])
			continue
		}
		raw, err := marshalPackage(pkg, pretty, elemIndent, nl)
		if err != nil {
			return nil
		}
		parts = append(parts, raw)
	}
	return joinArray(parts, pretty, elemIndent, closeIndent, nl)
}

func marshalPackage(pkg schema.Package, pretty bool, elemIndent, nl string) ([]byte, error) {
	if !pretty {
		return json.Marshal(pkg)
	}
	b, err := json.MarshalIndent(pkg, "", "  ")
	if err != nil {
		return nil, err
	}
	if nl == "\r\n" {
		b = bytes.ReplaceAll(b, []byte("\n"), []byte("\r\n"))
	}
	lines := bytes.Split(b, []byte(nl))
	var out bytes.Buffer
	for i, line := range lines {
		if i > 0 {
			out.WriteString(nl)
			out.WriteString(elemIndent)
		}
		out.Write(line)
	}
	return out.Bytes(), nil
}

func joinArray(elems [][]byte, pretty bool, elemIndent, closeIndent, nl string) []byte {
	var b bytes.Buffer
	if !pretty {
		b.WriteByte('[')
		for i, el := range elems {
			if i > 0 {
				b.WriteByte(',')
			}
			b.Write(el)
		}
		b.WriteByte(']')
		return b.Bytes()
	}
	b.WriteByte('[')
	if len(elems) == 0 {
		b.WriteByte(']')
		return b.Bytes()
	}
	b.WriteString(nl)
	for i, el := range elems {
		if i > 0 {
			b.WriteString("," + nl)
		}
		b.WriteString(elemIndent)
		b.Write(el)
	}
	b.WriteString(nl)
	b.WriteString(closeIndent)
	b.WriteByte(']')
	return b.Bytes()
}

func packageElements(data []byte, spans map[string]jsonSpan, path string) ([]string, [][]byte) {
	var ids []string
	var raw [][]byte
	for i := 0; ; i++ {
		span, ok := spans[fmt.Sprintf("%s[%d]", path, i)]
		if !ok {
			break
		}
		chunk := data[span.start:span.end]
		ids = append(ids, packageID(chunk))
		raw = append(raw, append([]byte(nil), chunk...))
	}
	return ids, raw
}

func packageElemSpans(spans map[string]jsonSpan, path string) []jsonSpan {
	var out []jsonSpan
	for i := 0; ; i++ {
		span, ok := spans[fmt.Sprintf("%s[%d]", path, i)]
		if !ok {
			return out
		}
		out = append(out, span)
	}
}

func packageID(raw []byte) string {
	var obj struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(raw, &obj); err != nil {
		return ""
	}
	return obj.ID
}

func indexUnusedID(ids []string, used []bool, id string) int {
	for i, existing := range ids {
		if !used[i] && existing == id {
			return i
		}
	}
	return -1
}

func parentPath(path string) string {
	if i := strings.LastIndex(path, "."); i >= 0 {
		return path[:i]
	}
	return ""
}

func arrayStyle(data []byte, arr jsonSpan, elems []jsonSpan) (pretty bool, elemIndent, closeIndent string) {
	pretty = bytes.Contains(data[arr.start:arr.end], []byte("\n"))
	closeIndent = lineIndent(data, arr.end-1)
	if len(elems) > 0 {
		return pretty, lineIndent(data, elems[0].start), closeIndent
	}
	if pretty {
		return true, closeIndent + "  ", closeIndent
	}
	return false, "", closeIndent
}

func objectStyle(data []byte, obj jsonSpan) (pretty bool, childIndent, closeIndent string) {
	pretty = bytes.Contains(data[obj.start:obj.end], []byte("\n"))
	closeIndent = lineIndent(data, obj.end-1)
	if !pretty {
		return false, "", closeIndent
	}
	i := obj.start + 1
	for i < obj.end && isWS(data[i]) {
		i++
	}
	if i < obj.end && data[i] == '"' {
		return true, lineIndent(data, i), closeIndent
	}
	return true, closeIndent + "  ", closeIndent
}

func isEmptyObject(raw []byte) bool {
	raw = bytes.TrimSpace(raw)
	return len(raw) >= 2 && raw[0] == '{' && raw[len(raw)-1] == '}' &&
		len(bytes.TrimSpace(raw[1:len(raw)-1])) == 0
}

func lineIndent(data []byte, pos int) string {
	if pos < 0 {
		pos = 0
	}
	if pos > len(data) {
		pos = len(data)
	}
	start := pos
	for start > 0 && data[start-1] != '\n' {
		start--
	}
	end := start
	for end < len(data) && (data[end] == ' ' || data[end] == '\t') {
		end++
	}
	return string(data[start:end])
}

func detectNewline(data []byte) string {
	if bytes.Contains(data, []byte("\r\n")) {
		return "\r\n"
	}
	return "\n"
}

func concat(parts ...[]byte) []byte {
	return bytes.Join(parts, nil)
}

func locateJSONSpans(data []byte) map[string]jsonSpan {
	dec := json.NewDecoder(bytes.NewReader(data))
	spans := make(map[string]jsonSpan)
	start := skipWS(data, 0)
	if start >= len(data) || data[start] != '{' {
		return nil
	}
	tok, err := dec.Token()
	if err != nil {
		return nil
	}
	if d, ok := tok.(json.Delim); !ok || d != '{' {
		return nil
	}
	if !walkObjectSpans(dec, data, "", spans) {
		return nil
	}
	spans[""] = jsonSpan{start: start, end: int(dec.InputOffset())}
	return spans
}

func walkObjectSpans(dec *json.Decoder, data []byte, path string, spans map[string]jsonSpan) bool {
	for n := 0; n <= len(data) && dec.More(); n++ {
		keyTok, err := dec.Token()
		if err != nil {
			return false
		}
		key, ok := keyTok.(string)
		if !ok {
			return false
		}
		child := key
		if path != "" {
			child = path + "." + key
		}
		valStart := skipColonValue(data, int(dec.InputOffset()))
		if !walkValueSpans(dec, data, child, valStart, spans) {
			return false
		}
	}
	_, err := dec.Token()
	return err == nil
}

func walkArraySpans(dec *json.Decoder, data []byte, path string, spans map[string]jsonSpan) bool {
	for i := 0; i <= len(data) && dec.More(); i++ {
		childStart := skipCommaWS(data, int(dec.InputOffset()))
		child := fmt.Sprintf("%s[%d]", path, i)
		if !walkValueSpans(dec, data, child, childStart, spans) {
			return false
		}
	}
	_, err := dec.Token()
	return err == nil
}

func walkValueSpans(dec *json.Decoder, data []byte, path string, start int, spans map[string]jsonSpan) bool {
	tok, err := dec.Token()
	if err != nil {
		return false
	}
	switch v := tok.(type) {
	case json.Delim:
		switch v {
		case '{':
			if !walkObjectSpans(dec, data, path, spans) {
				return false
			}
			spans[path] = jsonSpan{start: start, end: int(dec.InputOffset())}
			return true
		case '[':
			if !walkArraySpans(dec, data, path, spans) {
				return false
			}
			spans[path] = jsonSpan{start: start, end: int(dec.InputOffset())}
			return true
		}
	default:
		spans[path] = jsonSpan{start: start, end: int(dec.InputOffset())}
		return true
	}
	return false
}

func skipWS(data []byte, i int) int {
	for i < len(data) && isWS(data[i]) {
		i++
	}
	return i
}

func skipColonValue(data []byte, afterKey int) int {
	i := skipWS(data, afterKey)
	if i < len(data) && data[i] == ':' {
		i++
	}
	return skipWS(data, i)
}

func skipCommaWS(data []byte, from int) int {
	i := skipWS(data, from)
	if i < len(data) && data[i] == ',' {
		i++
		i = skipWS(data, i)
	}
	return i
}

func isWS(b byte) bool {
	return b == ' ' || b == '\n' || b == '\r' || b == '\t'
}
