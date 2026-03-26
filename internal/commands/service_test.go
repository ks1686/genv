package commands

import (
	"bytes"
	"strings"
	"testing"

	"github.com/ks1686/genv/internal/schema"
)

func TestServiceCommands(t *testing.T) {
	f := &schema.GenvFile{
		SchemaVersion: schema.Version3,
	}

	// Test ServiceAdd
	err := ServiceAdd(f, "test-svc", []string{"echo", "start"}, []string{"echo", "stop"}, nil, []string{"true"})
	if err != nil {
		t.Fatalf("ServiceAdd failed: %v", err)
	}
	if f.SchemaVersion != schema.Version4 {
		t.Errorf("expected SchemaVersion %s, got %s", schema.Version4, f.SchemaVersion)
	}
	if _, ok := f.Services["test-svc"]; !ok {
		t.Error("service 'test-svc' not found in spec")
	}

	// Test ServiceList
	var buf bytes.Buffer
	ServiceList(f, &buf)
	output := buf.String()
	if !strings.Contains(output, "test-svc") {
		t.Errorf("expected list output to contain 'test-svc', got:\n%s", output)
	}

	// Test ServiceRemove
	err = ServiceRemove(f, "test-svc")
	if err != nil {
		t.Fatalf("ServiceRemove failed: %v", err)
	}
	if _, ok := f.Services["test-svc"]; ok {
		t.Error("service 'test-svc' still exists in spec after removal")
	}

	// Test ServiceRemove not found
	err = ServiceRemove(f, "missing-svc")
	if err == nil || !strings.Contains(err.Error(), "service not found") {
		t.Errorf("expected 'service not found' error, got %v", err)
	}
}
