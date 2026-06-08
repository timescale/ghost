package serve

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/timescale/ghost/internal/log"
)

// sessionDisconnectTimeout is how long a session lingers after the client
// disconnects from /api/sessionEvents before it is automatically closed. While
// the events stream is connected the timeout is paused (see AcquireSession), so
// this is effectively the post-disconnect grace period.
const sessionDisconnectTimeout = 15 * time.Second

// When a session has been acquired, the timeout is paused by resetting the
// timer to this large value (approximately a month). It's reset to the real
// timeout once the session is released. We use a long pause rather than
// stopping the timer entirely so that a leaked acquisition can't keep a session
// alive forever.
const persistedSessionTimeout = 30 * 24 * time.Hour

type sessionExp struct {
	*Session
	timer *time.Timer // Triggers session expiration function after timeout
	held  uint64      // The number of callers actively using the session
	lock  sync.Mutex  // Protects access to timer and held
}

// Store maintains in-memory lookups of all current sessions and in-progress
// runs (keyed by ID), and persists the serve UI state to disk.
type Store struct {
	logger *slog.Logger

	sessions     map[uuid.UUID]*sessionExp
	sessionsLock sync.RWMutex
	runs         map[uuid.UUID]*Run
	runsLock     sync.RWMutex

	// Persisted `ghost serve` web UI state. statePath is the JSON file the
	// state is written to; stateLock serializes reads/writes to it.
	statePath string
	stateLock sync.Mutex
}

// NewStore initializes and returns a new [Store] instance. configDir is the
// directory in which the serve UI state file is persisted.
func NewStore(configDir string, logger *slog.Logger) *Store {
	return &Store{
		logger: logger,

		sessions: map[uuid.UUID]*sessionExp{},
		runs:     map[uuid.UUID]*Run{},

		statePath: filepath.Join(configDir, stateFileName),
	}
}

// GetSession retrieves a session from the store and returns it. It does not
// pause or extend the session timeout. If the session is being retrieved in
// order to be used by an end-user (e.g. in order to run a query or listen for
// session status events), [Store.AcquireSession] should be used instead.
func (s *Store) GetSession(sessionID uuid.UUID) (*Session, error) {
	session, ok := s.getSession(sessionID)
	if !ok {
		return nil, &InvalidSessionIDError{ID: sessionID}
	}
	return session.Session, nil
}

// AcquireSession retrieves a session from the store and returns it. It also
// pauses the session timeout, if active (i.e. if it was not already paused by
// another acquisition). ReleaseSession should be called when the session is
// done being used to reset the timeout.
func (s *Store) AcquireSession(sessionID uuid.UUID) (*Session, error) {
	session, ok := s.getSession(sessionID)
	if !ok {
		return nil, &InvalidSessionIDError{ID: sessionID}
	}

	session.lock.Lock()
	defer session.lock.Unlock()

	if session.held == 0 {
		// NOTE: If session.timer.Reset() returns false, it means the session
		// has already timed out (but presumably the expiration function hasn't
		// actually executed yet, since we were able to find the session in the
		// map). In that case, there are some extremely unlikely race conditions
		// where the timer could end up being reset in ReleaseSession after being
		// closed in the expiration function, and the expiration function might
		// therefore run again. However, I don't believe that to be a major
		// concern, since it should be idempotent. I therefore think it's okay to
		// not check the Reset() return value here.
		session.timer.Reset(persistedSessionTimeout)
	}
	session.held += 1

	return session.Session, nil
}

// ReleaseSession resets the session timeout that was paused by a call to
// [Store.AcquireSession], if there are no other callers still using the
// session. It should always be called when a session obtained via that method
// is done being used. Failure to call it could create a leaked session that is
// never closed.
func (s *Store) ReleaseSession(session *Session) {
	sessionExp, ok := s.getSession(session.ID)
	if !ok {
		// Session has been deleted since being acquired. This is okay.
		return
	}

	sessionExp.lock.Lock()
	defer sessionExp.lock.Unlock()

	sessionExp.held -= 1
	if sessionExp.held == 0 {
		sessionExp.timer.Reset(sessionDisconnectTimeout)
	}
}

func (s *Store) getSession(sessionID uuid.UUID) (*sessionExp, bool) {
	s.sessionsLock.RLock()
	defer s.sessionsLock.RUnlock()

	session, ok := s.sessions[sessionID]
	return session, ok
}

func (s *Store) InsertSession(session *Session) error {
	s.sessionsLock.Lock()
	defer s.sessionsLock.Unlock()

	if _, exists := s.sessions[session.ID]; exists {
		return &SessionIDConflictError{ID: session.ID}
	}

	s.sessions[session.ID] = &sessionExp{
		Session: session,
		timer:   s.expireSession(session),
	}
	return nil
}

func (s *Store) expireSession(session *Session) *time.Timer {
	return time.AfterFunc(sessionDisconnectTimeout, func() {
		ctx := s.newSessionContext(session)
		s.TryCloseSession(ctx, session)
		s.TryDeleteSession(ctx, session)
	})
}

func (s *Store) DeleteSession(session *Session) {
	s.sessionsLock.Lock()
	defer s.sessionsLock.Unlock()

	if session, ok := s.sessions[session.ID]; ok {
		session.lock.Lock()
		defer session.lock.Unlock()

		session.timer.Stop()
		delete(s.sessions, session.ID)
	}
}

func (s *Store) GetRun(runID uuid.UUID) (*Run, error) {
	s.runsLock.RLock()
	defer s.runsLock.RUnlock()

	run, ok := s.runs[runID]
	if !ok {
		return nil, &InvalidRunIDError{ID: runID}
	}
	return run, nil
}

func (s *Store) InsertRun(run *Run) error {
	s.runsLock.Lock()
	defer s.runsLock.Unlock()

	if _, exists := s.runs[run.ID]; exists {
		return &RunIDConflictError{ID: run.ID}
	}

	s.runs[run.ID] = run
	return nil
}

func (s *Store) DeleteRun(run *Run) {
	s.runsLock.Lock()
	defer s.runsLock.Unlock()
	delete(s.runs, run.ID)
}

// Close attempts to close all outstanding database sessions and remove them
// from the store. It waits for any in-progress queries to complete before
// returning. Errors closing or removing database connections are logged.
func (s *Store) Close() {
	var wg sync.WaitGroup
	defer wg.Wait()

	s.sessionsLock.Lock()
	defer s.sessionsLock.Unlock()

	for _, session := range s.sessions {
		ctx := s.newSessionContext(session.Session)

		wg.Add(1)
		go func(session *Session) {
			defer wg.Done()
			s.TryCloseSession(ctx, session)
			s.TryDeleteSession(ctx, session)
		}(session.Session)
	}
}

func (s *Store) newSessionContext(session *Session) context.Context {
	ctx, _ := log.NewContext(context.Background(), s.logger.With(
		slog.String("sessionId", session.ID.String()),
	))
	return ctx
}

func (s *Store) TryCloseSession(ctx context.Context, session *Session) {
	logger := log.FromContext(ctx)

	logger.Debug("Closing database session")
	if err := session.Close(); err != nil {
		logger.Error("Error closing database session", slog.Any("error", err))
		return
	}
	logger.Debug("Database session closed")
}

func (s *Store) TryDeleteSession(ctx context.Context, session *Session) {
	logger := log.FromContext(ctx)

	logger.Debug("Deleting database session")
	s.DeleteSession(session)
	logger.Debug("Database session deleted")
}

func (s *Store) TryDeleteRun(ctx context.Context, run *Run) {
	logger := log.FromContext(ctx)

	logger.Debug("Deleting run")
	s.DeleteRun(run)
	logger.Debug("Run deleted")
}

const stateFileName = "serve-state.json"

// LoadState reads the persisted serve UI state from the store's state file. A
// missing file is not an error - it yields a zero State.
func (s *Store) LoadState() (State, error) {
	s.stateLock.Lock()
	defer s.stateLock.Unlock()

	data, err := os.ReadFile(s.statePath)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return State{}, nil
		}
		return State{}, fmt.Errorf("failed to read state file: %w", err)
	}
	var state State
	if err := json.Unmarshal(data, &state); err != nil {
		return State{}, fmt.Errorf("failed to parse state file: %w", err)
	}
	return state, nil
}

// SaveState persists the serve UI state to the store's state file. Writes are
// atomic (temp file + rename) and serialized via stateLock so concurrent PUTs
// can't interleave.
func (s *Store) SaveState(state State) error {
	s.stateLock.Lock()
	defer s.stateLock.Unlock()

	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to encode state: %w", err)
	}

	dir := filepath.Dir(s.statePath)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("failed to create config dir: %w", err)
	}

	tmp, err := os.CreateTemp(dir, ".serve-state.json.tmp")
	if err != nil {
		return fmt.Errorf("failed to create temp file: %w", err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	defer tmp.Close()

	if _, err := tmp.Write(data); err != nil {
		return fmt.Errorf("failed to write temp file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("failed to close temp file: %w", err)
	}
	if err := os.Chmod(tmpName, 0o600); err != nil {
		return fmt.Errorf("failed to chmod temp file: %w", err)
	}
	if err := os.Rename(tmpName, s.statePath); err != nil {
		return fmt.Errorf("failed to rename temp file: %w", err)
	}
	return nil
}
