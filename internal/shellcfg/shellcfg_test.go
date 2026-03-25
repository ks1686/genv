package shellcfg

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ks1686/genv/internal/genvfile"
	"github.com/ks1686/genv/internal/schema"
)

// ─── singleQuote ─────────────────────────────────────────────────────────────

func TestSingleQuote(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"simple", "'simple'"},
		{"it's a test", `'it'\''s a test'`},
		{"", "''"},
	}
	for _, c := range cases {
		got := singleQuote(c.in)
		if got != c.want {
			t.Errorf("singleQuote(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// ─── WriteFragment ────────────────────────────────────────────────────────────

func TestWriteFragment_Aliases(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "shell.sh")

	cfg := &schema.ShellConfig{
		Aliases: map[string]schema.ShellAlias{
			"ll": {Value: "ls -la"},
			"gs": {Value: "git status"},
		},
	}
	if err := WriteFragment(path, cfg); err != nil {
		t.Fatalf("WriteFragment: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	content := string(data)

	if !strings.Contains(content, "alias ll=") {
		t.Error("expected alias ll in fragment")
	}
	if !strings.Contains(content, "alias gs=") {
		t.Error("expected alias gs in fragment")
	}
	if !strings.Contains(content, "'ls -la'") {
		t.Error("expected single-quoted value for ll")
	}
}

func TestWriteFragment_ShellGuard_Bash(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "shell.sh")

	cfg := &schema.ShellConfig{
		Aliases: map[string]schema.ShellAlias{
			"ll": {Value: "ls -la", Shell: "bash"},
		},
	}
	if err := WriteFragment(path, cfg); err != nil {
		t.Fatalf("WriteFragment: %v", err)
	}

	data, _ := os.ReadFile(path)
	content := string(data)

	if !strings.Contains(content, "$BASH_VERSION") {
		t.Error("expected BASH_VERSION guard for bash-only alias")
	}
}

func TestWriteFragment_ShellGuard_Zsh(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "shell.sh")

	cfg := &schema.ShellConfig{
		Aliases: map[string]schema.ShellAlias{
			"gc": {Value: "git commit", Shell: "zsh"},
		},
	}
	if err := WriteFragment(path, cfg); err != nil {
		t.Fatalf("WriteFragment: %v", err)
	}

	data, _ := os.ReadFile(path)
	if !strings.Contains(string(data), "$ZSH_VERSION") {
		t.Error("expected ZSH_VERSION guard for zsh-only alias")
	}
}

func TestWriteFragment_Functions(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "shell.sh")

	cfg := &schema.ShellConfig{
		Functions: map[string]schema.ShellFunction{
			"mkcd": {Body: "mkdir -p \"$1\" && cd \"$1\""},
		},
	}
	if err := WriteFragment(path, cfg); err != nil {
		t.Fatalf("WriteFragment: %v", err)
	}

	data, _ := os.ReadFile(path)
	content := string(data)

	if !strings.Contains(content, "mkcd()") {
		t.Error("expected function definition")
	}
	if !strings.Contains(content, "mkdir -p") {
		t.Error("expected function body")
	}
}

func TestWriteFragment_Source(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "shell.sh")

	cfg := &schema.ShellConfig{
		Source: []string{"/path/to/script.sh"},
	}
	if err := WriteFragment(path, cfg); err != nil {
		t.Fatalf("WriteFragment: %v", err)
	}

	data, _ := os.ReadFile(path)
	if !strings.Contains(string(data), ". /path/to/script.sh") {
		t.Error("expected source line in fragment")
	}
}

func TestWriteFragment_Empty_RemovesFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "shell.sh")

	// Create first.
	cfg := &schema.ShellConfig{Aliases: map[string]schema.ShellAlias{"ll": {Value: "ls -la"}}}
	if err := WriteFragment(path, cfg); err != nil {
		t.Fatalf("WriteFragment: %v", err)
	}

	// Write empty — should remove.
	if err := WriteFragment(path, nil); err != nil {
		t.Fatalf("WriteFragment(nil): %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Error("expected fragment to be removed")
	}
}

func TestWriteFragment_Deterministic(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "shell.sh")

	cfg := &schema.ShellConfig{
		Aliases: map[string]schema.ShellAlias{
			"zzz": {Value: "z"},
			"aaa": {Value: "a"},
			"mmm": {Value: "m"},
		},
	}
	if err := WriteFragment(path, cfg); err != nil {
		t.Fatalf("WriteFragment: %v", err)
	}
	data, _ := os.ReadFile(path)
	content := string(data)

	iAAA := strings.Index(content, "alias aaa=")
	iMMM := strings.Index(content, "alias mmm=")
	iZZZ := strings.Index(content, "alias zzz=")
	if !(iAAA < iMMM && iMMM < iZZZ) {
		t.Errorf("fragment not sorted: aaa@%d mmm@%d zzz@%d", iAAA, iMMM, iZZZ)
	}
}

// ─── ShellStatus ─────────────────────────────────────────────────────────────

func TestShellStatus_AllOK(t *testing.T) {
	spec := &schema.ShellConfig{
		Aliases: map[string]schema.ShellAlias{"ll": {Value: "ls -la"}},
	}
	lock := &genvfile.LockedShellConfig{
		Aliases: []genvfile.LockedShellAlias{{Name: "ll", Value: "ls -la"}},
	}
	entries := ShellStatus(spec, lock)
	for _, e := range entries {
		if e.Kind != ShellStatusOK {
			t.Errorf("%s: expected ok, got %s", e.Name, e.Kind)
		}
	}
}

func TestShellStatus_Missing(t *testing.T) {
	spec := &schema.ShellConfig{
		Aliases: map[string]schema.ShellAlias{"ll": {Value: "ls -la"}},
	}
	entries := ShellStatus(spec, nil)
	if len(entries) != 1 || entries[0].Kind != ShellStatusMissing {
		t.Errorf("expected 1 missing entry, got %v", entries)
	}
}

func TestShellStatus_Extra(t *testing.T) {
	lock := &genvfile.LockedShellConfig{
		Aliases: []genvfile.LockedShellAlias{{Name: "orphan", Value: "x"}},
	}
	entries := ShellStatus(nil, lock)
	if len(entries) != 1 || entries[0].Kind != ShellStatusExtra {
		t.Errorf("expected 1 extra entry, got %v", entries)
	}
}

func TestShellStatus_Modified(t *testing.T) {
	spec := &schema.ShellConfig{
		Aliases: map[string]schema.ShellAlias{"ll": {Value: "ls -la"}},
	}
	lock := &genvfile.LockedShellConfig{
		Aliases: []genvfile.LockedShellAlias{{Name: "ll", Value: "ls -l"}},
	}
	entries := ShellStatus(spec, lock)
	if len(entries) != 1 || entries[0].Kind != ShellStatusModified {
		t.Errorf("expected 1 modified entry, got %v", entries)
	}
}

func TestShellStatus_Functions(t *testing.T) {
	spec := &schema.ShellConfig{
		Functions: map[string]schema.ShellFunction{"mkcd": {Body: "mkdir $1 && cd $1"}},
	}
	lock := &genvfile.LockedShellConfig{
		Functions: []genvfile.LockedShellFunction{{Name: "mkcd", Body: "mkdir $1 && cd $1"}},
	}
	entries := ShellStatus(spec, lock)
	if len(entries) != 1 || entries[0].Kind != ShellStatusOK {
		t.Errorf("expected 1 ok function entry, got %v", entries)
	}
	if entries[0].EntryType != "function" {
		t.Errorf("expected EntryType=function, got %q", entries[0].EntryType)
	}
}

func TestShellStatus_Source(t *testing.T) {
	spec := &schema.ShellConfig{
		Source: []string{"/path/to/script.sh"},
	}
	lock := &genvfile.LockedShellConfig{
		Source: []string{"/path/to/script.sh"},
	}
	entries := ShellStatus(spec, lock)
	if len(entries) != 1 || entries[0].Kind != ShellStatusOK {
		t.Errorf("expected 1 ok source entry, got %v", entries)
	}
	if entries[0].EntryType != "source" {
		t.Errorf("expected EntryType=source, got %q", entries[0].EntryType)
	}
}

// ─── SpecToLock ───────────────────────────────────────────────────────────────

func TestSpecToLock_Nil(t *testing.T) {
	if SpecToLock(nil) != nil {
		t.Error("expected nil for nil spec")
	}
}

func TestSpecToLock_Roundtrip(t *testing.T) {
	spec := &schema.ShellConfig{
		Aliases: map[string]schema.ShellAlias{
			"ll": {Value: "ls -la", Shell: "zsh"},
		},
		Functions: map[string]schema.ShellFunction{
			"mkcd": {Body: "mkdir $1 && cd $1"},
		},
		Source: []string{"/some/path.sh"},
	}
	lsc := SpecToLock(spec)
	if lsc == nil {
		t.Fatal("expected non-nil LockedShellConfig")
	}
	if len(lsc.Aliases) != 1 || lsc.Aliases[0].Name != "ll" || lsc.Aliases[0].Value != "ls -la" {
		t.Errorf("alias roundtrip failed: %+v", lsc.Aliases)
	}
	if len(lsc.Functions) != 1 || lsc.Functions[0].Name != "mkcd" {
		t.Errorf("function roundtrip failed: %+v", lsc.Functions)
	}
	if len(lsc.Source) != 1 || lsc.Source[0] != "/some/path.sh" {
		t.Errorf("source roundtrip failed: %+v", lsc.Source)
	}
}
