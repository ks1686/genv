package external

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
)

// ErrElevationRequired is returned when a system-scope destination is not
// writable and interactive elevation is required.
var ErrElevationRequired = errors.New("run from an elevated session")

// ErrUnattendedElevation is returned when a scheduler run would need to
// escalate privileges. Unattended execution never prompts and never sudoes.
var ErrUnattendedElevation = errors.New("unattended execution cannot elevate")

// ElevationHint describes explicit system-scope elevation; it never implies a silent retry.
func ElevationHint(scope string) string {
	if scope != "system" {
		return ""
	}
	if runtime.GOOS == "windows" {
		return "requires elevated PowerShell"
	}
	return "requires sudo"
}

func wrapSystemScopeError(scope, destination string, err error) error {
	if err == nil || scope != "system" {
		return err
	}
	if errors.Is(err, ErrElevationRequired) || errors.Is(err, ErrUnattendedElevation) {
		return err
	}
	if !os.IsPermission(err) {
		return err
	}
	return fmt.Errorf("system-scope destination %s is not writable; %w: %w", destination, ErrElevationRequired, err)
}

type installPolicy struct {
	Scope  string
	Mode   ExecutionMode
	Stdin  io.Reader
	Output io.Writer
}

// destinationRequiresElevation reports whether installing to destination needs
// privileges. Tests replace it so Unix permission bits are not required on
// every host.
var destinationRequiresElevation = defaultDestinationRequiresElevation

func defaultDestinationRequiresElevation(destination string) bool {
	dir := filepath.Dir(destination)
	for {
		if _, err := os.Stat(dir); err == nil {
			return !dirWritable(dir)
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return true
		}
		dir = parent
	}
}

var runElevated = defaultRunElevated

func defaultRunElevated(stdin io.Reader, stderr io.Writer, argv []string) error {
	if len(argv) == 0 || argv[0] == "" {
		return fmt.Errorf("empty elevated command")
	}
	cmd := exec.Command(argv[0], argv[1:]...)
	if stdin != nil {
		cmd.Stdin = stdin
	} else {
		cmd.Stdin = os.Stdin
	}
	if stderr != nil {
		cmd.Stdout = stderr
		cmd.Stderr = stderr
	}
	return cmd.Run()
}

func elevateSystemScopeInstall(source, destination string, mode os.FileMode, policy installPolicy) (restore func(), finish func() error, err error) {
	if policy.Mode == ExecutionUnattended {
		return nil, nil, fmt.Errorf("system-scope destination %s is not writable: %w", destination, ErrUnattendedElevation)
	}
	if runtime.GOOS == "windows" {
		return nil, nil, fmt.Errorf("system-scope destination %s is not writable; %w", destination, ErrElevationRequired)
	}
	sudo, err := exec.LookPath("sudo")
	if err != nil {
		return nil, nil, fmt.Errorf("system-scope destination %s is not writable; %w", destination, ErrElevationRequired)
	}
	hadExisting := false
	if _, statErr := os.Lstat(destination); statErr == nil {
		hadExisting = true
	} else if !os.IsNotExist(statErr) {
		return nil, nil, statErr
	}
	modeStr := fmt.Sprintf("%04o", mode.Perm())
	destDir := filepath.Dir(destination)
	staged := destination + ".genv-install"
	backup := destination + ".genv-backup"
	run := func(args ...string) error {
		argv := append([]string{sudo}, args...)
		return runElevated(policy.Stdin, policy.Output, argv)
	}
	if err := run("mkdir", "-p", destDir); err != nil {
		return nil, nil, fmt.Errorf("system-scope destination %s is not writable; %w", destination, ErrElevationRequired)
	}
	if err := run("install", "-m", modeStr, source, staged); err != nil {
		return nil, nil, fmt.Errorf("elevating staging write for %s: %w", destination, err)
	}
	if hadExisting {
		_ = run("rm", "-f", backup)
		if err := run("mv", destination, backup); err != nil {
			_ = run("rm", "-f", staged)
			return nil, nil, fmt.Errorf("elevating backup of %s: %w", destination, err)
		}
	}
	if err := run("mv", staged, destination); err != nil {
		if hadExisting {
			_ = run("mv", backup, destination)
		}
		_ = run("rm", "-f", staged)
		return nil, nil, fmt.Errorf("elevating replacement of %s: %w", destination, err)
	}
	restored := false
	restore = func() {
		if restored {
			return
		}
		restored = true
		_ = run("rm", "-f", destination)
		if hadExisting {
			_ = run("mv", backup, destination)
		}
	}
	finish = func() error {
		if hadExisting {
			return run("rm", "-f", backup)
		}
		return nil
	}
	return restore, finish, nil
}
