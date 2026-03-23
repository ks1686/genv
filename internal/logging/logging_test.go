package logging_test

import (
	"log/slog"
	"testing"

	"github.com/ks1686/gpm/internal/logging"
)

func TestInit_NoCrash(t *testing.T) {
	// Verifies Init does not panic in either mode.
	logging.Init(false)
	logging.Init(true)
	logging.Init(false) // restore to warn level so other tests are unaffected
}

func TestInit_SetsDefaultLogger(t *testing.T) {
	logging.Init(true)
	// slog.Default() must be callable after Init; the handler must not be nil.
	l := slog.Default()
	if l == nil {
		t.Fatal("slog.Default() returned nil after Init")
	}
	// Debug messages must be enabled when debug=true.
	if !l.Enabled(nil, slog.LevelDebug) {
		t.Error("expected DEBUG level to be enabled after Init(true)")
	}

	logging.Init(false)
	l = slog.Default()
	// After Init(false) the debug level should NOT be enabled.
	if l.Enabled(nil, slog.LevelDebug) {
		t.Error("expected DEBUG level to be disabled after Init(false)")
	}
}
