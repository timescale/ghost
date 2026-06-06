package log

import (
	"context"
	"io"
	stdlog "log"
	"log/slog"
)

// New configures the standard library's default logger to write to w and
// returns the default slog logger (which writes through it), at slog's default
// log level. It's intended for long-running commands (e.g. `ghost serve`,
// `ghost mcp`) that act as backend processes and emit structured logs to
// stderr.
func New(w io.Writer) *slog.Logger {
	return NewWithLevel(w, slog.LevelInfo)
}

// NewWithLevel is like [New], but emits only logs at the given level and above.
func NewWithLevel(w io.Writer, level slog.Level) *slog.Logger {
	stdlog.SetOutput(w)
	slog.SetLogLoggerLevel(level)
	return slog.Default()
}

type contextKey int

const loggerKey contextKey = iota

// NewContext returns a copy of ctx carrying the provided logger, along with the
// logger itself (for convenient assignment).
func NewContext(ctx context.Context, logger *slog.Logger) (context.Context, *slog.Logger) {
	return context.WithValue(ctx, loggerKey, logger), logger
}

// FromContext returns the logger carried by ctx, or the default logger if none
// is set.
func FromContext(ctx context.Context) *slog.Logger {
	if logger, ok := ctx.Value(loggerKey).(*slog.Logger); ok {
		return logger
	}
	return slog.Default()
}
