package serve

import (
	"github.com/timescale/ghost/internal/serve/dbdriver"
)

// Wire-format types matching what the query widget's client sends to and
// expects back from the hosted query service. These mirror the widget's
// TimescaleQueryClient request/response shapes.

// executeQueryRequest matches TimescaleExecuteQueryRequest. The widget
// emits both a top-level `query` field (legacy / unused in practice) and a
// `statements` array containing the editor text split + trimmed by the
// widget's own SQL parser. We prefer `statements` when present.
type executeQueryRequest struct {
	ProjectID  string   `json:"projectId"`
	ServiceID  string   `json:"serviceId"`
	Query      string   `json:"query"`
	Statements []string `json:"statements"`
	RunID      string   `json:"runId"`
	Persist    bool     `json:"persist,omitempty"`
	Timeout    *int64   `json:"timeout,omitempty"`
}

// executeSessionQueryRequest matches TimescaleExecuteSessionQueryRequest.
type executeSessionQueryRequest struct {
	executeQueryRequest
	SessionID string `json:"sessionId"`
}

// arrowResultsRequest matches TimescaleArrowResultsRequest.
type arrowResultsRequest struct {
	ProjectID string `json:"projectId"`
	ServiceID string `json:"serviceId"`
	RunID     string `json:"runId"`
}

// cancelQueryRequest matches TimescaleCancelQueryRequest.
type cancelQueryRequest struct {
	ProjectID string `json:"projectId"`
	ServiceID string `json:"serviceId"`
	RunID     string `json:"runId"`
}

// createSessionRequest matches TimescaleCreateSessionRequest.
type createSessionRequest struct {
	ProjectID string `json:"projectId"`
	ServiceID string `json:"serviceId"`
}

// sessionRefRequest matches the body of closeSession/sessionEvents.
type sessionRefRequest struct {
	ProjectID string `json:"projectId"`
	ServiceID string `json:"serviceId"`
	SessionID string `json:"sessionId"`
}

// createSessionResponse matches CreateSessionResponse (one of two shapes).
type createSessionResponse struct {
	Success bool                      `json:"success"`
	ID      string                    `json:"id,omitempty"`
	Error   *dbdriver.NormalizedError `json:"error,omitempty"`
}

// columnsResult is the first NDJSON line written by executeQuery. The widget
// uses 'columns' as the discriminator.
type columnsResult struct {
	RunID   string           `json:"runId"`
	Columns dbdriver.Columns `json:"columns"`
}

// successResult is the final NDJSON line on a successful run.
type successResult struct {
	RunID              string `json:"runId"`
	Success            bool   `json:"success"`
	RowCount           int64  `json:"rowCount"`
	RowsAffected       *int64 `json:"rowsAffected,omitempty"`
	ExecutedStatements int64  `json:"executedStatements,omitempty"`
}

// errorResult is the final NDJSON line on a failed (or canceled) run.
type errorResult struct {
	RunID   string                    `json:"runId"`
	Success bool                      `json:"success"`
	Error   *dbdriver.NormalizedError `json:"error"`
}

// sessionEvent matches the SessionEvent NDJSON line shape.
type sessionEvent struct {
	Status string                    `json:"status"`
	Error  *dbdriver.NormalizedError `json:"error,omitempty"`
}

const (
	sessionStatusConnecting = "connecting"
	sessionStatusConnected  = "connected"
	sessionStatusClosed     = "closed"
	sessionStatusError      = "error"
)
