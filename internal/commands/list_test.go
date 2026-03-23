package commands

import (
	"bytes"
	"strings"
	"testing"

	"github.com/ks1686/genv/internal/schema"
)

func TestList_NilFile(t *testing.T) {
	var buf bytes.Buffer
	List(nil, &buf)
	if !strings.Contains(buf.String(), "no packages tracked") {
		t.Errorf("expected 'no packages tracked', got: %q", buf.String())
	}
}

func TestList_EmptyPackages(t *testing.T) {
	var buf bytes.Buffer
	List(newFile(), &buf)
	if !strings.Contains(buf.String(), "no packages tracked") {
		t.Errorf("expected 'no packages tracked', got: %q", buf.String())
	}
}

func TestList_SinglePackage(t *testing.T) {
	f := newFile(schema.Package{ID: "git", Version: "*"})
	var buf bytes.Buffer
	List(f, &buf)
	out := buf.String()

	if !strings.Contains(out, "git") {
		t.Errorf("expected 'git' in output, got: %q", out)
	}
	if !strings.Contains(out, "*") {
		t.Errorf("expected '*' version in output, got: %q", out)
	}
}

func TestList_EmptyVersionShowsStar(t *testing.T) {
	f := newFile(schema.Package{ID: "git"})
	var buf bytes.Buffer
	List(f, &buf)
	if !strings.Contains(buf.String(), "*") {
		t.Errorf("expected '*' for empty version, got: %q", buf.String())
	}
}

func TestList_PreferAndManagers(t *testing.T) {
	f := newFile(schema.Package{
		ID:     "firefox",
		Prefer: "flatpak",
		Managers: map[string]string{
			"brew":    "firefox",
			"flatpak": "org.mozilla.firefox",
		},
	})
	var buf bytes.Buffer
	List(f, &buf)
	out := buf.String()

	if !strings.Contains(out, "flatpak") {
		t.Errorf("expected 'flatpak' in output, got: %q", out)
	}
	if !strings.Contains(out, "brew=firefox") {
		t.Errorf("expected 'brew=firefox' in output, got: %q", out)
	}
	if !strings.Contains(out, "flatpak=org.mozilla.firefox") {
		t.Errorf("expected 'flatpak=org.mozilla.firefox' in output, got: %q", out)
	}
}

func TestList_ManagersSortedAlphabetically(t *testing.T) {
	f := newFile(schema.Package{
		ID: "pkg",
		Managers: map[string]string{
			"snap":    "pkg-snap",
			"apt":     "pkg-apt",
			"flatpak": "pkg-flatpak",
		},
	})
	var buf bytes.Buffer
	List(f, &buf)
	out := buf.String()

	aptIdx := strings.Index(out, "apt=")
	flatpakIdx := strings.Index(out, "flatpak=")
	snapIdx := strings.Index(out, "snap=")
	if !(aptIdx < flatpakIdx && flatpakIdx < snapIdx) {
		t.Errorf("expected managers sorted apt < flatpak < snap, got: %q", out)
	}
}

func TestList_HeaderPresent(t *testing.T) {
	f := newFile(schema.Package{ID: "git"})
	var buf bytes.Buffer
	List(f, &buf)
	out := buf.String()

	for _, col := range []string{"ID", "VERSION", "PREFER", "MANAGERS"} {
		if !strings.Contains(out, col) {
			t.Errorf("expected header column %q in output, got: %q", col, out)
		}
	}
}

// TestList_DashForNoPrefer verifies that a package with an empty prefer field
// displays "-" in the PREFER column.
func TestList_DashForNoPrefer(t *testing.T) {
	f := newFile(schema.Package{ID: "git"}) // no prefer set
	var buf bytes.Buffer
	List(f, &buf)
	out := buf.String()
	if !strings.Contains(out, "-") {
		t.Errorf("expected '-' for empty prefer, got: %q", out)
	}
}

// TestList_DashForNoManagers verifies that a package with no managers map
// displays "-" in the MANAGERS column.
func TestList_DashForNoManagers(t *testing.T) {
	f := newFile(schema.Package{ID: "git", Prefer: "brew"}) // no managers
	var buf bytes.Buffer
	List(f, &buf)
	out := buf.String()
	// There should be a "-" for managers.  The prefer column shows "brew", so
	// the "-" is from the managers column.
	if !strings.Contains(out, "-") {
		t.Errorf("expected '-' for empty managers, got: %q", out)
	}
}

// TestList_MultiplePackages verifies all rows are present when there is more
// than one package in the file.
func TestList_MultiplePackages(t *testing.T) {
	ids := []string{"git", "neovim", "firefox"}
	pkgs := make([]schema.Package, len(ids))
	for i, id := range ids {
		pkgs[i] = schema.Package{ID: id}
	}
	f := newFile(pkgs...)
	var buf bytes.Buffer
	List(f, &buf)
	out := buf.String()
	for _, id := range ids {
		if !strings.Contains(out, id) {
			t.Errorf("expected %q in list output, got: %q", id, out)
		}
	}
}

// TestList_EmptyManagersMapShowsDash verifies that an explicitly empty (non-nil)
// managers map displays "-" rather than blank or panic.
func TestList_EmptyManagersMapShowsDash(t *testing.T) {
	f := newFile(schema.Package{ID: "git", Managers: map[string]string{}})
	var buf bytes.Buffer
	List(f, &buf)
	out := buf.String()
	if !strings.Contains(out, "-") {
		t.Errorf("expected '-' for empty managers map, got: %q", out)
	}
}
