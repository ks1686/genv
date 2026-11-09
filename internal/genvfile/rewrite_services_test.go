package genvfile

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ks1686/genv/internal/schema"
)

func TestWrite_ServiceAddPreservesHookBytes(t *testing.T) {
	path := filepath.Join(t.TempDir(), "genv.json")
	const original = `{
  "schemaVersion": "9",
  "targets": {
    "macos": {
      "hooks": {
        "postApply": [
          { "command": "echo ok >/dev/null 2>&1 && true" },
          { "command": "printf '<'" }
        ]
      }
    }
  }
}
`
	if err := os.WriteFile(path, []byte(original), 0o600); err != nil {
		t.Fatal(err)
	}
	f, err := Read(path)
	if err != nil {
		t.Fatal(err)
	}
	f.Targets["macos"].Services = map[string]*schema.Service{
		"demo": {Launchd: &schema.LaunchdSpec{Plist: "demo.plist"}},
	}
	if err := Write(path, f); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	const want = `{
  "schemaVersion": "9",
  "targets": {
    "macos": {
      "hooks": {
        "postApply": [
          { "command": "echo ok >/dev/null 2>&1 && true" },
          { "command": "printf '<'" }
        ]
      },
      "services": {
        "demo": {
          "launchd": {
            "plist": "demo.plist"
          }
        }
      }
    }
  }
}
`
	if string(got) != want {
		t.Fatalf("service insert\nwant:\n%s\ngot:\n%s", want, got)
	}
}

func TestWrite_ServiceAddKeepsExistingServiceText(t *testing.T) {
	path := filepath.Join(t.TempDir(), "genv.json")
	const original = `{
  "schemaVersion": "9",
  "targets": {
    "macos": {
      "hooks": {
        "postApply": [
          { "command": "echo ok >/dev/null 2>&1 && true" }
        ]
      },
      "services": {
        "kept": { "start": ["echo", "kept"] }
      }
    }
  }
}
`
	if err := os.WriteFile(path, []byte(original), 0o600); err != nil {
		t.Fatal(err)
	}
	f, err := Read(path)
	if err != nil {
		t.Fatal(err)
	}
	f.Targets["macos"].Services["extra"] = &schema.Service{
		Start: []string{"sh", "-c", "echo a && b < /tmp/x"},
	}
	if err := Write(path, f); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(got)
	if !strings.Contains(text, `"kept": { "start": ["echo", "kept"] }`) {
		t.Fatalf("existing service was rewritten:\n%s", text)
	}
	if !strings.Contains(text, `{ "command": "echo ok >/dev/null 2>&1 && true" }`) {
		t.Fatalf("hook command was rewritten:\n%s", text)
	}
	if strings.Contains(text, `\u0026`) || strings.Contains(text, `\u003c`) {
		t.Fatalf("new service command was HTML-escaped:\n%s", text)
	}
	if !strings.Contains(text, `echo a && b < /tmp/x`) {
		t.Fatalf("new service command missing:\n%s", text)
	}
}

func TestWrite_ServiceUpdateAndRemoveStayInPlace(t *testing.T) {
	path := filepath.Join(t.TempDir(), "genv.json")
	const original = `{
  "schemaVersion": "6",
  "packages": [],
  "hooks": {
    "postApply": [
      { "command": "echo ok >/dev/null 2>&1 && true" }
    ]
  },
  "services": {
    "keep": {"start": ["echo", "keep"]},
    "drop": {"start": ["echo", "drop"]}
  }
}
`
	if err := os.WriteFile(path, []byte(original), 0o600); err != nil {
		t.Fatal(err)
	}
	f, err := Read(path)
	if err != nil {
		t.Fatal(err)
	}
	keep := f.Services["keep"]
	keep.Start = []string{"echo", "changed"}
	f.Services["keep"] = keep
	delete(f.Services, "drop")
	if err := Write(path, f); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(got)
	if strings.Contains(text, "drop") {
		t.Fatalf("removed service still present:\n%s", text)
	}
	if !strings.Contains(text, `{ "command": "echo ok >/dev/null 2>&1 && true" }`) {
		t.Fatalf("hook command was rewritten:\n%s", text)
	}
	if !strings.Contains(text, `"changed"`) {
		t.Fatalf("updated start command missing:\n%s", text)
	}
}

func TestWrite_ServiceTombstoneAndCompactAndCRLF(t *testing.T) {
	t.Run("tombstone", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "genv.json")
		const original = `{
  "schemaVersion": "9",
  "defaults": {
    "services": {
      "demo": {"start": ["true"]}
    }
  },
  "targets": {
    "macos": {}
  }
}
`
		if err := os.WriteFile(path, []byte(original), 0o600); err != nil {
			t.Fatal(err)
		}
		f, err := Read(path)
		if err != nil {
			t.Fatal(err)
		}
		f.Targets["macos"].Services = map[string]*schema.Service{"demo": nil}
		if err := Write(path, f); err != nil {
			t.Fatal(err)
		}
		got, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(got), `"demo":null`) {
			t.Fatalf("tombstone not inserted:\n%s", got)
		}
		if !strings.Contains(string(got), `"defaults"`) || !strings.Contains(string(got), `{"start": ["true"]}`) {
			t.Fatalf("defaults service was rewritten:\n%s", got)
		}
	})

	t.Run("compact", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "genv.json")
		original := `{"schemaVersion":"9","targets":{"macos":{"hooks":{"postApply":[{"command":"echo ok >/dev/null 2>&1 && true"}]}}}}` + "\n"
		if err := os.WriteFile(path, []byte(original), 0o600); err != nil {
			t.Fatal(err)
		}
		f, err := Read(path)
		if err != nil {
			t.Fatal(err)
		}
		f.Targets["macos"].Services = map[string]*schema.Service{
			"demo": {Launchd: &schema.LaunchdSpec{Plist: "demo.plist"}},
		}
		if err := Write(path, f); err != nil {
			t.Fatal(err)
		}
		got, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		want := `{"schemaVersion":"9","targets":{"macos":{"hooks":{"postApply":[{"command":"echo ok >/dev/null 2>&1 && true"}]},"services":{"demo":{"launchd":{"plist":"demo.plist"}}}}}}` + "\n"
		if string(got) != want {
			t.Fatalf("compact insert\nwant %s\ngot  %s", want, got)
		}
	})

	t.Run("crlf", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "genv.json")
		original := "{\r\n  \"schemaVersion\": \"9\",\r\n  \"targets\": {\r\n    \"macos\": {\r\n      \"hooks\": {\r\n        \"postApply\": [\r\n          { \"command\": \"echo ok >/dev/null 2>&1 && true\" }\r\n        ]\r\n      }\r\n    }\r\n  }\r\n}\r\n"
		if err := os.WriteFile(path, []byte(original), 0o600); err != nil {
			t.Fatal(err)
		}
		f, err := Read(path)
		if err != nil {
			t.Fatal(err)
		}
		f.Targets["macos"].Services = map[string]*schema.Service{
			"demo": {Start: []string{"true"}},
		}
		if err := Write(path, f); err != nil {
			t.Fatal(err)
		}
		got, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(got), "\r\n") {
			t.Fatalf("CRLF file was rewritten with LF:\n%s", got)
		}
		if !strings.Contains(string(got), `{ "command": "echo ok >/dev/null 2>&1 && true" }`) {
			t.Fatalf("hook command changed:\n%s", got)
		}
	})
}

func TestWrite_NonServiceEditStillRemarshalsWithoutEscaping(t *testing.T) {
	path := filepath.Join(t.TempDir(), "genv.json")
	const original = `{
  "schemaVersion": "9",
  "targets": {
    "macos": {
      "hooks": {
        "postApply": [
          { "command": "echo ok >/dev/null 2>&1 && true" }
        ]
      }
    }
  }
}
`
	if err := os.WriteFile(path, []byte(original), 0o600); err != nil {
		t.Fatal(err)
	}
	f, err := Read(path)
	if err != nil {
		t.Fatal(err)
	}
	f.Targets["macos"].Hooks.PostApply[0].Command = "echo changed >/dev/null 2>&1 && true"
	f.Targets["macos"].Services = map[string]*schema.Service{
		"demo": {Start: []string{"true"}},
	}
	if err := Write(path, f); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(got)
	if strings.Contains(text, `\u003e`) || strings.Contains(text, `\u0026`) {
		t.Fatalf("fallback remarshal HTML-escaped the hook:\n%s", text)
	}
	if !strings.Contains(text, `echo changed >/dev/null 2>&1 && true`) {
		t.Fatalf("updated hook missing:\n%s", text)
	}
}

func TestRewriteServicesInPlace_RejectsInvalidOriginal(t *testing.T) {
	if _, ok := rewriteServicesInPlace([]byte(`{`), &schema.GenvFile{SchemaVersion: schema.Version9}); ok {
		t.Fatal("expected invalid JSON to fail service rewrite")
	}
}
