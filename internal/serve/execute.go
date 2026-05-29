package serve

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/timescale/ghost/internal/serve/dbdriver"
)

// handleExecuteQuery serves POST /api/executeQuery for one-shot mode (no
// sessionId). A fresh driver is opened, the query runs, columns are
// streamed, then arrowResults consumes the rows; we wait for it to finish
// and emit the success/error terminator before closing.
func (s *Server) handleExecuteQuery(w http.ResponseWriter, r *http.Request) {
	var req executeQueryRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body: "+err.Error(), http.StatusBadRequest)
		return
	}
	if !s.checkProject(w, req.RunID, req.ProjectID) {
		return
	}

	driver, connErr := openDriverForService(r.Context(), s.cfg.Client, req.ProjectID, req.ServiceID)
	if connErr != nil {
		ce := new(connectErr)
		if errors.As(connErr, &ce) {
			writeErrorTerminator(w, req.RunID, ce.Normalized())
		} else {
			writeErrorTerminator(w, req.RunID, &dbdriver.NormalizedError{Message: connErr.Error(), Source: "ghost", Connect: true})
		}
		return
	}
	defer driver.Close()

	s.runQuery(w, r, req, driver, true)
}

// handleExecuteSessionQuery serves POST /api/executeSessionQuery. The
// session-owned driver is reused; closing/cleanup is done in
// handleCloseSession.
func (s *Server) handleExecuteSessionQuery(w http.ResponseWriter, r *http.Request) {
	var req executeSessionQueryRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body: "+err.Error(), http.StatusBadRequest)
		return
	}
	if !s.checkProject(w, req.RunID, req.ProjectID) {
		return
	}

	session := s.sessions.get(req.SessionID)
	if session == nil {
		// 404 trips the widget's SessionError path, which prompts a fresh
		// createSession on the next query attempt.
		http.NotFound(w, r)
		return
	}

	s.runQuery(w, r, req.executeQueryRequest, session.driver, false)
}

// runQuery is the shared body of handleExecuteQuery / handleExecuteSessionQuery.
// ownsDriver controls whether arrowResults' cleanup will close the driver
// (true in one-shot mode, false in session mode).
func (s *Server) runQuery(w http.ResponseWriter, r *http.Request, req executeQueryRequest, driver dbdriver.Driver, ownsDriver bool) {
	driverCtx, driverCleanup := driver.Context(r.Context())
	defer driverCleanup()

	rows, err := driver.Query(driverCtx, dbdriver.QueryArgs{Query: req.SQL()})
	if err != nil {
		writeErrorTerminator(w, req.RunID, driver.NormalizeError(driverCtx, err))
		return
	}
	defer rows.Close()

	columns, err := rows.Columns()
	if err != nil {
		writeErrorTerminator(w, req.RunID, driver.NormalizeError(driverCtx, err))
		return
	}

	queryCtx, cancelQuery := context.WithCancel(r.Context())
	defer cancelQuery()

	run := &Run{
		id:            req.RunID,
		projectID:     req.ProjectID,
		serviceID:     req.ServiceID,
		startedAt:     time.Now(),
		rows:          rows,
		columns:       columns,
		queryCtx:      queryCtx,
		cancelQuery:   cancelQuery,
		driverCleanup: driverCleanup,
		ready:         make(chan struct{}),
		done:          make(chan struct{}),
	}
	if ownsDriver {
		run.driver = driver
	}
	s.runs.add(run)
	defer s.runs.delete(req.RunID)
	close(run.ready)

	w.Header().Set("Content-Type", "application/x-ndjson")
	w.Header().Set("Cache-Control", "no-store")

	enc := json.NewEncoder(w)
	if err := enc.Encode(columnsResult{RunID: req.RunID, Columns: columns}); err != nil {
		// Client gone before the columns line could be written; bail.
		cancelQuery()
		return
	}
	flushWriter(w)

	// Wait for arrowResults to finish, or for the client to disconnect.
	select {
	case <-run.done:
	case <-r.Context().Done():
		cancelQuery()
		// Give arrowResults a brief window to wrap up cleanly so it sets the
		// real rowCount + error. If it doesn't return promptly we mark the
		// run as canceled and proceed.
		select {
		case <-run.done:
		case <-time.After(2 * time.Second):
			run.setError(&dbdriver.NormalizedError{Message: "request canceled", Source: "ghost", Cancel: true})
			run.closeDone()
		}
	}

	if run.err != nil {
		_ = enc.Encode(errorResult{RunID: req.RunID, Success: false, Error: run.err})
	} else {
		_ = enc.Encode(successResult{
			RunID:        req.RunID,
			Success:      true,
			RowCount:     run.rowCount,
			RowsAffected: run.rowsAffected,
		})
	}
	flushWriter(w)
}

// checkProject rejects requests for a different project than the one the CLI
// is logged into. Single-user defense in depth.
func (s *Server) checkProject(w http.ResponseWriter, runID, projectID string) bool {
	if projectID == s.cfg.ProjectID {
		return true
	}
	writeErrorTerminator(w, runID, &dbdriver.NormalizedError{
		Message: "projectId does not match the active ghost project",
		Source:  "ghost",
	})
	return false
}

// writeErrorTerminator writes a single-line NDJSON error response. Used when
// the query never gets far enough to register a Run.
func writeErrorTerminator(w http.ResponseWriter, runID string, norm *dbdriver.NormalizedError) {
	w.Header().Set("Content-Type", "application/x-ndjson")
	w.Header().Set("Cache-Control", "no-store")
	_ = json.NewEncoder(w).Encode(errorResult{RunID: runID, Success: false, Error: norm})
	flushWriter(w)
}

// flushWriter calls Flush if the writer supports it, which is needed so the
// widget sees columns + success lines as they're written rather than batched
// at the end.
func flushWriter(w http.ResponseWriter) {
	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	}
}
