package serve

import (
	"context"
	"log/slog"
	"time"

	"github.com/timescale/ghost/internal/log"
	"github.com/timescale/ghost/internal/serve/api"
)

// statusEventInterval is the amount of time between session status events
// returned from [Session.Events]. It is also the cadence at which the database
// is pinged to check the health of the connection.
const statusEventInterval = 5 * time.Second

// Events returns a channel on which session status events are returned. Pings
// the database periodically to check the health of the connection and generate
// status events. The channel is closed when the context is canceled, the
// session is closed, or a connection ping fails. In the future, this could
// return other types of session events as well (e.g. NOTICE or LISTEN/NOTIFY
// messages).
func (s *Session) Events(ctx context.Context) <-chan api.Event {
	logger := log.FromContext(ctx)

	events := make(chan api.Event, 1)
	go func() {
		defer logger.Debug("Done sending session events")

		defer close(events)

		ticker := time.NewTicker(statusEventInterval)
		defer ticker.Stop()

		ping := func() bool {
			if err := s.ping(ctx); err != nil {
				// NOTE: If the context was canceled, assume the ping failed
				// because of that, and return early.
				if ctx.Err() != nil {
					return false
				}

				// Treat all ping errors as fatal (I think this is the case
				// anyways, but be explicit here just to be safe).
				err.Fatal = true
				s.SetBroken()

				events <- api.ErrorEvent{
					Status: api.StatusError,
					Error:  err,
				}
				return false
			}
			events <- api.ConnectedEvent{
				Status: api.StatusConnected,
			}
			return true
		}

		if !ping() {
			return
		}

		for {
			select {
			case <-ctx.Done():
				return
			case <-s.closed:
				events <- api.ClosedEvent{
					Status: api.StatusClosed,
				}
				return
			case <-ticker.C:
				if !ping() {
					return
				}
			}
		}
	}()

	return events
}

func (s *Session) ping(ctx context.Context) *api.NormalizedError {
	logger := log.FromContext(ctx)

	pingCtx, cancel := context.WithTimeout(ctx, SessionOpenTimeout)
	defer cancel()

	// Acquire the session lock, as driver.Ping is not safe to call
	// concurrently with driver.Query. Note that this could theoretically delay
	// query execution, but the delay should hopefully not be significant
	// unless there is a problem pinging the database, in which case there
	// would be a problem querying it too. If the lock cannot be acquired (i.e.
	// because a query is currently in-progress), treat it like a successful
	// ping.
	if !s.lock.TryLock() {
		return nil
	}
	defer s.lock.Unlock()

	logger.Debug("Pinging database")
	if err := s.driver.Ping(pingCtx); err != nil {
		// Do not use the pingCtx here, or it could cause the
		// NormalizedError.Timeout field to be set to true, which is intended
		// only for query timeouts.
		logger.Debug("Ping failed", slog.Any("error", err))
		return s.driver.NormalizeError(ctx, err)
	}

	logger.Debug("Ping successful")
	return nil
}
