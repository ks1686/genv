package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/ks1686/genv/internal/service"
	"github.com/ks1686/genv/internal/verify"
)

// serviceRestartCmd implements `genv service restart <name>`.
//
// The point of the command is that an operator whose service is misbehaving
// should not have to know whether it is brew-, launchd-, systemd- or
// command-managed, nor type a stop and a start by hand. Precedence, refusal
// rules and the dependency policy live in service.RestartDeclared; this layer
// owns argument handling, the pre-flight dependency report, readiness
// verification and the exit codes.
//
// Exit codes: 0 restarted, exitUsage for a missing name, exitLogic for an
// unknown service, a refused restart, a failed restart or a failed readiness
// check. Readiness is reported separately from the restart itself because the
// remedies differ — one means the command failed, the other means it worked and
// the service is unhealthy.
func serviceRestartCmd(args []string) int {
	fs := flag.NewFlagSet("service restart", flag.ContinueOnError)
	file := fs.String("file", defaultSpecPath(), "path to genv.json")
	targetFlag := fs.String("target", "", targetFlagHelpWithDefault)

	name, flagArgs := extractPositional(args)
	if err := fs.Parse(flagArgs); err != nil {
		return flagParseExit(err)
	}
	if name == "" {
		fPrintln(os.Stderr, "genv service restart: name is required")
		return exitUsage
	}

	f, code := readMaterializedSpec("service restart", *file, "", *targetFlag)
	if code != exitOK {
		return code
	}

	svc, ok := f.Services[name]
	if !ok {
		fprintf(os.Stderr, "genv: service %q not found in spec\n", name)
		return exitLogic
	}

	sourceRoot := sourceRootForSpec(*file, f)
	ctx := context.Background()

	// A service that requires something which is down will start and then
	// fail. Say which dependency is down instead of producing that; the
	// dependency itself is never restarted behind the operator's back.
	if missing := service.MissingDependencies(ctx, name, svc, f.Services, sourceRoot); len(missing) > 0 {
		fprintf(os.Stderr, "genv: service %q requires %s, which %s not running\n",
			name, strings.Join(missing, ", "), pluralIsAre(len(missing)))
		fprintf(os.Stderr, "genv: start %s first, or restart it deliberately, then retry\n",
			strings.Join(missing, " and "))
		return exitLogic
	}

	fprintf(os.Stdout, "Restarting service %q", name)
	if svc.BrewFormula != "" {
		fprintf(os.Stdout, " via brew services: %s\n", svc.BrewFormula)
	} else if svc.DeclaresScheduledTask() {
		// RestartDeclared will refuse this below. Do not falsely describe an
		// empty raw stop/start fallback while it does so.
		fPrintln(os.Stdout, " is not supported for a scheduled task")
	} else if svc.DeclaresLaunchd() {
		fprintf(os.Stdout, " via launchd\n")
	} else if svc.DeclaresSystemd() {
		fprintf(os.Stdout, " via systemd --user\n")
	} else if len(svc.Restart) > 0 {
		fprintf(os.Stdout, ": %s\n", strings.Join(svc.Restart, " "))
	} else {
		fprintf(os.Stdout, ": %s (stop) then %s (start)\n",
			strings.Join(svc.Stop, " "), strings.Join(svc.Start, " "))
	}

	if err := service.RestartDeclared(ctx, name, svc, sourceRoot); err != nil {
		fprintf(os.Stderr, "genv: failed to restart service %q: %v\n", name, err)
		if service.IsSystemdAvailable() && svc.DeclaresSystemd() {
			fprintf(os.Stderr, "Tip: to view logs run: %s\n", service.SystemdLogsHint(name))
		}
		return exitLogic
	}
	fprintf(os.Stdout, "Restarted service %q\n", name)

	// Readiness is a separate, optional verdict. It is never inferred from a
	// successful restart: a health check that was declared is run, and a
	// failure is reported as a failure even though the restart itself worked.
	if hc := svc.HealthCheck; hc != nil {
		if err := verify.Health(ctx, hc, func(ctx context.Context) error {
			return runHealthProbe(ctx, hc.Command)
		}); err != nil {
			fprintf(os.Stderr, "genv: service %q restarted but is not ready: %v\n", name, err)
			return exitLogic
		}
		fprintf(os.Stdout, "Service %q is ready\n", name)
	}
	return exitOK
}

func pluralIsAre(n int) string {
	if n == 1 {
		return "is"
	}
	return "are"
}

// runHealthProbe executes one health-check command. Shared with the upgrade
// restart phase so a manual restart and an automatic one judge readiness the
// same way.
func runHealthProbe(ctx context.Context, command []string) error {
	if len(command) == 0 {
		return fmt.Errorf("health_check declares no command")
	}
	return exec.CommandContext(ctx, command[0], command[1:]...).Run()
}
