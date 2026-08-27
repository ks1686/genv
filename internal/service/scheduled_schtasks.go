package service

import (
	"context"
	"encoding/binary"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf16"
)

const (
	// Task Scheduler's documented minimum repetition interval is one minute.
	schtasksMinInterval = time.Minute
	// Repetition duration long enough to outlive a typical machine lifetime
	// (indefinite duration is version-dependent on /XML import).
	schtasksRepetitionDuration = "P3650D"
)

// schtasksRun invokes schtasks.exe. Tests replace it so Linux CI can cover
// register/status/stop without a Windows host.
var schtasksRun = func(ctx context.Context, args ...string) ([]byte, error) {
	return exec.CommandContext(ctx, "schtasks", args...).CombinedOutput()
}

func schtasksTaskName(name string) string {
	name = strings.ReplaceAll(name, "\\", "/")
	name = filepath.Base(name)
	if name == "." || name == "/" || name == "" {
		name = "default"
	}
	return "genv-" + name
}

func schtasksArtifactDir() (string, error) {
	home, err := homeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "genv", "scheduled"), nil
}

func schtasksCmdFileName(name string) string {
	return schtasksTaskName(name) + ".cmd"
}

func schtasksXMLFileName(name string) string {
	return schtasksTaskName(name) + ".xml"
}

func schtasksCmdExe() string {
	root := os.Getenv("SystemRoot")
	if root == "" {
		root = `C:\Windows`
	}
	return root + `\System32\cmd.exe`
}

func startSchtasksScheduledJob(ctx context.Context, job ScheduledJob) error {
	dir, err := schtasksArtifactDir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("creating scheduled task directory: %w", err)
	}
	scriptPath := filepath.Join(dir, schtasksCmdFileName(job.Name))
	xmlPath := filepath.Join(dir, schtasksXMLFileName(job.Name))
	if err := os.WriteFile(scriptPath, []byte(SchtasksScheduledCmdContent(job)), 0o644); err != nil {
		return fmt.Errorf("writing scheduled task script %q: %w", scriptPath, err)
	}
	xmlContent := SchtasksScheduledTaskXML(job.Name, schtasksCmdExe(), scriptPath, job.Interval)
	if err := os.WriteFile(xmlPath, encodeUTF16LE(xmlContent), 0o644); err != nil {
		return fmt.Errorf("writing scheduled task XML %q: %w", xmlPath, err)
	}
	taskName := schtasksTaskName(job.Name)
	if out, err := schtasksRun(ctx, "/Create", "/TN", taskName, "/XML", xmlPath, "/F"); err != nil {
		return fmt.Errorf("creating scheduled task %q: %w\n%s", taskName, err, decodeSchtasksOutput(out))
	}
	// Kick once through Task Scheduler so the process is a child of the
	// scheduler service, not this session. OpenSSH on Windows assigns a job
	// object to the session; processes that do not break away die on disconnect.
	_, _ = schtasksRun(ctx, "/Run", "/TN", taskName)
	return nil
}

func stopSchtasksScheduledJob(ctx context.Context, name string) error {
	taskName := schtasksTaskName(name)
	if out, err := schtasksRun(ctx, "/Delete", "/TN", taskName, "/F"); err != nil {
		decoded := decodeSchtasksOutput(out)
		if !schtasksTaskMissing(decoded) {
			return fmt.Errorf("deleting scheduled task %q: %w\n%s", taskName, err, decoded)
		}
	}
	dir, err := schtasksArtifactDir()
	if err != nil {
		return err
	}
	for _, path := range []string{
		filepath.Join(dir, schtasksCmdFileName(name)),
		filepath.Join(dir, schtasksXMLFileName(name)),
	} {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("removing scheduled task file %q: %w", path, err)
		}
	}
	return nil
}

// SchtasksScheduledCmdContent renders the .cmd wrapper that sets PATH (and any
// other environment) then execs the one-shot genv command.
func SchtasksScheduledCmdContent(job ScheduledJob) string {
	var b strings.Builder
	b.WriteString("@echo off\r\n")
	if len(job.Command) > 0 {
		fmt.Fprintf(&b, "rem genv-program: %s\r\n", stripLineBreaks(job.Command[0]))
	}
	for _, name := range sortedEnvironmentNames(job.Environment) {
		fmt.Fprintf(&b, "%s\r\n", cmdSetAssignment(name, job.Environment[name]))
	}
	if len(job.Command) == 0 {
		return b.String()
	}
	parts := make([]string, 0, len(job.Command))
	for _, arg := range job.Command {
		parts = append(parts, cmdQuoteArg(stripLineBreaks(arg)))
	}
	b.WriteString(strings.Join(parts, " "))
	b.WriteString("\r\n")
	return b.String()
}

// SchtasksScheduledTaskXML renders a Task Scheduler 1.3 XML definition:
// logon trigger (reboot/logon) plus repetition matching interval.
func SchtasksScheduledTaskXML(name, cmdExe, scriptPath string, interval time.Duration) string {
	name = stripLineBreaks(name)
	taskName := schtasksTaskName(name)
	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-16"?>
<Task version="1.3" xmlns="http://schemas.microsoft.com/windows/2004/02/mit/task">
  <RegistrationInfo>
    <Description>genv scheduled job: %s</Description>
    <URI>\%s</URI>
  </RegistrationInfo>
  <Triggers>
    <LogonTrigger>
      <Enabled>true</Enabled>
      <Delay>PT1M</Delay>
      <Repetition>
        <Interval>%s</Interval>
        <Duration>%s</Duration>
        <StopAtDurationEnd>false</StopAtDurationEnd>
      </Repetition>
    </LogonTrigger>
  </Triggers>
  <Principals>
    <Principal id="Author">
      <LogonType>InteractiveToken</LogonType>
      <RunLevel>LeastPrivilege</RunLevel>
    </Principal>
  </Principals>
  <Settings>
    <MultipleInstancesPolicy>IgnoreNew</MultipleInstancesPolicy>
    <DisallowStartIfOnBatteries>false</DisallowStartIfOnBatteries>
    <StopIfGoingOnBatteries>false</StopIfGoingOnBatteries>
    <AllowHardTerminate>true</AllowHardTerminate>
    <StartWhenAvailable>true</StartWhenAvailable>
    <RunOnlyIfNetworkAvailable>false</RunOnlyIfNetworkAvailable>
    <IdleSettings>
      <StopOnIdleEnd>false</StopOnIdleEnd>
      <RestartOnIdle>false</RestartOnIdle>
    </IdleSettings>
    <AllowStartOnDemand>true</AllowStartOnDemand>
    <Enabled>true</Enabled>
    <Hidden>true</Hidden>
    <RunOnlyIfIdle>false</RunOnlyIfIdle>
    <DisallowStartOnRemoteAppSession>false</DisallowStartOnRemoteAppSession>
    <UseUnifiedSchedulingEngine>true</UseUnifiedSchedulingEngine>
    <WakeToRun>false</WakeToRun>
    <ExecutionTimeLimit>%s</ExecutionTimeLimit>
    <Priority>7</Priority>
  </Settings>
  <Actions Context="Author">
    <Exec>
      <Command>%s</Command>
      <Arguments>%s</Arguments>
    </Exec>
  </Actions>
</Task>
`, xmlEscape(name), xmlEscape(taskName), schtasksRepetitionInterval(interval), schtasksRepetitionDuration, schtasksExecutionTimeLimit(interval), xmlEscape(stripLineBreaks(cmdExe)), xmlEscape(schtasksCmdArguments(scriptPath)))
}

func schtasksCmdArguments(scriptPath string) string {
	return `/d /c call ` + cmdQuoteArg(stripLineBreaks(scriptPath))
}

func schtasksRepetitionInterval(interval time.Duration) string {
	sec := max(int(interval.Seconds()), int(schtasksMinInterval.Seconds()))
	if sec%3600 == 0 {
		return fmt.Sprintf("PT%dH", sec/3600)
	}
	if sec%60 == 0 {
		return fmt.Sprintf("PT%dM", sec/60)
	}
	return fmt.Sprintf("PT%dS", sec)
}

func schtasksExecutionTimeLimit(interval time.Duration) string {
	return fmt.Sprintf("PT%dS", scheduledJobTimeOutSeconds(interval))
}

func cmdQuoteArg(arg string) string {
	return `"` + strings.ReplaceAll(arg, `"`, `""`) + `"`
}

func cmdSetAssignment(name, value string) string {
	value = stripLineBreaks(value)
	value = strings.ReplaceAll(value, `"`, "")
	value = strings.ReplaceAll(value, "%", "%%")
	return fmt.Sprintf("set \"%s=%s\"", stripLineBreaks(name), value)
}

func encodeUTF16LE(s string) []byte {
	u := utf16.Encode([]rune(s))
	buf := make([]byte, 2+len(u)*2)
	buf[0], buf[1] = 0xFF, 0xFE
	for i, r := range u {
		binary.LittleEndian.PutUint16(buf[2+i*2:], r)
	}
	return buf
}

func decodeSchtasksOutput(b []byte) string {
	if len(b) >= 2 && b[0] == 0xFF && b[1] == 0xFE {
		u16 := make([]uint16, 0, (len(b)-2)/2)
		for i := 2; i+1 < len(b); i += 2 {
			u16 = append(u16, binary.LittleEndian.Uint16(b[i:i+2]))
		}
		return string(utf16.Decode(u16))
	}
	return string(b)
}

func schtasksTaskMissing(output string) bool {
	message := strings.ToLower(output)
	return strings.Contains(message, "the system cannot find the file specified") ||
		strings.Contains(message, "cannot find the path specified") ||
		strings.Contains(message, "does not exist in the system") ||
		(strings.Contains(message, "the specified task name") && strings.Contains(message, "does not exist"))
}
