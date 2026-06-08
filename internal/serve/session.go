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

// DefaultSessionTimeout is the default amount of time that an idle session
// will remain in the [Store] before being automatically closed and removed.
const DefaultSessionTimeout = 24 * time.Hour

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

	// ID of the user to whom the session belongs. Defaults to zero for the old
	// endpoints (which did not take a user ID parameter).
	UserID int64

	// Whether the session is an ephemeral session (i.e. only exists for the
	// lifetime of a single run).
	Ephemeral bool

	// The time at which the session was opened.
	Start time.Time

	// The length of time after which the session will automatically be closed
	// when idle. Issuing queries on a session will reset the timeout. Care
	// should be taken to ensure that the run timeout is greater than the
	// session timeout, or a session could time out while a run is in progress.
	// Defaults to [DefaultSessionTimeout] if not provided in the call to
	// [NewSession].
	Timeout time.Duration

	driver  *driver.Driver
	lock    sync.Mutex
	broken  atomic.Bool
	closeFn func() error
	closed  chan bool
}

// NewSession opens new database [Session] given a DSN and optional session
// timeout, which determines how long the session can be idle before it will be
// automatically closed. It returns an [api.NormalizedError] if the database
// connection could not be established, or if the connection attempt took longer
// than [SessionOpenTimeout].
func (h *Handler) NewSession(ctx context.Context, userID int64, dsn string, ephemeral bool, timeout *time.Duration) (session *Session, err error) {
	ctx, cancel := context.WithTimeout(ctx, SessionOpenTimeout)
	defer cancel()

	d, err := driver.Open(ctx, dsn)
	if err != nil {
		// TODO: Only errors caused by bad user input or an inability to
		// connect to the database should be returned as NormalizedError.
		// Should probably move the logic into the driver package itself so we
		// can discriminate more easily.
		return nil, &api.NormalizedError{
			Message: err.Error(),
			Source:  driver.Source,
			Connect: true,
		}
	}

	start := time.Now()
	closed := make(chan bool)
	closeFn := sync.OnceValue(func() error {
		close(closed)
		return d.Close()
	})

	return &Session{
		ID:        uuid.New(),
		UserID:    userID,
		Ephemeral: ephemeral,
		Start:     start,
		Timeout:   sessionTimeout(timeout),
		driver:    d,
		closeFn:   closeFn,
		closed:    closed,
	}, nil
}

func sessionTimeout(timeout *time.Duration) time.Duration {
	if timeout != nil {
		return *timeout
	}
	return DefaultSessionTimeout
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
