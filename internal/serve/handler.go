package serve

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/pprof"
	"time"

	"github.com/google/uuid"
	ghostapi "github.com/timescale/ghost/internal/api"
	"github.com/timescale/ghost/internal/common"
	"github.com/timescale/ghost/internal/config"
	"github.com/timescale/ghost/internal/log"
	"github.com/timescale/ghost/internal/serve/api"
	"github.com/timescale/ghost/internal/serve/driver"
	"github.com/timescale/ghost/internal/serve/writer"
)

type HandlerConfig struct {
	App    *common.App
	Store  *Store
	Logger *slog.Logger
}

type Handler struct {
	app    *common.App
	store  *Store
	logger *slog.Logger
}

func NewHandler(config HandlerConfig) *Handler {
	return &Handler{
		app:    config.App,
		store:  config.Store,
		logger: config.Logger,
	}
}

func (h *Handler) Handler() http.Handler {
	router := NewRouter(
		logRequests(h.logger),
		handlePanics(),
	)
	router.GET("/health",
		h.healthHandler,
	)
	router.GET("/api/bootstrap",
		h.bootstrapHandler,
	)
	router.GET("/api/databases",
		h.databasesHandler,
	)
	router.GET("/api/state",
		h.loadStateHandler,
	)
	router.PUT("/api/state",
		h.saveStateHandler,
		contentTypeJSON(),
		unmarshalRequest[SaveStateRequest](),
	)
	router.POST("/api/executeQuery",
		h.executeQueryHandler,
		contentTypeJSON(),
		unmarshalRequest[ExecuteQueryRequest](),
		validateRequest(),
	)
	router.POST("/api/executeSessionQuery",
		h.executeSessionQueryHandler,
		contentTypeJSON(),
		unmarshalRequest[ExecuteSessionQueryRequest](),
		validateRequest(),
	)
	router.POST("/api/arrowResults",
		h.arrowResultsHandler,
		contentTypeJSON(),
		unmarshalRequest[ArrowResultsRequest](),
		validateRequest(),
	)
	router.POST("/api/cancelQuery",
		h.cancelQueryHandler,
		contentTypeJSON(),
		unmarshalRequest[CancelQueryRequest](),
		validateRequest(),
	)
	router.POST("/api/createSession",
		h.createSessionHandler,
		contentTypeJSON(),
		unmarshalRequest[CreateSessionRequest](),
		validateRequest(),
	)
	router.POST("/api/sessionEvents",
		h.sessionEventsHandler,
		contentTypeJSON(),
		unmarshalRequest[SessionEventsRequest](),
		validateRequest(),
	)
	router.POST("/api/closeSession",
		h.closeSessionHandler,
		contentTypeJSON(),
		unmarshalRequest[CloseSessionRequest](),
		validateRequest(),
	)

	router.Handler(http.MethodGet, "/debug/*path", h.debugHandlers())

	// Unmatched routes fall through to the embedded SPA assets (with index.html
	// SPA fallback). Note this means unknown /api/* paths return the SPA rather
	// than a JSON 404.
	router.NotFound(newAssetHandler().ServeHTTP)
	router.MethodNotAllowed(h.methodNotAllowedHandler)
	return router
}

// HealthResponse is the response body of the GET /health endpoint.
type HealthResponse struct {
	Success bool `json:"success"`
}

func (h *Handler) healthHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	logger := log.FromContext(ctx)
	response := HealthResponse{
		Success: true,
	}
	writeJSON(w, http.StatusOK, response, logger)
}

// GetBootstrapResponse is the response body of the GET /api/bootstrap endpoint.
type GetBootstrapResponse struct {
	ProjectID string `json:"projectId"`
	Version   string `json:"version"`
}

func (h *Handler) bootstrapHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	logger := log.FromContext(ctx)

	_, projectID, err := h.loadClient(ctx)
	if err != nil {
		logger.Warn("Error loading client", slog.Any("error", err))
		writeError(w, http.StatusUnauthorized, err, logger)
		return
	}

	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, GetBootstrapResponse{
		ProjectID: projectID,
		Version:   config.Version,
	}, logger)
}

// DatabasesResponse is the response body of the GET /api/databases endpoint.
type DatabasesResponse struct {
	Databases []Database `json:"databases"`
}

// Database is a single entry in a [DatabasesResponse].
type Database struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Status string `json:"status"`
	Type   string `json:"type"`
}

func (h *Handler) databasesHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	logger := log.FromContext(ctx)

	client, projectID, err := h.loadClient(ctx)
	if err != nil {
		logger.Warn("Error loading client", slog.Any("error", err))
		writeError(w, http.StatusUnauthorized, err, logger)
		return
	}

	resp, err := client.ListDatabasesWithResponse(ctx, projectID)
	if err != nil {
		logger.Error("Error listing databases", slog.Any("error", err))
		writeError(w, http.StatusBadGateway, fmt.Errorf("failed to list databases: %w", err), logger)
		return
	}
	if resp.StatusCode() != http.StatusOK {
		err := common.ExitWithErrorFromStatusCode(resp.StatusCode(), resp.JSONDefault)
		logger.Error("Error response listing databases", slog.Any("error", err))
		writeError(w, resp.StatusCode(), err, logger)
		return
	}
	if resp.JSON200 == nil {
		logger.Error("Empty response from ghost-api")
		writeError(w, http.StatusBadGateway, errors.New("empty response from API"), logger)
		return
	}

	databases := make([]Database, len(*resp.JSON200))
	for i, db := range *resp.JSON200 {
		databases[i] = Database{
			ID:     db.Id,
			Name:   db.Name,
			Status: string(db.Status),
			Type:   string(db.Type),
		}
	}

	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, DatabasesResponse{Databases: databases}, logger)
}

// State is the persisted UI state for `ghost serve`, shared by the GET and PUT
// /api/state endpoints. A PUT replaces the stored state wholesale, so clients
// send the full snapshot; omitempty just keeps zero-valued fields out of the
// JSON.
type State struct {
	SelectedDatabaseID string `json:"selectedDatabaseId,omitempty"`
	EditorHeight       int    `json:"editorHeight,omitempty"`
	EditorSQL          string `json:"editorSql,omitempty"`
}

// GetStateResponse is the response body of the GET /api/state endpoint.
type GetStateResponse struct {
	State
}

func (h *Handler) loadStateHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	logger := log.FromContext(ctx)

	state, err := h.store.LoadState()
	if err != nil {
		logger.Error("Error loading state", slog.Any("error", err))
		internalServerError(w, logger)
		return
	}

	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, GetStateResponse{State: state}, logger)
}

// SaveStateRequest is the request body of the PUT /api/state endpoint.
type SaveStateRequest struct {
	State
}

func (h *Handler) saveStateHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	logger := log.FromContext(ctx)
	req := requestFromContext(ctx).(*SaveStateRequest)

	if err := h.store.SaveState(req.State); err != nil {
		logger.Error("Error saving state", slog.Any("error", err))
		internalServerError(w, logger)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// ServiceRequest holds the fields common to every query endpoint: the project
// and database ("service") the request targets.
type ServiceRequest struct {
	ProjectID string `json:"projectId"`
	ServiceID string `json:"serviceId"`
}

// Validate returns an error if a required field is missing.
func (r ServiceRequest) Validate() error {
	if r.ProjectID == "" {
		return &RequiredFieldError{Field: "projectId"}
	}
	if r.ServiceID == "" {
		return &RequiredFieldError{Field: "serviceId"}
	}
	return nil
}

// ExecuteRequest holds the fields common to the executeQuery and
// executeSessionQuery endpoints. Clients send both a top-level query (legacy)
// and a statements array (the editor text split by the client's SQL parser);
// statements is preferred when present. Timeout is accepted for compatibility
// but currently unused (runs use the default timeout).
type ExecuteRequest struct {
	ServiceRequest
	RunID      uuid.UUID `json:"runId"`
	Query      string    `json:"query"`
	Statements []string  `json:"statements"`
	Timeout    *int64    `json:"timeout,omitempty"`
}

// Validate returns an error if a required field is missing.
func (r ExecuteRequest) Validate() error {
	if err := r.ServiceRequest.Validate(); err != nil {
		return err
	}
	if r.RunID == uuid.Nil {
		return &RequiredFieldError{Field: "runId"}
	}
	return nil
}

// ExecuteQueryRequest is the request body of POST /api/executeQuery.
type ExecuteQueryRequest struct {
	ExecuteRequest
}

func (h *Handler) executeQueryHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	logger := log.FromContext(ctx)
	userID := serveUserID
	req := requestFromContext(ctx).(*ExecuteQueryRequest)

	sessionReq, err := h.sessionRequestForService(ctx, w, req.ServiceID)
	if err != nil {
		return // response already written
	}
	runReq := newQueryRunRequest(req.ExecuteRequest)

	// Create the run first so that the run timeout applies to opening the
	// database session as well.
	run, ctx := NewRun(ctx, userID, runReq)
	defer run.Close()

	if err := h.store.InsertRun(run); err != nil {
		h.handleInsertRunError(ctx, w, err)
		return
	}
	defer h.store.TryDeleteRun(ctx, run)

	logger.Debug("Opening database session")
	session, err := h.NewSession(ctx, userID, sessionReq, true, nil)
	if err != nil {
		h.handleNewSessionError(ctx, w, err)
		return
	}
	ctx, logger = log.NewContext(ctx, logger.With(
		slog.String("sessionId", session.ID.String()),
	))
	logger.Debug("Database session created")
	defer h.store.TryCloseSession(ctx, session)

	results := session.Query(ctx, run)
	rw := writer.NewResultWriter(run.Outputs, w)
	rw.Write(ctx, results)
}

// ArrowResultsRequest is the request body of POST /api/arrowResults.
type ArrowResultsRequest struct {
	ServiceRequest
	RunID uuid.UUID `json:"runId"`
}

// Validate returns an error if a required field is missing.
func (r ArrowResultsRequest) Validate() error {
	if err := r.ServiceRequest.Validate(); err != nil {
		return err
	}
	if r.RunID == uuid.Nil {
		return &RequiredFieldError{Field: "runId"}
	}
	return nil
}

func (h *Handler) arrowResultsHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	logger := log.FromContext(ctx)
	userID := serveUserID
	req := requestFromContext(ctx).(*ArrowResultsRequest)
	runID := req.RunID
	// This endpoint only ever returns the Arrow IPC stream.
	format := api.OutputFormatArrowStream

	run, err := h.store.GetRun(userID, runID)
	if err != nil {
		h.handleGetRunError(ctx, w, err)
		return
	}

	output, ok := run.Outputs.EndpointOutput(format)
	if !ok {
		err := &RunFormatError{RunID: runID, Format: format}
		logger.Warn("Run is not configured to return results in this format")
		writeError(w, http.StatusBadRequest, err, logger)
		return
	}

	// This protects against the case where multiple concurrent requests are
	// made to this endpoint with the same run ID and format. In that case,
	// only one of them will receive the result pipeReader, and the other will
	// receive an error.
	pipeReader, ok := <-output.PipeReaderChan
	if !ok {
		err := &ResultsUnavailableError{RunID: runID}
		logger.Warn("Results are unavailable for run")
		writeError(w, http.StatusBadRequest, err, logger)
		return
	}

	// Close the read-half of the pipe when finished. If the request was
	// canceled, propagate that back to the write-half of the pipe as the
	// cause.
	defer func() {
		if err := pipeReader.CloseWithError(ctx.Err()); err != nil {
			logger.Error("Error closing pipe reader", slog.Any("error", err))
		}
	}()

	if contentType := output.ContentType(); contentType != "" {
		w.Header().Set("Content-Type", contentType)
	}
	if contentEncoding := output.ContentEncoding(); contentEncoding != "" {
		w.Header().Set("Content-Encoding", contentEncoding)
	}

	fw := writer.NewFlushWriter(w)
	if _, err := io.Copy(fw, pipeReader); err != nil {
		logger.Log(ctx, writer.ErrLevel(ctx, err), "Error copying pipe reader to response writer", slog.Any("error", err))
	}
}

// CancelQueryRequest is the request body of POST /api/cancelQuery.
type CancelQueryRequest struct {
	ServiceRequest
	RunID uuid.UUID `json:"runId"`
}

// Validate returns an error if a required field is missing.
func (r CancelQueryRequest) Validate() error {
	if err := r.ServiceRequest.Validate(); err != nil {
		return err
	}
	if r.RunID == uuid.Nil {
		return &RequiredFieldError{Field: "runId"}
	}
	return nil
}

// CancelQueryResponse is the response body of the POST /api/cancelQuery
// endpoint.
type CancelQueryResponse struct {
	Success bool `json:"success"`
}

func (h *Handler) cancelQueryHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	logger := log.FromContext(ctx)
	userID := serveUserID
	req := requestFromContext(ctx).(*CancelQueryRequest)
	runID := req.RunID

	run, err := h.store.GetRun(userID, runID)
	if err != nil {
		h.handleGetRunError(ctx, w, err)
		return
	}

	run.Cancel()

	response := CancelQueryResponse{
		Success: true,
	}
	writeJSON(w, http.StatusOK, response, logger)
}

// SessionEventsRequest is the request body of POST /api/sessionEvents.
type SessionEventsRequest struct {
	ServiceRequest
	SessionID uuid.UUID `json:"sessionId"`
}

// Validate returns an error if a required field is missing.
func (r SessionEventsRequest) Validate() error {
	if err := r.ServiceRequest.Validate(); err != nil {
		return err
	}
	if r.SessionID == uuid.Nil {
		return &RequiredFieldError{Field: "sessionId"}
	}
	return nil
}

func (h *Handler) sessionEventsHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID := serveUserID
	req := requestFromContext(ctx).(*SessionEventsRequest)
	sessionID := req.SessionID

	session, err := h.store.AcquireSession(userID, sessionID)
	if err != nil {
		h.handleGetSessionError(ctx, w, err)
		return
	}
	defer h.releaseSession(ctx, session)

	events := session.Events(ctx)
	rw := writer.NewEventWriter(w)
	rw.Write(ctx, events)
}

// CreateSessionRequest is the request body of POST /api/createSession.
type CreateSessionRequest struct {
	ServiceRequest
}

// CreateSessionResponse is the response body of the POST /api/createSession
// endpoint.
type CreateSessionResponse struct {
	Success bool      `json:"success"`
	ID      uuid.UUID `json:"id"`
}

func (h *Handler) createSessionHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	logger := log.FromContext(ctx)
	userID := serveUserID
	req := requestFromContext(ctx).(*CreateSessionRequest)

	sessionReq, err := h.sessionRequestForService(ctx, w, req.ServiceID)
	if err != nil {
		return // response already written
	}

	logger.Debug("Opening database session")
	timeout := sessionDisconnectTimeout
	session, err := h.NewSession(ctx, userID, sessionReq, false, &timeout)
	if err != nil {
		h.handleNewSessionError(ctx, w, err)
		return
	}
	ctx, logger = log.NewContext(ctx, logger.With(
		slog.String("sessionId", session.ID.String()),
	))
	logger.Debug("Database session created")

	if err := h.store.InsertSession(session); err != nil {
		logger.Error("Error inserting session", slog.Any("error", err))
		internalServerError(w, logger)

		h.store.TryCloseSession(ctx, session)
		h.store.TryDeleteSession(ctx, session)
		return
	}

	response := CreateSessionResponse{
		Success: true,
		ID:      session.ID,
	}
	writeJSON(w, http.StatusOK, response, logger)
}

// ExecuteSessionQueryRequest is the request body of POST /api/executeSessionQuery.
type ExecuteSessionQueryRequest struct {
	ExecuteRequest
	SessionID uuid.UUID `json:"sessionId"`
}

// Validate returns an error if a required field is missing.
func (r ExecuteSessionQueryRequest) Validate() error {
	if err := r.ExecuteRequest.Validate(); err != nil {
		return err
	}
	if r.SessionID == uuid.Nil {
		return &RequiredFieldError{Field: "sessionId"}
	}
	return nil
}

func (h *Handler) executeSessionQueryHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID := serveUserID
	req := requestFromContext(ctx).(*ExecuteSessionQueryRequest)
	sessionID := req.SessionID
	runReq := newQueryRunRequest(req.ExecuteRequest)

	session, err := h.store.AcquireSession(userID, sessionID)
	if err != nil {
		h.handleGetSessionError(ctx, w, err)
		return
	}
	defer h.releaseSession(ctx, session)

	run, ctx := NewRun(ctx, userID, runReq)
	defer run.Close()

	if err := h.store.InsertRun(run); err != nil {
		h.handleInsertRunError(ctx, w, err)
		return
	}
	defer h.store.TryDeleteRun(ctx, run)

	results := session.Query(ctx, run)
	rw := writer.NewResultWriter(run.Outputs, w)
	rw.Write(ctx, results)
}

// CloseSessionRequest is the request body of POST /api/closeSession.
type CloseSessionRequest struct {
	ServiceRequest
	SessionID uuid.UUID `json:"sessionId"`
}

// Validate returns an error if a required field is missing.
func (r CloseSessionRequest) Validate() error {
	if err := r.ServiceRequest.Validate(); err != nil {
		return err
	}
	if r.SessionID == uuid.Nil {
		return &RequiredFieldError{Field: "sessionId"}
	}
	return nil
}

// CloseSessionResponse is the response body of the POST /api/closeSession
// endpoint.
type CloseSessionResponse struct {
	Success bool `json:"success"`
}

func (h *Handler) closeSessionHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	logger := log.FromContext(ctx)
	userID := serveUserID
	req := requestFromContext(ctx).(*CloseSessionRequest)
	sessionID := req.SessionID

	session, err := h.store.GetSession(userID, sessionID)
	if err != nil {
		h.handleGetSessionError(ctx, w, err)
		return
	}

	logger.Debug("Closing database session")
	if err := session.Close(); err != nil {
		logger.Error("Error closing database session", slog.Any("error", err))
		internalServerError(w, logger)
		return
	}
	logger.Debug("Database session closed")

	logger.Debug("Deleting database session")
	h.store.DeleteSession(session)
	logger.Debug("Database session deleted")

	response := CloseSessionResponse{
		Success: true,
	}
	writeJSON(w, http.StatusOK, response, logger)
}

func (h *Handler) debugHandlers() http.Handler {
	mux := http.NewServeMux()
	//	mux.HandleFunc("/debug/events", trace.Events)
	//	mux.HandleFunc("/debug/requests", trace.Traces)
	mux.HandleFunc("/debug/pprof/", pprof.Index)
	mux.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
	mux.HandleFunc("/debug/pprof/profile", pprof.Profile)
	mux.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
	mux.HandleFunc("/debug/pprof/trace", pprof.Trace)
	return mux
}

func (h *Handler) methodNotAllowedHandler(w http.ResponseWriter, r *http.Request) {
	logger := log.FromContext(r.Context())
	logger.Warn("Method not allowed")
	writeError(w, http.StatusMethodNotAllowed, api.ErrMethodNotAllowed, logger)
}

// loadClient reloads credentials from disk (refreshing the OAuth token if
// needed) and returns an API client bound to the active project. Called per
// request so a long-running server doesn't keep using a stale token after it
// expires.
func (h *Handler) loadClient(ctx context.Context) (ghostapi.ClientWithResponsesInterface, string, error) {
	_, client, projectID, err := h.app.Load(ctx)
	if err != nil {
		return nil, "", err
	}
	if client == nil {
		_, _, clientErr := h.app.GetClient()
		if clientErr != nil {
			return nil, "", clientErr
		}
		return nil, "", errors.New("authentication required")
	}
	return client, projectID, nil
}

// sessionRequestForService resolves the database connection for the given
// service (a database ref) and returns a session request that opens it via
// DSN. On failure it writes an error response (via handleNewSessionError) and
// returns a non-nil error, so callers can just return. The active project is
// authoritative; the request's projectId is accepted for compatibility but not
// used for routing.
func (h *Handler) sessionRequestForService(ctx context.Context, w http.ResponseWriter, serviceID string) (SessionRequest, error) {
	client, projectID, err := h.loadClient(ctx)
	if err != nil {
		h.handleNewSessionError(ctx, w, err)
		return SessionRequest{}, err
	}
	connStr, err := connectionStringForService(ctx, client, projectID, serviceID, h.app.GetConfig().ReadOnly)
	if err != nil {
		h.handleNewSessionError(ctx, w, err)
		return SessionRequest{}, err
	}
	return SessionRequest{
		Client: driver.Timescale,
		DSN:    connStr,
	}, nil
}

func (h *Handler) handleNewSessionError(ctx context.Context, w http.ResponseWriter, err error) {
	logger := log.FromContext(ctx)
	logger.Warn("Error opening database session", slog.Any("error", err))

	// Normalized errors are returned for connection errors, which are
	// likely due to bad user input. In these cases, return 200 OK, similar
	// to query errors, to signal to the front-end to display the error to
	// the end-user.
	var normalizedErr *api.NormalizedError
	if errors.As(err, &normalizedErr) {
		writeNormalizedError(w, http.StatusOK, normalizedErr, logger)
		return
	}
	writeError(w, http.StatusBadRequest, err, logger)
}

func (h *Handler) handleGetSessionError(ctx context.Context, w http.ResponseWriter, err error) {
	logger := log.FromContext(ctx)

	var invalidSessionIDError *InvalidSessionIDError
	if errors.As(err, &invalidSessionIDError) {
		logger.Warn("Invalid session ID", slog.Any("error", err))
		writeError(w, http.StatusNotFound, invalidSessionIDError, logger)
		return
	}
	logger.Error("Error getting session", slog.Any("error", err))
	internalServerError(w, logger)
}

func (h *Handler) releaseSession(ctx context.Context, session *Session) {
	// The session is marked as broken if the query returned a fatal error
	// (indicating the underlying connection is broken) or a database ping
	// failed. In that case, automatically close/delete the session. The error
	// returned in the response will include '"fatal": true' to indicate to the
	// caller that the session has ended. Future requests involving this
	// session ID will return 404 Not Found.
	if session.Broken() {
		h.store.TryCloseSession(ctx, session)
		h.store.TryDeleteSession(ctx, session)
		return
	}
	h.store.ReleaseSession(session)
}

func (h *Handler) handleInsertRunError(ctx context.Context, w http.ResponseWriter, err error) {
	logger := log.FromContext(ctx)

	var runIDConflictError *RunIDConflictError
	if errors.As(err, &runIDConflictError) {
		logger.Warn("Run with ID already exists", slog.Any("error", err))
		writeError(w, http.StatusBadRequest, runIDConflictError, logger)
		return
	}
	logger.Error("Error inserting run", slog.Any("error", err))
	internalServerError(w, logger)
}

func (h *Handler) handleGetRunError(ctx context.Context, w http.ResponseWriter, err error) {
	logger := log.FromContext(ctx)

	var invalidRunIDError *InvalidRunIDError
	if errors.As(err, &invalidRunIDError) {
		logger.Warn("Invalid run ID", slog.Any("error", err))
		writeError(w, http.StatusNotFound, invalidRunIDError, logger)
		return
	}
	logger.Error("Error getting run", slog.Any("error", err))
	internalServerError(w, logger)
}

// serveUserID is the user ID used for all sessions/runs. ghost serve is a
// single local user, so the per-user keying is unused (0).
const serveUserID int64 = 0

// sessionDisconnectTimeout is how long a session lingers after the client
// disconnects from /api/sessionEvents before it is automatically closed. While
// the events stream is connected the timeout is paused (see AcquireSession), so
// this is effectively the post-disconnect grace period.
const sessionDisconnectTimeout = 15 * time.Second

// arrowStreamEndpointOutput is the single output every run is configured with:
// an Arrow IPC stream piped to the results endpoint, which the client fetches
// via POST /api/arrowResults.
var arrowStreamEndpointOutput = api.Output{
	Format:      api.OutputFormatArrowStream,
	Destination: api.OutputDestinationEndpoint,
	Compression: api.OutputCompressionGzip,
	RowNum:      true,
}

// newQueryRunRequest builds a [RunRequest] from an execute request,
// configured to stream results as Arrow over the results endpoint.
func newQueryRunRequest(req ExecuteRequest) RunRequest {
	runID := req.RunID
	return RunRequest{
		RunID:      &runID,
		Query:      req.Query,
		Statements: req.Statements,
		Outputs:    api.Outputs{arrowStreamEndpointOutput},
	}
}
