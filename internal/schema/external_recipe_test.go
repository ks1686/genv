package schema

import (
	"strings"
	"testing"
)

func TestParseAndValidateVersion9GitHubExternalRecipe(t *testing.T) {
	input := `{
  "schemaVersion":"9",
  "targets":{"linux":{"packages":[{
    "id":"tool",
    "prefer":"external",
    "external":{
      "detect":{"command":["tool","--version"],"versionRegex":"tool ([0-9.]+)"},
      "source":{"type":"githubRelease","repository":"owner/tool","release":"stable","tagRegex":"^v?(.+)$"},
      "platforms":[{
        "os":["linux"],"arch":["amd64"],"assetRegex":"^tool_linux_amd64\\.tar\\.gz$",
        "install":{"type":"archive","stripComponents":1,"files":[{"from":"tool","to":"~/.local/bin/tool","mode":"0755"}]}
      }],
      "verify":[{"type":"sha256File","assetRegex":"^checksums\\.txt$"}]
    }
  }]}}
}`

	f, errs, parseErr := ParseAndValidate([]byte(input))
	if parseErr != nil {
		t.Fatalf("ParseAndValidate() parse error: %v", parseErr)
	}
	if len(errs) != 0 {
		t.Fatalf("ParseAndValidate() errors: %v", errs)
	}
	if f.SchemaVersion != Version9 || !IsPortableVersion(f.SchemaVersion) {
		t.Fatalf("schemaVersion = %q, portable = %v", f.SchemaVersion, IsPortableVersion(f.SchemaVersion))
	}
	pkg := f.Targets["linux"].Packages[0]
	if pkg.External == nil || pkg.External.Source.Repository != "owner/tool" {
		t.Fatalf("external recipe not parsed: %+v", pkg.External)
	}
}

func TestParseAndValidateVersion9HTTPExternalRecipe(t *testing.T) {
	input := `{
  "schemaVersion":"9",
  "targets":{"windows":{"packages":[{
    "id":"tool","prefer":"external",
    "external":{
      "detect":{"command":["tool.exe","--version"],"versionRegex":"([0-9.]+)"},
      "source":{"type":"httpRelease","versionURL":"https://example.test/latest.json","format":"json","versionPointer":"/version"},
      "platforms":[{
        "os":["windows"],"arch":["arm64"],"artifactURL":"https://example.test/tool-{version}.exe",
        "install":{"type":"direct","destination":"~/bin/tool.exe"}
      }],
      "verify":[{"type":"sha256","valuePointer":"/sha256"}]
    }
  }]}}
}`

	_, errs, parseErr := ParseAndValidate([]byte(input))
	if parseErr != nil {
		t.Fatalf("ParseAndValidate() parse error: %v", parseErr)
	}
	if len(errs) != 0 {
		t.Fatalf("ParseAndValidate() errors: %v", errs)
	}
}

func TestExternalRecipeRequiresVersion9AndExternalManager(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "version 8",
			input: `{"schemaVersion":"8","targets":{"linux":{"packages":[{"id":"tool","prefer":"external","external":{}}]}}}`,
			want:  "requires schemaVersion \"9\"",
		},
		{
			name:  "wrong manager",
			input: `{"schemaVersion":"9","targets":{"linux":{"packages":[{"id":"tool","prefer":"apt","external":{}}]}}}`,
			want:  `requires prefer "external"`,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, errs, parseErr := ParseAndValidate([]byte(tc.input))
			if parseErr != nil {
				t.Fatalf("ParseAndValidate() parse error: %v", parseErr)
			}
			if !validationErrorsContain(errs, tc.want) {
				t.Fatalf("errors = %v, want substring %q", errs, tc.want)
			}
		})
	}
}

func TestExternalRecipeValidationRejectsUnsafeOrAmbiguousDefinitions(t *testing.T) {
	valid := `"detect":{"command":["tool","--version"],"versionRegex":"([0-9.]+)"},` +
		`"source":{"type":"githubRelease","repository":"owner/tool"},` +
		`"platforms":[{"os":["linux"],"arch":["amd64"],"assetRegex":"tool","install":{"type":"direct","destination":"~/.local/bin/tool"}}],`
	tests := []struct {
		name   string
		recipe string
		want   string
	}{
		{name: "verification required", recipe: strings.TrimSuffix(valid, ","), want: "verify is required unless allowUnverified is true"},
		{name: "detect capture", recipe: strings.Replace(valid, `([0-9.]+)`, `[0-9.]+`, 1) + `"allowUnverified":true`, want: "exactly one capture group"},
		{name: "unknown source", recipe: strings.Replace(valid, `githubRelease`, `website`, 1) + `"allowUnverified":true`, want: "source type"},
		{name: "invalid os", recipe: strings.Replace(valid, `"linux"`, `"plan9"`, 1) + `"allowUnverified":true`, want: "unknown os"},
		{name: "direct destination", recipe: strings.Replace(valid, `,"destination":"~/.local/bin/tool"`, ``, 1) + `"allowUnverified":true`, want: "destination is required"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			input := `{"schemaVersion":"9","targets":{"linux":{"packages":[{"id":"tool","prefer":"external","external":{` + tc.recipe + `}}]}}}`
			_, errs, parseErr := ParseAndValidate([]byte(input))
			if parseErr != nil {
				t.Fatalf("ParseAndValidate() parse error: %v", parseErr)
			}
			if !validationErrorsContain(errs, tc.want) {
				t.Fatalf("errors = %v, want substring %q", errs, tc.want)
			}
		})
	}
}

func TestExternalRecipeRejectsUnknownNestedField(t *testing.T) {
	input := `{"schemaVersion":"9","targets":{"linux":{"packages":[{"id":"tool","prefer":"external","external":{"mystery":true}}]}}}`
	_, errs, parseErr := ParseAndValidate([]byte(input))
	if parseErr != nil {
		t.Fatalf("ParseAndValidate() parse error: %v", parseErr)
	}
	if !validationErrorsContain(errs, `unknown field "mystery"`) {
		t.Fatalf("errors = %v", errs)
	}
}

func TestExternalRecipeAllowsHTTPArtifactWithExplicitAcknowledgement(t *testing.T) {
	input := `{"schemaVersion":"9","targets":{"linux":{"packages":[{"id":"tool","prefer":"external","external":{` +
		`"detect":{"command":["tool","--version"],"versionRegex":"([0-9.]+)"},` +
		`"source":{"type":"httpRelease","versionURL":"http://example.test/latest","format":"text","versionRegex":"([0-9.]+)"},` +
		`"platforms":[{"os":["linux"],"arch":["amd64"],"artifactURL":"http://example.test/tool-{version}","install":{"type":"direct","destination":"~/.local/bin/tool"}}],` +
		`"allowUnverified":true,"allowInsecureHTTP":true}}]}}}`
	_, errs, parseErr := ParseAndValidate([]byte(input))
	if parseErr != nil {
		t.Fatalf("ParseAndValidate() parse error: %v", parseErr)
	}
	if len(errs) != 0 {
		t.Fatalf("ParseAndValidate() errors: %v", errs)
	}
}

func TestExternalScriptValidationRejectsUnsafeDefinitions(t *testing.T) {
	tests := []struct {
		name    string
		install ExternalInstall
		want    string
	}{
		{name: "unknown template", install: ExternalInstall{Type: "script", Interpreter: "sh", Args: []string{"{unknown}"}}, want: "unknown template placeholder"},
		{name: "shell uninstall", install: ExternalInstall{Type: "script", Interpreter: "sh", Uninstall: []string{"sh", "-c", "rm tool"}}, want: "explicit argv"},
		{name: "conflicting direct fields", install: ExternalInstall{Type: "direct", Destination: "~/tool", Args: []string{"--install"}}, want: "another install type"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errs := validateExternalInstall(tt.install, "external.install", nil)
			if !validationErrorsContain(errs, tt.want) {
				t.Fatalf("errors = %v, want %q", errs, tt.want)
			}
		})
	}
}

func validationErrorsContain(errs []ValidationError, want string) bool {
	for _, err := range errs {
		if strings.Contains(err.Error(), want) {
			return true
		}
	}
	return false
}
