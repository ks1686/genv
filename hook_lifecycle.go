package main

import (
	"context"
	"io"
	"os"
	"strings"
	"time"

	"github.com/ks1686/genv/internal/genvfile"
	"github.com/ks1686/genv/internal/hooks"
	"github.com/ks1686/genv/internal/resolver"
	"github.com/ks1686/genv/internal/schema"
)

type hookContext struct {
	Event           string
	Phase           string
	Host            string
	Profile         string
	DryRun          bool
	Installed       []string
	Removed         []string
	Upgraded        []string
	Failed          []string
	Skipped         []string
	UpgradeManagers []string
}

type hookPhaseRun struct {
	Hooks   []schema.Hook
	Context hookContext
	Timeout time.Duration
	Stdout  io.Writer
	Stderr  io.Writer
}

func runHookPhase(ctx context.Context, req hookPhaseRun) []string {
	if len(req.Hooks) == 0 {
		return nil
	}
	exec := hooks.NewExecutor(req.Stdout, req.Stderr)
	opts := hooks.RunOptions{
		Host:    req.Context.Host,
		DryRun:  req.Context.DryRun,
		Env:     hookEnv(req.Context),
		Timeout: req.Timeout,
	}
	var err error
	switch req.Context.Phase {
	case "pre-apply":
		err = exec.PreApplyWithOptions(ctx, req.Hooks, opts)
	case "post-apply":
		err = exec.PostApplyWithOptions(ctx, req.Hooks, opts)
	case "pre-add":
		err = exec.PreAddWithOptions(ctx, req.Hooks, opts)
	case "post-add":
		err = exec.PostAddWithOptions(ctx, req.Hooks, opts)
	case "pre-remove":
		err = exec.PreRemoveWithOptions(ctx, req.Hooks, opts)
	case "post-remove":
		err = exec.PostRemoveWithOptions(ctx, req.Hooks, opts)
	case "pre-upgrade":
		err = exec.PreUpgradeWithOptions(ctx, req.Hooks, opts)
	case "post-upgrade":
		err = exec.PostUpgradeWithOptions(ctx, req.Hooks, opts)
	}
	if err != nil {
		return []string{err.Error()}
	}
	return nil
}

func hookEnv(ctx hookContext) []string {
	return []string{
		"GENV_EVENT=" + ctx.Event,
		"GENV_PHASE=" + ctx.Phase,
		"GENV_HOST=" + ctx.Host,
		"GENV_PROFILE=" + ctx.Profile,
		"GENV_DRY_RUN=" + boolString(ctx.DryRun),
		"GENV_INSTALLED=" + strings.Join(ctx.Installed, ","),
		"GENV_REMOVED=" + strings.Join(ctx.Removed, ","),
		"GENV_UPGRADED=" + strings.Join(ctx.Upgraded, ","),
		"GENV_FAILED=" + strings.Join(ctx.Failed, ","),
		"GENV_SKIPPED=" + strings.Join(ctx.Skipped, ","),
		"GENV_UPGRADE_MANAGERS=" + strings.Join(ctx.UpgradeManagers, ","),
	}
}

func boolString(v bool) string {
	if v {
		return "true"
	}
	return "false"
}

func hookWriters(jsonOut bool) (io.Writer, io.Writer) {
	if jsonOut {
		return os.Stderr, os.Stderr
	}
	return os.Stdout, os.Stderr
}

func runApplyHookPhase(ctx context.Context, f *schema.GenvFile, hctx hookContext, timeout time.Duration, jsonOut bool) []string {
	if f == nil || f.Hooks == nil {
		return nil
	}
	stdout, stderr := hookWriters(jsonOut)
	hooksForPhase := f.Hooks.PreApply
	if hctx.Phase == "post-apply" {
		hooksForPhase = f.Hooks.PostApply
	}
	return runHookPhase(ctx, hookPhaseRun{Hooks: hooksForPhase, Context: hctx, Timeout: timeout, Stdout: stdout, Stderr: stderr})
}

func plannedInstallIDs(result resolver.ReconcileResult) []string {
	ids := make([]string, 0, len(result.ToInstall))
	for _, action := range result.ToInstall {
		ids = append(ids, action.Pkg.ID)
	}
	return ids
}

func plannedRemoveIDs(result resolver.ReconcileResult) []string {
	ids := make([]string, 0, len(result.ToRemove))
	for _, action := range result.ToRemove {
		ids = append(ids, action.Pkg.ID)
	}
	return ids
}

func lockedPackageIDs(pkgs []genvfile.LockedPackage) []string {
	ids := make([]string, len(pkgs))
	for i, pkg := range pkgs {
		ids[i] = pkg.ID
	}
	return ids
}

func applyFailedIDs(errs []error) []string {
	if len(errs) == 0 {
		return nil
	}
	out := make([]string, len(errs))
	for i, err := range errs {
		out[i] = err.Error()
	}
	return out
}
