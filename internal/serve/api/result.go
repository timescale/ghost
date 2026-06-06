package api

import (
	"reflect"

	"github.com/google/uuid"
)

// Result is an interface representing the types that are streamed back from
// the GET /run and GET /session/:sessionID/run endpoints. The isResult method
// is a no-op that gives us compile-time assurance that only the result types
// defined below implement the interface.,
type Result interface {
	isResult()
}

type result struct{}

func (r result) isResult() {}

// Column represents a column in a result set. It includes the name and type
// of the column, and can also include additional information about the column
// type, such as the length, precision, and scale - e.g. for a VARCHAR(10) or
// NUMERIC(5, 3) column type.
// NOTE: The ScanType field specifies the Go type that column values will be
// scanned into, but is only used internally - it should not be considered part
// of the public interface, and may be removed in the future.
type Column struct {
	Name      string `json:"name"`
	Type      string `json:"type,omitempty"`
	Length    int64  `json:"length,omitempty"`
	Precision int64  `json:"precision,omitempty"`
	Scale     int64  `json:"scale,omitempty"`
	Object    bool   `json:"isObject,omitempty"`
	Numeric   bool   `json:"isNumeric,omitempty"`

	// TODO: Remove from public interface.
	ScanType reflect.Type `json:"-"`
}

// ColumnResult is the first [Result] type streamed back in the results
// (besides potential [StatusResult] messages), assuming there wasn't an error.
// It includes information about the result columns being returned, including
// their names and types. It also includes the run ID.
type ColumnResult struct {
	result
	RunID   uuid.UUID `json:"runId"`
	Columns []Column  `json:"columns"`
}

// RowResult is the [Result] type returned after [ColumnResult], assuming there
// wasn't an error, and the query is returning at least one row. One RowResult
// will be returned for each row in the result set.
type RowResult struct {
	result
	Row []any `json:"row,omitempty"`
}

// QueryStatus represents a query status value, as returned in a [StatusResult].
type QueryStatus string

// Valid [QueryStatus] values.
const (
	StatusPending QueryStatus = "pending"
)

// StatusResult is a [Result] type that is returned every so often when the
// query is in progress, but no other [Result] types have been returned for
// awhile. Its primary purpose is to keep the underlying TCP connection
// active, to avoid idle connection timeouts that would otherwise forcibly
// close the connection (such as the AWS Application Load Balancer idle
// connection timeout).
// See: https://docs.aws.amazon.com/elasticloadbalancing/latest/application/application-load-balancers.html#connection-idle-timeout).
type StatusResult struct {
	result
	Status QueryStatus `json:"status"`
}

// SuccessResult is the final [Result] type returned if the query was
// successful. It is returned after all [RowResult] values have been returned,
// and includes the total row count as well as the overall elapsed query
// execution duration. It also returns the number of rows affected, if
// supported by the underlying driver.
type SuccessResult struct {
	result
	Success      bool     `json:"success"`
	RowCount     int      `json:"rowCount"`
	RowsAffected *int64   `json:"rowsAffected,omitempty"`
	Elapsed      Duration `json:"elapsed"`
}

// ErrorResult is the final [Result] type returned if there was an error
// executing the query. Its fields are a superset of [NormalizedErrorResponse].
// It can be returned at any point, so long as [SuccessResult] was not yet
// returned - i.e. it can be the first [Result] returned, or it can be returned
// after some number of [RowResult] values have already been returned.
type ErrorResult struct {
	result
	Success  bool             `json:"success"`
	RowCount int              `json:"rowCount"`
	Elapsed  Duration         `json:"elapsed"`
	Error    *NormalizedError `json:"error"`
}
