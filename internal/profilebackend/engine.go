// Package profilebackend selects and applies env/shell profile fragments for
// POSIX shells and Windows PowerShell.
package profilebackend

import (
	"os/exec"
	"path/filepath"
	"strings"
)

// Engine is a detected PowerShell binary (pwsh preferred over Windows PowerShell).
type Engine struct {
	// Bin is the absolute path returned by LookPath, or the bare name when that
	// is all LookPath provides.
	Bin string
}

// lookPath is exec.LookPath; tests replace it with PATH fakes.
var lookPath = exec.LookPath

// DetectEngine looks up pwsh, then powershell / powershell.exe on PATH.
// It returns false when no PowerShell engine is available.
func DetectEngine() (Engine, bool) {
	for _, name := range []string{"pwsh", "powershell", "powershell.exe"} {
		path, err := lookPath(name)
		if err != nil {
			continue
		}
		return Engine{Bin: path}, true
	}
	return Engine{}, false
}

// IsPwsh reports whether eng looks like PowerShell 7+ (pwsh) rather than
// Windows PowerShell 5.1.
func (eng Engine) IsPwsh() bool {
	base := strings.ToLower(filepath.Base(eng.Bin))
	base = strings.TrimSuffix(base, ".exe")
	return base == "pwsh"
}
