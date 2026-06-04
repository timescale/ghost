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
// The query context is deliberately not a child of parent because pgx reacts
// to context cancellation by closing the underlying database connection, which
// tears down the session (TEMP tables, SET state, in-progress transactions).
// By intercepting the cancellation ourselves and issuing a normal
// pg_cancel_backend() over a side channel instead, we cancel just the running
// query while keeping the connection alive for subsequent queries.
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
