package main

import (
	"bufio"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// withConfirmInput points the shared prompt reader at input for one test.
// TestMain sets GENV_NO_INTERACTIVE, so isTerminal is false here: an empty
// reader is the closed-stdin case, and a lone newline is a blank line off a TTY.
func withConfirmInput(t *testing.T, input string) {
	t.Helper()
	old := confirmReader
	confirmReader = bufio.NewReader(strings.NewReader(input))
	t.Cleanup(func() { confirmReader = old })
}

func writeApplyServiceSpec(t *testing.T, path, marker string) {
	t.Helper()
	// A declared service with no lock entry is pending work, so apply must
	// ask before it runs. The marker records whether that run happened.
	// Marshal the command: a Windows temp path contains backslashes, and a
	// raw paste is not valid JSON (\U is not an escape).
	command, err := json.Marshal("touch " + marker)
	if err != nil {
		t.Fatal(err)
	}
	spec := `{
  "schemaVersion": "4",
  "packages": [],
  "services": {
    "demo": {"start": ["sh", "-c", ` + string(command) + `]}
  }
}
`
	if err := os.WriteFile(path, []byte(spec), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestWriteApplyServiceSpec_EscapesWindowsPath(t *testing.T) {
	path := filepath.Join(t.TempDir(), "genv.json")
	writeApplyServiceSpec(t, path, `C:\Users\RUNNER~1\AppData\Local\Temp\marker`)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var spec map[string]any
	if err := json.Unmarshal(data, &spec); err != nil {
		t.Fatalf("spec is not valid JSON: %v\n%s", err, data)
	}
}

func runApplyWithoutYes(t *testing.T, path string) (code int, output string) {
	t.Helper()
	var stderr string
	stdout := captureStdout(t, func() {
		stderr = captureStderr(t, func() {
			code = run([]string{
				"apply", "--file", path,
				"--skip-packages", "--no-hooks",
			})
		})
	})
	return code, stdout + stderr
}

// Closed stdin and a non-interactive blank line are not a typed "no".
// apply must fail and name --yes, and must not reconcile the service (#245).
func TestApply_NonInteractiveEOFDoesNotSucceed(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{"eof", ""},
		{"blank line off a tty", "\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			t.Setenv("XDG_CONFIG_HOME", dir)
			path := filepath.Join(dir, "genv.json")
			marker := filepath.Join(dir, "applied")
			writeApplyServiceSpec(t, path, marker)
			withConfirmInput(t, tt.input)

			code, out := runApplyWithoutYes(t, path)
			if code == exitOK {
				t.Fatalf("apply with no confirmation returned exitOK\n%s", out)
			}
			if code != exitLogic {
				t.Fatalf("apply exit = %d, want %d\n%s", code, exitLogic, out)
			}
			if !strings.Contains(out, "--yes") {
				t.Fatalf("output = %q, want a message naming --yes", out)
			}
			if _, err := os.Stat(marker); !os.IsNotExist(err) {
				t.Fatalf("apply ran the service despite missing confirmation (stat %v)", err)
			}
		})
	}
}

func TestClassifyConfirm(t *testing.T) {
	tests := []struct {
		name     string
		tty      bool
		answer   string
		err      error
		accepted bool
		declined bool
	}{
		{name: "yes", answer: "y\n", accepted: true},
		{name: "yes upper with eof", tty: true, answer: "Y", err: io.EOF, accepted: true},
		{name: "explicit n off a tty", answer: "n\n", declined: true},
		{name: "other text", answer: "no\n", declined: true},
		{name: "blank enter on a tty", tty: true, answer: "\n", declined: true},
		{name: "blank line off a tty", answer: "\n"},
		{name: "eof", answer: "", err: io.EOF},
		{name: "eof on a tty", tty: true, answer: "", err: io.EOF},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			accepted, declined := classifyConfirm(tt.tty, tt.answer, tt.err)
			if accepted != tt.accepted || declined != tt.declined {
				t.Fatalf("classifyConfirm(%v, %q) = (%v, %v), want (%v, %v)", tt.tty, tt.answer, accepted, declined, tt.accepted, tt.declined)
			}
		})
	}
}

// A typed "n" is a decline. That stays exit 0, matching scan and the previous
// apply contract, and still does not apply.
func TestApply_TypedDeclineStaysExitOK(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	path := filepath.Join(dir, "genv.json")
	marker := filepath.Join(dir, "applied")
	writeApplyServiceSpec(t, path, marker)
	withConfirmInput(t, "n\n")

	code, out := runApplyWithoutYes(t, path)
	if code != exitOK {
		t.Fatalf("typed decline exit = %d, want %d\n%s", code, exitOK, out)
	}
	if !strings.Contains(out, "Aborted.") {
		t.Fatalf("output = %q, want Aborted.", out)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("declined apply still ran the service (stat %v)", err)
	}
}
