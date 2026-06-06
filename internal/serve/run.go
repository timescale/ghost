package serve

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/timescale/ghost/internal/serve/api"
	"github.com/timescale/ghost/internal/serve/writer"
)

// DefaultRunTimeout is the default amount of time that a query can be
// executing before it will time out and be canceled.
const DefaultRunTimeout = 30 * time.Minute

// RunRequest represents the common fields of a request capable of issuing a
// query against a database.
type RunRequest struct {
	RunID      *uuid.UUID    `json:"runId,omitempty"`
	Query      string        `json:"query,omitempty"`
	Statements []string      `json:"statements,omitempty"`
	Outputs    api.Outputs   `json:"outputs,omitempty"`
	Timeout    *api.Duration `json:"timeout,omitempty"`
}

// Run represents an in-progress query. After being created via [NewRun], it is
// passed to [Session.Query], which executes the query against the database.
// All runs are stored in the [Store] until they complete, are canceled, or
// time out.
type Run struct {
	// Unique identifier for the run. Automatically generated if not provided
	// in the [RunRequest].
	ID uuid.UUID

	// ID of the user to whom the session belongs. Defaults to zero for the old
	// endpoints (which did not take a user ID parameter).
	UserID int64

	// The list of SQL statements that the run is executing. Either this field
	// or Query will be present, but not both.
	Statements []string

	// The text of the SQL query that the run is executing. Either this field
	// or Statements will be present, but not both.
	Query string

	// Destinations and formats to write arrow records to.
	Outputs writer.Outputs

	// The length of time after which the run will time out. Defaults to
	// DefaultRunTimeout if not provided in the [RunRequest].
	Timeout time.Duration

	// A function which, when called, triggers cancellation of the run.
	Cancel context.CancelFunc
}

// NewRun creates a new [Run] given a [RunRequest]. It returns the fully
// initialized run, along with a context that times out when the run times out.
// The timeout is determined by the Timeout field of the [RunRequest], or
// by [DefaultRunTimeout] if the Timeout field was nil.
func NewRun(ctx context.Context, userID int64, req RunRequest) (*Run, context.Context) {
	timeout := runTimeout(req)
	ctx, cancel := context.WithTimeout(ctx, timeout)

	return &Run{
		ID:         runID(req),
		UserID:     userID,
		Statements: req.Statements,
		Query:      req.Query,
		Outputs:    writer.NewOutputs(req.Outputs),
		Timeout:    timeout,
		Cancel:     cancel,
	}, ctx
}

func runTimeout(req RunRequest) time.Duration {
	if timeout := req.Timeout.Value(); timeout != nil {
		return *timeout
	}
	return DefaultRunTimeout
}

func runID(req RunRequest) uuid.UUID {
	if req.RunID != nil {
		return *req.RunID
	}
	return uuid.New()
}

// LeadingStatements returns all of the statements in the run's statement list
// except the last (which is returned from [Run.FinalQuery] instead). Returns an
// empty list if the statement list is empty (because the Query field is being
// used instead) or only contains a single statement. The intention is for these
// statements to be executed sequentially via [driver.Driver.Query], but without
// returning any results, before the run's final query is executed.
func (r *Run) LeadingStatements() []string {
	if len(r.Statements) <= 1 {
		return nil
	}
	return r.Statements[:len(r.Statements)-1]
}

// FinalQuery returns the run's query (or final statement in the statement
// list), which should be passed to [driver.Driver.Query] to execute the query.
// If there are additional statements in the statement list, they should be
// executed via [Run.LeadingStatements] before this query is executed.
func (r *Run) FinalQuery() string {
	if len(r.Statements) > 0 {
		return r.Statements[len(r.Statements)-1]
	}
	return r.Query
}

// Close cancels the run if it's still in progress, and cleans up associated
// resources.
func (r *Run) Close() {
	r.Cancel()
	r.Outputs.Close()
}
