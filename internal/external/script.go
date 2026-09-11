package external

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/ks1686/genv/internal/profilebackend"
	"github.com/ks1686/genv/internal/schema"
)

// RunInstallerScript executes a downloaded script through its declared interpreter.
func RunInstallerScript(ctx context.Context, scriptPath string, install schema.ExternalInstall, values TemplateValues, stdin io.Reader, stderr io.Writer) error {
	interpreter, prefix, err := resolveInterpreter(install.Interpreter)
	if err != nil {
		return err
	}
	scriptPath, err = ensureScriptExtension(scriptPath, install.Interpreter)
	if err != nil {
		return err
	}
	values.Script = scriptPath
	args := append(prefix, scriptPath)
	for _, arg := range install.Args {
		expanded, err := ExpandInstallTemplate(arg, values)
		if err != nil {
			return err
		}
		args = append(args, expanded)
	}
	env := environmentMap(os.Environ())
	for key, value := range install.Env {
		expanded, err := ExpandInstallTemplate(value, values)
		if err != nil {
			return fmt.Errorf("expand installer environment %s: %w", key, err)
		}
		env[key] = expanded
	}
	cmd := exec.CommandContext(ctx, interpreter, args...)
	cmd.Stdin = stdin
	cmd.Stderr = stderr
	cmd.Stdout = stderr
	cmd.Env = sortedEnvironment(env)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("external installer failed: %w", err)
	}
	return nil
}

func expandCommand(command []string, values TemplateValues) ([]string, error) {
	expanded := make([]string, len(command))
	for i, arg := range command {
		value, err := ExpandInstallTemplate(arg, values)
		if err != nil {
			return nil, err
		}
		expanded[i] = value
	}
	return expanded, nil
}

// RunUninstall executes the exact argv persisted in a script installation receipt.
func RunUninstall(ctx context.Context, command []string, stdin io.Reader, output io.Writer) error {
	if len(command) == 0 {
		return fmt.Errorf("external script has no uninstall action")
	}
	path, err := exec.LookPath(command[0])
	if err != nil {
		return fmt.Errorf("external uninstall executable %q is unavailable", command[0])
	}
	cmd := exec.CommandContext(ctx, path, command[1:]...)
	cmd.Stdin = stdin
	cmd.Stdout = output
	cmd.Stderr = output
	cmd.Env = os.Environ()
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("external uninstall failed: %w", err)
	}
	return nil
}

func resolveInterpreter(name string) (string, []string, error) {
	switch name {
	case "sh", "bash", "pwsh":
		path, err := exec.LookPath(name)
		if err != nil {
			return "", nil, fmt.Errorf("external installer interpreter %q is unavailable", name)
		}
		if name == "pwsh" {
			return path, []string{"-NoProfile", "-File"}, nil
		}
		return path, nil, nil
	case "powershell":
		engine, ok := profilebackend.DetectEngine()
		if !ok {
			return "", nil, fmt.Errorf("external installer interpreter %q is unavailable", name)
		}
		return engine.Bin, []string{"-NoProfile", "-File"}, nil
	default:
		return "", nil, fmt.Errorf("unsupported external installer interpreter %q", name)
	}
}


// ensureScriptExtension gives PowerShell installers a .ps1 path. Download
// staging uses extensionless temp names, and pwsh -File rejects those on Windows.
func ensureScriptExtension(scriptPath, interpreter string) (string, error) {
	var ext string
	switch interpreter {
	case "pwsh", "powershell":
		ext = ".ps1"
	default:
		return scriptPath, nil
	}
	if strings.EqualFold(filepath.Ext(scriptPath), ext) {
		return scriptPath, nil
	}
	staged := scriptPath + ext
	if err := os.Rename(scriptPath, staged); err != nil {
		in, err := os.ReadFile(scriptPath)
		if err != nil {
			return "", fmt.Errorf("stage installer script extension: %w", err)
		}
		if err := os.WriteFile(staged, in, 0o700); err != nil {
			return "", fmt.Errorf("stage installer script extension: %w", err)
		}
	}
	return staged, nil
}

func environmentMap(values []string) map[string]string {
	out := make(map[string]string, len(values))
	for _, value := range values {
		key, item, ok := strings.Cut(value, "=")
		if ok {
			out[key] = item
		}
	}
	return out
}

func sortedEnvironment(values map[string]string) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	out := make([]string, 0, len(values))
	for _, key := range keys {
		out = append(out, key+"="+values[key])
	}
	return out
}
