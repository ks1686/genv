package service

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
)

// AgentProgramIssue describes a genv-managed supervisor artifact whose primary
// executable path is missing or not executable.
type AgentProgramIssue struct {
	Label  string
	Path   string
	Detail string
}

var (
	launchdProgramArgsRE = regexp.MustCompile(`(?s)<key>ProgramArguments</key>\s*<array>\s*<string>([^<]*)</string>`)
	systemdExecStartRE   = regexp.MustCompile(`(?m)^ExecStart=(.*)$`)
)

// FirstLaunchdProgramArgument returns ProgramArguments[0] from a launchd plist.
func FirstLaunchdProgramArgument(plist []byte) (string, error) {
	m := launchdProgramArgsRE.FindSubmatch(plist)
	if m == nil {
		return "", fmt.Errorf("ProgramArguments not found")
	}
	return string(m[1]), nil
}

// FirstSystemdExecStartArgument returns the first ExecStart token from a unit file.
func FirstSystemdExecStartArgument(unit string) (string, error) {
	m := systemdExecStartRE.FindStringSubmatch(unit)
	if m == nil {
		return "", fmt.Errorf("ExecStart not found")
	}
	toks := splitSystemdExecStart(m[1])
	if len(toks) == 0 || toks[0] == "" {
		return "", fmt.Errorf("ExecStart has no command")
	}
	return toks[0], nil
}

func splitSystemdExecStart(line string) []string {
	line = strings.TrimSpace(line)
	var out []string
	for len(line) > 0 {
		if line[0] == '"' {
			end := 1
			for end < len(line) {
				if line[end] == '\\' && end+1 < len(line) {
					end += 2
					continue
				}
				if line[end] == '"' {
					break
				}
				end++
			}
			if end >= len(line) {
				out = append(out, unescapeSystemdArg(line[1:]))
				break
			}
			out = append(out, unescapeSystemdArg(line[1:end]))
			line = strings.TrimSpace(line[end+1:])
			continue
		}
		sp := strings.IndexByte(line, ' ')
		if sp < 0 {
			out = append(out, line)
			break
		}
		out = append(out, line[:sp])
		line = strings.TrimSpace(line[sp+1:])
	}
	return out
}

func unescapeSystemdArg(s string) string {
	s = strings.ReplaceAll(s, `\"`, `"`)
	return strings.ReplaceAll(s, `\\`, `\`)
}

// ExecutablePathStatus reports whether path exists and is executable by the current user.
func ExecutablePathStatus(path string) error {
	path = strings.TrimSpace(path)
	if path == "" {
		return fmt.Errorf("empty executable path")
	}
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	if info.IsDir() {
		return fmt.Errorf("%q is a directory", path)
	}
	mode := info.Mode()
	// Windows does not persist POSIX execute bits; existence is enough.
	if runtime.GOOS != "windows" && mode&0o111 == 0 {
		return fmt.Errorf("%q is not executable", path)
	}
	return nil
}

// ListManagedAgentProgramIssues scans genv-managed launchd plists and systemd
// units under home for primary executables that are missing or not executable.
func ListManagedAgentProgramIssues(home string) []AgentProgramIssue {
	var issues []AgentProgramIssue
	issues = append(issues, launchdAgentProgramIssues(home)...)
	issues = append(issues, systemdAgentProgramIssues(home)...)
	return issues
}

func launchdAgentProgramIssues(home string) []AgentProgramIssue {
	agentDir := filepath.Join(home, "Library", "LaunchAgents")
	entries, err := os.ReadDir(agentDir)
	if err != nil {
		return nil
	}
	var issues []AgentProgramIssue
	for _, ent := range entries {
		name := ent.Name()
		if ent.IsDir() || !strings.HasPrefix(name, "genv.") || !strings.HasSuffix(name, ".plist") {
			continue
		}
		content, err := os.ReadFile(filepath.Join(agentDir, name))
		if err != nil {
			continue
		}
		prog, err := FirstLaunchdProgramArgument(content)
		if err != nil || prog == "" {
			continue
		}
		// Skip sentinel commands used by placeholder services.
		if prog == "true" || prog == "/usr/bin/true" || prog == "/bin/true" {
			continue
		}
		if err := ExecutablePathStatus(prog); err != nil {
			label := strings.TrimSuffix(name, ".plist")
			issues = append(issues, AgentProgramIssue{
				Label:  label,
				Path:   prog,
				Detail: fmt.Sprintf("%s ProgramArguments[0]=%q: %v", label, prog, err),
			})
		}
	}
	return issues
}

func systemdAgentProgramIssues(home string) []AgentProgramIssue {
	unitDir := filepath.Join(home, ".config", "systemd", "user")
	entries, err := os.ReadDir(unitDir)
	if err != nil {
		return nil
	}
	var issues []AgentProgramIssue
	for _, ent := range entries {
		name := ent.Name()
		if ent.IsDir() || !strings.HasPrefix(name, "genv-") || !strings.HasSuffix(name, ".service") {
			continue
		}
		content, err := os.ReadFile(filepath.Join(unitDir, name))
		if err != nil {
			continue
		}
		prog, err := FirstSystemdExecStartArgument(string(content))
		if err != nil || prog == "" {
			continue
		}
		if prog == "true" || prog == "/usr/bin/true" || prog == "/bin/true" {
			continue
		}
		if err := ExecutablePathStatus(prog); err != nil {
			label := strings.TrimSuffix(name, ".service")
			issues = append(issues, AgentProgramIssue{
				Label:  label,
				Path:   prog,
				Detail: fmt.Sprintf("%s ExecStart[0]=%q: %v", label, prog, err),
			})
		}
	}
	return issues
}
