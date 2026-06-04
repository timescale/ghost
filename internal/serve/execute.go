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
	client, projectID, err := s.loadClient(r.Context())
	if err != nil {
		writeErrorTerminator(w, req.RunID, &dbdriver.NormalizedError{Message: err.Error(), Source: "ghost", Connect: true})
		return
	}
	if !checkProject(w, req.RunID, req.ProjectID, projectID) {
		return
	}

	driver, connErr := openDriverForService(r.Context(), client, req.ProjectID, req.ServiceID, s.cfg.App.GetConfig().ReadOnly)
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

	s.runQuery(w, r, req, driver)
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
	_, projectID, err := s.loadClient(r.Context())
	if err != nil {
		writeErrorTerminator(w, req.RunID, &dbdriver.NormalizedError{Message: err.Error(), Source: "ghost", Connect: true})
		return
	}
	if !checkProject(w, req.RunID, req.ProjectID, projectID) {
		return
	}

	session := s.sessions.get(req.SessionID)
	if session == nil {
		// 404 trips the widget's SessionError path, which prompts a fresh
		// createSession on the next query attempt.
		http.NotFound(w, r)
		return
	}

	s.runQuery(w, r, req.executeQueryRequest, session.driver)
}

// bufferedResultSet is one result set materialized in memory: its column
// descriptors plus every scanned row in order.
type bufferedResultSet struct {
	columns      dbdriver.Columns
	rows         [][]any
	rowsAffected *int64
}

// runQuery is the shared body of handleExecuteQuery / handleExecuteSessionQuery.
//
// Multi-statement behavior: the widget's worker splits the editor text into a
// statements array which we join with `; ` and run via pgx's simple text
// protocol. PG returns one result set per statement. We buffer all of them
// and surface the last result set that has columns; if none have columns
// (e.g. only DDL/DML), we surface the last result set so its rowsAffected
// still shows.
func (s *Server) runQuery(w http.ResponseWriter, r *http.Request, req executeQueryRequest, driver dbdriver.Driver) {
	driverCtx, driverCleanup := driver.Context(r.Context())
	defer driverCleanup()

	queryCtx, cancelQuery := context.WithCancel(driverCtx)
	defer cancelQuery()

	run := &Run{
		id:          req.RunID,
		projectID:   req.ProjectID,
		serviceID:   req.ServiceID,
		startedAt:   time.Now(),
		cancelQuery: cancelQuery,
		ready:       make(chan struct{}),
		done:        make(chan struct{}),
	}
	s.runs.add(run)
	defer s.runs.delete(req.RunID)

	statements := req.Statements
	if len(statements) == 0 && req.Query != "" {
		statements = []string{req.Query}
	}

	results, bufErr := bufferStatements(queryCtx, driver, statements)
	if bufErr != nil {
		writeErrorTerminator(w, req.RunID, driver.NormalizeError(queryCtx, bufErr))
		return
	}
	chosen := pickResultSetToSurface(results)
	if chosen == nil {
		// Should not happen: bufferStatements always emits at least one entry
		// on success. Defensively surface an empty result.
		chosen = &bufferedResultSet{}
	}

	run.columns = chosen.columns
	run.bufferedRows = chosen.rows
	run.rowCount = int64(len(chosen.rows))
	run.rowsAffected = chosen.rowsAffected
	run.executedStatements = int64(len(results))
	close(run.ready)

	w.Header().Set("Content-Type", "application/x-ndjson")
	w.Header().Set("Cache-Control", "no-store")

	enc := json.NewEncoder(w)
	if err := enc.Encode(columnsResult{RunID: req.RunID, Columns: chosen.columns}); err != nil {
		// Client disconnected before columns reached the wire.
		cancelQuery()
		return
	}
	flushWriter(w)

	// Wait for arrowResults to finish, or for the client to disconnect.
	select {
	case <-run.done:
	case <-r.Context().Done():
		cancelQuery()
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
			RunID:              req.RunID,
			Success:            true,
			RowCount:           run.rowCount,
			RowsAffected:       run.rowsAffected,
			ExecutedStatements: run.executedStatements,
		})
	}
	flushWriter(w)
}

// bufferStatements runs each statement in order against the same driver
// connection (so TEMP tables and other session state from earlier
// statements are visible to later ones) and buffers the rows from each
// result set into memory.
//
// Iterating per-statement (rather than relying on sql.Rows.NextResultSet)
// is necessary because pgx's stdlib wrapper only surfaces the first
// result set of a multi-statement Query call. The widget already does the
// SQL-aware statement split for us, so we leverage that here.
func bufferStatements(ctx context.Context, driver dbdriver.Driver, statements []string) ([]bufferedResultSet, error) {
	out := make([]bufferedResultSet, 0, len(statements))
	for _, stmt := range statements {
		rs, err := bufferOneStatement(ctx, driver, stmt)
		if err != nil {
			return out, err
		}
		out = append(out, rs)
	}
	return out, nil
}

func bufferOneStatement(ctx context.Context, driver dbdriver.Driver, stmt string) (bufferedResultSet, error) {
	rows, err := driver.Query(ctx, dbdriver.QueryArgs{Query: stmt})
	if err != nil {
		return bufferedResultSet{}, err
	}
	defer rows.Close()

	cols, err := rows.Columns()
	if err != nil {
		return bufferedResultSet{}, err
	}

	buf := bufferedResultSet{columns: cols}
	targets := cols.ScanTargets()
	for rows.Next() {
		if err := ctx.Err(); err != nil {
			return buf, err
		}
		if err := rows.Scan(targets...); err != nil {
			return buf, err
		}
		buf.rows = append(buf.rows, targets.Values())
	}
	if err := rows.Err(); err != nil {
		return buf, err
	}
	if ra, _ := rows.RowsAffected(ctx); ra != nil {
		buf.rowsAffected = ra
	}
	return buf, nil
}

// pickResultSetToSurface picks the result set we display to the widget,
// matching the rule the user asked for: prefer the last result set that
// returned columns; fall back to the last result set if none have columns;
// nil if the slice is empty.
func pickResultSetToSurface(results []bufferedResultSet) *bufferedResultSet {
	if len(results) == 0 {
		return nil
	}
	for i := len(results) - 1; i >= 0; i-- {
		if len(results[i].columns) > 0 {
			return &results[i]
		}
	}
	return &results[len(results)-1]
}

// checkProject rejects requests for a different project than the one the CLI
// is logged into. Single-user defense in depth.
func checkProject(w http.ResponseWriter, runID, requestProjectID, activeProjectID string) bool {
	if requestProjectID == activeProjectID {
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
