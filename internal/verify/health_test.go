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
