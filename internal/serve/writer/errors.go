package writer

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
)

// ErrLevel returns a logging level to be used for logging the provided error.
// Usually returns error level, but returns debug level if the error was caused
// by a context being canceled, or if the passed context was canceled (under the
// assumption that the error was ultimately caused by that), since errors caused
// by request cancellation are intentional and therefore not worth logging at
// the error level.
func ErrLevel(ctx context.Context, err error) slog.Level {
	if ctx.Err() == context.Canceled || errors.Is(err, context.Canceled) {
		return slog.LevelDebug
	}
	return slog.LevelError
}

type WriteError struct {
	Msg string
	Err error
}

func (e *WriteError) Error() string {
	return fmt.Sprintf("%s: %s", e.Msg, e.Err)
}

type ArrowTypeError[T any] struct {
	Actual any
}

func (e *ArrowTypeError[T]) Error() string {
	var expected T
	return fmt.Sprintf("invalid arrow type: %T, expected: %T", e.Actual, expected)
}
