package schema

import (
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
							"flatpak": "org.mozilla.firefox",
							"brew": "firefox",
							"snap": "firefox"
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
				t.Fatal("expected non-nil GpmFile")
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
	input := "{\n  \"schemaVersion\": \"2\",\n  \"packages\": []\n}"
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
	if !strings.Contains(e.Message, "2") {
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
				"prefer": "flatpak",
				"managers": {
					"flatpak": "org.mozilla.firefox",
					"brew": "firefox",
					"snap": "firefox"
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
	if p.ID != "firefox" || p.Version != "123.*" || p.Prefer != "flatpak" {
		t.Errorf("unexpected package fields: %+v", p)
	}
	if p.Managers["flatpak"] != "org.mozilla.firefox" {
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
				"apt":       "pkg-apt",
				"dnf":       "pkg-dnf",
				"pacman":    "pkg-pacman",
				"flatpak":   "io.pkg",
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
	input := `{"schemaVersion":"1","packages":[{"id":"git","managers":{"yum":"git","zypper":"git"}}]}`
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
