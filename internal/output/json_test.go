package output_test

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/ks1686/genv/internal/output"
)

func TestWrite_ProducesValidJSON(t *testing.T) {
	var buf bytes.Buffer
	env := output.Envelope{Command: "apply", OK: true}
	if err := output.Write(&buf, env); err != nil {
		t.Fatalf("Write: %v", err)
	}
	var got map[string]interface{}
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatalf("output is not valid JSON: %v\noutput: %q", err, buf.String())
	}
}

func TestWrite_EnvelopeFields(t *testing.T) {
	var buf bytes.Buffer
	env := output.Envelope{
		Command: "status",
		OK:      false,
		Errors:  []string{"something went wrong"},
	}
	if err := output.Write(&buf, env); err != nil {
		t.Fatalf("Write: %v", err)
	}
	var got map[string]interface{}
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatalf("not valid JSON: %v", err)
	}
	if got["command"] != "status" {
		t.Errorf("command: got %v, want %q", got["command"], "status")
	}
	if got["ok"] != false {
		t.Errorf("ok: got %v, want false", got["ok"])
	}
	errs, ok := got["errors"].([]interface{})
	if !ok || len(errs) != 1 {
		t.Errorf("errors: got %v, want one entry", got["errors"])
	}
}

func TestWrite_OmitsNilData(t *testing.T) {
	var buf bytes.Buffer
	env := output.Envelope{Command: "scan", OK: true}
	if err := output.Write(&buf, env); err != nil {
		t.Fatalf("Write: %v", err)
	}
	var got map[string]interface{}
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatalf("not valid JSON: %v", err)
	}
	if _, present := got["data"]; present {
		t.Error("data field should be omitted when nil")
	}
}

func TestWrite_OmitsEmptyErrors(t *testing.T) {
	var buf bytes.Buffer
	env := output.Envelope{Command: "apply", OK: true, Data: output.ScanResult{Added: 3, Skipped: 1}}
	if err := output.Write(&buf, env); err != nil {
		t.Fatalf("Write: %v", err)
	}
	var got map[string]interface{}
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatalf("not valid JSON: %v", err)
	}
	if _, present := got["errors"]; present {
		t.Error("errors field should be omitted when nil")
	}
}

func TestWrite_PlanResult_Roundtrip(t *testing.T) {
	plan := output.PlanResult{
		ToInstall: []output.PlanPackage{
			{ID: "git", Manager: "apt", Cmd: "sudo apt-get install -y git"},
		},
		ToRemove:  []output.PlanPackage{{ID: "htop", Manager: "apt"}},
		Unchanged: []output.PlanPackage{{ID: "curl", Manager: "apt"}},
	}
	var buf bytes.Buffer
	if err := output.Write(&buf, output.Envelope{Command: "apply", OK: true, Data: plan}); err != nil {
		t.Fatalf("Write: %v", err)
	}
	var env output.Envelope
	if err := json.Unmarshal(buf.Bytes(), &env); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if env.Command != "apply" {
		t.Errorf("command: got %q, want %q", env.Command, "apply")
	}
}

func TestWrite_StatusResult(t *testing.T) {
	sr := output.StatusResult{
		Entries: []output.StatusEntry{
			{ID: "git", Manager: "apt", Kind: "ok", InstalledVersion: "2.43.0"},
			{ID: "htop", Kind: "missing"},
		},
	}
	var buf bytes.Buffer
	if err := output.Write(&buf, output.Envelope{Command: "status", OK: true, Data: sr}); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if !bytes.Contains(buf.Bytes(), []byte(`"status"`)) {
		t.Errorf("output missing command field: %s", buf.String())
	}
}

func TestWrite_ScanResult(t *testing.T) {
	var buf bytes.Buffer
	if err := output.Write(&buf, output.Envelope{
		Command: "scan",
		OK:      true,
		Data:    output.ScanResult{Added: 42, Skipped: 7},
	}); err != nil {
		t.Fatalf("Write: %v", err)
	}
	var got map[string]interface{}
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatalf("not valid JSON: %v", err)
	}
	data, _ := got["data"].(map[string]interface{})
	if data["added"] != float64(42) {
		t.Errorf("data.added: got %v, want 42", data["added"])
	}
	if data["skipped"] != float64(7) {
		t.Errorf("data.skipped: got %v, want 7", data["skipped"])
	}
}

func TestWrite_ApplyResult(t *testing.T) {
	var buf bytes.Buffer
	if err := output.Write(&buf, output.Envelope{
		Command: "apply",
		OK:      true,
		Data: output.ApplyResult{
			Installed:   []string{"git", "neovim"},
			Uninstalled: []string{"htop"},
		},
	}); err != nil {
		t.Fatalf("Write: %v", err)
	}
	var got map[string]interface{}
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatalf("not valid JSON: %v", err)
	}
	data, _ := got["data"].(map[string]interface{})
	installed, _ := data["installed"].([]interface{})
	if len(installed) != 2 {
		t.Errorf("data.installed: got %d entries, want 2", len(installed))
	}
}

func TestWrite_EndsWithNewline(t *testing.T) {
	var buf bytes.Buffer
	output.Write(&buf, output.Envelope{Command: "apply", OK: true}) //nolint
	b := buf.Bytes()
	if len(b) == 0 || b[len(b)-1] != '\n' {
		t.Errorf("Write should end with newline, got: %q", string(b))
	}
}
