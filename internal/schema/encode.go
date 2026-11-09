package schema

import (
	"bytes"
	"encoding/json"
)

// EncodeJSON serializes v without HTML-escaping &, <, or >.
//
// encoding/json escapes those bytes so a document can be embedded in HTML.
// A genv spec is a source file: shell hooks use the characters literally, and
// escaping them rewrites lines the command was not editing. indent is the
// SetIndent string; empty means compact. The result has no trailing newline.
// Callers that persist a spec append one, matching the historical
// MarshalIndent-plus-newline layout.
func EncodeJSON(v any, indent string) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if indent != "" {
		enc.SetIndent("", indent)
	}
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	return bytes.TrimSuffix(buf.Bytes(), []byte("\n")), nil
}
