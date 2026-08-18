package resolver

import (
	"context"
	"time"
)

type subprocessTimeoutKey struct{}

// WithSubprocessTimeout stashes a per-spawn deadline on ctx. Unlike
// context.WithTimeout on the parent, this does not starve later commands
// or hooks when one subprocess runs long.
func WithSubprocessTimeout(ctx context.Context, d time.Duration) context.Context {
	if d <= 0 {
		return ctx
	}
	return context.WithValue(ctx, subprocessTimeoutKey{}, d)
}

// SubprocessTimeout returns the per-spawn duration stored by
// WithSubprocessTimeout, or 0 if none is set.
func SubprocessTimeout(ctx context.Context) time.Duration {
	d, _ := ctx.Value(subprocessTimeoutKey{}).(time.Duration)
	return d
}
