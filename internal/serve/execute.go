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
		writeJSONError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
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
	defer func() {
		if err := driver.Close(); err != nil {
			s.logger.Warn("error closing database connection", "err", err)
		}
	}()

	s.runQuery(w, r, req, driver)
}

// handleExecuteSessionQuery serves POST /api/executeSessionQuery. The
// session-owned driver is reused; closing/cleanup is done in
// handleCloseSession.
func (s *Server) handleExecuteSessionQuery(w http.ResponseWriter, r *http.Request) {
	var req executeSessionQueryRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
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

// runQuery is the shared body of handleExecuteQuery / handleExecuteSessionQuery.
//
// The query executes in a dedicated goroutine (streamQuery) that streams rows
// over run.rows. This handler writes the columns NDJSON line as soon as the
// columns are known, which prompts the widget to POST /api/arrowResults; that
// handler drains run.rows into an Arrow IPC stream with backpressure. Rows are
// never collected in full — they flow from the database straight to the wire.
//
// Multi-statement behavior mirrors the upstream query service: the widget's worker splits
// the editor text into a statements array; we run every statement except the
// last against the same connection (so TEMP tables and other session state
// persist), discarding their results, then stream the final statement's result
// set. This is the result the widget displays.
func (s *Server) runQuery(w http.ResponseWriter, r *http.Request, req executeQueryRequest, driver dbdriver.Driver) {
	driverCtx, driverCleanup := driver.Context(r.Context())
	defer driverCleanup()

	queryCtx, cancelQuery := context.WithCancel(driverCtx)
	defer cancelQuery()

	statements := req.Statements
	if len(statements) == 0 && req.Query != "" {
		statements = []string{req.Query}
	}

	run := &Run{
		id:                 req.RunID,
		projectID:          req.ProjectID,
		serviceID:          req.ServiceID,
		startedAt:          time.Now(),
		rows:               make(chan []any, rowChanBuffer),
		executedStatements: int64(len(statements)),
		cancelQuery:        cancelQuery,
		ready:              make(chan struct{}),
		done:               make(chan struct{}),
	}
	s.runs.add(run)
	defer s.runs.delete(req.RunID)

	go s.streamQuery(queryCtx, run, driver, statements)

	// Wait for the query goroutine to produce columns (or fail before it got
	// that far), or for the client to disconnect.
	select {
	case <-run.ready:
	case <-r.Context().Done():
		cancelQuery()
		return
	}

	w.Header().Set("Content-Type", "application/x-ndjson")
	w.Header().Set("Cache-Control", "no-store")
	enc := json.NewEncoder(w)

	// If the query failed before producing a result set, the widget never sees
	// a columns line and so never fetches arrow results. Emit the error
	// terminator directly.
	if run.err != nil && len(run.columns) == 0 {
		_ = enc.Encode(errorResult{RunID: req.RunID, Success: false, Error: run.err})
		flushWriter(w)
		run.closeDone()
		return
	}

	if err := enc.Encode(columnsResult{RunID: req.RunID, Columns: run.columns}); err != nil {
		// Client disconnected before columns reached the wire.
		cancelQuery()
		return
	}
	flushWriter(w)

	// Wait for arrowResults to finish streaming, or for the client to
	// disconnect. arrowResults closes done once it has drained run.rows.
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

// streamQuery runs the run's statements against the driver connection and
// streams the final statement's rows over run.rows. It mirrors the upstream
// query session: run prior statements fire-and-forget, then stream the last.
//
// Iterating per-statement (rather than relying on sql.Rows.NextResultSet) is
// necessary because pgx's stdlib wrapper only surfaces the first result set of
// a multi-statement Query call. The widget already does the SQL-aware
// statement split for us, so we leverage that here.
//
// On any error it records a NormalizedError on the run; columns/rows produced
// so far are still streamed, and executeQuery emits the error terminator once
// arrowResults finishes. run.rows is always closed on return so arrowResults
// unblocks.
func (s *Server) streamQuery(ctx context.Context, run *Run, driver dbdriver.Driver, statements []string) {
	var rowCount int64
	readyOnce := func() {
		select {
		case <-run.ready:
		default:
			close(run.ready)
		}
	}
	defer readyOnce()
	defer close(run.rows)

	fail := func(err error) {
		run.rowCount = rowCount
		run.setError(driver.NormalizeError(ctx, err))
	}

	// Bail early if the context was already canceled, to avoid racing the
	// server-side cancel against the start of query execution.
	if err := ctx.Err(); err != nil {
		fail(err)
		return
	}

	// Run every statement except the last fire-and-forget.
	for i := 0; i+1 < len(statements); i++ {
		if err := runStatement(ctx, driver, statements[i]); err != nil {
			fail(err)
			return
		}
	}

	final := ""
	if len(statements) > 0 {
		final = statements[len(statements)-1]
	}

	rows, err := driver.Query(ctx, final)
	if err != nil {
		fail(err)
		return
	}
	defer func() {
		if err := rows.Close(); err != nil {
			s.logger.Debug("error closing rows", "err", err)
		}
	}()

	columns, err := rows.Columns()
	if err != nil {
		fail(err)
		return
	}
	run.columns = columns
	readyOnce()

	targets := columns.ScanTargets()
	for rows.Next() {
		if err := rows.Scan(targets...); err != nil {
			fail(err)
			return
		}
		select {
		case run.rows <- targets.Values():
			rowCount++
		case <-ctx.Done():
			fail(ctx.Err())
			return
		}
	}
	if err := rows.Err(); err != nil {
		fail(err)
		return
	}
	if err := rows.Close(); err != nil {
		fail(err)
		return
	}

	run.rowCount = rowCount
	if ra, _ := rows.RowsAffected(ctx); ra != nil {
		run.rowsAffected = ra
	}
}

// runStatement runs a single statement to completion and discards its result
// set. Used for every statement except the last in a multi-statement run.
func runStatement(ctx context.Context, driver dbdriver.Driver, stmt string) error {
	rows, err := driver.Query(ctx, stmt)
	if err != nil {
		return err
	}
	return rows.Close()
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
