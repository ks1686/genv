package profilebackend

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/ks1686/genv/internal/genvfile"
	"github.com/ks1686/genv/internal/schema"
)

// PowerShellBackend writes env.ps1 / shell.ps1 and injects into the CurrentUser
// CurrentHost PowerShell profile.
type PowerShellBackend struct {
	// Home overrides os.UserHomeDir in tests.
	Home string
	// Engine overrides DetectEngine in tests.
	Engine *Engine
}

func (PowerShellBackend) Name() string { return "powershell" }

func (b PowerShellBackend) ApplyEnv(vars map[string]schema.EnvVar) error {
	fragPath, err := b.envFragmentPath()
	if err != nil {
		return err
	}
	if err := WriteEnvPS1(fragPath, vars); err != nil {
		return err
	}
	if len(vars) == 0 {
		return nil
	}
	profile, err := b.profilePath()
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "genv: warning: could not resolve PowerShell profile: %v\n", err)
		return nil
	}
	if err := InjectProfileLine(profile, fragPath, "env"); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "genv: warning: could not inject PowerShell env into %s: %v\n", profile, err)
	}
	return nil
}

func (b PowerShellBackend) ApplyShell(cfg *schema.ShellConfig) error {
	fragPath, err := b.shellFragmentPath()
	if err != nil {
		return err
	}
	if err := WriteShellPS1(fragPath, cfg); err != nil {
		return err
	}
	if !hasPowerShellFragmentContent(cfg) {
		return nil
	}
	profile, err := b.profilePath()
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "genv: warning: could not resolve PowerShell profile: %v\n", err)
		return nil
	}
	if err := InjectProfileLine(profile, fragPath, "shell"); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "genv: warning: could not inject PowerShell shell into %s: %v\n", profile, err)
	}
	return nil
}

func (b PowerShellBackend) envFragmentPath() (string, error) {
	dir, err := genvfile.DefaultDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "env.ps1"), nil
}

func (b PowerShellBackend) shellFragmentPath() (string, error) {
	dir, err := genvfile.DefaultDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "shell.ps1"), nil
}

func (b PowerShellBackend) homeDir() (string, error) {
	if b.Home != "" {
		return b.Home, nil
	}
	return os.UserHomeDir()
}

func (b PowerShellBackend) engine() (Engine, bool) {
	if b.Engine != nil {
		return *b.Engine, true
	}
	return DetectEngine()
}

// profilePath returns the CurrentUserCurrentHost profile path for the detected engine.
func (b PowerShellBackend) profilePath() (string, error) {
	home, err := b.homeDir()
	if err != nil {
		return "", err
	}
	eng, ok := b.engine()
	if !ok {
		return "", fmt.Errorf("no PowerShell engine")
	}
	docs := filepath.Join(home, "Documents")
	if eng.IsPwsh() {
		return filepath.Join(docs, "PowerShell", "Microsoft.PowerShell_profile.ps1"), nil
	}
	return filepath.Join(docs, "WindowsPowerShell", "Microsoft.PowerShell_profile.ps1"), nil
}

// WriteEnvPS1 atomically writes a PowerShell fragment that sets every variable
// in vars. If vars is empty the fragment is removed.
func WriteEnvPS1(path string, vars map[string]schema.EnvVar) error {
	if len(vars) == 0 {
		err := os.Remove(path)
		if err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("removing empty fragment %s: %w", path, err)
		}
		return nil
	}

	var sb strings.Builder
	sb.WriteString("# genv managed env — do not edit between these markers\n")
	sb.WriteString("# BEGIN genv env\n")

	names := make([]string, 0, len(vars))
	for name := range vars {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		ev := vars[name]
		sb.WriteString("$env:")
		sb.WriteString(name)
		sb.WriteString(" = ")
		sb.WriteString(psSingleQuote(ev.Value))
		sb.WriteString("\n")
	}
	sb.WriteString("# END genv env\n")

	return writeAtomic(path, sb.String())
}

// WriteShellPS1 writes PowerShell-targeted aliases/functions plus shared source
// entries. Non-powershell shell targets are ignored. Empty content removes the file.
func WriteShellPS1(path string, cfg *schema.ShellConfig) error {
	if !hasPowerShellFragmentContent(cfg) {
		err := os.Remove(path)
		if err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("removing empty shell fragment %s: %w", path, err)
		}
		return nil
	}

	var sb strings.Builder
	sb.WriteString("# genv managed shell config — do not edit between these markers\n")
	sb.WriteString("# BEGIN genv shell\n")

	aliases := make([]string, 0)
	for name, a := range cfg.Aliases {
		if a.Shell != "powershell" {
			continue
		}
		aliases = append(aliases, name)
	}
	sort.Strings(aliases)
	if len(aliases) > 0 {
		sb.WriteString("\n# aliases\n")
		for _, name := range aliases {
			a := cfg.Aliases[name]
			_, _ = fmt.Fprintf(&sb, "function %s { %s }\n", name, a.Value)
		}
	}

	funcs := make([]string, 0)
	for name, fn := range cfg.Functions {
		if fn.Shell != "powershell" {
			continue
		}
		funcs = append(funcs, name)
	}
	sort.Strings(funcs)
	if len(funcs) > 0 {
		sb.WriteString("\n# functions\n")
		for _, name := range funcs {
			fn := cfg.Functions[name]
			_, _ = fmt.Fprintf(&sb, "function %s {\n%s\n}\n", name, indentPS(fn.Body))
		}
	}

	if len(cfg.Source) > 0 {
		sb.WriteString("\n# source\n")
		for _, s := range cfg.Source {
			_, _ = fmt.Fprintf(&sb, ". %s\n", psSingleQuote(s))
		}
	}

	sb.WriteString("\n# END genv shell\n")
	return writeAtomic(path, sb.String())
}

func hasPowerShellFragmentContent(cfg *schema.ShellConfig) bool {
	if cfg == nil {
		return false
	}
	if len(cfg.Source) > 0 {
		return true
	}
	for _, a := range cfg.Aliases {
		if a.Shell == "powershell" {
			return true
		}
	}
	for _, fn := range cfg.Functions {
		if fn.Shell == "powershell" {
			return true
		}
	}
	return false
}

// InjectProfileLine ensures fragmentPath is dot-sourced exactly once in profilePath
// inside a marked block for kind ("env" or "shell").
func InjectProfileLine(profilePath, fragmentPath, kind string) error {
	begin := fmt.Sprintf("# BEGIN genv %s", kind)
	end := fmt.Sprintf("# END genv %s", kind)
	dot := ". " + psSingleQuote(fragmentPath)

	data, err := os.ReadFile(profilePath)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("reading %s: %w", profilePath, err)
	}
	content := string(data)
	if strings.Contains(content, fragmentPath) || strings.Contains(content, begin) {
		return nil
	}

	dir := filepath.Dir(profilePath)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("creating directory %s: %w", dir, err)
	}

	f, err := os.OpenFile(profilePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("opening %s: %w", profilePath, err)
	}
	defer func() { _ = f.Close() }()

	_, err = fmt.Fprintf(f, "\n%s\n%s\n%s\n", begin, dot, end)
	return err
}

// psSingleQuote wraps v in PowerShell single quotes (' → ”).
func psSingleQuote(v string) string {
	var b strings.Builder
	b.Grow(len(v) + 2)
	b.WriteByte('\'')
	for i := 0; i < len(v); i++ {
		if v[i] == '\'' {
			b.WriteString("''")
			continue
		}
		b.WriteByte(v[i])
	}
	b.WriteByte('\'')
	return b.String()
}

func indentPS(s string) string {
	lines := strings.Split(s, "\n")
	for i, l := range lines {
		lines[i] = "  " + l
	}
	return strings.Join(lines, "\n")
}

func writeAtomic(path, content string) error {
	return genvfile.WritePrivate(path, []byte(content))
}
