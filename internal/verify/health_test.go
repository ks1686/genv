package verify

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/ks1686/genv/internal/schema"
)

// probeRecorder counts attempts and flips to ready after `readyAfter` calls.
func probeRecorder(readyAfter int, calls *int) func(context.Context) error {
	return func(context.Context) error {
		*calls++
		if *calls >= readyAfter {
			return nil
		}
		return errors.New("not ready")
	}
}

func TestHealth_retries_until_ready(t *testing.T) {
	calls := 0
	err := Health(context.Background(), &schema.HealthCheck{
		Command:  []string{"true"},
		Timeout:  "2s",
		Interval: "10ms",
	}, probeRecorder(3, &calls))
	if err != nil {
		t.Fatalf("Health: %v", err)
	}
	if calls != 3 {
		t.Errorf("probe called %d times, want 3", calls)
	}
}

func TestHealth_returns_on_first_success(t *testing.T) {
	calls := 0
	err := Health(context.Background(), &schema.HealthCheck{
		Command:  []string{"true"},
		Timeout:  "2s",
		Interval: "10ms",
	}, probeRecorder(1, &calls))
	if err != nil {
		t.Fatalf("Health: %v", err)
	}
	if calls != 1 {
		t.Errorf("probe called %d times, want 1 once ready", calls)
	}
}

func TestHealth_times_out(t *testing.T) {
	calls := 0
	start := time.Now()
	err := Health(context.Background(), &schema.HealthCheck{
		Command:  []string{"true"},
		Timeout:  "120ms",
		Interval: "10ms",
	}, probeRecorder(1<<30, &calls))
	if err == nil {
		t.Fatal("a probe that never succeeds must fail")
	}
	// The message must say it never became ready, not that the command failed:
	// the distinction is what a user acts on.
	if !strings.Contains(err.Error(), "did not become ready") {
		t.Errorf("timeout message = %v", err)
	}
	if calls == 0 {
		t.Error("the probe should have been attempted at least once")
	}
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Errorf("Health ran for %v, want it bounded by the timeout", elapsed)
	}
}

func TestHealth_missing_command_is_an_error_not_a_pass(t *testing.T) {
	calls := 0
	err := Health(context.Background(), &schema.HealthCheck{Timeout: "50ms"}, probeRecorder(1, &calls))
	if err == nil {
		t.Fatal("a health check with no command must fail, not report ready")
	}
	if calls != 0 {
		t.Errorf("probe ran %d times despite no command", calls)
	}
}

func TestHealth_nil_check_reports_ready_without_probing(t *testing.T) {
	// A service without a health check has nothing to wait for. Returning nil
	// keeps callers from special-casing the common case.
	if err := Health(context.Background(), nil, probeRecorder(1, new(int))); err != nil {
		t.Errorf("nil health check = %v, want nil", err)
	}
}

func TestHealth_stops_on_cancelled_context(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	calls := 0
	err := Health(ctx, &schema.HealthCheck{Command: []string{"true"}, Timeout: "5s", Interval: "10ms"},
		probeRecorder(1<<30, &calls))
	if err == nil {
		t.Fatal("a cancelled context must stop the wait")
	}
	if !errors.Is(err, context.Canceled) {
		t.Errorf("err = %v, want a context.Canceled cause", err)
	}
}

func TestHealth_propagates_context_deadline(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	calls := 0
	err := Health(ctx, &schema.HealthCheck{Command: []string{"true"}, Timeout: "10s", Interval: "10ms"},
		probeRecorder(1<<30, &calls))
	if err == nil {
		t.Fatal("expected an error")
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("err = %v, want a context.DeadlineExceeded cause", err)
	}
}

func TestHealth_records_last_probe_error(t *testing.T) {
	calls := 0
	err := Health(context.Background(), &schema.HealthCheck{Command: []string{"true"}, Timeout: "80ms", Interval: "10ms"},
		probeRecorder(1<<30, &calls))
	if err == nil {
		t.Fatal("expected a timeout error")
	}
	if !strings.Contains(err.Error(), "not ready") {
		t.Errorf("timeout should quote the last probe failure: %v", err)
	}
}

func TestHealth_defaults_apply_when_unset(t *testing.T) {
	// Unset timeout/interval must not mean "no wait" or "busy loop".
	hc := &schema.HealthCheck{Command: []string{"true"}}
	if hc.HealthTimeout() != schema.DefaultHealthTimeout {
		t.Errorf("timeout = %v", hc.HealthTimeout())
	}
	if hc.HealthInterval() != schema.DefaultHealthInterval {
		t.Errorf("interval = %v", hc.HealthInterval())
	}
}

// A hung probe must not outlive the documented timeout. The loop used to apply
// the deadline only *between* attempts, so the probe was handed a context with
// no deadline of its own and a command stuck on a socket held genv open for as
// long as the socket took.
func TestHealth_bounds_a_single_hung_probe(t *testing.T) {
	hc := &schema.HealthCheck{
		Command:  []string{"true"},
		Timeout:  "200ms",
		Interval: "10ms",
	}
	var sawDeadline bool
	start := time.Now()
	err := Health(context.Background(), hc, func(ctx context.Context) error {
		_, sawDeadline = ctx.Deadline()
		// Models exec.CommandContext: a hung child is killed when ctx expires.
		<-ctx.Done()
		return ctx.Err()
	})
	elapsed := time.Since(start)

	if !sawDeadline {
		t.Error("probe was called with no deadline; a hung command would never be interrupted")
	}
	if err == nil {
		t.Fatal("Health: want an error, a hung probe is not readiness")
	}
	if elapsed > 5*time.Second {
		t.Fatalf("Health returned after %s, want it bounded by the %s check timeout", elapsed, hc.Timeout)
	}
	if !strings.Contains(err.Error(), "did not become ready") {
		t.Errorf("Health error = %v, want the readiness-timeout message", err)
	}
}

func TestHealth_deadline_covers_only_the_remaining_budget(t *testing.T) {
	// Each probe gets the time left in the check, so N attempts cannot add up to
	// N times the timeout.
	hc := &schema.HealthCheck{Command: []string{"true"}, Timeout: "300ms", Interval: "10ms"}
	var budgets []time.Duration
	start := time.Now()
	err := Health(context.Background(), hc, func(ctx context.Context) error {
		deadline, ok := ctx.Deadline()
		if !ok {
			t.Fatal("probe got no deadline")
		}
		budgets = append(budgets, time.Until(deadline))
		return errors.New("not ready") // fail fast, so the retries fit in the budget
	})
	if err == nil {
		t.Fatal("want an error")
	}
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Fatalf("Health took %s for a %s timeout", elapsed, hc.Timeout)
	}
	if len(budgets) < 2 {
		t.Fatalf("probe ran %d times, want repeated attempts", len(budgets))
	}
	if budgets[len(budgets)-1] >= budgets[0] {
		t.Errorf("per-probe budgets = %v, want each later attempt to get a smaller budget", budgets)
	}
}

func TestHealth_cancelled_caller_still_wins_over_a_hung_probe(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()
	err := Health(ctx, &schema.HealthCheck{
		Command:  []string{"true"},
		Timeout:  "30s",
		Interval: "10ms",
	}, func(probeCtx context.Context) error {
		<-probeCtx.Done()
		return probeCtx.Err()
	})
	if !errors.Is(err, context.Canceled) {
		t.Errorf("Health error = %v, want context.Canceled", err)
	}
}
