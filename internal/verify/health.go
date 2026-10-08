package verify

import (
	"context"
	"fmt"
	"time"

	"github.com/ks1686/genv/internal/schema"
)

// Health waits for a service to report itself ready.
//
// It is called only after genv has started or restarted a service — never from
// status, never from a dry run. That is a rule, not a convention: a probe runs a
// command, and a plan that runs commands is not a plan.
//
// probe is called until it returns nil, the check's timeout elapses, or ctx is
// done. The returned error distinguishes "never became ready" (the service is
// probably still starting, or wedged) from a cancelled or expired caller
// context, because those call for different responses.
//
// Each probe is bounded by the time left in the check's own budget. Passing the
// caller's context through unchanged would apply the deadline only *between*
// attempts, so one probe that never exited would hold the command open past the
// timeout the user configured.
func Health(ctx context.Context, hc *schema.HealthCheck, probe func(context.Context) error) error {
	if hc == nil {
		return nil
	}
	if len(hc.Command) == 0 {
		return fmt.Errorf("health_check declares no command, so readiness cannot be established")
	}

	timeout := hc.HealthTimeout()
	interval := hc.HealthInterval()
	deadline := time.Now().Add(timeout)

	var lastErr error
	attempted := false
	for {
		remaining := time.Until(deadline)
		if remaining <= 0 {
			break
		}
		attempted = true
		probeCtx, cancelProbe := context.WithTimeout(ctx, remaining)
		err := probe(probeCtx)
		cancelProbe()
		if err == nil {
			return nil
		}
		lastErr = err

		if !time.Now().Add(interval).Before(deadline) {
			break
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(interval):
		}
	}
	if ctxErr := ctx.Err(); ctxErr != nil {
		return ctxErr
	}
	if !attempted {
		return fmt.Errorf("health_check was never probed")
	}
	return fmt.Errorf("%s did not become ready within %s: %w", hc.Command[0], timeout, lastErr)
}
