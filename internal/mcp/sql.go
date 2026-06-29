package mcp

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/timescale/ghost/internal/common"
	"github.com/timescale/ghost/internal/util"
)

// SQLInput represents input for ghost_sql
type SQLInput struct {
	Ref        string   `json:"name_or_id"`
	Query      string   `json:"query,omitempty"`
	File       string   `json:"file,omitempty"`
	Parameters []string `json:"parameters,omitempty"`
}

func (SQLInput) Schema() *jsonschema.Schema {
	schema := util.Must(jsonschema.For[SQLInput](nil))
	databaseRefInputProperties(schema)
	schema.Properties["query"].Description = "SQL query to execute. Multi-statement queries are supported when no parameters are provided. Exactly one of 'query' or 'file' must be provided."
	schema.Properties["file"].Description = "Path to a SQL file on disk to execute. Multi-statement files are supported when no parameters are provided. Exactly one of 'query' or 'file' must be provided."
	schema.Properties["parameters"].Description = "Query parameters. Values are substituted for $1, $2, etc. placeholders in the query. Only supported for single-statement queries"
	return schema
}

// SQLOutput represents output for ghost_sql
type SQLOutput struct {
	ResultSets    []common.ResultSet `json:"result_sets"`
	ExecutionTime string             `json:"execution_time"`
}

func (SQLOutput) Schema() *jsonschema.Schema {
	schema := util.Must(jsonschema.For[SQLOutput](nil))
	schema.Properties["execution_time"].Description = "Total client-side elapsed time for all statements"
	return schema
}

func newSQLTool() *mcp.Tool {
	return &mcp.Tool{
		Name:         "ghost_sql",
		Title:        "Execute SQL",
		Description:  "Execute a SQL query against a database. If the connection fails, the database may not be running - use ghost_list to check its status.",
		InputSchema:  SQLInput{}.Schema(),
		OutputSchema: SQLOutput{}.Schema(),
		Annotations: &mcp.ToolAnnotations{
			ReadOnlyHint:    false,
			DestructiveHint: new(true),
			IdempotentHint:  false,
			OpenWorldHint:   new(true),
			Title:           "Execute SQL",
		},
	}
}

func (s *Server) handleSQL(ctx context.Context, req *mcp.CallToolRequest, input SQLInput) (*mcp.CallToolResult, SQLOutput, error) {
	cfg, client, projectID, err := s.app.GetAll()
	if err != nil {
		return nil, SQLOutput{}, err
	}

	if (input.Query == "") == (input.File == "") {
		return nil, SQLOutput{}, errors.New("exactly one of 'query' or 'file' must be provided")
	}

	query := input.Query
	if input.File != "" {
		data, err := os.ReadFile(util.ExpandPath(input.File))
		if err != nil {
			return nil, SQLOutput{}, fmt.Errorf("failed to read SQL file: %w", err)
		}
		query = string(data)
	}

	// Execute the query
	result, err := common.ExecuteQuery(ctx, common.ExecuteQueryArgs{
		Client:      client,
		ProjectID:   projectID,
		DatabaseRef: input.Ref,
		Query:       query,
		Role:        "tsdbadmin",
		Parameters:  input.Parameters,
		ReadOnly:    cfg.ReadOnly,
	})
	if err != nil {
		return nil, SQLOutput{}, handleDatabaseError(err)
	}

	return nil, SQLOutput{
		ResultSets:    result.ResultSets,
		ExecutionTime: result.ExecutionTime.String(),
	}, nil
}
