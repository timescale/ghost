package serve

import (
	"context"

	"github.com/google/uuid"
	"github.com/timescale/ghost/internal/serve/api"
	"github.com/timescale/ghost/internal/serve/writer"
)

// arrowStreamEndpointOutput is the single output every run is configured with:
// an Arrow IPC stream piped to the results endpoint, which the client fetches
// via POST /api/arrowResults.
var arrowStreamEndpointOutput = api.Output{
	Format:      api.OutputFormatArrowStream,
	Destination: api.OutputDestinationEndpoint,
	Compression: api.OutputCompressionGzip,
	RowNum:      true,
}

// Run represents an in-progress query. After being created via [NewRun], it is
// passed to [Session.Query], which executes the query against the database.
// All runs are stored in the [Store] until they complete or are canceled.
type Run struct {
	// Unique identifier for the run.
	ID uuid.UUID

	// The list of SQL statements that the run is executing. Either this field
	// or Query will be present, but not both.
	Statements []string

	// The text of the SQL query that the run is executing. Either this field
	// or Statements will be present, but not both.
	Query string

	// Destinations and formats to write arrow records to.
	Outputs writer.Outputs

	// A function which, when called, triggers cancellation of the run.
	Cancel context.CancelFunc
}

// NewRun creates a new [Run] from an [ExecuteRequest], configured to stream
// results as Arrow over the results endpoint. It returns the fully initialized
// run, along with a context that is canceled when the run is canceled (via
// [Run.Cancel] or [Run.Close]). Runs do not time out.
func NewRun(ctx context.Context, req ExecuteRequest) (*Run, context.Context) {
	ctx, cancel := context.WithCancel(ctx)

	return &Run{
		ID:         req.RunID,
		Statements: req.Statements,
		Query:      req.Query,
		Outputs:    writer.NewOutputs(api.Outputs{arrowStreamEndpointOutput}),
		Cancel:     cancel,
	}, ctx
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
