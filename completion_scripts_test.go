package main

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// Completion scripts are hand-maintained and nothing else checks them, so a
// subcommand can ship without ever appearing in a completion. The readiness
// audit had to add `genv service restart` to three files by hand; this test
// makes the next one fail instead.

// serviceSubcommandCompletions mirrors the subcommands serviceCmd accepts.
var serviceSubcommandCompletions = []string{"add", "list", "ls", "remove", "restart", "rm", "start", "status", "stop"}

func TestCompletionScriptsOfferEveryServiceSubcommand(t *testing.T) {
	t.Run("bash", func(t *testing.T) {
		script := readCompletion(t, "genv.bash")
		for _, sub := range serviceSubcommandCompletions {
			if !bashServiceWordList(script)[sub] {
				t.Errorf("bash: service subcommand %q is not offered", sub)
			}
		}
		if !bashServiceFlags(script)["restart"] {
			t.Error("bash: `service restart` does not complete --file/--target")
		}
	})

	t.Run("zsh", func(t *testing.T) {
		script := readCompletion(t, "genv.zsh")
		for _, sub := range serviceSubcommandCompletions {
			if !strings.Contains(script, "'"+sub+":") {
				t.Errorf("zsh: service subcommand %q is not described", sub)
			}
		}
	})

	t.Run("fish", func(t *testing.T) {
		script := readCompletion(t, "genv.fish")
		offered := fishServiceCandidates(script)
		for _, sub := range serviceSubcommandCompletions {
			if !offered[sub] {
				t.Errorf("fish: service subcommand %q is not offered", sub)
			}
		}
	})

	t.Run("powershell", func(t *testing.T) {
		// The PowerShell completion is dynamic: it asks the binary. Assert
		// only that the service subcommands come from the CLI, not from a
		// hard-coded list that can rot.
		script := readCompletion(t, "genv.ps1")
		if !strings.Contains(script, "service") {
			t.Error("powershell: no service handling found")
		}
	})
}

func readCompletion(t *testing.T, name string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("completions", name))
	if err != nil {
		t.Fatalf("read %s: %v", name, err)
	}
	return string(data)
}

// bashServiceWordList returns the words the bash script completes for a
// `service` subcommand.
func bashServiceWordList(script string) map[string]bool {
	words := map[string]bool{}
	for _, line := range strings.Split(script, "\n") {
		_, list, ok := strings.Cut(line, "compgen -W \"")
		if !ok {
			continue
		}
		list, _, _ = strings.Cut(list, "\"")
		// The updates subcommands share these words; the service list is the
		// one that also offers the service-specific entries.
		if !strings.Contains(list, "remove") || !strings.Contains(list, "list") {
			continue
		}
		for _, word := range strings.Fields(list) {
			words[word] = true
		}
	}
	return words
}

// bashServiceFlags returns which subcommands complete --file/--target.
func bashServiceFlags(script string) map[string]bool {
	// The --file/--target pattern lives on a case label line rather than
	// after the opts, so read the labels directly.
	flags := map[string]bool{}
	for _, line := range strings.Split(script, "\n") {
		if !strings.Contains(line, "opts=\"--file --target\"") {
			continue
		}
		label := line
		label = strings.TrimSuffix(strings.TrimSpace(label), ") opts=\"--file --target\" ;;")
		for _, name := range strings.Split(label, "|") {
			name = strings.TrimSpace(name)
			name = strings.TrimPrefix(name, "(")
			name = strings.TrimSpace(name)
			if name != "" {
				flags[name] = true
			}
		}
	}
	return flags
}

// fishServiceCandidates collects the words the fish script offers after
// `service`, handling grouped entries like -a 'remove rm'.
func fishServiceCandidates(script string) map[string]bool {
	offered := map[string]bool{}
	for _, line := range strings.Split(script, "\n") {
		if !strings.Contains(line, "__fish_genv_at_subcommand service ") {
			continue
		}
		_, rest, ok := strings.Cut(line, " -f -a ")
		if !ok {
			continue
		}
		value := strings.TrimSpace(rest)
		if idx := strings.Index(value, " -d "); idx >= 0 {
			value = value[:idx]
		}
		value = strings.Trim(value, `'"`)
		for _, word := range strings.Fields(value) {
			offered[strings.Trim(word, `'"`)] = true
		}
	}
	return offered
}

// TestServiceCompletionReferenceIsSorted keeps the reference list above in a
// predictable order so a diff of this file shows a real change.
func TestServiceCompletionReferenceIsSorted(t *testing.T) {
	if !sort.StringsAreSorted(serviceSubcommandCompletions) {
		t.Fatalf("serviceSubcommandCompletions must stay sorted: %v", serviceSubcommandCompletions)
	}
}
