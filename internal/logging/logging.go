// Package logging configures the process-wide structured logger.
// Other packages use [log/slog] directly (slog.Debug, slog.Info, etc.);
// this package only sets up the handler and level.
package logging

import (
	"log/slog"
	"os"
)

// Init configures the global slog logger. Call once from main before
// dispatching to any subcommand. In debug mode every DEBUG-level message
// is emitted to stderr; otherwise only WARN and above are shown.
func Init(debug bool) {
	level := slog.LevelWarn
	if debug {
		level = slog.LevelDebug
	}
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: level})))
}
