package common

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/timescale/ghost/internal/api"
	"github.com/timescale/ghost/internal/util"
)

// Column represents a column in the query result.
type Column struct {
	Name string `json:"name"`
	Type string `json:"type"`
}

// ResultSet represents a single query result set.
type ResultSet struct {
	CommandTag   string     `json:"command_tag"`
	Columns      []Column   `json:"columns,omitempty"`
	Rows         [][]string `json:"rows,omitempty"`
	RowsAffected int64      `json:"rows_affected"`
}

// QueryResult represents the complete result of a query execution.
type QueryResult struct {
	ResultSets    []ResultSet   `json:"result_sets"`
	ExecutionTime time.Duration `json:"execution_time"`
}

// ExecuteQueryArgs contains arguments for query execution.
type ExecuteQueryArgs struct {
	Client      api.ClientWithResponsesInterface
	ProjectID   string
	DatabaseRef string
	Query       string
	Role        string
	Parameters  []string
	ReadOnly    bool
}

// ExecuteQuery executes a SQL query against a database.
// It handles fetching the password, building the connection string,
// connecting to the database, and executing the query.
//
// Multi-statement queries (semicolon-separated) are supported when no
// parameters are provided. When parameters are provided, only single
// statements are supported.
//
// Declared as a var so tests can replace it with a stub that doesn't
// require a real database connection.
var ExecuteQuery = func(ctx context.Context, args ExecuteQueryArgs) (*QueryResult, error) {
	// Fetch database details
	database, err := fetchDatabase(ctx, args.Client, args.ProjectID, args.DatabaseRef)
	if err != nil {
		return nil, err
	}

	// Check if the database is ready to accept connections
	if err := CheckReady(database); err != nil {
		return nil, err
	}

	// Get password for database
	password, err := GetPassword(database, args.Role)
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve password: %w", err)
	}

	// Build connection string
	connStr, err := BuildConnectionString(ConnectionStringArgs{
		Database: database,
		Role:     args.Role,
		Password: password,
		ReadOnly: args.ReadOnly,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to build connection string: %w", err)
	}

	// Execute the query
	return executeQueryWithConnection(ctx, connStr, args)
}

// fetchDatabase retrieves the database details from the API.
func fetchDatabase(ctx context.Context, client api.ClientWithResponsesInterface, projectID, databaseRef string) (api.Database, error) {
	resp, err := client.GetDatabaseWithResponse(ctx, projectID, databaseRef)
	if err != nil {
		return api.Database{}, fmt.Errorf("failed to get database: %w", err)
	}

	if resp.StatusCode() != http.StatusOK {
		return api.Database{}, ExitWithErrorFromStatusCode(resp.StatusCode(), resp.JSONDefault)
	}

	if resp.JSON200 == nil {
		return api.Database{}, errors.New("empty response from API")
	}

	return *resp.JSON200, nil
}

// executeQueryWithConnection executes a query using the given connection string.
func executeQueryWithConnection(ctx context.Context, connStr string, opts ExecuteQueryArgs) (*QueryResult, error) {
	// Parse connection string into config
	connConfig, err := pgx.ParseConfig(connStr)
	if err != nil {
		return nil, fmt.Errorf("failed to parse connection string: %w", err)
	}

	// Choose query execution mode based on whether parameters are present.
	// Simple protocol supports multi-statement queries but interpolates
	// parameters client-side (which we don't want to do, for security's sake).
	// Extended protocol sends parameters separately but doesn't support
	// multi-statement queries. This means we don't support multi-statement
	// queries with parameters (pgx will return an error for them when using
	// QueryExecModeExec). See [pgx.QueryExecMode] for details.
	if len(opts.Parameters) > 0 {
		// Use extended protocol to send parameters separately (more secure,
		// but doesn't support multi-statement queries). We use
		// QueryExecModeExec instead of QueryExecModeDescribeExec because
		// DescribeExec requests results in each column's preferred binary
		// format, but we scan all columns into *string which only works
		// with text format. QueryExecModeExec uses text format for results
		// and also avoids the extra describe round trip.
		connConfig.DefaultQueryExecMode = pgx.QueryExecModeExec
	} else {
		// Use simple protocol to support multi-statement queries when no
		// parameters are given.
		connConfig.DefaultQueryExecMode = pgx.QueryExecModeSimpleProtocol
	}

	// Connect to database
	conn, err := pgx.ConnectConfig(ctx, connConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to database: %w", err)
	}
	defer conn.Close(context.Background())

	// Execute query and measure time
	startTime := time.Now()

	// Queue the query. When using QueryExecModeSimpleProtocol (no parameters),
	// it's valid to queue a single multi-statement SQL query as the batch.
	// See the [pgx.Batch.Queue] documentation for details. When using
	// QueryExecModeDescribeExec (with parameters), queueing a multi-statement
	// query here will result in an error when executing it below.
	batch := &pgx.Batch{}
	batch.Queue(opts.Query, util.ConvertSliceToAny(opts.Parameters)...)

	br := conn.SendBatch(ctx, batch)
	defer br.Close()

	// Process all result sets, collecting them all
	resultSets := make([]ResultSet, 0)
	for {
		rows, err := br.Query()
		if err != nil {
			// Check if we've reached the final result set and stop iteration.
			// NOTE: It would be nice if there was a real sentinel error type
			// we could check here instead of comparing error strings, but pgx
			// doesn't expose one. We will just need to verify that the error
			// message doesn't change when we update the pgx dependency.
			if err.Error() == "no more results in batch" {
				break
			}
			return nil, fmt.Errorf("query execution failed: %w", err)
		}

		// Process this result set
		result, err := processResultSet(conn, rows)
		if err != nil {
			return nil, err
		}

		// Collect this result set
		resultSets = append(resultSets, result)
	}

	if err := br.Close(); err != nil {
		return nil, fmt.Errorf("failed to close batch: %w", err)
	}

	return &QueryResult{
		ResultSets:    resultSets,
		ExecutionTime: time.Since(startTime),
	}, nil
}

// processResultSet reads all data from a pgx.Rows result set.
func processResultSet(conn *pgx.Conn, rows pgx.Rows) (ResultSet, error) {
	defer rows.Close()

	// Get column metadata from field descriptions
	fieldDescriptions := rows.FieldDescriptions()
	columns := make([]Column, len(fieldDescriptions))
	for i, fd := range fieldDescriptions {
		// Get the type name from the connection's type map
		typeName := "unknown"
		dataType, ok := conn.TypeMap().TypeForOID(fd.DataTypeOID)
		if ok && dataType != nil {
			typeName = dataType.Name
		}
		columns[i] = Column{
			Name: fd.Name,
			Type: typeName,
		}
	}

	// Collect all rows from this result set
	var resultRows [][]string
	if len(columns) > 0 {
		// If any columns were returned, initialize resultRows to an empty
		// slice to ensure we always return a slice in the results, even
		// if empty (we want to be completely clear when a SELECT query returns
		// no rows). On the other hand, if no columns were returned, it's not a
		// result returning query (e.g. it's DDL or an INSERT/UPDATE/DELETE/etc.),
		// so we leave resultRows nil so it gets omitted.
		resultRows = make([][]string, 0)
	}

	// Create scan destinations for each column as *string (to handle NULLs)
	numCols := len(columns)
	for rows.Next() {
		// Create string pointers for scanning (NULL values will be nil)
		scanDest := make([]*string, numCols)
		scanDestAny := make([]any, numCols)
		for i := range scanDest {
			scanDestAny[i] = &scanDest[i]
		}

		if err := rows.Scan(scanDestAny...); err != nil {
			return ResultSet{}, fmt.Errorf("failed to scan row: %w", err)
		}

		// Convert *string to string (nil becomes "NULL")
		rowValues := make([]string, numCols)
		for i, ptr := range scanDest {
			if ptr != nil {
				rowValues[i] = *ptr
			} else {
				rowValues[i] = "NULL"
			}
		}
		resultRows = append(resultRows, rowValues)
	}

	// Check for errors during iteration
	if err := rows.Err(); err != nil {
		return ResultSet{}, fmt.Errorf("error iterating rows: %w", err)
	}

	commandTag := rows.CommandTag()
	return ResultSet{
		CommandTag:   commandTag.String(),
		Columns:      columns,
		Rows:         resultRows,
		RowsAffected: commandTag.RowsAffected(),
	}, nil
}
