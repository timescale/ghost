package dbdriver

import (
	"context"
)

// canceler runs when the parent context of a query is canceled. The
// Postgres driver passes a closure that issues `pg_cancel_backend()` via a
// side-channel connection.
type canceler func(ctx context.Context) error

// cancelContext returns a fresh context (NOT a child of parent) plus a
// CancelFunc. When parent is canceled the supplied canceler is invoked; if
// the canceler returns an error we propagate parent's cancellation cause
// into the returned context as a fallback. The returned CancelFunc must be
// called when the query is finished to release the watcher goroutine.
//
// The reason for the not-a-child context is so that pgx does not abort the
// query mid-flight on its own — we want a graceful cancel through Postgres,
// so we can still surface a useful error to the client.
func cancelContext(parent context.Context, fn canceler) (context.Context, context.CancelFunc) {
	newCtx, cancel := context.WithCancelCause(context.Background())

	quit := make(chan struct{})
	done := make(chan struct{})
	go func() {
		defer close(done)
		select {
		case <-parent.Done():
			if err := fn(newCtx); err != nil {
				// Fall back to immediate cancel if the server-side cancel
				// failed (e.g. the backend connection is already dead).
				cancel(parent.Err())
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
