package serve

import (
	"context"
	"sync"
	"time"

	"github.com/timescale/ghost/internal/serve/dbdriver"
)

// Run coordinates a single in-flight query between the executeQuery and
// arrowResults handlers. executeQuery runs the query to completion (all
// result sets buffered), populates the chosen result set's columns + rows,
// signals ready, writes the columns NDJSON line, then blocks on done.
// arrowResults waits for ready, walks bufferedRows into an Arrow IPC stream,
// then closes done so executeQuery can emit the success/error terminator.
type Run struct {
	id        string
	projectID string
	serviceID string
	startedAt time.Time

	// Populated by executeQuery before closing ready. These describe the
	// single result set we surface to the widget (per the user-facing rule:
	// last result set with columns, or the last result set if none had
	// columns).
	columns      dbdriver.Columns
	bufferedRows [][]any
	rowCount     int64
	rowsAffected *int64

	// Number of result sets the database returned for this run — used by the
	// UI to show "Executed N statements" when N > 1.
	executedStatements int64

	// cancelQuery aborts the in-flight query via pg_cancel_backend (wired
	// through driver.Context's cancelContext). Used by /api/cancelRun and
	// by client-disconnect detection in executeQuery.
	cancelQuery context.CancelFunc

	ready chan struct{}
	done  chan struct{}

	err     *dbdriver.NormalizedError
	errOnce sync.Once
}

func (r *Run) setError(e *dbdriver.NormalizedError) {
	r.errOnce.Do(func() { r.err = e })
}

func (r *Run) closeDone() {
	select {
	case <-r.done:
	default:
		close(r.done)
	}
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

	closed    chan struct{}
	closeErr  *dbdriver.NormalizedError
	closeOnce sync.Once
}

func (s *Session) close(reason *dbdriver.NormalizedError) {
	s.closeOnce.Do(func() {
		s.closeErr = reason
		close(s.closed)
		if s.driver != nil {
			_ = s.driver.Close()
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
