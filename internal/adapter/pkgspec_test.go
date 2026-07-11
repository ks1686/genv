package adapter

import (
	"slices"
	"testing"
)

func TestJSBasePackageName_whenSpecsIncludeScopesAndVersions(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"scoped package with version", "@scope/pkg@1.0.0", "@scope/pkg"},
		{"scoped package without version", "@scope/pkg", "@scope/pkg"},
		{"unscoped package with version", "pkg@1.2.3", "pkg"},
		{"unscoped package with latest", "pkg@latest", "pkg"},
		{"plain package", "pkg", "pkg"},
		{"empty package", "", ""},
		{"lone at sign", "@", "@"},
		{"scope without package", "@scope", "@scope"},
		{"multiple at signs", "pkg@1.0.0@sha", "pkg"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Given: a JavaScript global package spec string.
			// When: the shared JS base-name parser is applied.
			got := jsBasePackageName(tc.input)

			// Then: only a version suffix is stripped, preserving scoped names.
			if got != tc.want {
				t.Errorf("jsBasePackageName(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

func TestAtVersionBaseName_whenSpecsUseFirstAtSeparator(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"unscoped version", "tool@1.2", "tool"},
		{"unscoped latest", "tool@latest", "tool"},
		{"plain tool", "tool", "tool"},
		{"empty", "", ""},
		{"lone at sign", "@", ""},
		{"scoped input keeps legacy first at behavior", "@scope/pkg@1", ""},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Given: a tool spec whose call site historically cuts at the first @.
			// When: the shared first-@ base parser is applied.
			got := atVersionBaseName(tc.input)

			// Then: legacy first-@ semantics are preserved exactly.
			if got != tc.want {
				t.Errorf("atVersionBaseName(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

func TestPythonBasePackageName_whenSpecsIncludeExtrasAndVersions(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"equality version", "tool==1.2", "tool"},
		{"minimum version", "tool>=1.2", "tool"},
		{"maximum version", "tool<=1.2", "tool"},
		{"compatible version", "tool~=1.2", "tool"},
		{"not equal version", "tool!=1.2", "tool"},
		{"extras with equality", "tool[extra]==1", "tool"},
		{"extras without version", "tool[extra]", "tool"},
		{"direct reference", "tool @ https://example.invalid/tool.whl", "tool"},
		{"plain package", "tool", "tool"},
		{"empty package", "", ""},
		{"lone at sign", "@", "@"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Given: a Python tool requirement-like spec string.
			// When: the shared Python base-name parser is applied.
			got := PythonBasePackageName(tc.input)

			// Then: extras and conservative version markers are stripped.
			if got != tc.want {
				t.Errorf("PythonBasePackageName(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

func TestNonEmptyLines_whenOutputHasBlankLines(t *testing.T) {
	// Given: command output with blank lines and indentation that carries meaning.
	input := "ruff v0.6.9\n  ruff\n\nblack v24.10.0\n"

	// When: preserving non-empty line splitting is used.
	got := nonEmptyLines(input)

	// Then: non-empty lines retain their original whitespace.
	want := []string{"ruff v0.6.9", "  ruff", "black v24.10.0"}
	if !slices.Equal(got, want) {
		t.Errorf("nonEmptyLines = %q, want %q", got, want)
	}
}

func TestTrimmedNonEmptyLines_whenOutputHasPadding(t *testing.T) {
	// Given: command output where leading/trailing whitespace is not meaningful.
	input := "  one  \n\n\ttwo\t\n"

	// When: trimmed non-empty line splitting is used.
	got := trimmedNonEmptyLines(input)

	// Then: empty lines are dropped and remaining lines are trimmed.
	want := []string{"one", "two"}
	if !slices.Equal(got, want) {
		t.Errorf("trimmedNonEmptyLines = %q, want %q", got, want)
	}
}
