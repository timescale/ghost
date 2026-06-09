package mcp

import (
	"context"
	"encoding/json"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/timescale/ghost/internal/common"
	"github.com/timescale/ghost/internal/util"
)

// SchemaInput represents input for ghost_schema
type SchemaInput struct {
	Ref        string `json:"name_or_id"`
	SchemaName string `json:"schema,omitempty"`
	Internal   bool   `json:"internal,omitempty"`
}

func (SchemaInput) Schema() *jsonschema.Schema {
	schema := util.Must(jsonschema.For[SchemaInput](nil))
	databaseRefInputProperties(schema)
	schema.Properties["schema"].Description = "Restrict output to a single Postgres schema (e.g. 'public', 'reporting'). May target a system schema such as 'pg_catalog'. Only objects the connecting user can access are returned. If omitted, all accessible non-system schemas are returned."
	schema.Properties["internal"].Description = "Include system schemas (information_schema, pg_*, _timescaledb_*) and extension-owned objects. Defaults to false."
	schema.Properties["internal"].Default = json.RawMessage("false")
	return schema
}

func newSchemaTool() *mcp.Tool {
	return &mcp.Tool{
		Name:        "ghost_schema",
		Title:       "Show Database Schema",
		Description: "Display database schema including tables, views, materialized views, enum types, functions, procedures, indexes, triggers, and TimescaleDB hypertable metadata. Only objects the connecting user can access are returned.",
		InputSchema: SchemaInput{}.Schema(),
		Annotations: &mcp.ToolAnnotations{
			ReadOnlyHint:  true,
			OpenWorldHint: new(true),
			Title:         "Show Database Schema",
		},
	}
}

func (s *Server) handleSchema(ctx context.Context, req *mcp.CallToolRequest, input SchemaInput) (*mcp.CallToolResult, any, error) {
	client, projectID, err := s.app.GetClient()
	if err != nil {
		return nil, nil, err
	}

	schema, err := common.FetchDatabaseSchema(ctx, common.FetchDatabaseSchemaArgs{
		Client:          client,
		ProjectID:       projectID,
		DatabaseRef:     input.Ref,
		Schema:          input.SchemaName,
		IncludeInternal: input.Internal,
	})
	if err != nil {
		return nil, nil, handleDatabaseError(err)
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: common.FormatSchema(schema)},
		},
	}, nil, nil
}
