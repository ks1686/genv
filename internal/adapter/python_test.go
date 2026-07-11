package adapter

import (
	"reflect"
	"testing"
)

func TestParsePipxListJSON(t *testing.T) {
	data := []byte(`{
  "venvs": {
    "black": {
      "metadata": {
        "main_package": {
          "package": "black",
          "package_version": "24.2.0"
        }
      }
    },
    "ruff": {
      "metadata": {
        "main_package": {
          "package": "ruff",
          "package_version": "0.3.0"
        }
      }
    }
  }
}`)
	entries, err := parsePipxListJSON(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expected := []pythonEntry{
		{"black", "24.2.0"},
		{"ruff", "0.3.0"},
	}
	// Order might not be guaranteed due to map iteration, so we check length and contents
	if len(entries) != len(expected) {
		t.Fatalf("got %d entries, want %d", len(entries), len(expected))
	}
	for _, exp := range expected {
		found := false
		for _, got := range entries {
			if got.name == exp.name && got.version == exp.version {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("missing expected entry %v", exp)
		}
	}
}

func TestParsePipListJSON(t *testing.T) {
	data := []byte(`[
  {"name": "black", "version": "24.2.0"},
  {"name": "ruff", "version": "0.3.0"}
]`)
	entries, err := parsePipListJSON(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expected := []pythonEntry{
		{"black", "24.2.0"},
		{"ruff", "0.3.0"},
	}
	if !reflect.DeepEqual(entries, expected) {
		t.Errorf("got %v, want %v", entries, expected)
	}
}

func TestParsePoetryPluginsText(t *testing.T) {
	out := `
poetry-plugin-export (1.6.0) Poetry plugin to export the dependencies to various formats
poetry-plugin-up (0.2.0) Poetry plugin to update dependencies
`
	entries, err := parsePoetryPluginsText(out)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expected := []pythonEntry{
		{"poetry-plugin-export", "1.6.0"},
		{"poetry-plugin-up", "0.2.0"},
	}
	if !reflect.DeepEqual(entries, expected) {
		t.Errorf("got %v, want %v", entries, expected)
	}
}

func TestParsePixiListText(t *testing.T) {
	out := `
Package Version
ruff    0.3.0
black   24.2.0
`
	entries, err := parsePixiListText(out)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expected := []pythonEntry{
		{"ruff", "0.3.0"},
		{"black", "24.2.0"},
	}
	if !reflect.DeepEqual(entries, expected) {
		t.Errorf("got %v, want %v", entries, expected)
	}
}
