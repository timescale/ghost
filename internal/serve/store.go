package serve

import (
	"context"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/timescale/ghost/internal/serve/dbdriver"
)

// rowChanBuffer is the capacity of Run.rows. A small buffer smooths out
// per-row scheduling jitter between the producer (the query goroutine) and
// the consumer (the arrowResults handler) without letting memory usage grow
// unbounded: once the buffer fills, the producer blocks on its next send,
// which throttles how fast we read from the database. This backpressure is
// what keeps memory flat for arbitrarily large result sets.
const rowChanBuffer = 100

// Run coordinates a single in-flight query between the executeQuery and
// arrowResults handlers. The query runs in a dedicated goroutine that streams
// scanned rows over the rows channel (see streamQuery). executeQuery waits for
// ready (columns known), writes the columns NDJSON line, then blocks on done.
// arrowResults waits for ready, ranges over rows building an Arrow IPC stream
// with backpressure, then closes done so executeQuery can emit the
// success/error terminator. Rows are never buffered in full — they flow from
// the database straight to the wire.
type Run struct {
	id        string
	projectID string
	serviceID string
	startedAt time.Time

	// columns is set by the query goroutine before it closes ready.
	columns dbdriver.Columns

	// rows streams scanned rows from the query goroutine to arrowResults. It
	// is closed by the query goroutine once the result set is exhausted (or on
	// error/cancellation). Backpressure on this channel bounds memory use.
	rows chan []any

	// rowCount and rowsAffected are set by the query goroutine before it
	// closes rows; they are safe to read after done is closed.
	rowCount     int64
	rowsAffected *int64

	// Number of statements the database executed for this run — used by the
	// UI to show "Executed N statements" when N > 1.
	executedStatements int64

	// arrowStarted guards against more than one arrowResults handler draining
	// the rows channel. Only the first caller wins; concurrent/duplicate
	// fetches are rejected (mirrors the upstream single-reader pipe design).
	arrowStarted atomic.Bool

	// cancelQuery aborts the in-flight query via pg_cancel_backend (wired
	// through driver.Context's cancelContext). Used by /api/cancelRun and
	// by client-disconnect detection in executeQuery / arrowResults.
	cancelQuery context.CancelFunc

	ready chan struct{}
	done  chan struct{}

	err      *dbdriver.NormalizedError
	errOnce  sync.Once
	doneOnce sync.Once
}

func (r *Run) setError(e *dbdriver.NormalizedError) {
	r.errOnce.Do(func() { r.err = e })
}

func (r *Run) closeDone() {
	r.doneOnce.Do(func() { close(r.done) })
}

// runStore holds all in-flight runs keyed by their widget-generated run id.
type runStore struct {
	mu   sync.Mutex
	runs map[string]*Run
}

func newRunStore() *runStore {
	return &runStore{runs: make(map[string]*Run)}
}

func (s *runStore) add(r *Run) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.runs[r.id] = r
}

func (s *runStore) get(id string) *Run {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.runs[id]
}

func (s *runStore) delete(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.runs, id)
}

// Session holds a long-lived driver across multiple queries from one widget
// tab. Sessions live until /api/closeSession is invoked or the server shuts
// down. There is no idle timeout for now.
type Session struct {
	id        string
	projectID string
	serviceID string
	startedAt time.Time

	driver dbdriver.Driver
	logger *slog.Logger

	closed    chan struct{}
	closeErr  *dbdriver.NormalizedError
	closeOnce sync.Once
}

func (s *Session) close(reason *dbdriver.NormalizedError) {
	s.closeOnce.Do(func() {
		s.closeErr = reason
		close(s.closed)
		if s.driver != nil {
			if err := s.driver.Close(); err != nil && s.logger != nil {
				s.logger.Warn("error closing session database connection", "err", err)
			}
		}
	})
}

// sessionStore holds active sessions.
type sessionStore struct {
	mu       sync.Mutex
	sessions map[string]*Session
}

func newSessionStore() *sessionStore {
	return &sessionStore{sessions: make(map[string]*Session)}
}

func (s *sessionStore) add(sess *Session) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sessions[sess.id] = sess
}

func (s *sessionStore) get(id string) *Session {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.sessions[id]
}

func (s *sessionStore) delete(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.sessions, id)
}

// closeAll terminates every session. Called when the server shuts down.
func (s *sessionStore) closeAll() {
	s.mu.Lock()
	defer s.mu.Unlock()
	for id, sess := range s.sessions {
		sess.close(&dbdriver.NormalizedError{Message: "server shutting down", Source: "ghost", Fatal: true})
		delete(s.sessions, id)
	}
}
