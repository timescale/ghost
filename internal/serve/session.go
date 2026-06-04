package serve

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/google/uuid"

	"github.com/timescale/ghost/internal/serve/dbdriver"
)

// handleCreateSession serves POST /api/createSession. Opens a driver, stores
// it as a Session, returns the assigned ID. Mirrors the
// CreateSessionResponse shape from @popsql/types.
func (s *Server) handleCreateSession(w http.ResponseWriter, r *http.Request) {
	var req createSessionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body: "+err.Error(), http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	enc := json.NewEncoder(w)

	client, projectID, err := s.loadClient(r.Context())
	if err != nil {
		_ = enc.Encode(createSessionResponse{
			Success: false,
			Error:   &dbdriver.NormalizedError{Message: err.Error(), Source: "ghost", Connect: true},
		})
		return
	}
	if req.ProjectID != projectID {
		_ = enc.Encode(createSessionResponse{
			Success: false,
			Error: &dbdriver.NormalizedError{
				Message: "projectId does not match the active ghost project",
				Source:  "ghost",
			},
		})
		return
	}

	driver, err := openDriverForService(r.Context(), client, req.ProjectID, req.ServiceID, s.cfg.App.GetConfig().ReadOnly)
	if err != nil {
		ce := new(connectErr)
		if errors.As(err, &ce) {
			_ = enc.Encode(createSessionResponse{Success: false, Error: ce.Normalized()})
		} else {
			_ = enc.Encode(createSessionResponse{
				Success: false,
				Error:   &dbdriver.NormalizedError{Message: err.Error(), Source: "ghost", Connect: true},
			})
		}
		return
	}

	sess := &Session{
		id:        uuid.NewString(),
		projectID: req.ProjectID,
		serviceID: req.ServiceID,
		startedAt: time.Now(),
		driver:    driver,
		closed:    make(chan struct{}),
	}
	s.sessions.add(sess)

	_ = enc.Encode(createSessionResponse{Success: true, ID: sess.id})
}

// handleCloseSession serves POST /api/closeSession. Cleanly tears down a
// session's driver. Returns 204 on success, 404 if the session is unknown.
func (s *Server) handleCloseSession(w http.ResponseWriter, r *http.Request) {
	var req sessionRefRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body: "+err.Error(), http.StatusBadRequest)
		return
	}
	sess := s.sessions.get(req.SessionID)
	if sess == nil {
		http.NotFound(w, r)
		return
	}
	sess.close(nil)
	s.sessions.delete(req.SessionID)
	w.WriteHeader(http.StatusNoContent)
}

// handleSessionEvents serves POST /api/sessionEvents. Long-lived NDJSON
// stream: emits {"status":"connected"} immediately, then blocks until the
// session is closed (or the request is canceled). The widget's
// BaseSessionManager re-establishes this stream up to 15 times before giving
// up; if it eventually 404s the widget treats the session as dead.
func (s *Server) handleSessionEvents(w http.ResponseWriter, r *http.Request) {
	var req sessionRefRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body: "+err.Error(), http.StatusBadRequest)
		return
	}

	sess := s.sessions.get(req.SessionID)
	if sess == nil {
		http.NotFound(w, r)
		return
	}

	w.Header().Set("Content-Type", "application/x-ndjson")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Accel-Buffering", "no")

	enc := json.NewEncoder(w)
	if err := enc.Encode(sessionEvent{Status: sessionStatusConnected}); err != nil {
		return
	}
	flushWriter(w)

	select {
	case <-sess.closed:
		if sess.closeErr != nil {
			_ = enc.Encode(sessionEvent{Status: sessionStatusError, Error: sess.closeErr})
		} else {
			_ = enc.Encode(sessionEvent{Status: sessionStatusClosed})
		}
		flushWriter(w)
	case <-r.Context().Done():
		// Client disconnected; nothing to write. The widget will reconnect.
	}
}
