package serve

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/timescale/ghost/internal/serve/api"
	"github.com/timescale/ghost/internal/serve/driver"
)

// SessionOpenTimeout is the maximum amount of time that the service will wait
// while attempting to open a database connection before returning an error.
const SessionOpenTimeout = 10 * time.Second

// Session represents a user's database connection. Ephemeral sessions are
// closed automatically after a run, but long-lived sessions are stored in the
// [Store] until explicitly removed, or until the session times out.
type Session struct {
	// Unique identifier for the session, which is automatically generated when
	// the session is created.
	ID uuid.UUID

	driver  *driver.Driver
	lock    sync.Mutex
	broken  atomic.Bool
	closeFn func() error
	closed  chan bool
}

// NewSession opens new database [Session] given a DSN. It returns an
// [api.NormalizedError] if the database connection could not be established, or
// if the connection attempt took longer than [SessionOpenTimeout].
func (h *Handler) NewSession(ctx context.Context, dsn string) (session *Session, err error) {
	ctx, cancel := context.WithTimeout(ctx, SessionOpenTimeout)
	defer cancel()

	d, err := driver.Open(ctx, dsn)
	if err != nil {
		return nil, &api.NormalizedError{
			Message: err.Error(),
			Source:  driver.Source,
			Connect: true,
		}
	}

	closed := make(chan bool)
	closeFn := sync.OnceValue(func() error {
		close(closed)
		return d.Close()
	})

	return &Session{
		ID:      uuid.New(),
		driver:  d,
		closeFn: closeFn,
		closed:  closed,
	}, nil
}

// SetBroken marks the underlying database connection as broken, which signals
// for it to be closed and deleted from the store.
func (s *Session) SetBroken() {
	s.broken.Store(true)
}

// Broken reports whether the underlying database connection has been broken,
// at which point the session can no longer be used. This is set to true after
// a query using the session returns a fatal error, or if a database ping
// fails.
func (s *Session) Broken() bool {
	return s.broken.Load()
}

// Close attempts to close the underlying database connection. It will wait for
// any in-progress queries to finish before closing the connection and
// returning.
func (s *Session) Close() error {
	return s.closeFn()
}
