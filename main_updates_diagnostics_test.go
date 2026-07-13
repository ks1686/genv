package main

import (
	"strings"
	"testing"
)

func TestUpdatesDiagnosticWriter_caps_output_with_truncation_marker(t *testing.T) {
	// Given: a diagnostic writer with a deliberately small retention limit.
	writer := newUpdatesDiagnosticWriter(64)

	// When: diagnostics exceed that limit across multiple writes.
	if _, err := writer.Write([]byte(strings.Repeat("a", 40))); err != nil {
		t.Fatalf("first Write: %v", err)
	}
	if _, err := writer.Write([]byte(strings.Repeat("b", 40))); err != nil {
		t.Fatalf("second Write: %v", err)
	}

	// Then: rendered diagnostics are capped and explicitly marked as truncated.
	got := writer.String()
	if len(got) > 64 {
		t.Fatalf("diagnostic length = %d, want at most 64", len(got))
	}
	if !strings.Contains(got, updatesDiagnosticTruncationMarker) {
		t.Fatalf("diagnostic = %q, want truncation marker", got)
	}
}

func TestUpdatesDiagnosticWriter_is_deterministic_across_write_boundaries(t *testing.T) {
	// Given: identical oversized diagnostics written with different chunking.
	input := strings.Repeat("0123456789", 20)
	oneWrite := newUpdatesDiagnosticWriter(96)
	manyWrites := newUpdatesDiagnosticWriter(96)

	// When: one writer receives one chunk and the other receives several.
	if _, err := oneWrite.Write([]byte(input)); err != nil {
		t.Fatalf("oneWrite.Write: %v", err)
	}
	for _, chunk := range []string{input[:17], input[17:81], input[81:]} {
		if _, err := manyWrites.Write([]byte(chunk)); err != nil {
			t.Fatalf("manyWrites.Write: %v", err)
		}
	}

	// Then: retained diagnostics do not depend on subprocess write boundaries.
	if oneWrite.String() != manyWrites.String() {
		t.Fatalf("chunked output differs:\none=%q\nmany=%q", oneWrite.String(), manyWrites.String())
	}
}

func TestUpdatesDiagnosticSanitizer_redacts_synthetic_credentials(t *testing.T) {
	// Given: synthetic manager diagnostics containing common credential forms.
	input := strings.Join([]string{
		"APIKEY=apikey-value",
		"API-KEY=api-hyphen-value",
		"API_KEY=api-underscore-value",
		`{"token":"json-token-value","secret": "json-secret-value", "password":"json-password-value"}`,
		"//registry.example/:_auth=auth-value",
		"Authorization: Bearer bearer-value",
		"Authorization: Basic basic-value",
		"fetch https://url-user:url-password@example.com/archive?token=url-token-value&mode=check",
		"command --token flag-token-value --password=flag-password-value --verbose",
		"ordinary=status-ok",
	}, "\n")

	// When: the scheduled diagnostic payload is sanitized.
	got := sanitizeUpdatesDiagnostic(input)

	// Then: credentials are absent while unrelated diagnostics remain useful.
	for _, secret := range []string{
		"apikey-value",
		"api-hyphen-value",
		"api-underscore-value",
		"json-token-value",
		"json-secret-value",
		"json-password-value",
		"auth-value",
		"bearer-value",
		"basic-value",
		"url-user",
		"url-password",
		"url-token-value",
		"flag-token-value",
		"flag-password-value",
	} {
		if strings.Contains(got, secret) {
			t.Fatalf("sanitized diagnostic contains synthetic secret %q: %q", secret, got)
		}
	}
	for _, diagnostic := range []string{"ordinary=status-ok"} {
		if !strings.Contains(got, diagnostic) {
			t.Fatalf("sanitized diagnostic = %q, want non-sensitive text %q retained", got, diagnostic)
		}
	}
}

func TestUpdatesDiagnosticSanitizer_redacts_entire_credential_lines(t *testing.T) {
	// Given: credential syntax that defeats value-oriented quoted-value matching.
	input := strings.Join([]string{
		"fetch https://example.com/archive?auth=query-secret",
		"command --access-token compound-token --verbose",
		"command --client-secret=compound-secret --quiet",
		`manager returned {\"client_secret\":\"escaped-json-secret\"}`,
		`manager returned PASSWORD=\"escaped-assignment-secret\"`,
	}, "\n")

	// When: the scheduled diagnostic payload is sanitized.
	got := sanitizeUpdatesDiagnostic(input)

	// Then: every credential-bearing line is replaced by one stable marker.
	want := strings.Join([]string{
		updatesDiagnosticRedactionMarker,
		updatesDiagnosticRedactionMarker,
		updatesDiagnosticRedactionMarker,
		updatesDiagnosticRedactionMarker,
		updatesDiagnosticRedactionMarker,
	}, "\n")
	if got != want {
		t.Fatalf("sanitized diagnostic = %q, want %q", got, want)
	}
}

func TestUpdatesDiagnosticSanitizer_preserves_noncredential_authentication_failure(t *testing.T) {
	// Given: an ordinary authentication error without credential-bearing syntax.
	input := "authentication failed"

	// When: the scheduled diagnostic payload is sanitized.
	got := sanitizeUpdatesDiagnostic(input)

	// Then: useful non-secret context remains unchanged.
	if got != input {
		t.Fatalf("sanitized diagnostic = %q, want %q", got, input)
	}
}

func TestUpdatesDiagnosticSanitizer_bounds_after_redaction_expansion(t *testing.T) {
	// Given: many short secrets whose redaction markers expand the payload.
	input := strings.Repeat("TOKEN=x\n", updatesDiagnosticLimit/2)

	// When: the payload crosses the final sanitize-and-bound boundary.
	got := sanitizeAndBoundUpdatesDiagnostic(input, updatesDiagnosticLimit)

	// Then: the final sanitized value remains bounded and visibly truncated.
	if len(got) > updatesDiagnosticLimit {
		t.Fatalf("sanitized diagnostic length = %d, want at most %d", len(got), updatesDiagnosticLimit)
	}
	if !strings.HasSuffix(got, updatesDiagnosticTruncationMarker) {
		t.Fatalf("sanitized diagnostic missing truncation marker suffix")
	}
	if strings.Contains(got, "TOKEN=x") {
		t.Fatalf("sanitized diagnostic retained a short secret")
	}
}
