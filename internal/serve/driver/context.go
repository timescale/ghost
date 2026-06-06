package driver

import (
	"context"
	"log/slog"

	"github.com/timescale/ghost/internal/log"
)

type canceler func(ctx context.Context) error

// cancelContext calls fn when ctx is canceled or times out. It returns a new
// context that is not canceled when ctx is canceled, unless fn returns an
// error. It is used to alter the default behavior of a database driver when a
// query is canceled. It also returns a cleanup function that should be called
// to release resources when the returned context is no longer in use.
func cancelContext(ctx context.Context, fn canceler) (context.Context, context.CancelFunc) {
	logger := log.FromContext(ctx)
	newCtx, cancel := context.WithCancelCause(context.Background())

	quit := make(chan struct{})
	done := make(chan struct{})
	go func() {
		defer close(done)

		select {
		case <-ctx.Done():
			logger.Debug("Canceling query")

			// TODO: Timeout for cancellation request itself?
			if err := fn(newCtx); err != nil {
				logger.Warn("Error canceling query", slog.Any("error", err))

				// Fall back to built-in cancellation behavior
				cancel(ctx.Err())
			}
		case <-quit:
		}
	}()

	return newCtx, func() {
		close(quit)
		<-done
		cancel(nil)
	}
}
