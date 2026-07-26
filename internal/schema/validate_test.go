package schema

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseAndValidate_Valid(t *testing.T) {
	tests := []struct {
		name string
		json string
	}{
		{
			name: "minimal empty packages",
			json: `{"schemaVersion":"1","packages":[]}`,
		},
		{
			name: "single package id only",
			json: `{"schemaVersion":"1","packages":[{"id":"git"}]}`,
		},
		{
			name: "full package with all fields",
			json: `{
				"schemaVersion": "1",
				"packages": [
					{
						"id": "neovim",
						"version": "0.10.*",
						"prefer": "brew"
					},
					{
						"id": "firefox",
						"version": "*",
						"managers": {
							"snap": "firefox",
							"brew": "firefox"
						}
					}
				]
			}`,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			f, errs, err := ParseAndValidate([]byte(tc.json))
			if err != nil {
				t.Fatalf("unexpected fatal error: %v", err)
			}
			if len(errs) > 0 {
				t.Fatalf("unexpected validation errors: %v", errs)
			}
			if f == nil {
				t.Fatal("expected non-nil GenvFile")
			}
		})
	}
}

func TestParseAndValidate_MissingRequiredFields(t *testing.T) {
	tests := []struct {
		name      string
		json      string
		wantField string
	}{
		{
			name:      "missing schemaVersion",
			json:      `{"packages":[]}`,
			wantField: "schemaVersion",
		},
		{
			name:      "missing packages",
			json:      `{"schemaVersion":"1"}`,
			wantField: "packages",
		},
		{
			name:      "package missing id",
			json:      `{"schemaVersion":"1","packages":[{"version":"*"}]}`,
			wantField: "packages[0].id",
		},
		{
			name:      "package empty id",
			json:      `{"schemaVersion":"1","packages":[{"id":""}]}`,
			wantField: "packages[0].id",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, errs, err := ParseAndValidate([]byte(tc.json))
			if err != nil {
				t.Fatalf("unexpected fatal error: %v", err)
			}
			if len(errs) == 0 {
				t.Fatal("expected at least one validation error, got none")
			}
			found := false
			for _, e := range errs {
				if e.Field == tc.wantField {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("expected error for field %q, got: %v", tc.wantField, errs)
			}
		})
	}
}

func TestParseAndValidate_WrongSchemaVersion(t *testing.T) {
	// Use "99" — both "1" and "2" are now accepted.
	input := "{\n  \"schemaVersion\": \"99\",\n  \"packages\": []\n}"
	_, errs, err := ParseAndValidate([]byte(input))
	if err != nil {
		t.Fatalf("unexpected fatal error: %v", err)
	}
	if len(errs) == 0 {
		t.Fatal("expected validation error for wrong schemaVersion")
	}
	e := errs[0]
	if e.Field != "schemaVersion" {
		t.Errorf("expected field %q, got %q", "schemaVersion", e.Field)
	}
	if !strings.Contains(e.Message, "99") {
		t.Errorf("expected message to mention the bad value, got: %q", e.Message)
	}
	if e.Line != 2 {
		t.Errorf("expected line 2, got %d", e.Line)
	}
}

func TestParseAndValidate_DuplicateID(t *testing.T) {
	input := `{"schemaVersion":"1","packages":[{"id":"git"},{"id":"git"}]}`
	_, errs, err := ParseAndValidate([]byte(input))
	if err != nil {
		t.Fatalf("unexpected fatal error: %v", err)
	}
	if len(errs) == 0 {
		t.Fatal("expected duplicate-id validation error")
	}
	found := false
	for _, e := range errs {
		if e.Field == "packages[1].id" && strings.Contains(e.Message, "duplicate") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected duplicate error on packages[1].id, got: %v", errs)
	}
}

func TestParseAndValidate_UnknownPrefer(t *testing.T) {
	input := `{"schemaVersion":"1","packages":[{"id":"git","prefer":"yum"}]}`
	_, errs, err := ParseAndValidate([]byte(input))
	if err != nil {
		t.Fatalf("unexpected fatal error: %v", err)
	}
	if len(errs) == 0 {
		t.Fatal("expected validation error for unknown prefer")
	}
	e := errs[0]
	if e.Field != "packages[0].prefer" {
		t.Errorf("expected field packages[0].prefer, got %q", e.Field)
	}
	if !strings.Contains(e.Message, "yum") {
		t.Errorf("expected message to mention the bad value, got: %q", e.Message)
	}
}

func TestParseAndValidate_UnknownManagerInMap(t *testing.T) {
	input := `{"schemaVersion":"1","packages":[{"id":"git","managers":{"yum":"git"}}]}`
	_, errs, err := ParseAndValidate([]byte(input))
	if err != nil {
		t.Fatalf("unexpected fatal error: %v", err)
	}
	if len(errs) == 0 {
		t.Fatal("expected validation error for unknown manager key")
	}
	found := false
	for _, e := range errs {
		if e.Field == "packages[0].managers.yum" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected error on packages[0].managers.yum, got: %v", errs)
	}
}

func TestParseAndValidate_SyntaxError(t *testing.T) {
	input := `{"schemaVersion": "1", "packages": [`
	_, errs, err := ParseAndValidate([]byte(input))
	if err == nil {
		t.Fatalf("expected fatal error for malformed JSON, got errs=%v", errs)
	}
	if !strings.Contains(err.Error(), "line") {
		t.Errorf("expected error to contain line info, got: %v", err)
	}
}

func TestParseAndValidate_TypeErrorPackagesNotArray(t *testing.T) {
	input := `{"schemaVersion":"1","packages":"nope"}`
	_, errs, err := ParseAndValidate([]byte(input))
	if err != nil {
		t.Fatalf("unexpected fatal error: %v", err)
	}
	if len(errs) == 0 {
		t.Fatal("expected type validation error for packages=string")
	}
}

func TestParseAndValidate_LineNumbers(t *testing.T) {
	// Verify that line numbers are reported correctly for a multi-line file.
	input := "{\n" +
		"  \"schemaVersion\": \"1\",\n" +
		"  \"packages\": [\n" +
		"    {\n" +
		"      \"id\": \"git\",\n" +
		"      \"prefer\": \"yum\"\n" +
		"    }\n" +
		"  ]\n" +
		"}"
	_, errs, err := ParseAndValidate([]byte(input))
	if err != nil {
		t.Fatalf("unexpected fatal error: %v", err)
	}
	if len(errs) == 0 {
		t.Fatal("expected validation error")
	}
	e := errs[0]
	if e.Line != 6 {
		t.Errorf("expected error on line 6 (prefer field), got line %d", e.Line)
	}
}

func TestOffsetToPosition(t *testing.T) {
	data := []byte("{\n  \"id\": \"git\"\n}")
	// Offset 0 → line 1, col 1
	p := offsetToPosition(data, 0)
	if p.Line != 1 || p.Column != 1 {
		t.Errorf("offset 0: want line 1 col 1, got %+v", p)
	}
	// After '\n' at offset 1 → line 2
	p = offsetToPosition(data, 2)
	if p.Line != 2 {
		t.Errorf("offset 2: want line 2, got line %d", p.Line)
	}
}

// ─── Env block (v2) tests ────────────────────────────────────────────────────

func TestParseAndValidate_V2WithEnv(t *testing.T) {
	input := `{"schemaVersion":"2","packages":[],"env":{"FOO":{"value":"bar"}}}`
	f, errs, err := ParseAndValidate([]byte(input))
	if err != nil {
		t.Fatalf("unexpected fatal error: %v", err)
	}
	if len(errs) != 0 {
		t.Errorf("unexpected validation errors: %v", errs)
	}
	if f.Env == nil || f.Env["FOO"].Value != "bar" {
		t.Errorf("expected env FOO=bar, got %v", f.Env)
	}
}

func TestParseAndValidate_EnvOnV1_RejectsIt(t *testing.T) {
	// env block on schemaVersion "1" is a validation error.
	input := `{"schemaVersion":"1","packages":[],"env":{"FOO":{"value":"bar"}}}`
	_, errs, err := ParseAndValidate([]byte(input))
	if err != nil {
		t.Fatalf("unexpected fatal error: %v", err)
	}
	found := false
	for _, e := range errs {
		if e.Field == "env" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected error on env field for v1 spec, got: %v", errs)
	}
}

func TestParseAndValidate_InvalidEnvName(t *testing.T) {
	input := `{"schemaVersion":"2","packages":[],"env":{"1bad":{"value":"x"}}}`
	_, errs, err := ParseAndValidate([]byte(input))
	if err != nil {
		t.Fatalf("unexpected fatal error: %v", err)
	}
	found := false
	for _, e := range errs {
		if strings.Contains(e.Field, "1bad") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected invalid-name error, got: %v", errs)
	}
}

func TestValidEnvName(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  bool
	}{
		// Empty is invalid
		{"empty", "", false},

		// Valid leading characters (letters, underscore)
		{"leading upper", "A", true},
		{"leading lower", "a", true},
		{"leading underscore", "_", true},
		{"upper word", "FOO", true},
		{"lower word", "foo", true},
		{"mixed word", "fOo", true},
		{"underscore word", "_foo", true},

		// Valid subsequent characters (letters, numbers, underscores)
		{"trailing number", "A1", true},
		{"trailing numbers", "A123", true},
		{"number in middle", "A1B", true},
		{"trailing underscore", "A_", true},
		{"multiple underscores", "A_B_C", true},
		{"mixed letters numbers underscores", "a_1_B", true},

		// Invalid leading digit
		{"leading digit single", "1", false},
		{"leading digit multiple", "123", false},
		{"leading digit mixed", "1a", false},
		{"leading digit underscore", "1_", false},

		// Invalid other characters
		{"space", "A B", false},
		{"hyphen", "A-B", false},
		{"dot", "A.B", false},
		{"equals", "A=B", false},
		{"slash", "A/B", false},
		{"newline", "A\nB", false},
		{"tab", "A\tB", false},
		{"quotes", "\"A\"", false},
		{"dollar", "$A", false},
		{"percent", "A%", false},
		{"unicode", "AüB", false},
		{"emoji", "A😀", false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := ValidEnvName(tc.input)
			if got != tc.want {
				t.Errorf("ValidEnvName(%q) = %v, want %v", tc.input, got, tc.want)
			}
		})
	}
}

func TestParseAndValidate_V2NoEnv(t *testing.T) {
	// schemaVersion "2" with no env block is valid.
	input := `{"schemaVersion":"2","packages":[]}`
	_, errs, err := ParseAndValidate([]byte(input))
	if err != nil {
		t.Fatalf("unexpected fatal error: %v", err)
	}
	if len(errs) != 0 {
		t.Errorf("unexpected validation errors: %v", errs)
	}
}

func TestValidationError_Error_WithPosition(t *testing.T) {
	e := ValidationError{
		Position: Position{Line: 3, Column: 10},
		Field:    "schemaVersion",
		Message:  "unsupported version",
	}
	got := e.Error()
	if !strings.Contains(got, "line 3:10") {
		t.Errorf("expected 'line 3:10' in error string, got: %q", got)
	}
	if !strings.Contains(got, "schemaVersion") {
		t.Errorf("expected field name in error string, got: %q", got)
	}
	if !strings.Contains(got, "unsupported version") {
		t.Errorf("expected message in error string, got: %q", got)
	}
}

func TestValidationError_Error_NoPosition(t *testing.T) {
	e := ValidationError{
		// Line == 0 → no position prefix
		Field:   "packages",
		Message: "required field is missing",
	}
	got := e.Error()
	if strings.Contains(got, "line") {
		t.Errorf("expected no 'line' prefix when Line==0, got: %q", got)
	}
	if !strings.Contains(got, "packages") {
		t.Errorf("expected field name in error string, got: %q", got)
	}
	if !strings.Contains(got, "required field is missing") {
		t.Errorf("expected message in error string, got: %q", got)
	}
}

func TestLocateFields_NonObjectInput(t *testing.T) {
	// locateFields must not panic when the top-level token is not '{'.
	pos := make(map[string]Position)
	locateFields([]byte(`["not","an","object"]`), pos)
	if len(pos) != 0 {
		t.Errorf("expected empty positions for non-object JSON, got: %v", pos)
	}
}

func TestLocateFields_EmptyInput(t *testing.T) {
	// locateFields must not panic on empty input.
	pos := make(map[string]Position)
	locateFields([]byte(""), pos)
	if len(pos) != 0 {
		t.Errorf("expected empty positions for empty input, got: %v", pos)
	}
}

// TestParseAndValidate_ValidAllFields verifies that a package with all optional
// fields set is accepted without errors.
func TestParseAndValidate_ValidAllFields(t *testing.T) {
	input := `{
		"schemaVersion": "1",
		"packages": [
			{
				"id": "firefox",
				"version": "123.*",
				"prefer": "snap",
				"managers": {
					"snap": "hello",
					"brew": "hello"
				}
			}
		]
	}`
	f, errs, err := ParseAndValidate([]byte(input))
	if err != nil {
		t.Fatalf("unexpected fatal error: %v", err)
	}
	if len(errs) > 0 {
		t.Fatalf("unexpected validation errors: %v", errs)
	}
	if f == nil || len(f.Packages) != 1 {
		t.Fatalf("expected 1 package, got: %v", f)
	}
	p := f.Packages[0]
	if p.ID != "firefox" || p.Version != "123.*" || p.Prefer != "snap" {
		t.Errorf("unexpected package fields: %+v", p)
	}
	if p.Managers["snap"] != "hello" {
		t.Errorf("managers map not populated correctly: %v", p.Managers)
	}
}

// TestParseAndValidate_MultipleValidPackages verifies that a file with several
// valid packages is accepted.
func TestParseAndValidate_MultipleValidPackages(t *testing.T) {
	input := `{"schemaVersion":"1","packages":[{"id":"git"},{"id":"neovim"},{"id":"firefox"}]}`
	f, errs, err := ParseAndValidate([]byte(input))
	if err != nil {
		t.Fatalf("unexpected fatal error: %v", err)
	}
	if len(errs) > 0 {
		t.Fatalf("unexpected validation errors: %v", errs)
	}
	if f == nil {
		t.Fatal("ParseAndValidate returned nil")
		return
	}
	if len(f.Packages) != 3 {
		t.Fatalf("expected 3 packages, got %d", len(f.Packages))
	}
}

// TestParseAndValidate_PackageWithAllKnownManagers verifies that a managers map
// containing all known manager keys is accepted.
func TestParseAndValidate_PackageWithAllKnownManagers(t *testing.T) {
	input := `{
		"schemaVersion": "1",
		"packages": [{
			"id": "pkg",
			"managers": {
				"paru":      "pkg-paru",
				"yay":       "pkg-yay",
				"snap":      "pkg-snap",
				"brew":      "pkg-brew",
				"linuxbrew": "pkg-linuxbrew"
			}
		}]
	}`
	_, errs, err := ParseAndValidate([]byte(input))
	if err != nil {
		t.Fatalf("unexpected fatal error: %v", err)
	}
	if len(errs) > 0 {
		t.Fatalf("unexpected validation errors for all-known managers: %v", errs)
	}
}

// TestParseAndValidate_EmptyInput verifies that completely empty input returns a
// fatal parse error.
func TestParseAndValidate_EmptyInput(t *testing.T) {
	_, _, err := ParseAndValidate([]byte(""))
	if err == nil {
		t.Fatal("expected fatal error for empty input")
	}
}

// TestParseAndValidate_MultipleDuplicates verifies that each pair of duplicate
// IDs generates a validation error.
func TestParseAndValidate_MultipleDuplicates(t *testing.T) {
	input := `{"schemaVersion":"1","packages":[{"id":"git"},{"id":"git"},{"id":"vim"},{"id":"vim"}]}`
	_, errs, err := ParseAndValidate([]byte(input))
	if err != nil {
		t.Fatalf("unexpected fatal error: %v", err)
	}
	// Expect two duplicate errors (one for each repeated id).
	dupCount := 0
	for _, e := range errs {
		if strings.Contains(e.Message, "duplicate") {
			dupCount++
		}
	}
	if dupCount < 2 {
		t.Errorf("expected at least 2 duplicate errors, got %d: %v", dupCount, errs)
	}
}

// TestParseAndValidate_MultipleUnknownManagers verifies that each unknown
// manager key in the managers map produces its own validation error.
func TestParseAndValidate_MultipleUnknownManagers(t *testing.T) {
	input := `{"schemaVersion":"1","packages":[{"id":"git","managers":{"yum":"git","dnf":"git"}}]}`
	_, errs, err := ParseAndValidate([]byte(input))
	if err != nil {
		t.Fatalf("unexpected fatal error: %v", err)
	}
	if len(errs) < 2 {
		t.Errorf("expected at least 2 validation errors for 2 unknown managers, got %d: %v", len(errs), errs)
	}
}

// TestLocateFields_NestedManagers verifies that locateFields records positions
// for manager keys inside a nested managers object.
func TestLocateFields_NestedManagers(t *testing.T) {
	data := []byte(`{
  "schemaVersion": "1",
  "packages": [
    {
      "id": "firefox",
      "managers": {
        "brew": "firefox"
      }
    }
  ]
}`)
	pos := make(map[string]Position)
	locateFields(data, pos)

	// packages[0].managers.brew must be tracked.
	key := "packages[0].managers.brew"
	if _, ok := pos[key]; !ok {
		t.Errorf("expected position for %q to be tracked; got keys: %v", key, pos)
	}
}

func TestParseAndValidate_RejectsShellSourceMetacharacters(t *testing.T) {
	input := `{
		"schemaVersion":"3",
		"packages":[],
		"shell":{"source":["/tmp/env.sh; rm -rf /"]}
	}`
	_, errs, err := ParseAndValidate([]byte(input))
	if err != nil {
		t.Fatalf("unexpected fatal error: %v", err)
	}
	found := false
	for _, e := range errs {
		if e.Field == "shell.source[0]" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected shell source validation error, got: %v", errs)
	}
}

func TestParseAndValidate_RejectsShellFunctionMetacharacters(t *testing.T) {
	input := `{
		"schemaVersion":"3",
		"packages":[],
		"shell":{"functions":{"bad":{"body":"echo hi; rm -rf /"}}}
	}`
	_, errs, err := ParseAndValidate([]byte(input))
	if err != nil {
		t.Fatalf("unexpected fatal error: %v", err)
	}
	found := false
	for _, e := range errs {
		if e.Field == "shell.functions.bad.body" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected shell function validation error, got: %v", errs)
	}
}

func TestParseAndValidate_RejectsServiceNewlineInjection(t *testing.T) {
	input := `{
		"schemaVersion":"4",
		"packages":[],
		"services":{
			"svc\nbad":{"start":["echo","ok"]},
			"ok":{"start":["echo","hello\nworld"]}
		}
	}`
	_, errs, err := ParseAndValidate([]byte(input))
	if err != nil {
		t.Fatalf("unexpected fatal error: %v", err)
	}
	if len(errs) == 0 {
		t.Fatal("expected newline validation errors")
	}
}

func TestParseAndValidate_V5FilesAndHooks(t *testing.T) {
	input := `{
		"schemaVersion": "5",
		"packages": [{"id": "git", "host": ["arch", "wsl2"]}],
		"env": {"EDITOR": {"value": "nvim"}},
		"shell": {"aliases": {"ll": {"value": "ls -lh", "shell": "zsh"}}},
		"files": {
			"links": [{"source": "a", "target": "~/b", "mode": "managed-link", "host": "macos"}],
			"templates": [{"source": "c", "target": "~/d"}],
			"dirs": [{"target": "~/.config/foo"}]
		},
		"hooks": {
			"preUpgrade": [{"command": "brew upgrade", "host": "macos"}],
			"postApply": [{"command": "echo done"}],
			"postUpgrade": [{"command": "echo upgraded"}]
		},
		"repo": {"url": "~/terminal-config", "ref": "main"}
	}`
	f, errs, err := ParseAndValidate([]byte(input))
	if err != nil {
		t.Fatalf("unexpected fatal error: %v", err)
	}
	if len(errs) > 0 {
		t.Fatalf("unexpected validation errors: %v", errs)
	}
	if f.SchemaVersion != Version5 {
		t.Errorf("schemaVersion = %q, want %q", f.SchemaVersion, Version5)
	}
	if f.Files == nil || len(f.Files.Links) != 1 || len(f.Files.Templates) != 1 || len(f.Files.Dirs) != 1 {
		t.Errorf("files block not parsed: %+v", f.Files)
	}
	if f.Hooks == nil || len(f.Hooks.PreUpgrade) != 1 || len(f.Hooks.PostApply) != 1 || len(f.Hooks.PostUpgrade) != 1 {
		t.Errorf("hooks block not parsed: %+v", f.Hooks)
	}
	if f.Repo == nil || f.Repo.URL == "" {
		t.Errorf("repo block not parsed: %+v", f.Repo)
	}
	if len(f.Packages[0].Host) != 2 {
		t.Errorf("package host predicate = %v, want 2 entries", f.Packages[0].Host)
	}
	if f.Env["EDITOR"].Value != "nvim" {
		t.Errorf("v5 env block not parsed: %+v", f.Env)
	}
	if f.Shell == nil || f.Shell.Aliases["ll"].Value != "ls -lh" {
		t.Errorf("v5 shell block not parsed: %+v", f.Shell)
	}
}

func TestParseAndValidate_V5OldHookArraysRemainValid(t *testing.T) {
	// Given: an existing v5 spec using only the original hook arrays.
	input := `{"schemaVersion":"5","packages":[],"hooks":{"preUpgrade":[{"command":"echo pre"}],"postApply":[{"command":"echo apply"}],"postUpgrade":[{"command":"echo post"}]}}`

	// When
	f, errs, err := ParseAndValidate([]byte(input))

	// Then
	if err != nil {
		t.Fatalf("unexpected fatal error: %v", err)
	}
	if len(errs) > 0 {
		t.Fatalf("unexpected validation errors: %v", errs)
	}
	if f.Hooks == nil || len(f.Hooks.PreUpgrade) != 1 || len(f.Hooks.PostApply) != 1 || len(f.Hooks.PostUpgrade) != 1 {
		t.Fatalf("old hook arrays not parsed: %+v", f.Hooks)
	}
}

func TestParseAndValidate_V6LifecycleHooksAndFiles(t *testing.T) {
	// Given: a v6 spec using every lifecycle hook phase and a script-file hook.
	input := `{
		"schemaVersion":"6",
		"packages":[],
		"hooks":{
			"preApply":[{"command":"echo pre-apply"}],
			"postApply":[{"file":"~/.config/genv/hooks/post-apply.sh"}],
			"preAdd":[{"command":"echo pre-add"}],
			"postAdd":[{"command":"echo post-add"}],
			"preRemove":[{"command":"echo pre-remove"}],
			"postRemove":[{"command":"echo post-remove"}],
			"preUpgrade":[{"command":"echo pre-upgrade"}],
			"postUpgrade":[{"command":"echo post-upgrade"}]
		}
	}`

	// When
	f, errs, err := ParseAndValidate([]byte(input))

	// Then
	if err != nil {
		t.Fatalf("unexpected fatal error: %v", err)
	}
	if len(errs) > 0 {
		t.Fatalf("unexpected validation errors: %v", errs)
	}
	if f.Hooks == nil || len(f.Hooks.PreApply) != 1 || len(f.Hooks.PostApply) != 1 || f.Hooks.PostApply[0].File == "" || len(f.Hooks.PreAdd) != 1 || len(f.Hooks.PostRemove) != 1 {
		t.Fatalf("v6 lifecycle hooks not parsed: %+v", f.Hooks)
	}
}

func TestParseAndValidate_NewHooksRequireV6(t *testing.T) {
	// Given: a v5 spec using a new lifecycle phase.
	input := `{"schemaVersion":"5","packages":[],"hooks":{"preApply":[{"command":"echo pre"}]}}`

	// When
	_, errs, err := ParseAndValidate([]byte(input))

	// Then
	if err != nil {
		t.Fatalf("unexpected fatal error: %v", err)
	}
	if !hasValidationField(errs, "hooks.preApply") {
		t.Fatalf("expected hooks.preApply v6 validation error, got: %v", errs)
	}
}

func TestParseAndValidate_HookCommandOrFileExactlyOne(t *testing.T) {
	tests := []struct {
		name  string
		hook  string
		field string
	}{
		{name: "both command and file", hook: `{"command":"echo hi","file":"~/hook.sh"}`, field: "hooks.postApply[0]"},
		{name: "neither command nor file", hook: `{}`, field: "hooks.postApply[0]"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Given
			input := `{"schemaVersion":"6","packages":[],"hooks":{"postApply":[` + tc.hook + `]}}`

			// When
			_, errs, err := ParseAndValidate([]byte(input))

			// Then
			if err != nil {
				t.Fatalf("unexpected fatal error: %v", err)
			}
			if !hasValidationField(errs, tc.field) {
				t.Fatalf("expected %s validation error, got: %v", tc.field, errs)
			}
		})
	}
}

func hasValidationField(errs []ValidationError, field string) bool {
	for _, e := range errs {
		if e.Field == field {
			return true
		}
	}
	return false
}

func TestParseAndValidate_MergeDirModeAccepted(t *testing.T) {
	input := `{
		"schemaVersion": "5",
		"packages": [],
		"files": {
			"links": [
				{"source": "zsh/common", "target": "~/.config/zsh", "mode": "merge-dir"},
				{"source": "zsh/arch", "target": "~/.config/zsh", "mode": "merge-dir", "host": "arch"}
			]
		}
	}`
	f, errs, err := ParseAndValidate([]byte(input))
	if err != nil {
		t.Fatalf("unexpected fatal error: %v", err)
	}
	if len(errs) > 0 {
		t.Fatalf("unexpected validation errors: %v", errs)
	}
	if len(f.Files.Links) != 2 || f.Files.Links[0].Mode != "merge-dir" {
		t.Errorf("merge-dir links not parsed: %+v", f.Files.Links)
	}
}

func TestParseAndValidate_InvalidLinkModeRejected(t *testing.T) {
	input := `{
		"schemaVersion": "5",
		"packages": [],
		"files": {
			"links": [{"source": "a", "target": "~/b", "mode": "bogus-mode"}]
		}
	}`
	_, errs, err := ParseAndValidate([]byte(input))
	if err != nil {
		t.Fatalf("unexpected fatal error: %v", err)
	}
	found := false
	for _, e := range errs {
		if e.Field == "files.links[0].mode" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected files.links[0].mode validation error, got: %v", errs)
	}
}

func TestParseAndValidate_V4StillValid(t *testing.T) {
	input := `{"schemaVersion":"4","packages":[],"services":{"svc":{"start":["true"]}}}`
	_, errs, err := ParseAndValidate([]byte(input))
	if err != nil {
		t.Fatalf("unexpected fatal error: %v", err)
	}
	if len(errs) > 0 {
		t.Fatalf("v4 spec should still validate: %v", errs)
	}
}

func TestParseAndValidate_AcceptsV7(t *testing.T) {
	input := `{"schemaVersion":"7","packages":[]}`
	f, errs, err := ParseAndValidate([]byte(input))
	if err != nil {
		t.Fatalf("unexpected fatal error: %v", err)
	}
	if len(errs) != 0 {
		t.Fatalf("expected no validation errors for v7, got: %v", errs)
	}
	if f.SchemaVersion != Version7 {
		t.Errorf("SchemaVersion = %q, want %q", f.SchemaVersion, Version7)
	}
}

func TestParseAndValidate_AcceptsPowerShellShellTarget(t *testing.T) {
	input := `{
		"schemaVersion":"7",
		"packages":[],
		"shell":{
			"aliases":{"ll":{"value":"Get-ChildItem","shell":"powershell"}},
			"functions":{"greet":{"body":"Write-Host hi","shell":"powershell"}}
		}
	}`
	_, errs, err := ParseAndValidate([]byte(input))
	if err != nil {
		t.Fatalf("unexpected fatal error: %v", err)
	}
	if len(errs) != 0 {
		t.Fatalf("expected no validation errors, got: %v", errs)
	}
}

func TestParseAndValidate_PowerShellRequiresV7(t *testing.T) {
	input := `{
		"schemaVersion":"3",
		"packages":[],
		"shell":{"aliases":{"ll":{"value":"Get-ChildItem","shell":"powershell"}}}
	}`
	_, errs, err := ParseAndValidate([]byte(input))
	if err != nil {
		t.Fatalf("unexpected fatal error: %v", err)
	}
	found := false
	for _, e := range errs {
		if e.Field == "shell.aliases.ll.shell" && strings.Contains(e.Message, "schemaVersion") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected powershell target rejected on v3, got: %v", errs)
	}
}

func TestParseAndValidate_V8RejectsTopLevelPackages(t *testing.T) {
	raw := `{"schemaVersion":"8","packages":[{"id":"git"}],"targets":{"arch":{"packages":[]}}}`
	_, errs, err := ParseAndValidate([]byte(raw))
	if err != nil {
		t.Fatal(err)
	}
	if len(errs) == 0 {
		t.Fatal("expected validation error for top-level packages on v8")
	}
}

func TestParseAndValidate_V8AcceptsDefaultsAndTargets(t *testing.T) {
	raw := `{
	  "schemaVersion":"8",
	  "defaults":{"env":{"EDITOR":{"value":"nvim"}}},
	  "targets":{
	    "arch":{"packages":[{"id":"git","prefer":"pacman"}],"env":{"EDITOR":null}},
	    "macos":{"packages":[{"id":"git","prefer":"brew"}]}
	  }
	}`
	f, errs, err := ParseAndValidate([]byte(raw))
	if err != nil || len(errs) > 0 {
		t.Fatalf("unexpected: err=%v errs=%v", err, errs)
	}
	if f.Targets["arch"].Env["EDITOR"] != nil {
		t.Fatal("expected tombstone nil pointer for EDITOR on arch")
	}
}

func TestParseAndValidate_V8RejectsUnknownEnvTombstone(t *testing.T) {
	raw := `{
	  "schemaVersion":"8",
	  "defaults":{"env":{"EDITOR":{"value":"nvim"}}},
	  "targets":{"arch":{"env":{"EDTIOR":null}}}
	}`
	_, errs, err := ParseAndValidate([]byte(raw))
	if err != nil {
		t.Fatal(err)
	}
	if !hasValidationField(errs, "targets.arch.env.EDTIOR") {
		t.Fatalf("expected unknown env tombstone validation error, got: %v", errs)
	}
}

func TestParseAndValidate_V8RejectsUnknownAliasTombstone(t *testing.T) {
	raw := `{
	  "schemaVersion":"8",
	  "defaults":{"shell":{"aliases":{"ll":{"value":"ls -la"}}}},
	  "targets":{"arch":{"shell":{"aliases":{"sl":null}}}}
	}`
	_, errs, err := ParseAndValidate([]byte(raw))
	if err != nil {
		t.Fatal(err)
	}
	if !hasValidationField(errs, "targets.arch.shell.aliases.sl") {
		t.Fatalf("expected unknown alias tombstone validation error, got: %v", errs)
	}
}

func TestV8JSONSchemaSeparatesDefaultAndTargetTombstones(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "schema", "v8", "genv.json"))
	if err != nil {
		t.Fatalf("ReadFile schema: %v", err)
	}
	var doc map[string]any
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatalf("Unmarshal schema: %v", err)
	}
	properties := doc["properties"].(map[string]any)
	defaults := properties["defaults"].(map[string]any)
	if defaults["$ref"] != "#/$defs/defaultBundle" {
		t.Fatalf("defaults ref = %v, want defaultBundle", defaults["$ref"])
	}
	targets := properties["targets"].(map[string]any)
	targetAdditional := targets["additionalProperties"].(map[string]any)
	if targetAdditional["$ref"] != "#/$defs/targetBundle" {
		t.Fatalf("targets additionalProperties ref = %v, want targetBundle", targetAdditional["$ref"])
	}

	defs := doc["$defs"].(map[string]any)
	defaultBundle := defs["defaultBundle"].(map[string]any)
	defaultProps := defaultBundle["properties"].(map[string]any)
	for _, field := range []string{"env", "services"} {
		cfg := defaultProps[field].(map[string]any)
		additional := cfg["additionalProperties"].(map[string]any)
		if schemaContainsNull(additional) {
			t.Fatalf("defaults.%s allows null tombstones: %#v", field, additional)
		}
	}

	targetBundle := defs["targetBundle"].(map[string]any)
	targetProps := targetBundle["properties"].(map[string]any)
	for _, field := range []string{"env", "services"} {
		cfg := targetProps[field].(map[string]any)
		additional := cfg["additionalProperties"].(map[string]any)
		if !schemaContainsNull(additional) {
			t.Fatalf("targets.*.%s should allow null tombstones: %#v", field, additional)
		}
	}
}

func schemaContainsNull(v any) bool {
	switch x := v.(type) {
	case map[string]any:
		if x["type"] == "null" {
			return true
		}
		for _, child := range x {
			if schemaContainsNull(child) {
				return true
			}
		}
	case []any:
		for _, child := range x {
			if schemaContainsNull(child) {
				return true
			}
		}
	}
	return false
}

func TestParseAndValidate_V8RejectsHostOnPackage(t *testing.T) {
	raw := `{"schemaVersion":"8","targets":{"arch":{"packages":[{"id":"git","host":["arch"]}]}}}`
	_, errs, err := ParseAndValidate([]byte(raw))
	if err != nil {
		t.Fatal(err)
	}
	if len(errs) == 0 {
		t.Fatal("expected error for host on v8 package")
	}
}

func TestParseAndValidate_FilesBlockRequiresV5(t *testing.T) {
	input := `{"schemaVersion":"4","packages":[],"files":{"links":[]}}`
	_, errs, err := ParseAndValidate([]byte(input))
	if err != nil {
		t.Fatalf("unexpected fatal error: %v", err)
	}
	found := false
	for _, e := range errs {
		if e.Field == "files" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected files block rejected on v4, got: %v", errs)
	}
}

func TestParseAndValidate_FilesRejectsEmptySource(t *testing.T) {
	input := `{"schemaVersion":"5","packages":[],"files":{"links":[{"source":"","target":"~/x"}]}}`
	_, errs, err := ParseAndValidate([]byte(input))
	if err != nil {
		t.Fatalf("unexpected fatal error: %v", err)
	}
	found := false
	for _, e := range errs {
		if e.Field == "files.links[0].source" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected empty source error, got: %v", errs)
	}
}

func TestParseAndValidate_FilesRejectsInvalidLinkMode(t *testing.T) {
	input := `{"schemaVersion":"5","packages":[],"files":{"links":[{"source":"a","target":"~/x","mode":"copy"}]}}`
	_, errs, err := ParseAndValidate([]byte(input))
	if err != nil {
		t.Fatalf("unexpected fatal error: %v", err)
	}
	found := false
	for _, e := range errs {
		if e.Field == "files.links[0].mode" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected invalid mode error, got: %v", errs)
	}
}

func TestHostPredicate_UnmarshalString(t *testing.T) {
	var hp HostPredicate
	if err := hp.UnmarshalJSON([]byte(`"macos"`)); err != nil {
		t.Fatalf("unmarshal string: %v", err)
	}
	if len(hp) != 1 || hp[0] != "macos" {
		t.Errorf("got %v, want [macos]", hp)
	}
}

func TestHostPredicate_UnmarshalArray(t *testing.T) {
	var hp HostPredicate
	if err := hp.UnmarshalJSON([]byte(`["arch","wsl2"]`)); err != nil {
		t.Fatalf("unmarshal array: %v", err)
	}
	want := []string{"arch", "wsl2"}
	if len(hp) != len(want) {
		t.Fatalf("got %v, want %v", hp, want)
	}
	for i, w := range want {
		if hp[i] != w {
			t.Errorf("hp[%d] = %q, want %q", i, hp[i], w)
		}
	}
}

func TestHostPredicate_RejectsInt(t *testing.T) {
	var hp HostPredicate
	if err := hp.UnmarshalJSON([]byte(`42`)); err == nil {
		t.Error("expected error for numeric host predicate")
	}
}

// ─── Updates block (v6) tests ────────────────────────────────────────────────

// TestParseAndValidate_V6MinimalUpdates verifies that a minimal updates block on
// schemaVersion "6" loads and round-trips through marshal/unmarshal unchanged.
func TestParseAndValidate_V6MinimalUpdates(t *testing.T) {
	input := `{
		"schemaVersion": "6",
		"packages": [],
		"updates": {
			"enabled": true,
			"interval": "24h",
			"autoApply": false,
			"notify": true
		}
	}`
	f, errs, err := ParseAndValidate([]byte(input))
	if err != nil {
		t.Fatalf("unexpected fatal error: %v", err)
	}
	if len(errs) > 0 {
		t.Fatalf("unexpected validation errors: %v", errs)
	}
	if f.Updates == nil {
		t.Fatal("expected updates block to be parsed")
	}
	if !f.Updates.Enabled || f.Updates.Interval != "24h" || f.Updates.AutoApply || !f.Updates.Notify {
		t.Errorf("updates block not parsed correctly: %+v", f.Updates)
	}

	// Round-trip: marshal then re-parse and confirm the same values survive and
	// still validate cleanly.
	data, err := json.Marshal(f)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	rt, rtErrs, rtErr := ParseAndValidate(data)
	if rtErr != nil {
		t.Fatalf("round-trip fatal error: %v", rtErr)
	}
	if len(rtErrs) > 0 {
		t.Fatalf("round-trip validation errors: %v", rtErrs)
	}
	if rt.Updates == nil || rt.Updates.Interval != "24h" || !rt.Updates.Enabled || !rt.Updates.Notify {
		t.Errorf("round-trip lost updates data: %+v", rt.Updates)
	}
}

// TestParseAndValidate_UpdatesBlockRequiresV6 verifies the updates block is
// rejected on schema versions below v6.
func TestParseAndValidate_UpdatesBlockRequiresV6(t *testing.T) {
	input := `{"schemaVersion":"5","packages":[],"updates":{"enabled":false}}`
	_, errs, err := ParseAndValidate([]byte(input))
	if err != nil {
		t.Fatalf("unexpected fatal error: %v", err)
	}
	found := false
	for _, e := range errs {
		if e.Field == "updates" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected updates block rejected on v5, got: %v", errs)
	}
}

// TestParseAndValidate_UpdatesInvalidInterval verifies an unparseable interval
// yields a validation error with an actionable hint.
func TestParseAndValidate_UpdatesInvalidInterval(t *testing.T) {
	input := `{"schemaVersion":"6","packages":[],"updates":{"enabled":true,"interval":"soon"}}`
	_, errs, err := ParseAndValidate([]byte(input))
	if err != nil {
		t.Fatalf("unexpected fatal error: %v", err)
	}
	var found *ValidationError
	for i := range errs {
		if errs[i].Field == "updates.interval" {
			found = &errs[i]
			break
		}
	}
	if found == nil {
		t.Fatalf("expected updates.interval validation error, got: %v", errs)
		return
	}
	if !strings.Contains(found.Message, "24h") {
		t.Errorf("expected corrective hint mentioning a valid duration, got: %q", found.Message)
	}
}

// TestParseAndValidate_UpdatesNegativeInterval verifies a zero or negative
// interval is rejected when the checker is enabled.
func TestParseAndValidate_UpdatesNegativeInterval(t *testing.T) {
	tests := []struct {
		name     string
		interval string
	}{
		{"zero", "0s"},
		{"negative", "-1h"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			input := `{"schemaVersion":"6","packages":[],"updates":{"enabled":true,"interval":"` + tc.interval + `"}}`
			_, errs, err := ParseAndValidate([]byte(input))
			if err != nil {
				t.Fatalf("unexpected fatal error: %v", err)
			}
			found := false
			for _, e := range errs {
				if e.Field == "updates.interval" {
					found = true
				}
			}
			if !found {
				t.Errorf("expected updates.interval error for %q, got: %v", tc.interval, errs)
			}
		})
	}
}

// TestParseAndValidate_UpdatesMissingIntervalWhenEnabled verifies an enabled
// checker with no interval is rejected.
func TestParseAndValidate_UpdatesMissingIntervalWhenEnabled(t *testing.T) {
	input := `{"schemaVersion":"6","packages":[],"updates":{"enabled":true}}`
	_, errs, err := ParseAndValidate([]byte(input))
	if err != nil {
		t.Fatalf("unexpected fatal error: %v", err)
	}
	found := false
	for _, e := range errs {
		if e.Field == "updates.interval" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected updates.interval required error, got: %v", errs)
	}
}

// TestParseAndValidate_UpdatesDisabledSkipsIntervalCheck verifies that when the
// checker is disabled, an absent or invalid interval is tolerated (nothing runs).
func TestParseAndValidate_UpdatesDisabledSkipsIntervalCheck(t *testing.T) {
	input := `{"schemaVersion":"6","packages":[],"updates":{"enabled":false}}`
	_, errs, err := ParseAndValidate([]byte(input))
	if err != nil {
		t.Fatalf("unexpected fatal error: %v", err)
	}
	if len(errs) > 0 {
		t.Fatalf("disabled updates block should validate with no interval: %v", errs)
	}
}

// TestParseAndValidate_UpdatesUnknownManager verifies filter arrays reject
// unknown manager names.
func TestParseAndValidate_UpdatesUnknownManager(t *testing.T) {
	input := `{"schemaVersion":"6","packages":[],"updates":{"enabled":true,"interval":"24h","onlyManagers":["yum"]}}`
	_, errs, err := ParseAndValidate([]byte(input))
	if err != nil {
		t.Fatalf("unexpected fatal error: %v", err)
	}
	found := false
	for _, e := range errs {
		if e.Field == "updates.onlyManagers[0]" && strings.Contains(e.Message, "yum") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected unknown manager error, got: %v", errs)
	}
}

// TestParseAndValidate_V1ThroughV5StillLoadWithoutUpdates confirms adding v6 did
// not regress loading of older specs that never carry an updates block.
func TestParseAndValidate_V1ThroughV5StillLoadWithoutUpdates(t *testing.T) {
	specs := []string{
		`{"schemaVersion":"1","packages":[{"id":"git"}]}`,
		`{"schemaVersion":"2","packages":[],"env":{"FOO":{"value":"bar"}}}`,
		`{"schemaVersion":"3","packages":[],"shell":{"aliases":{"ll":{"value":"ls -lah"}}}}`,
		`{"schemaVersion":"4","packages":[],"services":{"svc":{"start":["true"]}}}`,
		`{"schemaVersion":"5","packages":[],"files":{"dirs":[{"target":"~/.config/foo"}]}}`,
	}
	for _, s := range specs {
		f, errs, err := ParseAndValidate([]byte(s))
		if err != nil {
			t.Fatalf("unexpected fatal error for %s: %v", s, err)
		}
		if len(errs) > 0 {
			t.Fatalf("v1-v5 spec should still validate (%s): %v", s, errs)
		}
		if f.Updates != nil {
			t.Errorf("did not expect updates block for %s: %+v", s, f.Updates)
		}
	}
}

func TestParseAndValidate_V6Additive(t *testing.T) {
	input := `{
		"schemaVersion": "6",
		"packages": [],
		"env": {"FOO": {"value": "bar"}},
		"shell": {"aliases": {"ll": {"value": "ls -lah"}}},
		"services": {"svc": {"start": ["true"]}},
		"files": {"dirs": [{"target": "~/.config/foo"}]},
		"hooks": {"postApply": [{"command": "echo done"}]},
		"repo": {"url": "https://github.com/example/dotfiles"},
		"updates": {
			"enabled": true,
			"interval": "24h"
		}
	}`
	_, errs, err := ParseAndValidate([]byte(input))
	if err != nil {
		t.Fatalf("unexpected fatal error: %v", err)
	}
	if len(errs) > 0 {
		t.Fatalf("v6 additive spec should validate with zero errors, got: %v", errs)
	}
}

func TestParseAndValidate_UpdatesDisabledUnknownManager(t *testing.T) {
	input := `{"schemaVersion":"6","packages":[],"updates":{"enabled":false,"onlyManagers":["yum"]}}`
	_, errs, err := ParseAndValidate([]byte(input))
	if err != nil {
		t.Fatalf("unexpected fatal error: %v", err)
	}
	found := false
	for _, e := range errs {
		if e.Field == "updates.onlyManagers[0]" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected unknown manager error even when disabled, got: %v", errs)
	}
}
